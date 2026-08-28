package handlers

import (
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v3"
	"thuanle/cse-mark/internal/delivery/tele/handlers/helpers"
	"thuanle/cse-mark/internal/delivery/tele/models"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
)

type Guest struct {
	courseRules *course.Rules

	markRepo mark.Repository

	// courseLister feeds the no-arg /mark summary (every enrolled course).
	courseLister courseLister

	// identity resolves the bound MSSV for a Telegram chat (v2 /mark). Nil in
	// v1-only wiring; GetMark falls back to the legacy courseId+studentId form.
	identity identityLookup
}

// identityLookup is the subset of identity.Service the guest /mark handler uses.
type identityLookup interface {
	GetBinding(platform, platformUserID string) (binding.Model, error)
}

// courseLister is the subset of course.Repository the guest /mark handler uses.
type courseLister interface {
	FindCoursesUpdatedAfter(since time.Time) ([]course.Model, error)
}

type GuestOpts func(*Guest)

// WithGuestIdentity injects identity so /mark resolves MSSV from the binding.
func WithGuestIdentity(id identityLookup) GuestOpts {
	return func(g *Guest) { g.identity = id }
}

// WithGuestCourseLister injects the course repository so /mark without a
// course arg can summarize every enrolled course.
func WithGuestCourseLister(cl courseLister) GuestOpts {
	return func(g *Guest) { g.courseLister = cl }
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
// MSSV from their binding — DM-only, keyed by sender (issue #38): /mark <course>
// shows that course; /mark alone summarizes every course the student has marks
// in (mirroring Discord /mark). When identity is NOT wired (v1), the legacy
// /mark <course> <studentId> form still works for compatibility.
func (h *Guest) GetMark(c telebot.Context) error {
	args := c.Args()

	// v2 path: identity present → must bind. Identity is personal: resolve by
	// sender, and only in DM — a group reply would expose the sender's MSSV
	// binding to everyone (issue #38).
	if h.identity != nil {
		if !isPrivate(c) {
			// Plain group chatter reaches GetMark via the OnText fallthrough —
			// answer command attempts only, so chatter stays silent instead of
			// bouncing a DM notice off every group message.
			if !strings.HasPrefix(c.Text(), "/") {
				return nil
			}
			return helpers.Send(c, dmOnlyMsg+" Hãy /bind rồi dùng /mark <mã lớp> qua DM.")
		}
		userID := c.Sender().ID
		b, err := h.identity.GetBinding(platformTelegram, platformUserID(userID))
		if err != nil || !b.Verified {
			return helpers.Send(c, "Chưa xác thực. Dùng /bind để liên kết MSSV.")
		}
		if len(args) < 1 {
			return h.sendAllCourses(c, b.MSSV)
		}
		courseId := args[0]
		if !h.courseRules.IsValidCourseId(courseId) {
			return models.NewArgValueMismatchError("course invalid")
		}
		log.Info().Int64("senderId", userID).Int64("chatId", c.Chat().ID).Str("course", courseId).Str("mssv", b.MSSV).Msg("Get mark (bound)")
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

// sendAllCourses summarizes the student's marks across every course, mirroring
// the Discord /mark summary: courses the student has no mark in are skipped,
// and with no marks at all the user gets a plain notice.
func (h *Guest) sendAllCourses(c telebot.Context, mssv string) error {
	if h.courseLister == nil {
		return helpers.Send(c, "Dùng /mark <mã lớp> để xem điểm môn đó.")
	}
	courses, err := h.courseLister.FindCoursesUpdatedAfter(time.Unix(0, 0))
	if err != nil {
		log.Warn().Err(err).Msg("list courses for /mark summary failed")
		return helpers.Send(c, "Không lấy được danh sách lớp lúc này. Thử lại sau.")
	}
	var b strings.Builder
	for _, cs := range courses {
		m, err := h.markRepo.GetMark(cs.Id, mssv)
		if err != nil {
			continue
		}
		b.WriteString(cs.Id + "\n" + m + "\n\n")
	}
	if b.Len() == 0 {
		return helpers.Send(c, "Chưa có điểm nào.")
	}
	return helpers.SendPre(c, strings.TrimSpace(b.String()))
}
