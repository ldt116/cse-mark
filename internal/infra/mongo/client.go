package mongo

import (
	"context"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"time"
)

type Client struct {
	mgClient *mongo.Client
	Timeout  time.Duration
	ctx      context.Context
}

func NewClient(config *configs.Config) (*Client, error) {
	connectionString := `mongodb://` + config.MongoHost + `:` + config.MongoPort

	ctx, cancel := context.WithTimeout(context.Background(), config.DbTransactionTimeout)
	defer cancel()

	// Connect to MongoDB
	client, err := mongo.Connect(options.Client().ApplyURI(connectionString))
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
