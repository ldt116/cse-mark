package main

import (
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/infra"
)

func main() {
	infra.InitZerolog()
	_ = infra.InitDotenv()

	app, err := InitializeApp()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
		return
	}
	app.ApiService.Start()
}
