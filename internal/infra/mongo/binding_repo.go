package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/binding"
)

type BindingRepo struct {
	collection *mongo.Collection
	timeout    time.Duration
}

func NewBindingRepo(client *Client, config *configs.Config) binding.Repository {
	db := client.mgClient.Database(config.DbSettings)
	return &BindingRepo{
		collection: db.Collection(config.DbSettingsBindings),
		timeout:    client.Timeout,
	}
}

// Upsert inserts or updates a binding keyed by (platform, platform_user_id).
// The unique indexes enforce the 1:1:1 constraint; a conflicting (platform,
// mssv) yields a duplicate-key error returned to the caller.
func (r *BindingRepo) Upsert(m binding.Model) error {
	filter := bson.M{"platform": m.Platform, "platform_user_id": m.PlatformUserID}
	update := bson.M{"$set": bson.M{
		"platform":         m.Platform,
		"platform_user_id": m.PlatformUserID,
		"mssv":             m.MSSV,
		"verified":         m.Verified,
		"bound_at":         m.BoundAt,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *BindingRepo) FindByPlatformUser(platform, platformUserID string) (binding.Model, error) {
	return r.findOne(bson.M{"platform": platform, "platform_user_id": platformUserID})
}

func (r *BindingRepo) FindByPlatformMSSV(platform, mssv string) (binding.Model, error) {
	return r.findOne(bson.M{"platform": platform, "mssv": mssv})
}

func (r *BindingRepo) FindByMSSV(mssv string) ([]binding.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cur, err := r.collection.Find(ctx, bson.M{"mssv": mssv})
	if err != nil {
		return nil, err
	}

	var out []binding.Model
	ctx2, cancel2 := context.WithTimeout(context.Background(), r.timeout)
	defer cancel2()
	if err := cur.All(ctx2, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *BindingRepo) findOne(filter bson.M) (binding.Model, error) {
	var res binding.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.collection.FindOne(ctx, filter).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = binding.ErrNotFound
	}
	return res, err
}
