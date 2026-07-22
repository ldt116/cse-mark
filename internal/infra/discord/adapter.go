package discord

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	discordport "thuanle/cse-mark/internal/domain/discord"
)

// sleeper abstracts time.Sleep so retry/backoff is testable without real waits.
type sleeper interface {
	Sleep(d time.Duration)
}

type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

// discordAPI is the subset of *discordgo.Session the adapter calls. Defining it
// as an interface lets tests substitute a fake; production binds the real
// *discordgo.Session (which satisfies it via its method set).
type discordAPI interface {
	GuildRoles(guildID string, opts ...discordgo.RequestOption) ([]*discordgo.Role, error)
	GuildRoleCreate(guildID string, data *discordgo.RoleParams, opts ...discordgo.RequestOption) (*discordgo.Role, error)
	GuildChannels(guildID string, opts ...discordgo.RequestOption) ([]*discordgo.Channel, error)
	GuildChannelCreateComplex(guildID string, data discordgo.GuildChannelCreateData, opts ...discordgo.RequestOption) (*discordgo.Channel, error)
	GuildMemberRoleAdd(guildID, userID, roleID string, opts ...discordgo.RequestOption) error
	GuildMemberRoleRemove(guildID, userID, roleID string, opts ...discordgo.RequestOption) error
	GuildMembers(guildID, after string, limit int, opts ...discordgo.RequestOption) ([]*discordgo.Member, error)
}

// Adapter implements Bot over a discordgo session. All calls are serialized
// through a single command queue: the role-sync scheduler provisions/assigns in
// a burst, and funneling everything through one worker keeps Discord rate
// limits (per-route buckets) from being blown out by concurrency. Each command
// retries on a rate-limit error, sleeping Retry-After, before giving up.
type Adapter struct {
	api     discordAPI
	guildID string
	sleep   sleeper

	cmds chan func() error // command queue, drained by Run
	errs chan error        // closed when the worker exits
}

// NewAdapter builds an adapter over a discordgo session. The caller owns the
// session lifecycle (Open/Close); the adapter only issues REST calls. Commands
// do not execute until Start runs the worker goroutine.
func NewAdapter(session *discordgo.Session, config *configs.Config) *Adapter {
	// Take full control of rate-limit handling so our queue can honor
	// Retry-After deterministically instead of discordgo's built-in retries
	// racing the scheduler.
	if session != nil {
		session.ShouldRetryOnRateLimit = false
	}
	a := &Adapter{
		api:     session,
		guildID: config.DiscordGuildId,
		sleep:   realSleeper{},
		cmds:    make(chan func() error),
		errs:    make(chan error),
	}
	return a
}

// Start launches the single command worker. It must be called once before any
// Bot method is used. The worker runs for the adapter's lifetime.
func (a *Adapter) Start() {
	go func() {
		defer close(a.errs)
		for cmd := range a.cmds {
			if err := cmd(); err != nil {
				log.Warn().Err(err).Msg("discord command failed")
			}
		}
	}()
}

// dispatch runs a command on the queue worker, retrying on rate limits, and
// returns its result. ctx is respected for cancellation before dispatch; once
// running, the Discord call honors its own timeouts.
func (a *Adapter) dispatch(ctx context.Context, cmd func() error) error {
	if a.api == nil {
		return discordport.ErrNotConfigured
	}
	type result struct{ err error }
	res := make(chan result, 1)
	wrapped := func() error {
		err := withRateLimitRetry(a.sleep, cmd)
		res <- result{err}
		return nil
	}
	select {
	case a.cmds <- wrapped:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case r := <-res:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// EnsureRole returns the id of the role named `name`, creating it if absent.
// Idempotent: an existing role by the same name is reused rather than
// duplicated. Naming convention (SRS §7): the role name is the courseId.
func (a *Adapter) EnsureRole(ctx context.Context, name string) (string, error) {
	var id string
	err := a.dispatch(ctx, func() error {
		roles, err := a.api.GuildRoles(a.guildID)
		if err != nil {
			return err
		}
		for _, r := range roles {
			if r.Name == name {
				id = r.ID
				return nil
			}
		}
		created, err := a.api.GuildRoleCreate(a.guildID, &discordgo.RoleParams{Name: name})
		if err != nil {
			return err
		}
		id = created.ID
		return nil
	})
	return id, err
}

// EnsureChannel returns the id of the text channel named `name`, creating it if
// absent and restricting it to roleID. Idempotent. Naming convention (SRS §7):
// the channel name is lowercase(courseId).
func (a *Adapter) EnsureChannel(ctx context.Context, name string, roleID string) (string, error) {
	var id string
	err := a.dispatch(ctx, func() error {
		chans, err := a.api.GuildChannels(a.guildID)
		if err != nil {
			return err
		}
		for _, c := range chans {
			if c.Name == name {
				id = c.ID
				return nil
			}
		}
		created, err := a.api.GuildChannelCreateComplex(a.guildID, permissionLockedChannel(name, roleID))
		if err != nil {
			return err
		}
		id = created.ID
		return nil
	})
	return id, err
}

// AssignRole adds roleID to userID. No-op-equivalent if already assigned:
// Discord returns 204 regardless, so the call is idempotent at the API level.
func (a *Adapter) AssignRole(ctx context.Context, userID, roleID string) error {
	return a.dispatch(ctx, func() error {
		return a.api.GuildMemberRoleAdd(a.guildID, userID, roleID)
	})
}

// RemoveRole removes roleID from userID. Idempotent at the API level.
func (a *Adapter) RemoveRole(ctx context.Context, userID, roleID string) error {
	return a.dispatch(ctx, func() error {
		return a.api.GuildMemberRoleRemove(a.guildID, userID, roleID)
	})
}

// MembersWithRole returns user IDs currently holding roleID. Used by role-sync
// to compute removals. It paginates all guild members and filters by role;
// guilds larger than Discord's member-list limits would need the GUILD_MEMBERS
// intent instead, noted as a known limitation for very large servers.
func (a *Adapter) MembersWithRole(ctx context.Context, roleID string) ([]string, error) {
	var out []string
	err := a.dispatch(ctx, func() error {
		var after string
		for {
			page, err := a.api.GuildMembers(a.guildID, after, 1000)
			if err != nil {
				return err
			}
			for _, m := range page {
				for _, rid := range m.Roles {
					if rid == roleID {
						out = append(out, m.User.ID)
						break
					}
				}
			}
			if len(page) < 1000 {
				return nil
			}
			after = page[len(page)-1].User.ID
		}
	})
	return out, err
}

// permissionLockedChannel builds the create payload for a class channel visible
// only to its role and admins. @everyone is denied ViewChannel; the role is
// granted View/Send/Read. Overwrites keep the channel private regardless of
// guild-wide @everyone settings (SRS §11.2).
func permissionLockedChannel(name, roleID string) discordgo.GuildChannelCreateData {
	denyEveryone := discordgo.GuildChannelCreateData{
		Name: name,
		Type: discordgo.ChannelTypeGuildText,
		PermissionOverwrites: []*discordgo.PermissionOverwrite{
			{ID: aEveryoneRoleID, Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel},
			{ID: roleID, Type: discordgo.PermissionOverwriteTypeRole, Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionReadMessageHistory},
		},
	}
	return denyEveryone
}

// aEveryoneRoleID is the fixed id of the @everyone role in every guild.
const aEveryoneRoleID = "@everyone"

// rateLimiter is the part of discordgo.RateLimitError we read.
type rateLimiter interface {
	RetryAfter() time.Duration
}

// withRateLimitRetry runs cmd, and if it returns a discordgo rate-limit error,
// sleeps Retry-After and retries, up to maxRetries times. Other errors are
// returned immediately. This is the explicit backoff the architecture calls for
// (§4.1), complementing the adapter's serialized queue.
func withRateLimitRetry(slp sleeper, cmd func() error) error {
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := cmd()
		if err == nil {
			return nil
		}
		lastErr = err
		var rl *discordgo.RateLimitError
		if errors.As(err, &rl) {
			d := rl.RetryAfter
			if d <= 0 {
				d = time.Second
			}
			log.Warn().Dur("retry_after", d).Int("attempt", attempt+1).Msg("discord rate limit; backing off")
			slp.Sleep(d)
			continue
		}
		// Some HTTP 429s surface as a RESTError body without the typed wrapper;
		// detect the textual marker and back off conservatively.
		if strings.Contains(err.Error(), "429") {
			slp.Sleep(time.Second)
			continue
		}
		return err
	}
	return lastErr
}

// Compile-time assertion that *Adapter satisfies the Bot port.
var _ discordport.Bot = (*Adapter)(nil)
