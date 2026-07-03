package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/verification"
)

type VerificationRepo struct {
	collection *mongo.Collection
	timeout    time.Duration
}

func NewVerificationRepo(client *Client, config *configs.Config) verification.Repository {
	db := client.mgClient.Database(config.DbSettings)
	return &VerificationRepo{
		collection: db.Collection(config.DbSettingsVerifications),
		timeout:    client.Timeout,
	}
}

// Upsert stores (or overwrites) the OTP for a platform user id. Each new OTP
// attempt simply replaces the previous record; the TTL index removes it once
// `expiry` passes.
func (r *VerificationRepo) Upsert(m verification.Model) error {
	update := bson.M{"$set": bson.M{
		"email":  m.Email,
		"otp":    m.OTP,
		"expiry": m.Expiry,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, m.PlatformUserID, update, options.Update().SetUpsert(true))
	return err
}

func (r *VerificationRepo) Find(platformUserID string) (verification.Model, error) {
	var res verification.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.collection.FindOne(ctx, bson.M{"_id": platformUserID}).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = verification.ErrNotFound
	}
	return res, err
}
