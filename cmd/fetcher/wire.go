//go:build wireinject

package main

import (
	"github.com/google/wire"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/infra/http"
	"thuanle/cse-mark/internal/infra/mongo"
	"thuanle/cse-mark/internal/usecases/coursequery"
	"thuanle/cse-mark/internal/usecases/markimport"
	"thuanle/cse-mark/internal/usecases/marksync"
	"thuanle/cse-mark/internal/usecases/rostersync"
)

type App struct {
	Config *configs.Config

	//infrastructures
	MongoClient *mongo.Client

	//usecases
	SyncService   *marksync.Service
	RosterService *rostersync.Service
}

func InitializeApp() (*App, error) {
	wire.Build(
		//configurations
		configs.LoadConfig,

		//infrastructures
		mongo.NewClient,
		mongo.NewCourseRepo,
		mongo.NewMarkRepo,
		mongo.NewStudentRepo,
		http.NewSimpleDownloader,

		//domain repositories and rules
		course.NewRules,

		//usecases
		coursequery.NewActiveCourseService,
		markimport.NewService,
		marksync.NewService,
		rostersync.NewService,

		//app
		wire.Struct(new(App), "*"),
	)
	return &App{}, nil
}
