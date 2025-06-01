package http

import (
	"thuanle/cse-mark/internal/delivery/http/handlers"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type Service struct {
	guestHandler   *handlers.GuestHandler
	teacherHandler *handlers.TeacherHandler
}

func NewHttpService(
	guestHandler *handlers.GuestHandler,
	teacherHandler *handlers.TeacherHandler,
) *Service {
	return &Service{
		guestHandler:   guestHandler,
		teacherHandler: teacherHandler,
	}
}

func (s *Service) Start() {
	r := gin.Default()

	public := r.Group("/api/guest")
	{
		public.GET("/mark", s.guestHandler.LookupMark)
	}

	teacher := r.Group("/api/teacher")
	{
		teacher.POST("/load", s.teacherHandler.LoadCourseLink)
		// teacher.GET("/my", s.teacherHandler.GetMyProfile)
	}

	log.Info().Msg("HTTP service started at :8080")
	_ = r.Run(":8080")
}
