package api

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type Service struct {
	engine *gin.Engine
}

func NewApiService() *Service {
	engine := gin.Default()

	return &Service{
		engine: engine,
	}
}

func (s *Service) Start() {
	log.Info().Msg("HTTP service started")
	if err := s.engine.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start HTTP service")
		return
	}
}
