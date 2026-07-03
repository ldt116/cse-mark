package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"thuanle/cse-mark/internal/configs"
)

// collectionKey is a stable logical name for a collection, independent of the
// env-configured collection name. It maps the declarative index specs below to
// the right physical collection at bootstrap time.
type collectionKey string

const (
	colStudents        collectionKey = "students"
	colBindings        collectionKey = "bindings"
	colVerifications   collectionKey = "verifications"
	colDiscordMappings collectionKey = "discord_mappings"
)

// indexSpec is the declarative form of an index. It is a value object that can
// be asserted on directly in tests, then translated to a mongo.IndexModel.
//
//   - TTLSeconds: nil = no TTL; a non-nil pointer (including 0) makes this a
//     TTL index with that expireAfterSeconds value.
type indexSpec struct {
	Name       string
	Keys       bson.D
	Unique     bool
	TTLSeconds *int32
}

// indexSpecs returns the v2 index specs keyed by logical collection.
// discord_mappings has no entry: it relies solely on the implicit _id index.
// v1 collections (courses, users, marks) are intentionally absent — this
// bootstrap must not alter v1 behavior.
func indexSpecs() map[collectionKey][]indexSpec {
	ttlZero := int32(0)
	return map[collectionKey][]indexSpec{
		colStudents: {
			{Name: "uniq_email", Keys: bson.D{{Key: "email", Value: 1}}, Unique: true},
		},
		colBindings: {
			{Name: "uniq_platform_user", Keys: bson.D{{Key: "platform", Value: 1}, {Key: "platform_user_id", Value: 1}}, Unique: true},
			{Name: "uniq_platform_mssv", Keys: bson.D{{Key: "platform", Value: 1}, {Key: "mssv", Value: 1}}, Unique: true},
			{Name: "idx_mssv", Keys: bson.D{{Key: "mssv", Value: 1}}},
		},
		colVerifications: {
			{Name: "ttl_expiry", Keys: bson.D{{Key: "expiry", Value: 1}}, TTLSeconds: &ttlZero},
		},
	}
}

func toIndexModel(s indexSpec) mongo.IndexModel {
	opts := options.Index().SetName(s.Name)
	if s.Unique {
		opts = opts.SetUnique(true)
	}
	if s.TTLSeconds != nil {
		opts = opts.SetExpireAfterSeconds(*s.TTLSeconds)
	}
	return mongo.IndexModel{
		Keys:    s.Keys,
		Options: opts,
	}
}

// EnsureIndexes creates the v2 indexes idempotently. Re-running is a no-op:
// re-Creating an identical, named index succeeds without error. Only the new
// v2 collections are touched; v1 collections are left untouched.
func EnsureIndexes(client *Client, config *configs.Config) error {
	collectionNames := map[collectionKey]string{
		colStudents:      config.DbSettingsStudents,
		colBindings:      config.DbSettingsBindings,
		colVerifications: config.DbSettingsVerifications,
	}

	db := client.mgClient.Database(config.DbSettings)

	for key, specs := range indexSpecs() {
		name, ok := collectionNames[key]
		if !ok {
			// No physical collection to provision (e.g. discord_mappings).
			continue
		}

		models := make([]mongo.IndexModel, 0, len(specs))
		for _, s := range specs {
			models = append(models, toIndexModel(s))
		}

		ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
		_, err := db.Collection(name).Indexes().CreateMany(ctx, models)
		cancel()
		if err != nil {
			return fmt.Errorf("ensure indexes for %s: %w", name, err)
		}
	}

	return nil
}
