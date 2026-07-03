package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/student"
)

type StudentRepo struct {
	collection *mongo.Collection
	timeout    time.Duration
}

func NewStudentRepo(client *Client, config *configs.Config) student.Repository {
	db := client.mgClient.Database(config.DbSettings)
	return &StudentRepo{
		collection: db.Collection(config.DbSettingsStudents),
		timeout:    client.Timeout,
	}
}

// Upsert inserts or updates a student by MSSV.
func (r *StudentRepo) Upsert(m student.Model) error {
	// _id (MSSV) comes from the UpdateByID filter; never $set the immutable _id.
	update := bson.M{"$set": bson.M{
		"name":  m.Name,
		"email": m.Email,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, m.MSSV, update, options.Update().SetUpsert(true))
	return err
}

func (r *StudentRepo) FindByEmail(email string) (student.Model, error) {
	var res student.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.collection.FindOne(ctx, bson.M{"email": email}).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = student.ErrNotFound
	}
	return res, err
}

func (r *StudentRepo) FindByMSSV(mssv string) (student.Model, error) {
	var res student.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.collection.FindOne(ctx, bson.M{"_id": mssv}).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = student.ErrNotFound
	}
	return res, err
}
