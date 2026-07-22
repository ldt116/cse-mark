package discord

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	discordport "thuanle/cse-mark/internal/domain/discord"
)

// NewSession creates and opens a discordgo session bound to the bot token. It
// returns the session (caller closes it on shutdown) plus an Adapter ready to
// Start. If the token is empty, it returns a not-configured adapter so the
// service can boot in a degraded/log-only mode (useful before secrets exist).
func NewSession(config *configs.Config) (*discordgo.Session, *Adapter, error) {
	if config.DiscordToken == "" || config.DiscordGuildId == "" {
		log.Warn().Msg("DISCORD_TOKEN/DISCORD_GUILD_ID not set; discord adapter is not configured")
		// Adapter with nil session → every op returns ErrNotConfigured.
		return nil, NewAdapter(nil, config), nil
	}
	session, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		return nil, nil, err
	}
	adapter := NewAdapter(session, config)
	adapter.Start()
	if err := session.Open(); err != nil {
		return nil, nil, err
	}
	return session, adapter, nil
}

// LogBot is a no-op Bot that logs each operation at info level. It is used in
// dev/tests when no Discord guild is configured, so the rest of the system
// (delivery, scheduler) can be exercised without touching Discord.
type LogBot struct{}

// NewLogBot returns a no-op Bot.
func NewLogBot() *LogBot { return &LogBot{} }

func (b *LogBot) EnsureRole(_ context.Context, name string) (string, error) {
	log.Info().Str("role", name).Msg("discord.EnsureRole (log bot)")
	return "log-role:" + name, nil
}

func (b *LogBot) EnsureChannel(_ context.Context, name, roleID string) (string, error) {
	log.Info().Str("channel", name).Str("role", roleID).Msg("discord.EnsureChannel (log bot)")
	return "log-channel:" + name, nil
}

func (b *LogBot) AssignRole(_ context.Context, userID, roleID string) error {
	log.Info().Str("user", userID).Str("role", roleID).Msg("discord.AssignRole (log bot)")
	return nil
}

func (b *LogBot) RemoveRole(_ context.Context, userID, roleID string) error {
	log.Info().Str("user", userID).Str("role", roleID).Msg("discord.RemoveRole (log bot)")
	return nil
}

func (b *LogBot) MembersWithRole(_ context.Context, roleID string) ([]string, error) {
	log.Info().Str("role", roleID).Msg("discord.MembersWithRole (log bot)")
	return nil, nil
}

// Compile-time: LogBot satisfies the port.
var _ discordport.Bot = (*LogBot)(nil)
