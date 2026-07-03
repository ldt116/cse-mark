package main

import (
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/infra"
	"thuanle/cse-mark/internal/infra/mongo"
)

func main() {
	infra.InitZerolog()
	_ = infra.InitDotenv()

	log.Info().Msg("Initialization completed successfully")

	app, err := InitializeApp()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
		return
	}

	if err := mongo.EnsureIndexes(app.MongoClient, app.Config); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure database indexes")
		return
	}

	app.SyncService.Run()

	defer app.MongoClient.Disconnect()
}
