//go:build wireinject

package main

import (
	"github.com/google/wire"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/api"
	"thuanle/cse-mark/internal/delivery/api/handlers"
	"thuanle/cse-mark/internal/delivery/api/middlewares"
	"thuanle/cse-mark/internal/infra/mongo"
)

type App struct {
	Config      *configs.Config
	MongoClient *mongo.Client
	ApiService  *api.Service
}

func InitializeApp() (*App, error) {
	wire.Build(
		//configurations
		configs.LoadConfig,

		//infrastructures
		mongo.NewClient,
		////mongo.NewCourseRepo,
		mongo.NewMarkRepo,
		//mongo.NewUserRepo,
		//http.NewSimpleDownloader,
		//
		////domain repositories and rules
		//course.NewRules,
		//
		////usecases
		//markimport.NewService,
		//iam.NewAuthzService,

		//delivery-api
		middlewares.NewAuthMiddleware,
		handlers.NewMarksHandler,
		//delivery
		api.NewApiService,
		//app
		wire.Struct(new(App), "*"),
	)
	return &App{}, nil
}
