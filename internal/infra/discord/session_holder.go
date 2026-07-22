package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	discordport "thuanle/cse-mark/internal/domain/discord"
)

// SessionHolder bundles a discordgo session with the Bot that backs it. When the
// bot is not configured (no token/guild), Session is nil and Bot is a LogBot, so
// callers keep a single code path that degrades gracefully. It exists so
// cmd/discord can wire session + bot together without dragging discordgo types
// through the use-case layer.
type SessionHolder struct {
	Session *discordgo.Session  // nil when not configured
	Bot     discordport.Bot
}

// NewSessionHolder opens a discordgo session (or returns a not-configured holder)
// from config. On success the session is Opened and the Adapter is started.
func NewSessionHolder(config *configs.Config) (*SessionHolder, error) {
	if config.DiscordToken == "" || config.DiscordGuildId == "" {
		log.Warn().Msg("DISCORD_TOKEN/DISCORD_GUILD_ID not set; discord bot is a no-op LogBot")
		return &SessionHolder{Session: nil, Bot: NewLogBot()}, nil
	}
	session, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		return nil, err
	}
	adapter := NewAdapter(session, config)
	adapter.Start()
	if err := session.Open(); err != nil {
		return nil, err
	}
	return &SessionHolder{Session: session, Bot: adapter}, nil
}

// Close releases the discordgo session if it was opened.
func (h *SessionHolder) Close() {
	if h.Session != nil {
		_ = h.Session.Close()
	}
}
