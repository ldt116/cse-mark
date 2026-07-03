package discordmapping

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestModel_BsonRoundTrip(t *testing.T) {
	in := Model{CourseId: "CO2003-L01", DiscordRoleId: "role-1", DiscordChannelId: "chan-1"}

	raw, err := bson.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal into M: %v", err)
	}

	// CourseId is the _id key (matches Class.CourseId).
	if doc["_id"] != in.CourseId {
		t.Errorf(`_id: want %q, got %v`, in.CourseId, doc["_id"])
	}
	if doc["discord_role_id"] != in.DiscordRoleId {
		t.Errorf(`discord_role_id: want %q, got %v`, in.DiscordRoleId, doc["discord_role_id"])
	}
	if doc["discord_channel_id"] != in.DiscordChannelId {
		t.Errorf(`discord_channel_id: want %q, got %v`, in.DiscordChannelId, doc["discord_channel_id"])
	}

	var out Model
	if err := bson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal into Model: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: want %+v, got %+v", in, out)
	}
}
