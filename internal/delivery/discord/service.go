package discord

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/usecases/courseadmin"
)

// courseAdminAPI is the subset of *courseadmin.Service the handlers call.
// Defining it lets handlers be unit-tested with a stub. courseadmin.Service
// satisfies it.
type courseAdminAPI interface {
	Create(ctx context.Context, courseId, link, actor string) (courseadmin.ProvisionResult, error)
	Sync(ctx context.Context, courseId, actor string) (int, error)
}

// Service is the Discord delivery layer. It owns the discordgo session's
// command registration and routes interactions to handlers. The session + mark
// imports are provided; handlers depend on use cases injected below.
type Service struct {
	cfg     *configs.Config
	gw      interactionGateway
	session *discordgo.Session

	admin *adminChecker

	courseAdmin courseAdminAPI

	// student-facing use cases are wired in M5; nil until then.
}

// NewService wires the delivery service. The discordgo session is owned by the
// caller (cmd/discord), which also Open/Close it; NewService only registers
// commands and interaction handlers. If session is nil (bot not configured),
// registration is skipped — the returned service is inert but safe, so the
// process can still boot in a degraded/log-only mode.
func NewService(
	cfg *configs.Config,
	session *discordgo.Session,
	courseAdmin *courseadmin.Service,
) (*Service, error) {
	s := &Service{
		cfg:         cfg,
		gw:          session,
		session:     session,
		admin:       &adminChecker{ids: cfg.DiscordAdminIds},
		courseAdmin: courseAdmin,
	}

	if session != nil {
		s.register()
	} else {
		log.Warn().Msg("discord session nil; delivery service is inert (not configured)")
	}
	return s, nil
}

// register installs the interaction-create handler and, on Ready, registers the
// guild slash commands (guild = immediate availability). It needs the bot's own
// application id, resolved from the token via User("@me") at ready time.
func (s *Service) register() {
	s.session.AddHandler(func(_ *discordgo.Session, r *discordgo.Ready) {
		log.Info().Str("user", r.User.Username).Msg("Discord bot ready")
		if s.cfg.DiscordGuildId == "" {
			log.Warn().Msg("DISCORD_GUILD_ID empty; skipping command registration")
			return
		}
		for _, cmd := range applicationCommands() {
			if _, err := s.gw.ApplicationCommandCreate(r.User.ID, s.cfg.DiscordGuildId, cmd); err != nil {
				log.Error().Err(err).Str("cmd", cmd.Name).Msg("register command")
			}
		}
		log.Info().Int("commands", len(applicationCommands())).Msg("Slash commands registered")
	})

	s.session.AddHandler(s.handleInteraction)
}

// handleInteraction routes an interaction to the right handler. Admin commands
// are gated by adminChecker; bind/mark/profile route to student handlers (M5).
func (s *Service) handleInteraction(sess *discordgo.Session, i *discordgo.Interaction) {
	// Use the discordgo helpers on the real session for accessors that need it.
	switch {
	case i.Type == discordgo.InteractionApplicationCommand:
		s.routeCommand(sess, i)
	case i.Type == discordgo.InteractionMessageComponent:
		s.routeComponent(sess, i) // buttons (bind verify) — M5
	case i.Type == discordgo.InteractionModalSubmit:
		s.routeModal(sess, i) // bind email/otp modals — M5
	default:
		_ = sess.InteractionRespond(i, ephemeralMsg("Không hỗ trợ tương tác này."))
	}
}

// routeCommand dispatches a slash command.
func (s *Service) routeCommand(sess *discordgo.Session, i *discordgo.Interaction) {
	name := i.ApplicationCommandData().Name
	switch name {
	case cmdCreate:
		s.handleCreate(i)
	case cmdSync:
		s.handleSync(i)
	case cmdBind, cmdMark, cmdProfile:
		// Wired in M5; respond with a placeholder so the command surface is coherent.
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Tính năng này sẽ khả dụng sớm."))
	default:
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Lệnh không xác định."))
	}
	_ = sess // session retained for future direct-send needs; responses go via gw
}

// not yet implemented stubs for component/modal routing (M5).
func (s *Service) routeComponent(_ *discordgo.Session, i *discordgo.Interaction) {
	_ = s.gw.InteractionRespond(i, ephemeralMsg("Tương tác chưa được kết nối."))
}
func (s *Service) routeModal(_ *discordgo.Session, i *discordgo.Interaction) {
	_ = s.gw.InteractionRespond(i, ephemeralMsg("Tương tác chưa được kết nối."))
}

// --- admin gating ---

type adminChecker struct {
	ids []string
}

// isAdmin reports whether a Discord user id is in the configured admin whitelist
// (DISCORD_ADMIN_IDS). v2 has only Admin and Student roles (SRS §5).
func (a *adminChecker) isAdmin(userID string) bool {
	for _, id := range a.ids {
		if id == userID {
			return true
		}
	}
	return false
}

// caller returns the invoking user's id from an interaction.
func caller(i *discordgo.Interaction) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// strOption reads a string option from the interaction's command data.
func strOption(i *discordgo.Interaction, name string) string {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == name {
			return opt.StringValue()
		}
	}
	// also support sub-options nesting (not used currently)
	return ""
}

// denyIfNotAdmin responds with an unauthorized ephemeral and returns true if the
// caller is not an admin, so handlers can early-return. Responses go through the
// gateway so handlers are testable with a fake session.
func (s *Service) denyIfNotAdmin(i *discordgo.Interaction) bool {
	if s.admin.isAdmin(caller(i)) {
		return false
	}
	_ = s.gw.InteractionRespond(i, ephemeralMsg("❌ Bạn không phải admin."))
	return true
}

// --- handlers ---

// handleCreate implements /create <course-id> <csv-url> for admins.
//
// Importing/provisioning (download CSV → import → EnsureRole/Channel) is
// network-bound and can exceed Discord's 3-second initial-response deadline, so
// we ACK with a deferred ("thinking") response immediately, then edit it with
// the result. If the initial ACK itself fails we cannot recover the
// interaction, so we log and bail.
func (s *Service) handleCreate(i *discordgo.Interaction) {
	if s.denyIfNotAdmin(i) {
		return
	}
	courseId := strOption(i, optCourse)
	link := strOption(i, optCsvURL)
	actor := caller(i)

	if err := s.gw.InteractionRespond(i, deferredMsg("Đang tạo lớp...")); err != nil {
		log.Warn().Err(err).Msg("create: deferred ACK failed")
		return
	}

	res, err := s.courseAdmin.Create(context.Background(), courseId, link, actor)
	if err != nil {
		_ = editInteraction(s.gw, i, fmt.Sprintf("❌ /create thất bại: %v", err))
		return
	}
	msg := fmt.Sprintf("✅ Lớp **%s** đã sẵn sàng.\n• Nhập %d dòng điểm.\n• Role `<%s>`, channel `#%s`.",
		res.CourseId, res.Imported, res.RoleID, res.ChannelID)
	_ = editInteraction(s.gw, i, msg)
}

// handleSync implements /sync <course-id> for admins. Same defer-then-edit
// pattern as /create because reloading marks can exceed the 3s deadline.
func (s *Service) handleSync(i *discordgo.Interaction) {
	if s.denyIfNotAdmin(i) {
		return
	}
	courseId := strOption(i, optCourse)
	actor := caller(i)

	if err := s.gw.InteractionRespond(i, deferredMsg("Đang đồng bộ...")); err != nil {
		log.Warn().Err(err).Msg("sync: deferred ACK failed")
		return
	}

	n, err := s.courseAdmin.Sync(context.Background(), courseId, actor)
	if err != nil {
		_ = editInteraction(s.gw, i, fmt.Sprintf("❌ /sync thất bại: %v", err))
		return
	}
	_ = editInteraction(s.gw, i, fmt.Sprintf("✅ Đã đồng bộ lại **%s** (%d dòng điểm). Role sẽ được cập nhật ở chu kỳ kế tiếp.", courseId, n))
}
