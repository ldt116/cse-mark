package configs

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	MongoHost            string
	MongoPort            string
	DbTransactionTimeout time.Duration
	DbMark               string
	DbSettings           string
	DbSettingsUsers      string
	DbSettingsCourses    string

	CourseActiveAge time.Duration

	DownloaderTimeout time.Duration

	TeleToken        string
	TeleAdminChatIds []int64

	ApiToken string
	ApiPort  string
}

func LoadConfig() *Config {
	return &Config{
		MongoHost:            loadEnv("MONGO_HOST", "localhost"),
		MongoPort:            loadEnv("MONGO_PORT", "27017"),
		DbTransactionTimeout: 30 * time.Second,
		DbMark:               loadEnv("DB_MARK", "mark-cse"),
		DbSettings:           loadEnv("DB_SETTINGS", "mark-settings"),
		DbSettingsUsers:      loadEnv("DB_SETTINGS_USERS", "users"),
		DbSettingsCourses:    loadEnv("DB_SETTINGS_COURSES", "courses"),

		CourseActiveAge: 9 * 30 * 24 * time.Hour, // 9 months

		DownloaderTimeout: 30 * time.Second,

		TeleToken:        loadEnv("TOKEN", ""),
		TeleAdminChatIds: loadEnvJsonSlice("ADMINS", []int64{}), // Default admin chat ID

		ApiToken: loadEnv("API_TOKEN", ""),
		ApiPort:  loadEnv("API_PORT", "8080"),
	}
}

func loadEnvJsonSlice[T any](key string, defaultValue []T) []T {
	envValue := os.Getenv(key)
	var retValue []T
	err := json.Unmarshal([]byte(envValue), &retValue)
	if err == nil {
		return retValue
	}
	return defaultValue
}

func loadEnv(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	return val
}
