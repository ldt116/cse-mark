package main

import (
	"context"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/discord"
	"thuanle/cse-mark/internal/domain/course"
	emailinfra "thuanle/cse-mark/internal/infra/email"
	"thuanle/cse-mark/internal/infra"
	infraDiscord "thuanle/cse-mark/internal/infra/discord"
	"thuanle/cse-mark/internal/infra/http"
	"thuanle/cse-mark/internal/infra/mongo"
	"thuanle/cse-mark/internal/usecases/classsync"
	"thuanle/cse-mark/internal/usecases/courseadmin"
	"thuanle/cse-mark/internal/usecases/identity"
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
	studentRepo := mongo.NewStudentRepo(mongoClient, cfg)
	bindingRepo := mongo.NewBindingRepo(mongoClient, cfg)
	verificationRepo := mongo.NewVerificationRepo(mongoClient, cfg)

	// Infra: HTTP downloader, email sender (OTP), Discord bot/session.
	downloader := http.NewSimpleDownloader(cfg)
	sender := emailinfra.NewSenderFromConfig(cfg) // SMTP if configured, fail-closed otherwise; OTP_SENDER=log for dev
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
	ident := identity.NewService(studentRepo, verificationRepo, bindingRepo, sender, cfg)

	// Role-sync scheduler: reconcile Discord roles from enrollment on its own
	// cadence (ROLE_SYNC_INTERVAL). Runs as a goroutine; a failure in one cycle
	// is logged inside Run and never affects the bot.
	if cfg.RoleSyncInterval > 0 {
		roleSync := classsync.NewService(mappingRepo, markRepo, classsync.NewBindingResolver(bindingRepo), holder.Bot, cfg.RoleSyncInterval)
		go roleSync.Run(context.Background())
	} else {
		log.Warn().Msg("ROLE_SYNC_INTERVAL <= 0; role-sync scheduler disabled")
	}

	// Delivery. When the bot is not configured, Session is nil; NewService still
	// works but slash commands are never registered (no session to register on).
	// NewService registers its handlers as a side effect; the returned service is
	// kept only if future methods are added.
	_, err = discord.NewService(cfg, holder.Session, courseAdmin, ident, markRepo, studentRepo, courseRepo)
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
