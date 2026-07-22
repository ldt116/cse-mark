package courseadmin

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/domain/course"
	discordport "thuanle/cse-mark/internal/domain/discord"
	"thuanle/cse-mark/internal/domain/discordmapping"
	"thuanle/cse-mark/internal/usecases/markimport"
)

// ProvisionResult is what /create and /sync report back: the import count and
// the Discord role/channel ids (so the delivery layer can echo them).
type ProvisionResult struct {
	CourseId   string
	Imported   int
	RoleID     string
	ChannelID  string
	Link       string
	Mapped     bool // true if a discord_mappings record exists after provisioning
}

// importer is the mark-import capability courseadmin needs. markimport.Service
// satisfies it (FetchMarkLinkIntoCourse). Defined as an interface so the use
// case is unit-testable without a network/downloader.
type importer interface {
	FetchMarkLinkIntoCourse(courseId string, link string) (int, error)
}

// Service provisions a course: registers/updates the course + CSV link, imports
// marks (reusing markimport), and on Discord ensures the role+channel exist and
// persists their ids into discord_mappings. It is the shared backend for
// /create and /sync (issues #11) and is reused by role-sync (#12) which only
// needs the mapping lookup.
type Service struct {
	courseRepo  course.Repository
	mappingRepo discordmapping.Repository
	imports     importer
	bot         discordport.Bot
	rules       *course.Rules
}

func NewService(
	courseRepo course.Repository,
	mappingRepo discordmapping.Repository,
	importService *markimport.Service,
	bot discordport.Bot,
	rules *course.Rules,
) *Service {
	return &Service{
		courseRepo:  courseRepo,
		mappingRepo: mappingRepo,
		imports:     importService,
		bot:         bot,
		rules:       rules,
	}
}

// Create provisions a course from scratch or refreshes an existing one. It:
//  1. Validates courseId/link.
//  2. Persists the course + link (admin ownership fields are blank — v2 has no
//     per-course ownership, SRS §7).
//  3. Imports marks (reuses markimport).
//  4. On Discord, ensures role (courseId) + channel (lowercase(courseId)),
//     and saves their ids into discord_mappings (idempotent).
//  5. Reconciles roles immediately so /create grants access without waiting for
//     the scheduler (the reconciliation itself lives in classsync; here we only
//     provision + persist mapping so the scheduler picks it up).
//
// `actor` is an admin identifier for logging only.
func (s *Service) Create(ctx context.Context, courseId, link, actor string) (ProvisionResult, error) {
	if !s.rules.IsValidCourseId(courseId) {
		return ProvisionResult{}, ErrInvalidCourseId
	}
	if !isValidURL(link) {
		return ProvisionResult{}, ErrInvalidLink
	}

	// v2 ownership is intentionally blank (SRS §7): all admins operate on all
	// courses, and the legacy by_id/by_user fields are kept only for backward
	// compatibility at the storage layer.
	if err := s.courseRepo.UpdateCourseLink(courseId, link, 0, ""); err != nil {
		return ProvisionResult{}, err
	}

	imported, err := s.imports.FetchMarkLinkIntoCourse(courseId, link)
	if err != nil {
		return ProvisionResult{}, err
	}

	res := ProvisionResult{CourseId: courseId, Imported: imported, Link: link}

	// Provision Discord role + channel and persist ids. If a mapping already
	// exists (re-/create), we still re-ensure and refresh the ids.
	roleID, err := s.bot.EnsureRole(ctx, courseId)
	if err != nil {
		return res, err
	}
	channelID, err := s.bot.EnsureChannel(ctx, strings.ToLower(courseId), roleID)
	if err != nil {
		return res, err
	}
	res.RoleID = roleID
	res.ChannelID = channelID

	if err := s.mappingRepo.Upsert(discordmapping.Model{
		CourseId:         courseId,
		DiscordRoleId:    roleID,
		DiscordChannelId: channelID,
	}); err != nil {
		return res, err
	}
	res.Mapped = true

	log.Info().
		Str("course", courseId).
		Str("actor", actor).
		Int("imported", imported).
		Str("role", roleID).
		Str("channel", channelID).
		Msg("Course provisioned")
	return res, nil
}

// Sync reloads marks for an already-provisioned course and returns the import
// count. Role reconciliation (assign/remove) is the scheduler's job (#12);
// /sync here means "refresh the data + the role-sync will pick it up". A
// lightweight immediate reconcile is delegated to the caller (delivery) via the
// classsync service if available. We keep this method focused on data refresh.
func (s *Service) Sync(ctx context.Context, courseId, actor string) (int, error) {
	if !s.rules.IsValidCourseId(courseId) {
		return 0, ErrInvalidCourseId
	}
	c, err := s.courseRepo.FindCourseById(courseId)
	if err != nil {
		return 0, err
	}
	if !isValidURL(c.Link) {
		return 0, ErrInvalidLink
	}
	imported, err := s.imports.FetchMarkLinkIntoCourse(courseId, c.Link)
	if err != nil {
		return 0, err
	}
	log.Info().
		Str("course", courseId).
		Str("actor", actor).
		Int("imported", imported).
		Msg("Course synced")
	return imported, nil
}
