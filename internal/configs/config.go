package configs

import (
	"encoding/json"
	"os"
	"strconv"
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

	// v2 — Mongo collections (mark-settings DB)
	DbSettingsStudents        string
	DbSettingsBindings        string
	DbSettingsVerifications   string
	DbSettingsDiscordMappings string

	// v2 — Discord
	DiscordToken    string
	DiscordGuildId  string
	DiscordAdminIds []string

	// v2 — Email / OTP
	SmtpHost     string
	SmtpPort     int
	SmtpUsername string
	SmtpPassword string
	SmtpFrom     string
	OtpLen       int
	OtpTtl       time.Duration

	// v2 — Sync
	RosterCsvUrl       string
	RosterSyncInterval time.Duration
	RoleSyncInterval   time.Duration
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

		// v2 — Mongo collections
		DbSettingsStudents:        loadEnv("DB_SETTINGS_STUDENTS", "students"),
		DbSettingsBindings:        loadEnv("DB_SETTINGS_BINDINGS", "bindings"),
		DbSettingsVerifications:   loadEnv("DB_SETTINGS_VERIFICATIONS", "verifications"),
		DbSettingsDiscordMappings: loadEnv("DB_SETTINGS_DISCORD_MAPPINGS", "discord_mappings"),

		// v2 — Discord
		DiscordToken:    loadEnv("DISCORD_TOKEN", ""),
		DiscordGuildId:  loadEnv("DISCORD_GUILD_ID", ""),
		DiscordAdminIds: loadEnvJsonSlice("DISCORD_ADMIN_IDS", []string{}),

		// v2 — Email / OTP
		SmtpHost:     loadEnv("SMTP_HOST", ""),
		SmtpPort:     loadEnvInt("SMTP_PORT", 587),
		SmtpUsername: loadEnv("SMTP_USERNAME", ""),
		SmtpPassword: loadEnv("SMTP_PASSWORD", ""),
		SmtpFrom:     loadEnv("SMTP_FROM", ""),
		OtpLen:       loadEnvInt("OTP_LEN", 6),
		OtpTtl:       loadEnvDuration("OTP_TTL", 5*time.Minute),

		// v2 — Sync
		RosterCsvUrl:       loadEnv("ROSTER_CSV_URL", ""),
		RosterSyncInterval: loadEnvDuration("ROSTER_SYNC_INTERVAL", 24*time.Hour),
		RoleSyncInterval:   loadEnvDuration("ROLE_SYNC_INTERVAL", 30*time.Minute),
	}
}

func loadEnvInt(key string, defaultValue int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func loadEnvDuration(key string, defaultValue time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		return defaultValue
	}
	return parsed
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
