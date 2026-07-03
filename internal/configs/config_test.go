package configs

import (
	"testing"
	"time"
)

// v2EnvKeys are the environment variables added for the v2 foundation. Tests
// force them empty to assert safe defaults, regardless of the host environment.
var v2EnvKeys = []string{
	"DB_SETTINGS_STUDENTS", "DB_SETTINGS_BINDINGS", "DB_SETTINGS_VERIFICATIONS", "DB_SETTINGS_DISCORD_MAPPINGS",
	"DISCORD_TOKEN", "DISCORD_GUILD_ID", "DISCORD_ADMIN_IDS",
	"SMTP_HOST", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_PORT",
	"OTP_LEN", "OTP_TTL",
	"ROSTER_CSV_URL", "ROSTER_SYNC_INTERVAL", "ROLE_SYNC_INTERVAL",
}

func TestLoadConfig_V2Defaults(t *testing.T) {
	for _, k := range v2EnvKeys {
		t.Setenv(k, "")
	}

	cfg := LoadConfig()

	// New Mongo collections.
	assertEqual(t, "DbSettingsStudents", cfg.DbSettingsStudents, "students")
	assertEqual(t, "DbSettingsBindings", cfg.DbSettingsBindings, "bindings")
	assertEqual(t, "DbSettingsVerifications", cfg.DbSettingsVerifications, "verifications")
	assertEqual(t, "DbSettingsDiscordMappings", cfg.DbSettingsDiscordMappings, "discord_mappings")

	// Discord defaults (secrets empty).
	assertEqual(t, "DiscordToken", cfg.DiscordToken, "")
	assertEqual(t, "DiscordGuildId", cfg.DiscordGuildId, "")
	if cfg.DiscordAdminIds == nil {
		t.Errorf("DiscordAdminIds: want non-nil empty slice, got nil")
	}
	if len(cfg.DiscordAdminIds) != 0 {
		t.Errorf("DiscordAdminIds: want empty, got %v", cfg.DiscordAdminIds)
	}

	// SMTP defaults (secret fields empty, port default).
	assertEqual(t, "SmtpHost", cfg.SmtpHost, "")
	assertEqual(t, "SmtpUsername", cfg.SmtpUsername, "")
	assertEqual(t, "SmtpPassword", cfg.SmtpPassword, "")
	assertEqual(t, "SmtpFrom", cfg.SmtpFrom, "")
	if cfg.SmtpPort != 587 {
		t.Errorf("SmtpPort: want 587, got %d", cfg.SmtpPort)
	}

	// OTP defaults.
	if cfg.OtpLen != 6 {
		t.Errorf("OtpLen: want 6, got %d", cfg.OtpLen)
	}
	if cfg.OtpTtl != 5*time.Minute {
		t.Errorf("OtpTtl: want 5m, got %v", cfg.OtpTtl)
	}

	// Sync defaults.
	assertEqual(t, "RosterCsvUrl", cfg.RosterCsvUrl, "")
	if cfg.RosterSyncInterval != 24*time.Hour {
		t.Errorf("RosterSyncInterval: want 24h, got %v", cfg.RosterSyncInterval)
	}
	if cfg.RoleSyncInterval != 30*time.Minute {
		t.Errorf("RoleSyncInterval: want 30m, got %v", cfg.RoleSyncInterval)
	}
}

func TestLoadConfig_V2FromEnv(t *testing.T) {
	t.Setenv("DB_SETTINGS_STUDENTS", "roster")
	t.Setenv("DB_SETTINGS_BINDINGS", "links")
	t.Setenv("DB_SETTINGS_VERIFICATIONS", "otps")
	t.Setenv("DB_SETTINGS_DISCORD_MAPPINGS", "dmap")
	t.Setenv("DISCORD_TOKEN", "dtok")
	t.Setenv("DISCORD_GUILD_ID", "guild-1")
	t.Setenv("DISCORD_ADMIN_IDS", `["111","222"]`)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USERNAME", "u")
	t.Setenv("SMTP_PASSWORD", "p")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	t.Setenv("OTP_LEN", "8")
	t.Setenv("OTP_TTL", "10m")
	t.Setenv("ROSTER_CSV_URL", "https://example.com/r.csv")
	t.Setenv("ROSTER_SYNC_INTERVAL", "12h")
	t.Setenv("ROLE_SYNC_INTERVAL", "15m")

	cfg := LoadConfig()

	assertEqual(t, "DbSettingsStudents", cfg.DbSettingsStudents, "roster")
	assertEqual(t, "DbSettingsBindings", cfg.DbSettingsBindings, "links")
	assertEqual(t, "DbSettingsVerifications", cfg.DbSettingsVerifications, "otps")
	assertEqual(t, "DbSettingsDiscordMappings", cfg.DbSettingsDiscordMappings, "dmap")
	assertEqual(t, "DiscordToken", cfg.DiscordToken, "dtok")
	assertEqual(t, "DiscordGuildId", cfg.DiscordGuildId, "guild-1")
	if len(cfg.DiscordAdminIds) != 2 || cfg.DiscordAdminIds[0] != "111" || cfg.DiscordAdminIds[1] != "222" {
		t.Errorf("DiscordAdminIds: want [111 222], got %v", cfg.DiscordAdminIds)
	}
	assertEqual(t, "SmtpHost", cfg.SmtpHost, "smtp.example.com")
	if cfg.SmtpPort != 2525 {
		t.Errorf("SmtpPort: want 2525, got %d", cfg.SmtpPort)
	}
	assertEqual(t, "SmtpUsername", cfg.SmtpUsername, "u")
	assertEqual(t, "SmtpPassword", cfg.SmtpPassword, "p")
	assertEqual(t, "SmtpFrom", cfg.SmtpFrom, "no-reply@example.com")
	if cfg.OtpLen != 8 {
		t.Errorf("OtpLen: want 8, got %d", cfg.OtpLen)
	}
	if cfg.OtpTtl != 10*time.Minute {
		t.Errorf("OtpTtl: want 10m, got %v", cfg.OtpTtl)
	}
	assertEqual(t, "RosterCsvUrl", cfg.RosterCsvUrl, "https://example.com/r.csv")
	if cfg.RosterSyncInterval != 12*time.Hour {
		t.Errorf("RosterSyncInterval: want 12h, got %v", cfg.RosterSyncInterval)
	}
	if cfg.RoleSyncInterval != 15*time.Minute {
		t.Errorf("RoleSyncInterval: want 15m, got %v", cfg.RoleSyncInterval)
	}
}

func TestLoadConfig_V1FieldsUnchanged(t *testing.T) {
	// Guard: v1 fields keep working and are unaffected by the v2 additions.
	t.Setenv("DB_MARK", "mark-cse")
	t.Setenv("DB_SETTINGS", "mark-settings")
	t.Setenv("DB_SETTINGS_USERS", "users")
	t.Setenv("DB_SETTINGS_COURSES", "courses")
	t.Setenv("API_PORT", "8080")
	for _, k := range v2EnvKeys {
		t.Setenv(k, "")
	}

	cfg := LoadConfig()

	assertEqual(t, "DbMark", cfg.DbMark, "mark-cse")
	assertEqual(t, "DbSettings", cfg.DbSettings, "mark-settings")
	assertEqual(t, "DbSettingsUsers", cfg.DbSettingsUsers, "users")
	assertEqual(t, "DbSettingsCourses", cfg.DbSettingsCourses, "courses")
	assertEqual(t, "ApiPort", cfg.ApiPort, "8080")
}

func assertEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %v, got %v", name, want, got)
	}
}
