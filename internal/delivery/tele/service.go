package tele

import (
	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/tele/handlers"
	"thuanle/cse-mark/internal/delivery/tele/middlewares"
	"time"
)

type Service struct {
	bot *telebot.Bot
}

var commands = []telebot.Command{
	{
		Text:        "bind",
		Description: "/bind - Liên kết tài khoản với MSSV (email OTP)",
	},
	{
		Text:        "mark",
		Description: "/mark <course> - Xem điểm môn (cần /bind trước)",
	},
	{
		Text:        "create",
		Description: "/create <course> <link> - Admin: nhập bảng điểm",
	},
	{
		Text:        "clear",
		Description: "/clear <course> - Admin: xoá lớp + điểm",
	},
	{
		Text:        "my",
		Description: "/my - Admin: danh sách lớp",
	},
	{
		Text:        "cancel",
		Description: "/cancel - Huỷ liên kết đang dở",
	},
}

func NewService(config *configs.Config,
	guestHandler *handlers.Guest, teacherHandler *handlers.Teacher, adminHandler *handlers.Admin,
	bindHandler *handlers.Bind,
	teacherOnlyMiddleware *middlewares.TeacherOnly) (*Service, error) {
	pref := telebot.Settings{
		Token:  config.TeleToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create telegram bot")
		return nil, err
	}

	if err := b.SetCommands(commands); err != nil {
		log.Fatal().Err(err).Msg("failed to set up telegram commands")
		return nil, err
	}

	b.Use(middlewares.SendErrorMiddleware)

	b.Handle("/start", guestHandler.Start)

	// /bind + conversation: OnText first goes through the bind flow; if a bind is
	// in progress the text is consumed, otherwise it falls through to the mark
	// path (legacy free-text /mark courseId studentId, kept for compatibility).
	b.Handle("/bind", bindHandler.Start)
	b.Handle("/cancel", bindHandler.Cancel)
	b.Handle("/mark", guestHandler.GetMark)
	b.Handle(telebot.OnText, func(c telebot.Context) error {
		if handled, err := bindHandler.OnText(c); err != nil {
			return err
		} else if handled {
			return nil
		}
		return guestHandler.GetMark(c)
	})

	teacherOnly := b.Group()
	teacherOnly.Use(teacherOnlyMiddleware.Handle)
	teacherOnly.Handle("/my", teacherHandler.GetMyProfile)
	// /create is the renamed /load (SRS §12.1); keep /load as an alias so existing
	// admins are not broken during rollout.
	teacherOnly.Handle("/create", teacherHandler.LoadCourseLink)
	teacherOnly.Handle("/load", teacherHandler.LoadCourseLink)
	teacherOnly.Handle("/clear", teacherHandler.ClearCourseLink)

	adminOnly := b.Group()
	adminOnly.Use(middleware.Whitelist(config.TeleAdminChatIds...))
	adminOnly.Handle("/teacher", adminHandler.SetTeacher)

	return &Service{
		bot: b,
	}, nil
}

func (s *Service) Run() {
	log.Info().Msg("Starting telegram bot")
	s.bot.Start()
}
