package verification

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestModel_BsonRoundTrip(t *testing.T) {
	expiry := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	in := Model{PlatformUserID: "999", Email: "a@hcmut.edu.vn", OTP: "123456", Expiry: expiry, Attempts: 3}

	raw, err := bson.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal into M: %v", err)
	}

	// PlatformUserID is the _id key.
	if doc["_id"] != "999" {
		t.Errorf(`_id: want "999", got %v`, doc["_id"])
	}

	// Failed-attempt counter for brute-force protection.
	if doc["attempts"] != int32(3) {
		t.Errorf(`attempts: want 3, got %v`, doc["attempts"])
	}

	// CRITICAL for the TTL index: expiry must serialize as a BSON Date,
	// not an int64 timestamp. A non-Date type silently breaks TTL deletion.
	dt, ok := doc["expiry"].(primitive.DateTime)
	if !ok {
		t.Fatalf("expiry: want primitive.DateTime (BSON Date), got %T", doc["expiry"])
	}
	if !dt.Time().Equal(expiry) {
		t.Errorf("expiry value: want %v, got %v", expiry, dt.Time())
	}

	var out Model
	if err := bson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal into Model: %v", err)
	}
	if !out.Expiry.Equal(in.Expiry) || out.Email != in.Email || out.OTP != in.OTP || out.PlatformUserID != in.PlatformUserID {
		t.Errorf("round-trip mismatch: want %+v, got %+v", in, out)
	}
}
