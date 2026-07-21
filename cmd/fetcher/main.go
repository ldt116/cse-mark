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

	// Roster sync runs on its own cadence (ROSTER_SYNC_INTERVAL), independent of
	// the 10-minute mark sync. A roster failure is logged inside Run() and never
	// affects mark sync.
	go app.RosterService.Run()

	app.SyncService.Run()

	defer app.MongoClient.Disconnect()
}
