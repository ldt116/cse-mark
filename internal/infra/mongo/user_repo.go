package mongo

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"sync"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/user"
	"time"
)

type UserRepo struct {
	db         *mongo.Database
	collection *mongo.Collection
	mu         sync.Mutex
	timeout    time.Duration
}

func NewUserRepo(client *Client, config *configs.Config) user.Repository {
	db := client.mgClient.Database(config.DbSettings)
	return &UserRepo{
		db:         db,
		collection: db.Collection(config.DbSettingsUsers),
		timeout:    client.Timeout,
		mu:         sync.Mutex{},
	}
}

func (r *UserRepo) UpdateUser(username string, isTeacher bool, grantedBy string) error {
	u := user.Model{
		UserId:    username,
		IsTeacher: isTeacher,
		GrantedBy: grantedBy,
	}
	update := bson.M{"$set": u}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, u.UserId, update, options.UpdateOne().SetUpsert(true))

	return err
}

func (r *UserRepo) FindUserById(username string) (user.Model, error) {
	filter := bson.M{"_id": username}
	var result user.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	err := r.collection.FindOne(ctx, filter).Decode(&result)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = user.ErrNotFound
	}
	return result, err
}
