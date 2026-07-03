# Cấu hình & biến môi trường v2

> Mở rộng config v1 (`internal/configs/config.go`, env-driven qua `godotenv`). Mọi giá trị có default an toàn trừ secret.

## 1. Nhóm MongoDB

| Env | Default | Mô tả |
|---|---|---|
| `MONGO_HOST` | `localhost` | host |
| `MONGO_PORT` | `27017` | port |
| `DB_MARK` | `mark-cse` | DB chứa mark cache (per-course collection) |
| `DB_SETTINGS` | `mark-settings` | DB chứa cấu hình/identity |
| `DB_SETTINGS_COURSES` | `courses` | collection Class |
| `DB_SETTINGS_USERS` | `users` | collection User/Account |
| `DB_SETTINGS_STUDENTS` | `students` | **mới** — collection Student (roster) |
| `DB_SETTINGS_BINDINGS` | `bindings` | **mới** — collection Binding |
| `DB_SETTINGS_VERIFICATIONS` | `verifications` | **mới** — collection Verification (TTL) |

## 2. Nhóm Telegram

| Env | Default | Mô tả |
|---|---|---|
| `TOKEN` | — | bot token (secret) |
| `ADMINS` | `[]` | JSON array chat ID của Admin TG |

## 3. Nhóm Discord (mới)

| Env | Default | Mô tả |
|---|---|---|
| `DISCORD_TOKEN` | — | bot token (secret) |
| `DISCORD_GUILD_ID` | — | ID server |
| `DISCORD_ADMIN_IDS` | `[]` | JSON array user ID Admin Discord |

## 4. Nhóm Email / OTP (mới)

| Env | Default | Mô tả |
|---|---|---|
| `SMTP_HOST` | — | SMTP server (secret kèm cred) |
| `SMTP_PORT` | `587` | port |
| `SMTP_USERNAME` | — | user (secret) |
| `SMTP_PASSWORD` | — | password (secret) |
| `SMTP_FROM` | — | email gửi OTP (vd `no-reply@...`) |
| `OTP_LEN` | `6` | số chữ số OTP |
| `OTP_TTL` | `5m` | thời hạn OTP |

## 5. Nhóm Sync

| Env | Default | Mô tả |
|---|---|---|
| `ROSTER_CSV_URL` | — | URL roster CSV (mssv,name,email) |
| `ROSTER_SYNC_INTERVAL` | `24h` | nhịp roster sync |
| `ROLE_SYNC_INTERVAL` | `30m` | nhịp role-sync (Discord) |

(Course active age `9 tháng`, mark sync `10 phút`, downloader timeout `30s` — giữ nguyên v1, hardcoded.)

## 6. Nhóm API

| Env | Default | Mô tả |
|---|---|---|
| `API_TOKEN` | — | token auth cho `GET /mark` |
| `API_PORT` | `8080` | port HTTP |

## 7. Cấu trúc Config (Go)

Mở rộng `configs.Config`:

```go
type Config struct {
    // v1
    MongoHost, MongoPort, DbMark, DbSettings string
    DbSettingsUsers, DbSettingsCourses       string
    TeleToken string; TeleAdminChatIds []int64
    ApiToken, ApiPort string
    CourseActiveAge, DownloaderTimeout, DbTransactionTimeout time.Duration

    // v2 — Mongo collections
    DbSettingsStudents, DbSettingsBindings, DbSettingsVerifications string
    // v2 — Discord
    DiscordToken, DiscordGuildId string; DiscordAdminIds []string
    // v2 — Email/OTP
    SmtpHost, SmtpUsername, SmtpPassword, SmtpFrom string; SmtpPort int
    OtpLen int; OtpTtl time.Duration
    // v2 — Sync
    RosterCsvUrl string; RosterSyncInterval, RoleSyncInterval time.Duration
}
```

Load helper dùng lại `loadEnv` / `loadEnvJsonSlice` hiện có.

## 8. Quản lý secret

Repo dùng **SOPS** (`.sops.yaml`, `enc.env`). Mọi secret (TOKEN, DISCORD_TOKEN, SMTP_*) giữ trong `enc.env` đã mã hoá; decrypt khi chạy. Không commit plaintext `.env`.

## 9. `.env` mẫu (không secret)

```ini
MONGO_HOST=localhost
MONGO_PORT=27017
DB_MARK=mark-cse
DB_SETTINGS=mark-settings
API_PORT=8080
OTP_LEN=6
OTP_TTL=5m
SMTP_PORT=587
ROSTER_SYNC_INTERVAL=24h
ROLE_SYNC_INTERVAL=30m
# secrets (qua enc.env / SOPS): TOKEN, ADMINS, DISCORD_TOKEN, DISCORD_GUILD_ID,
# DISCORD_ADMIN_IDS, SMTP_HOST, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM,
# API_TOKEN, ROSTER_CSV_URL
```
