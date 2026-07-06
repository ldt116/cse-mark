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

// Upsert stores (or overwrites) the OTP for a platform user id, resetting the
// failed-attempt counter. The TTL index removes the record once `expiry` passes.
func (r *VerificationRepo) Upsert(m verification.Model) error {
	update := bson.M{"$set": bson.M{
		"email":    m.Email,
		"otp":      m.OTP,
		"expiry":   m.Expiry,
		"attempts": 0,
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

// IncrementAttempts atomically increments the failed-attempt counter and returns
// the new value. It is meant to be called only when a record already exists; if
// none does, it returns ErrNotFound.
func (r *VerificationRepo) IncrementAttempts(platformUserID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	var res verification.Model
	err := r.collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": platformUserID},
		bson.M{"$inc": bson.M{"attempts": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = verification.ErrNotFound
	}
	return res.Attempts, err
}

func (r *VerificationRepo) FindByEmail(email string) ([]verification.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cur, err := r.collection.Find(ctx, bson.M{"email": email})
	if err != nil {
		return nil, err
	}

	var out []verification.Model
	ctx2, cancel2 := context.WithTimeout(context.Background(), r.timeout)
	defer cancel2()
	if err := cur.All(ctx2, &out); err != nil {
		return nil, err
	}
	return out, nil
}
