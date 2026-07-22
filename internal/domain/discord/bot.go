package discord

import "context"

// Bot is the port for the Discord integration used by /create provisioning
// and the role-sync scheduler. Implementations live in infra (the discordgo
// adapter). The interface is deliberately small and returns IDs (role/channel)
// so the use case layer can persist them without depending on discordgo types.
// See docs/v2/architecture.md §4.1.
type Bot interface {
	// Provisioning (returns IDs to persist into discord_mappings).
	EnsureRole(ctx context.Context, name string) (roleID string, err error)
	EnsureChannel(ctx context.Context, name string, roleID string) (channelID string, err error)

	// Role membership (uses a previously persisted roleID).
	AssignRole(ctx context.Context, userID string, roleID string) error
	RemoveRole(ctx context.Context, userID string, roleID string) error
	// MembersWithRole returns the user IDs currently holding roleID. The
	// role-sync scheduler diffs this against enrollment to compute removals.
	MembersWithRole(ctx context.Context, roleID string) ([]string, error)
}

// ErrNotConfigured is returned by operations when the adapter was constructed
// without a guild/token (e.g. LogBot in tests). Callers treat it as a no-op.
var (
	ErrNotConfigured = errSentinel("discord bot not configured")
)

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
