package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func findSpec(t *testing.T, specs []indexSpec, name string) indexSpec {
	t.Helper()
	for _, s := range specs {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("index spec %q not found", name)
	return indexSpec{}
}

func assertKeys(t *testing.T, keys bson.D, want ...string) {
	t.Helper()
	if len(keys) != len(want) {
		t.Fatalf("keys length: want %v (%v), got %d (%v)", len(want), want, len(keys), keys)
	}
	for i, w := range want {
		if keys[i].Key != w {
			t.Errorf("keys[%d]: want %q, got %q", i, w, keys[i].Key)
		}
	}
}

func TestIndexSpecs(t *testing.T) {
	specs := indexSpecs()

	// students: unique email (email -> MSSV lookup during bind).
	s := findSpec(t, specs[colStudents], "uniq_email")
	assertKeys(t, s.Keys, "email")
	if !s.Unique {
		t.Error("students.email must be unique")
	}
	if s.TTLSeconds != nil {
		t.Error("students.email must not be a TTL index")
	}

	// bindings: chat -> MSSV uniqueness.
	b1 := findSpec(t, specs[colBindings], "uniq_platform_user")
	assertKeys(t, b1.Keys, "platform", "platform_user_id")
	if !b1.Unique {
		t.Error("bindings(platform, platform_user_id) must be unique")
	}

	// bindings: 1:1:1 — one MSSV -> at most one chat id per platform.
	b2 := findSpec(t, specs[colBindings], "uniq_platform_mssv")
	assertKeys(t, b2.Keys, "platform", "mssv")
	if !b2.Unique {
		t.Error("bindings(platform, mssv) must be unique")
	}

	// bindings: list all bindings of an MSSV.
	b3 := findSpec(t, specs[colBindings], "idx_mssv")
	assertKeys(t, b3.Keys, "mssv")
	if b3.Unique {
		t.Error("bindings.mssv must NOT be unique")
	}

	// verifications: TTL on a BSON Date field.
	v := findSpec(t, specs[colVerifications], "ttl_expiry")
	assertKeys(t, v.Keys, "expiry")
	if v.Unique {
		t.Error("verifications.expiry must NOT be unique")
	}
	if v.TTLSeconds == nil {
		t.Fatal("verifications.expiry must be a TTL index (TTLSeconds set)")
	}
	if *v.TTLSeconds != 0 {
		t.Errorf("verifications.expiry expireAfterSeconds: want 0, got %d", *v.TTLSeconds)
	}

	// discord_mappings: only the implicit _id index — nothing to create.
	if _, ok := specs[colDiscordMappings]; ok {
		t.Error("discord_mappings must have no index specs (implicit _id only)")
	}
}

func TestIndexSpecs_NoDuplicates(t *testing.T) {
	specs := indexSpecs()
	for key, list := range specs {
		seen := map[string]bool{}
		for _, s := range list {
			if seen[s.Name] {
				t.Errorf("collection %v: duplicate index name %q", key, s.Name)
			}
			seen[s.Name] = true
		}
	}
}
