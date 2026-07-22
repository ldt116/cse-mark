package classsync

import "thuanle/cse-mark/internal/domain/binding"

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

func (a *bindingRepoAdapter) DiscordUserID(mssv string) (string, error) {
	bindings, err := a.repo.FindByMSSV(mssv)
	if err != nil {
		return "", nil // missing binding is not an error for role-sync
	}
	for _, b := range bindings {
		if b.Platform == PlatformDiscord && b.Verified {
			return b.PlatformUserID, nil
		}
	}
	return "", nil
}
