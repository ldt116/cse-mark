package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/mark"
	"time"
)

type MarkRepo struct {
	db      *mongo.Database
	mu      sync.Mutex
	timeout time.Duration
}

func NewMarkRepo(client *Client, config *configs.Config) mark.Repository {
	db := client.mgClient.Database(config.DbMark)
	return &MarkRepo{
		db:      db,
		timeout: client.Timeout,
		mu:      sync.Mutex{},
	}
}

func (r *MarkRepo) GetMark(courseId string, studentId string) (string, error) {
	filter := bson.M{"_id": studentId}
	var result map[string]interface{}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.db.Collection(courseId).FindOne(ctx, filter).Decode(&result)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = mark.ErrNotFound
		return "", err
	}

	jsonStr, err := json.MarshalIndent(result, "", " ")
	return string(jsonStr), nil
}

func (r *MarkRepo) RemoveMarksByCourseId(courseId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.db.Collection(courseId).Drop(ctx)
	if err != nil {
		log.Error().
			Str("course", courseId).
			Err(err).
			Msg("ClearCourse")
		return err
	}
	return nil
}

func (r *MarkRepo) AddCourseMarks(courseId string, marks []map[string]string) error {
	var bulkWrites []mongo.WriteModel
	for _, aMark := range marks {
		// Ensure _id is set, if not use id
		if len(aMark["_id"]) == 0 {
			aMark["_id"] = aMark["id"]
		}

		filter := bson.M{"_id": aMark["_id"]}
		update := bson.M{"$set": aMark}
		model := mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
		bulkWrites = append(bulkWrites, model)
	}
	bulkWriteOptions := options.BulkWrite().SetOrdered(false)

	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	result, err := r.db.Collection(courseId).BulkWrite(ctx, bulkWrites, bulkWriteOptions)
	if err != nil {
		log.Error().
			Str("course", courseId).
			Err(err).
			Msg("Bulk write error")
		return err
	}

	// Print the number of modified documents
	log.Info().
		Str("course", courseId).
		Interface("result", result).
		Msg("Store marks")

	return nil
}

func (r *MarkRepo) RemoveCourseMarks(courseId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.db.Collection(courseId).Drop(ctx)
	if err != nil {
		log.Error().
			Str("courseId", courseId).
			Err(err).
			Msg("ClearCourse")

		return err
	}
	return nil
}

// ListStudentIds returns every _id (MSSV) in a course's mark collection, i.e.
// the roster of students with marks for that course. It drives enrollment for
// role-sync. A non-existent/empty collection returns an empty slice (no
// students enrolled), not an error.
func (r *MarkRepo) ListStudentIds(courseId string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cur, err := r.db.Collection(courseId).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var ids []string
	for cur.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ids = append(ids, doc.ID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
