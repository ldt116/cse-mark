package student

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestModel_BsonRoundTrip(t *testing.T) {
	in := Model{MSSV: "123456", Name: "Nguyen Van A", Email: "a@hcmut.edu.vn"}

	raw, err := bson.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal into M: %v", err)
	}

	// MSSV is the _id key (roster lookup + uniqueness hinge on this).
	if doc["_id"] != in.MSSV {
		t.Errorf(`_id: want %q, got %v`, in.MSSV, doc["_id"])
	}
	if doc["email"] != in.Email {
		t.Errorf(`email: want %q, got %v`, in.Email, doc["email"])
	}

	var out Model
	if err := bson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal into Model: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: want %+v, got %+v", in, out)
	}
}
