package classsync

import (
	"errors"

	"thuanle/cse-mark/internal/domain/binding"
)

// PlatformDiscord is the platform constant the binding store uses for Discord.
const PlatformDiscord = "discord"

// bindingRepoAdapter satisfies bindingResolver by reading the binding.Repository
// and returning the Discord platform user id for an MSSV, if any.
type bindingRepoAdapter struct {
	repo binding.Repository
}

// NewBindingResolver wraps a binding.Repository as a bindingResolver.
func NewBindingResolver(repo binding.Repository) bindingResolver {
	return &bindingRepoAdapter{repo: repo}
}

// DiscordUserID returns the verified Discord user id for an MSSV.
//
// Only binding.ErrNotFound means "no binding → unbound": it returns ("", nil) so
// role-sync skips that MSSV. Every OTHER error (e.g. a Mongo timeout/outage) is
// PROPAGATED. This is critical: if a transient DB error were treated as
// "unbound", reconciliation would see all enrolled users as unbound, compute
// them as toRemove, and strip their existing course roles. Propagating makes
// reconcileCourse abort that course and skip it safely until the next cycle.
func (a *bindingRepoAdapter) DiscordUserID(mssv string) (string, error) {
	bindings, err := a.repo.FindByMSSV(mssv)
	if err != nil {
		if errors.Is(err, binding.ErrNotFound) {
			return "", nil // genuinely unbound → not an error
		}
		return "", err // transient failure → propagate, course is skipped
	}
	for _, b := range bindings {
		if b.Platform == PlatformDiscord && b.Verified {
			return b.PlatformUserID, nil
		}
	}
	return "", nil
}
