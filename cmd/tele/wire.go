//go:build wireinject

package main

import (
	"github.com/google/wire"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/tele"
	"thuanle/cse-mark/internal/delivery/tele/handlers"
	"thuanle/cse-mark/internal/delivery/tele/middlewares"
	"thuanle/cse-mark/internal/delivery/tele/views"
	"thuanle/cse-mark/internal/domain/course"
	emailport "thuanle/cse-mark/internal/domain/email"
	"thuanle/cse-mark/internal/infra/http"
	"thuanle/cse-mark/internal/infra/mongo"
	"thuanle/cse-mark/internal/usecases/iam"
	"thuanle/cse-mark/internal/usecases/identity"
	"thuanle/cse-mark/internal/usecases/markimport"
)

type App struct {
	Config      *configs.Config
	MongoClient *mongo.Client
	TeleService *tele.Service
}

func InitializeApp() (*App, error) {
	wire.Build(
		//configurations
		configs.LoadConfig,

		//infrastructures
		mongo.NewClient,
		mongo.NewCourseRepo,
		mongo.NewMarkRepo,
		mongo.NewUserRepo,
		mongo.NewStudentRepo,
		mongo.NewBindingRepo,
		mongo.NewVerificationRepo,
		http.NewSimpleDownloader,

		//domain repositories and rules
		course.NewRules,

		//usecases
		markimport.NewService,
		iam.NewAuthzService,
		identity.NewService,
		ProvideSender, // email.Sender: SMTP if configured else fail-closed (OTP_SENDER=log for dev)

		//delivery-view
		views.NewTeacherRenderer,

		//delivery
		ProvideGuestHandler,
		handlers.NewBindHandler,
		handlers.NewTeacherHandler,
		handlers.NewAdminHandler,
		middlewares.NewTeacherOnly,
		tele.NewService,

		//app
		wire.Struct(new(App), "*"),
	)
	return &App{}, nil
}
