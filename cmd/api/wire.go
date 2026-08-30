//go:build wireinject

package main

import (
	"time"

	"github.com/google/wire"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/api"
	"thuanle/cse-mark/internal/delivery/api/handlers"
	"thuanle/cse-mark/internal/delivery/api/middlewares"
	"thuanle/cse-mark/internal/domain/jwks"
	"thuanle/cse-mark/internal/infra/http"
	"thuanle/cse-mark/internal/infra/mongo"
	"thuanle/cse-mark/internal/usecases/assertion"
	"thuanle/cse-mark/internal/usecases/marksquery"
)

// JWKS client tuning (#44): timeout bounds each JWKS fetch, ttl bounds how
// long a fetched key set is served. Constants, not env — not operational
// tuning surface.
const (
	jwksRequestTimeout = 15 * time.Second
	jwksCacheTtl       = 5 * time.Minute
)

// newJwksRepository adapts config into the infra JWKS client.
func newJwksRepository(config *configs.Config) *http.JwksClient {
	return http.NewJwksClient(config.AuthJwksURL, jwksRequestTimeout, jwksCacheTtl)
}

// jwtAuthDeps bundles the two string options assertion.NewService takes, so
// wire can bind them as one value instead of two indistinguishable strings.
type jwtAuthDeps struct {
	issuer   string
	audience string
}

func newJwtAuthDeps(config *configs.Config) jwtAuthDeps {
	return jwtAuthDeps{
		issuer:   config.AuthJwtIssuer,
		audience: config.AuthJwtAudience,
	}
}

func newAssertionService(repo jwks.Repository, deps jwtAuthDeps) *assertion.Service {
	return assertion.NewService(repo, deps.issuer, deps.audience)
}

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
		mongo.NewCourseRepo,
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
		newJwksRepository,
		wire.Bind(new(jwks.Repository), new(*http.JwksClient)),
		newJwtAuthDeps,
		newAssertionService,
		marksquery.NewService,

		//delivery-api
		middlewares.NewAuthMiddleware,
		middlewares.NewJwtMiddleware,
		handlers.NewMarksHandler,
		handlers.NewStudentMarksHandler,
		handlers.NewHealthHandler,
		//delivery
		api.NewApiService,
		//app
		wire.Struct(new(App), "*"),
	)
	return &App{}, nil
}
