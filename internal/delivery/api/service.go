package api

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/api/handlers"
	"thuanle/cse-mark/internal/delivery/api/middlewares"
)

type Service struct {
	engine *gin.Engine
	port   string
}

func NewApiService(authMiddleware *middlewares.Auth,
	marksHandler *handlers.Marks, config *configs.Config) *Service {
	engine := gin.Default()

	engine.GET("/mark", authMiddleware.Handle, marksHandler.GetMark)

	return &Service{
		engine: engine,
		port:   config.ApiPort,
	}
}

func (s *Service) Start() {
	log.Info().
		Str("port", s.port).
		Msg("HTTP service started")
	if err := s.engine.Run(":" + s.port); err != nil {
		log.Fatal().Err(err).Msg("Failed to start HTTP service")
		return
	}
}
