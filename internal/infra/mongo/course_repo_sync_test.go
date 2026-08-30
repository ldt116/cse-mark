package mongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"thuanle/cse-mark/internal/domain/course"
)

func TestCourseRepo_FindSyncableCoursesExcludesInactive(t *testing.T) {
	client, cfg := setupMongo(t)
	cfg.DbSettingsCourses = "courses"
	repo := NewCourseRepo(client, cfg)

	now := time.Now().Unix()
	coll := client.mgClient.Database(cfg.DbSettings).Collection(cfg.DbSettingsCourses)
	docs := []interface{}{
		bson.M{"_id": "c1", "course": "c1", "link": "l1", "updated_at": now}, // legacy: no status field
		bson.M{"_id": "c2", "course": "c2", "link": "l2", "updated_at": now, "status": "stale"},
		bson.M{"_id": "c3", "course": "c3", "link": "l3", "updated_at": now, "status": "inactive"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := repo.FindSyncableCourses(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("FindSyncableCourses: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FindSyncableCourses: want 2 (legacy + stale), got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.Id == "c3" {
			t.Fatal("FindSyncableCourses returned inactive course c3")
		}
	}
}

func TestCourseRepo_SetCourseStatus(t *testing.T) {
	client, cfg := setupMongo(t)
	cfg.DbSettingsCourses = "courses"
	repo := NewCourseRepo(client, cfg)

	// Insert with an OLD updated_at so the "unchanged" assertion has signal:
	// if SetCourseStatus accidentally rewrote updated_at, the values differ.
	oldUpdated := time.Now().Unix() - 86400
	coll := client.mgClient.Database(cfg.DbSettings).Collection(cfg.DbSettingsCourses)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := coll.InsertOne(ctx, bson.M{"_id": "c1", "course": "c1", "link": "l1", "updated_at": oldUpdated}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.SetCourseStatus("c1", course.StatusInactive); err != nil {
		t.Fatalf("SetCourseStatus: %v", err)
	}

	got, err := repo.FindCourseById("c1")
	if err != nil {
		t.Fatalf("FindCourseById: %v", err)
	}
	if got.Status != course.StatusInactive {
		t.Errorf("Status: want %q, got %q", course.StatusInactive, got.Status)
	}
	if got.EffectiveStatus() != course.StatusInactive {
		t.Errorf("EffectiveStatus: want %q, got %q", course.StatusInactive, got.EffectiveStatus())
	}
	if got.UpdatedAt != oldUpdated {
		t.Errorf("updated_at must not change on SetCourseStatus: want %d (old value), got %d", oldUpdated, got.UpdatedAt)
	}
}
