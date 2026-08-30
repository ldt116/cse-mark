package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	mopts "go.mongodb.org/mongo-driver/v2/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/mark"
)

// TestMarkRepo_GetMark_InfraError locks the contract that an infrastructure
// failure (network down, timeout, ...) is returned as a non-nil error, never
// swallowed into a ("null", nil) success. It needs no live MongoDB: the client
// points at a closed local port, so FindOne fails during server selection with
// a network error distinct from mongo.ErrNoDocuments. The mapping
// ErrNoDocuments -> mark.ErrNotFound is exercised by the live integration
// tests in integration_test.go.
func TestMarkRepo_GetMark_InfraError(t *testing.T) {
	mc, err := mongo.Connect(mopts.Client().
		ApplyURI("mongodb://127.0.0.1:1").
		SetServerSelectionTimeout(500*time.Millisecond))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	t.Cleanup(func() { _ = mc.Disconnect(context.Background()) })

	client := &Client{
		mgClient: mc,
		Timeout:  2 * time.Second,
		ctx:      context.Background(),
	}
	repo := NewMarkRepo(client, &configs.Config{DbMark: "mark-itest"})

	got, err := repo.GetMark("CO9999", "2111111")
	if err == nil {
		t.Fatalf("GetMark on unreachable Mongo: error = nil, want non-nil (got result %q)", got)
	}
	if errors.Is(err, mark.ErrNotFound) {
		t.Fatalf("GetMark on unreachable Mongo: error = %v, want an infrastructure error, not mark.ErrNotFound", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("GetMark on unreachable Mongo: error = %v, want a network error, not ErrNoDocuments", err)
	}
	if got != "" {
		t.Errorf("GetMark on unreachable Mongo: result = %q, want empty string", got)
	}
}
