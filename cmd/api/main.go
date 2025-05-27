package main

import (
	"github.com/rs/zerolog/log"
	infrastructure "thuanle/cse-mark/internal/infra"
)

func main() {
	infrastructure.InitZerolog()
	_ = infrastructure.InitDotenv()

	app, err := InitializeApp()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
		return
	}
	app.ApiService.Start()
}
