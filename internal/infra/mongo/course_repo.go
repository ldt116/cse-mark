package mongo

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/course"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type CourseRepo struct {
	db         *mongo.Database
	collection *mongo.Collection
	timeout    time.Duration
}

func NewCourseRepo(client *Client, config *configs.Config) course.Repository {
	db := client.mgClient.Database(config.DbSettings)
	return &CourseRepo{
		db:         db,
		collection: db.Collection(config.DbSettingsCourses),
		timeout:    client.Timeout,
	}
}

func (r *CourseRepo) FindCoursesUpdatedAfter(sinceTime time.Time) ([]course.Model, error) {
	updatedAfter := sinceTime.Unix()
	filter := bson.M{
		"$and": []bson.M{
			{"updated_at": bson.M{"$gt": updatedAfter}},
			{"link": bson.M{"$ne": ""}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cur, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var courses []course.Model

	ctx2, cancel2 := context.WithTimeout(context.Background(), r.timeout)
	defer cancel2()
	err = cur.All(ctx2, &courses)
	if err != nil {
		return nil, err
	}

	return courses, err
}

func (r *CourseRepo) UpdateCourseRecordCount(courseId string, cnt int) error {
	update := bson.M{
		"$set": bson.M{
			"record_cnt": cnt,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, courseId, update)
	if err != nil {
		return err
	}
	return nil
}

func (r *CourseRepo) FindCoursesManagedByUser(username string) ([]course.Model, error) {
	filter := bson.M{"by_user": username}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cur, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var courses []course.Model
	ctx2, cancel2 := context.WithTimeout(context.Background(), r.timeout)
	defer cancel2()
	err = cur.All(ctx2, &courses)
	if err != nil {
		return nil, err
	}

	return courses, err
}

func (r *CourseRepo) FindCourseById(courseId string) (course.Model, error) {
	var res course.Model
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	err := r.collection.FindOne(ctx, bson.M{"_id": courseId}).Decode(&res)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		err = course.ErrNotFound
	}
	return res, err
}

func (r *CourseRepo) UpdateCourseLink(courseId string, link string, userId int64, username string) error {
	update := bson.M{
		"$set": bson.M{
			"link":       link,
			"updated_at": time.Now().Unix(),
		},
		"$setOnInsert": bson.M{
			"_id":     courseId,
			"course":  courseId,
			"by_id":   userId,
			"by_user": username,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, courseId, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	return nil
}

func (r *CourseRepo) RemoveCourse(courseId string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": courseId})
	return err
}

func (r *CourseRepo) SetCourseStatus(courseId string, status course.Status) error {
	update := bson.M{"$set": bson.M{"status": status}}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.UpdateByID(ctx, courseId, update)
	return err
}

func (r *CourseRepo) FindSyncableCourses(sinceTime time.Time) ([]course.Model, error) {
	filter := bson.M{
		"$and": []bson.M{
			{"updated_at": bson.M{"$gt": sinceTime.Unix()}},
			{"link": bson.M{"$ne": ""}},
			// $ne matches docs where status is missing — legacy courses stay active.
			{"status": bson.M{"$ne": string(course.StatusInactive)}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cur, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var courses []course.Model

	ctx2, cancel2 := context.WithTimeout(context.Background(), r.timeout)
	defer cancel2()
	err = cur.All(ctx2, &courses)
	if err != nil {
		return nil, err
	}

	return courses, err
}
