package mongo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/discordmapping"
	"thuanle/cse-mark/internal/domain/student"
	"thuanle/cse-mark/internal/domain/verification"
)

// These tests require a live MongoDB. They are skipped unless MONGO_TEST_HOST
// is set. Run e.g.:
//
//	MONGO_TEST_HOST=127.0.0.1 MONGO_TEST_PORT=27018 go test ./internal/infra/mongo/
func testConfig(t *testing.T) *configs.Config {
	t.Helper()
	host := os.Getenv("MONGO_TEST_HOST")
	if host == "" {
		t.Skip("MONGO_TEST_HOST not set; skipping Mongo integration test")
	}
	port := os.Getenv("MONGO_TEST_PORT")
	if port == "" {
		port = "27017"
	}
	return &configs.Config{
		MongoHost:                 host,
		MongoPort:                 port,
		DbTransactionTimeout:      10 * time.Second,
		DbSettings:                "mark-settings-itest",
		DbSettingsStudents:        "students",
		DbSettingsBindings:        "bindings",
		DbSettingsVerifications:   "verifications",
		DbSettingsDiscordMappings: "discord_mappings",
	}
}

func setupMongo(t *testing.T) (*Client, *configs.Config) {
	t.Helper()
	cfg := testConfig(t)

	uri := "mongodb://" + cfg.MongoHost + ":" + cfg.MongoPort
	mc, err := mongo.Connect(context.Background(), mopts.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	if err := mc.Ping(context.Background(), nil); err != nil {
		t.Fatalf("mongo ping: %v", err)
	}
	t.Cleanup(func() { _ = mc.Disconnect(context.Background()) })

	// Start each test from a clean test database.
	if err := mc.Database(cfg.DbSettings).Drop(context.Background()); err != nil {
		t.Fatalf("drop test db: %v", err)
	}

	client := &Client{
		mgClient: mc,
		Timeout:  cfg.DbTransactionTimeout,
		ctx:      context.Background(),
	}
	return client, cfg
}

func listIndexMap(t *testing.T, coll *mongo.Collection) map[string]bson.M {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		t.Fatalf("read indexes: %v", err)
	}
	m := make(map[string]bson.M, len(docs))
	for _, d := range docs {
		if name, ok := d["name"].(string); ok {
			m[name] = d
		}
	}
	return m
}

func asInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}

func TestEnsureIndexes_Idempotent(t *testing.T) {
	client, cfg := setupMongo(t)

	if err := EnsureIndexes(client, cfg); err != nil {
		t.Fatalf("first EnsureIndexes: %v", err)
	}
	if err := EnsureIndexes(client, cfg); err != nil {
		t.Fatalf("second EnsureIndexes (must be a no-op): %v", err)
	}
}

func TestEnsureIndexes_CreatesExpectedIndexes(t *testing.T) {
	client, cfg := setupMongo(t)
	if err := EnsureIndexes(client, cfg); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	db := client.mgClient.Database(cfg.DbSettings)

	students := listIndexMap(t, db.Collection(cfg.DbSettingsStudents))
	if s := students["uniq_email"]; s == nil {
		t.Fatal("students: uniq_email index missing")
	} else if s["unique"] != true {
		t.Errorf("students.uniq_email: want unique=true, got %v", s["unique"])
	}

	bindings := listIndexMap(t, db.Collection(cfg.DbSettingsBindings))
	for _, name := range []string{"uniq_platform_user", "uniq_platform_mssv"} {
		if b := bindings[name]; b == nil {
			t.Fatalf("bindings: %s index missing", name)
		} else if b["unique"] != true {
			t.Errorf("bindings.%s: want unique=true, got %v", name, b["unique"])
		}
	}
	if b := bindings["idx_mssv"]; b == nil {
		t.Fatal("bindings: idx_mssv index missing")
	} else if b["unique"] == true {
		t.Error("bindings.idx_mssv: must NOT be unique")
	}

	verifs := listIndexMap(t, db.Collection(cfg.DbSettingsVerifications))
	v := verifs["ttl_expiry"]
	if v == nil {
		t.Fatal("verifications: ttl_expiry index missing")
	}
	if got, ok := asInt64(v["expireAfterSeconds"]); !ok || got != 0 {
		t.Errorf("verifications.ttl_expiry expireAfterSeconds: want 0, got %v", v["expireAfterSeconds"])
	}
	if v["unique"] == true {
		t.Error("verifications.ttl_expiry: must NOT be unique")
	}
	if ve := verifs["idx_email"]; ve == nil {
		t.Fatal("verifications: idx_email index missing")
	} else if ve["unique"] == true {
		t.Error("verifications.idx_email: must NOT be unique")
	}
}

func TestStudentRepo_RoundTripAndUniqueEmail(t *testing.T) {
	client, cfg := setupMongo(t)
	_ = EnsureIndexes(client, cfg)
	repo := NewStudentRepo(client, cfg)

	s := student.Model{MSSV: "123", Name: "A", Email: "a@hcmut.edu.vn"}
	if err := repo.Upsert(s); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.FindByEmail("a@hcmut.edu.vn")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if got.MSSV != "123" {
		t.Errorf("FindByEmail MSSV: want 123, got %q", got.MSSV)
	}
	if _, err := repo.FindByMSSV("123"); err != nil {
		t.Errorf("FindByMSSV: %v", err)
	}
	if _, err := repo.FindByEmail("missing@hcmut.edu.vn"); !errors.Is(err, student.ErrNotFound) {
		t.Errorf("missing email: want ErrNotFound, got %v", err)
	}

	// Update existing (same MSSV) must not error on _id.
	if err := repo.Upsert(student.Model{MSSV: "123", Name: "A2", Email: "a2@hcmut.edu.vn"}); err != nil {
		t.Errorf("upsert update: %v", err)
	}

	// Unique email: a different MSSV with the same email must be rejected.
	err = repo.Upsert(student.Model{MSSV: "999", Name: "X", Email: "a2@hcmut.edu.vn"})
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("duplicate email: want duplicate-key error, got %v", err)
	}
}

func TestBindingRepo_RoundTripAnd1To1Constraint(t *testing.T) {
	client, cfg := setupMongo(t)
	_ = EnsureIndexes(client, cfg)
	repo := NewBindingRepo(client, cfg)

	a := binding.Model{Platform: "telegram", PlatformUserID: "u1", MSSV: "m1", Verified: true, BoundAt: 1}
	if err := repo.Upsert(a); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	got, err := repo.FindByPlatformUser("telegram", "u1")
	if err != nil {
		t.Fatalf("FindByPlatformUser: %v", err)
	}
	if got.MSSV != "m1" {
		t.Errorf("MSSV: want m1, got %q", got.MSSV)
	}

	// Same MSSV, different platform -> allowed (1:1:1 means one per platform).
	if err := repo.Upsert(binding.Model{Platform: "discord", PlatformUserID: "u2", MSSV: "m1", BoundAt: 2}); err != nil {
		t.Fatalf("upsert discord: %v", err)
	}
	all, err := repo.FindByMSSV("m1")
	if err != nil {
		t.Fatalf("FindByMSSV: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("FindByMSSV(m1): want 2 bindings, got %d", len(all))
	}

	// Same platform + MSSV with a new user id violates uniq(platform, mssv).
	err = repo.Upsert(binding.Model{Platform: "telegram", PlatformUserID: "u3", MSSV: "m1", BoundAt: 3})
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("duplicate platform+mssv: want duplicate-key error, got %v", err)
	}
}

func TestVerificationRepo_RoundTripAndExpiryDate(t *testing.T) {
	client, cfg := setupMongo(t)
	_ = EnsureIndexes(client, cfg)
	repo := NewVerificationRepo(client, cfg)

	expiry := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Millisecond)
	in := verification.Model{PlatformUserID: "u1", Email: "a@hcmut.edu.vn", OTP: "123456", Expiry: expiry}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.Find("u1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.OTP != "123456" || !got.Expiry.Equal(expiry) {
		t.Errorf("round-trip mismatch: got %+v, want expiry %v", got, expiry)
	}

	// The stored expiry must be a BSON Date so the TTL index works.
	db := client.mgClient.Database(cfg.DbSettings)
	var doc bson.M
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.Collection(cfg.DbSettingsVerifications).FindOne(ctx, bson.M{"_id": "u1"}).Decode(&doc); err != nil {
		t.Fatalf("raw find: %v", err)
	}
	dt, ok := doc["expiry"].(primitive.DateTime)
	if !ok {
		t.Fatalf("stored expiry: want primitive.DateTime, got %T", doc["expiry"])
	}
	if !dt.Time().Equal(expiry) {
		t.Errorf("stored expiry value: want %v, got %v", expiry, dt.Time())
	}

	// Failed-attempt counter: atomic increment, and Upsert resets it.
	n, err := repo.IncrementAttempts("u1")
	if err != nil || n != 1 {
		t.Errorf("IncrementAttempts #1: want 1, got %d (%v)", n, err)
	}
	if n, err := repo.IncrementAttempts("u1"); err != nil || n != 2 {
		t.Errorf("IncrementAttempts #2: want 2, got %d (%v)", n, err)
	}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("re-upsert (reset): %v", err)
	}
	if n, err := repo.IncrementAttempts("u1"); err != nil || n != 1 {
		t.Errorf("after Upsert reset, IncrementAttempts: want 1, got %d (%v)", n, err)
	}

	// Per-email cooldown lookup.
	if list, err := repo.FindByEmail("a@hcmut.edu.vn"); err != nil || len(list) != 1 {
		t.Errorf("FindByEmail: want 1 record, got %d (%v)", len(list), err)
	}

	// IncrementAttempts on a missing record reports ErrNotFound.
	if _, err := repo.IncrementAttempts("does-not-exist"); !errors.Is(err, verification.ErrNotFound) {
		t.Errorf("IncrementAttempts missing: want ErrNotFound, got %v", err)
	}
}

func TestDiscordMappingRepo_RoundTrip(t *testing.T) {
	client, cfg := setupMongo(t)
	_ = EnsureIndexes(client, cfg)
	repo := NewDiscordMappingRepo(client, cfg)

	in := discordmapping.Model{CourseId: "CO2003-L01", DiscordRoleId: "r1", DiscordChannelId: "c1"}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.Find("CO2003-L01")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.DiscordRoleId != "r1" || got.DiscordChannelId != "c1" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if err := repo.Remove("CO2003-L01"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := repo.Find("CO2003-L01"); !errors.Is(err, discordmapping.ErrNotFound) {
		t.Errorf("Find after Remove: want ErrNotFound, got %v", err)
	}
}
