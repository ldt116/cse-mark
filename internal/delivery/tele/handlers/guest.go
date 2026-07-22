package handlers

import (
	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v3"
	"strings"
	"thuanle/cse-mark/internal/delivery/tele/handlers/helpers"
	"thuanle/cse-mark/internal/delivery/tele/models"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
)

type Guest struct {
	courseRules *course.Rules

	markRepo mark.Repository

	// identity resolves the bound MSSV for a Telegram chat (v2 /mark). Nil in
	// v1-only wiring; GetMark falls back to the legacy courseId+studentId form.
	identity identityLookup
}

// identityLookup is the subset of identity.Service the guest /mark handler uses.
type identityLookup interface {
	GetBinding(platform, platformUserID string) (binding.Model, error)
}

type GuestOpts func(*Guest)

// WithGuestIdentity injects identity so /mark resolves MSSV from the binding.
func WithGuestIdentity(id identityLookup) GuestOpts {
	return func(g *Guest) { g.identity = id }
}

func NewGuestHandler(courseRules *course.Rules, markRepo mark.Repository, opts ...GuestOpts) *Guest {
	g := &Guest{
		courseRules: courseRules,
		markRepo:    markRepo,
	}
	for _, o := range opts {
		o(g)
	}
	return g
}

func (h *Guest) Start(c telebot.Context) error {
	chatId := c.Chat().ID
	chatUsername := c.Chat().Username

	return helpers.Sendf(c, "Hello @%s (%d)\nDùng /bind để liên kết tài khoản với MSSV.", chatUsername, chatId)
}

// GetMark serves /mark. In v2 (when identity is wired) it resolves the caller's
// MSSV from their binding: /mark <course> shows that course; /mark alone would
// show all — but Telegram groups every course, so with a course arg it returns
// one; without args and bound, it tells the user to specify a course. When
// identity is NOT wired (v1), the legacy /mark <course> <studentId> form still
// works for compatibility.
func (h *Guest) GetMark(c telebot.Context) error {
	args := c.Args()

	// v2 path: identity present → must bind.
	if h.identity != nil {
		chatID := c.Chat().ID
		b, err := h.identity.GetBinding(platformTelegram, platformUserID(chatID))
		if err != nil || !b.Verified {
			return helpers.Send(c, "Chưa xác thực. Dùng /bind để liên kết MSSV.")
		}
		if len(args) < 1 {
			return helpers.Send(c, "Dùng /mark <mã lớp> để xem điểm môn đó.")
		}
		courseId := args[0]
		if !h.courseRules.IsValidCourseId(courseId) {
			return models.NewArgValueMismatchError("course invalid")
		}
		log.Info().Int64("chatId", chatID).Str("course", courseId).Str("mssv", b.MSSV).Msg("Get mark (bound)")
		msg, err := h.markRepo.GetMark(courseId, b.MSSV)
		if err != nil {
			return helpers.Send(c, "Chưa có điểm cho "+courseId+".")
		}
		return helpers.SendPre(c, courseId+"\n"+msg)
	}

	// Legacy v1 path: /mark <course> <studentId>.
	courseId, studentId, err := helpers.Args2StrStr(c)
	if err != nil {
		parts := strings.Split(c.Text(), " ")
		if len(parts) != 2 {
			return err
		}
		courseId = parts[0]
		studentId = parts[1]
	}

	if !h.courseRules.IsValidCourseId(courseId) {
		return models.NewArgValueMismatchError("course invalid")
	}

	log.Info().
		Int64("chatId", c.Chat().ID).
		Str("course", courseId).
		Str("studentId", studentId).
		Msg("Get mark (legacy)")

	msg, err := h.markRepo.GetMark(courseId, studentId)
	if err != nil {
		return err
	}

	return helpers.SendPre(c, msg)
}
