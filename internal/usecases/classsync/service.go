package classsync

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	discordport "thuanle/cse-mark/internal/domain/discord"
	"thuanle/cse-mark/internal/domain/discordmapping"
	"thuanle/cse-mark/internal/domain/mark"
)

// bindingResolver maps MSSV → the platform user id(s) holding a verified
// binding on a given platform. Defined as an interface so the use case is
// testable without Mongo; the binding repo satisfies it via a thin adapter.
type bindingResolver interface {
	// DiscordUserID returns the Discord platform user id bound to mssv, or "" if
	// the MSSV has no Discord binding. (SRS §14: MSSV → Discord UserID via binding.)
	DiscordUserID(mssv string) (string, error)
}

// Service reconciles Discord roles from enrollment. For each provisioned course
// it computes enrolled MSSVs (from the mark cache), resolves their Discord user
// ids via bindings, diffs against the role's current members, and assigns/removes
// roles through discord.Bot. Only courses with a discord_mappings record are
// processed (SRS §10.3). Idempotent: re-running with no changes touches nothing.
type Service struct {
	mappings discordmapping.Repository
	marks    mark.Repository
	bindings bindingResolver
	bot      discordport.Bot

	interval time.Duration
}

func NewService(
	mappings discordmapping.Repository,
	marks mark.Repository,
	bindings bindingResolver,
	bot discordport.Bot,
	interval time.Duration,
) *Service {
	return &Service{
		mappings: mappings,
		marks:    marks,
		bindings: bindings,
		bot:      bot,
		interval: interval,
	}
}

// ReconcileOnce runs one role-sync cycle across all provisioned courses. It
// never aborts the whole cycle on a single course's error: the error is logged
// and that course is skipped, so one bad course never blocks the others.
func (s *Service) ReconcileOnce(ctx context.Context) {
	mappings, err := s.mappings.ListAll()
	if err != nil {
		log.Error().Err(err).Msg("role-sync: list mappings failed")
		return
	}

	for _, m := range mappings {
		if err := s.reconcileCourse(ctx, m); err != nil {
			log.Warn().Err(err).Str("course", m.CourseId).Msg("role-sync: course skipped")
			continue
		}
	}
}

// reconcileCourse handles a single course: enrolled = MSSVs with marks; resolve
// each to a Discord user id; diff against current role members; assign/remove.
func (s *Service) reconcileCourse(ctx context.Context, m discordmapping.Model) error {
	enrolled, err := s.marks.ListStudentIds(m.CourseId)
	if err != nil {
		return err
	}

	// Map enrolled MSSVs → Discord user ids, skipping any MSSV not yet bound on
	// Discord (SRS §14: unbound MSSVs are ignored, not an error).
	want := make(map[string]struct{}, len(enrolled))
	for _, mssv := range enrolled {
		uid, err := s.bindings.DiscordUserID(mssv)
		if err != nil {
			return err
		}
		if uid == "" {
			continue
		}
		want[uid] = struct{}{}
	}

	current, err := s.bot.MembersWithRole(ctx, m.DiscordRoleId)
	if err != nil {
		return err
	}
	have := make(map[string]struct{}, len(current))
	for _, uid := range current {
		have[uid] = struct{}{}
	}

	// toAdd = in want, not in have; toRemove = in have, not in want.
	added, removed := 0, 0
	for uid := range want {
		if _, ok := have[uid]; !ok {
			if err := s.bot.AssignRole(ctx, uid, m.DiscordRoleId); err != nil {
				log.Warn().Err(err).Str("user", uid).Str("role", m.DiscordRoleId).Msg("role-sync: assign failed")
				continue
			}
			added++
		}
	}
	for uid := range have {
		if _, ok := want[uid]; !ok {
			if err := s.bot.RemoveRole(ctx, uid, m.DiscordRoleId); err != nil {
				log.Warn().Err(err).Str("user", uid).Str("role", m.DiscordRoleId).Msg("role-sync: remove failed")
				continue
			}
			removed++
		}
	}

	log.Info().
		Str("course", m.CourseId).
		Int("enrolled", len(enrolled)).
		Int("added", added).
		Int("removed", removed).
		Msg("role-sync: course reconciled")
	return nil
}

// Run reconciles on the configured interval forever, tolerating per-cycle errors
// by logging and continuing. It blocks; the caller launches it as a goroutine.
func (s *Service) Run(ctx context.Context) {
	interval := s.interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	log.Info().Dur("interval", interval).Msg("role-sync scheduler started")
	// Run once immediately so a fresh start reconciles without waiting an interval.
	s.ReconcileOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.ReconcileOnce(ctx)
		case <-ctx.Done():
			log.Info().Msg("role-sync scheduler stopped")
			return
		}
	}
}
