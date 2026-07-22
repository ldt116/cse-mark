package main

import (
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/discord"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/infra"
	infraDiscord "thuanle/cse-mark/internal/infra/discord"
	"thuanle/cse-mark/internal/infra/http"
	"thuanle/cse-mark/internal/infra/mongo"
	"thuanle/cse-mark/internal/usecases/courseadmin"
	"thuanle/cse-mark/internal/usecases/markimport"
)

func main() {
	infra.InitZerolog()
	_ = infra.InitDotenv()

	log.Info().Msg("Initialization completed successfully")

	cfg := configs.LoadConfig()

	mongoClient, err := mongo.NewClient(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("mongo connect failed")
		return
	}
	defer mongoClient.Disconnect()

	if err := mongo.EnsureIndexes(mongoClient, cfg); err != nil {
		log.Fatal().Err(err).Msg("ensure indexes failed")
		return
	}

	// Repositories.
	courseRepo := mongo.NewCourseRepo(mongoClient, cfg)
	markRepo := mongo.NewMarkRepo(mongoClient, cfg)
	mappingRepo := mongo.NewDiscordMappingRepo(mongoClient, cfg)

	// Infra: HTTP downloader + Discord bot/session.
	downloader := http.NewSimpleDownloader(cfg)
	holder, err := infraDiscord.NewSessionHolder(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("discord session failed")
		return
	}
	defer holder.Close()

	// Use cases.
	rules := course.NewRules(cfg)
	importService := markimport.NewService(downloader, courseRepo, markRepo)
	courseAdmin := courseadmin.NewService(courseRepo, mappingRepo, importService, holder.Bot, rules)

	// Delivery. When the bot is not configured, Session is nil; NewService still
	// works but slash commands are never registered (no session to register on).
	// NewService registers its handlers as a side effect; the returned service is
	// kept only if future methods are added.
	_, err = discord.NewService(cfg, holder.Session, courseAdmin)
	if err != nil {
		log.Fatal().Err(err).Msg("discord service init failed")
		return
	}

	// Block forever: the discordgo session runs its own gateway goroutines.
	// When not configured there is nothing to run; exit cleanly after setup.
	if holder.Session == nil {
		log.Warn().Msg("Discord not configured; nothing to run. Exiting.")
		return
	}

	log.Info().Msg("Discord bot started")
	select {} // run until killed
}
