package mongo

import (
	"context"
	"thuanle/cse-mark/internal/configs"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Client struct {
	mgClient *mongo.Client
	Timeout  time.Duration
	ctx      context.Context
}

func NewClient(config *configs.Config) (*Client, error) {
	// connectionString := `mongodb://` + config.MongoHost + `:` + config.MongoPort
	connectionString := config.MongoURI
	ctx, cancel := context.WithTimeout(context.Background(), config.DbTransactionTimeout)
	defer cancel()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connectionString))
	if err != nil {
		log.Fatal().Err(err).Msg("mongo connect failed")
		return nil, err
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("mongo ping failed")
		return nil, err
	}

	log.Info().Msg("MongoDB connection established")

	return &Client{
		mgClient: client,
		ctx:      ctx,
		Timeout:  config.DbTransactionTimeout,
	}, nil
}

func (c *Client) Disconnect() {
	_ = c.mgClient.Disconnect(c.ctx)
}
