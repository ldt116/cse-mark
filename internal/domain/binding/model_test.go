package binding

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestModel_BsonRoundTrip(t *testing.T) {
	in := Model{
		Platform:       "discord",
		PlatformUserID: "999",
		MSSV:           "123456",
		Verified:       true,
		BoundAt:        1750000000,
	}

	raw, err := bson.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal into M: %v", err)
	}

	// The compound-index fields must land under these exact bson names.
	if doc["platform"] != "discord" {
		t.Errorf(`platform: want "discord", got %v`, doc["platform"])
	}
	if doc["platform_user_id"] != "999" {
		t.Errorf(`platform_user_id: want "999", got %v`, doc["platform_user_id"])
	}
	if doc["mssv"] != "123456" {
		t.Errorf(`mssv: want "123456", got %v`, doc["mssv"])
	}
	if doc["verified"] != true {
		t.Errorf(`verified: want true, got %v`, doc["verified"])
	}

	var out Model
	if err := bson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal into Model: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: want %+v, got %+v", in, out)
	}
}
