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
| `DB_SETTINGS_USERS` | `users` | collection legacy v1, không dùng cho auth v2 |
| `DB_SETTINGS_DISCORD_MAPPINGS` | `discord_mappings` | **mới** — collection DiscordMapping |
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
| `OTP_TTL` | `15m` | thời hạn OTP; **đồng thời là cửa sổ resend cooldown** (xem §4.1) |
| `OTP_MAX_ATTEMPTS` | `5` | số lần sai OTP tối đa trước khi OTP bị vô hiệu hoá (chống brute-force) |

## 4.1. Chính sách chống lạm dụng OTP

- **Resend cooldown = `OTP_TTL` (15m mặc định):** nếu đã có bản ghi `verification` chưa hết hạn cho `platformUserID` thì từ chối gửi lại. Vì bản ghi tồn tại đúng bằng `OTP_TTL`, "còn bản ghi" ≡ "đang trong cooldown" → không cần state riêng.
- **Cooldown theo `email` (chống Sybil):** tối đa 1 OTP/email/`OTP_TTL`, bất kể `platformUserID` (tra qua index `verifications.email`).
- **Giới hạn brute-force (`OTP_MAX_ATTEMPTS`):** mỗi lần sai, tăng bộ đếm `attempts` (atomic `$inc`); khi đạt ngưỡng, đánh dấu OTP vô hiệu **nhưng giữ bản ghi** để TTL vẫn chặn resend tới hết `OTP_TTL` (tránh lỗ hổng request lại ngay để nhận thêm lượt đoán). Bản ghi tự xoá theo TTL.
- Việc **enforce** các quy tắc trên thuộc use case `identity`; nền tảng (task #8) chỉ cung cấp config + schema (`verification.attempts`, index `email`) + primitive repo (`IncrementAttempts`, `FindByEmail`).

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
    DbSettingsDiscordMappings, DbSettingsStudents string
    DbSettingsBindings, DbSettingsVerifications   string
    // v2 — Discord
    DiscordToken, DiscordGuildId string; DiscordAdminIds []string
    // v2 — Email/OTP
    SmtpHost, SmtpUsername, SmtpPassword, SmtpFrom string; SmtpPort int
    OtpLen, OtpMaxAttempts int; OtpTtl time.Duration
    // v2 — Sync
    RosterCsvUrl string; RosterSyncInterval, RoleSyncInterval time.Duration
}
```

`DB_SETTINGS_USERS` được giữ để tương thích với dữ liệu v1, nhưng auth v2 không còn phụ thuộc vào collection này. Load helper dùng lại `loadEnv` / `loadEnvJsonSlice` hiện có.

## 8. Quản lý secret

Repo dùng **SOPS** (`.sops.yaml`, `enc.env`). Mọi secret (TOKEN, DISCORD_TOKEN, SMTP_*) giữ trong `enc.env` đã mã hoá; decrypt khi chạy. Không commit plaintext `.env`.

## 9. `.env` mẫu (không secret)

```ini
MONGO_HOST=localhost
MONGO_PORT=27017
DB_MARK=mark-cse
DB_SETTINGS=mark-settings
DB_SETTINGS_DISCORD_MAPPINGS=discord_mappings
API_PORT=8080
OTP_LEN=6
OTP_TTL=15m
OTP_MAX_ATTEMPTS=5
SMTP_PORT=587
ROSTER_SYNC_INTERVAL=24h
ROLE_SYNC_INTERVAL=30m
# secrets (qua enc.env / SOPS): TOKEN, ADMINS, DISCORD_TOKEN, DISCORD_GUILD_ID,
# DISCORD_ADMIN_IDS, SMTP_HOST, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM,
# API_TOKEN, ROSTER_CSV_URL
```
