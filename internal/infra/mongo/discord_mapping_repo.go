package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/discordmapping"
)

type DiscordMappingRepo struct {
	collection *mongo.Collection
	timeout    time.Duration
}

func NewDiscordMappingRepo(client *Client, config *configs.Config) discordmapping.Repository {
	db := client.mgClient.Database(config.DbSettings)
	return &DiscordMappingRepo{
		collection: db.Collection(config.DbSettingsDiscordMappings),
		timeout:    client.Timeout,
	}
}

// Upsert stores the provisioned Discord role/channel ids for a course.
func (r *DiscordMappingRepo) Upsert(m discordmapping.Model) error {
	update := bson.M{"$set": bson.M{
		"discord_role_id":    m.DiscordRoleId,
		"discord_channel_id": m.DiscordChannelId,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, m.CourseId, update, options.Update().SetUpsert(true))
	return err
}

func (r *DiscordMappingRepo) Find(courseId string) (discordmapping.Model, error) {
	var res discordmapping.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.collection.FindOne(ctx, bson.M{"_id": courseId}).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = discordmapping.ErrNotFound
	}
	return res, err
}

func (r *DiscordMappingRepo) Remove(courseId string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": courseId})
	return err
}

// ListAll returns every mapping. Used by the role-sync scheduler.
func (r *DiscordMappingRepo) ListAll() ([]discordmapping.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cur, err := r.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var out []discordmapping.Model
	ctx2, cancel2 := context.WithTimeout(context.Background(), r.timeout)
	defer cancel2()
	if err := cur.All(ctx2, &out); err != nil {
		return nil, err
	}
	return out, nil
}
