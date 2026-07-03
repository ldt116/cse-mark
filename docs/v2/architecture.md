# Kiến trúc v2

> Đặc tả kỹ thuật kiến trúc. Yêu cầu ở `SRS-v2.md`. Mô hình dữ liệu ở `data-model.md`, luồng ở `flows.md`.

## 1. Nguyên tắc

- Mở rộng backend hiện có theo **clean architecture** (xem `docs/v1/DEVELOPMENT.md`), không viết lại.
- Mỗi service là một binary độc lập (`cmd/*`), compose bằng **Google Wire**.
- Tích hợp nền tảng (Discord, SMTP) nằm sau **port** (interface) ở domain/use case; implementation ở `infra`.
- Pipeline mark hiện có (platform-agnostic) được tái dùng nguyên vẹn.

## 2. Services

| Service | Binary | Loại | Trách nhiệm |
|---|---|---|---|
| API | `cmd/api` | HTTP (Gin) | `GET /healthz`, `GET /mark` (giữ nguyên) |
| Fetcher | `cmd/fetcher` | Scheduler | mark sync 10p (hiện có) **+ roster sync** (mới) |
| Tele | `cmd/tele` | Bot (long-poll) | Telegram: tra cứu + bind (mở rộng tối thiểu) |
| Discord | `cmd/discord` | Bot + Scheduler | Discord bot + role-sync scheduler (mới) |

Mỗi service có `main.go` + `wire.go`/`wire_gen.go` riêng (pattern hiện tại).

## 3. Lớp hóa (layering)

```text
cmd/{api,fetcher,tele,discord}        ← entrypoints (Wire DI)
        │
        ▼
internal/delivery/{api,tele,discord}  ← giao tiếp nền tảng (HTTP/TG/Discord)
        │
        ▼
internal/usecases                     ← logic ứng dụng
        │
        ▼
internal/domain                       ← entity + interface (port)
        ▲
        │  (implements)
internal/infra/{mongo,http,discord,email}  ← framework & driver
```

### 3.1 Domain (entity + port)

| Gói | Trạng thái | Nội dung |
|---|---|---|
| `domain/course` | **mở rộng** | entity Class (thêm `DiscordRoleId`, `DiscordChannelId`), `Repository` (hỗ trợ lưu ID), `Rules` |
| `domain/user` | **mở rộng** | `Model` (thêm `Role`), `Repository` (hỗ trợ query theo `MSSV` / `_id` cũ) |
| `domain/mark` | hiện có | `Repository` (per-course) |
| `domain/downloader` | hiện có | `Repository.DownloadCSV` |
| `domain/teleuser` | hiện có | validation |
| `domain/student` | **mới** | `Model{MSSV,Name,Email}`, `Repository` |
| `domain/binding` | **mới** | `Model{Platform,PlatformUserID,MSSV,Verified,BoundAt}`, `Repository` (index unique `platform + mssv` và `platform + platform_user_id`) |
| `domain/verification` | **mới** | `Model{PlatformUserID,Email,OTP,Expiry time.Time}`, `Repository` (TTL qua kiểu Date) |
| `domain/discord` | **mới (port)** | interface `Bot` (xem §4) |
| `domain/email` | **mới (port)** | interface `Sender` (xem §4) |

### 3.2 Use cases

| Gói | Trạng thái | Vai trò |
|---|---|---|
| `usecases/iam` | **mở rộng** | `AuthzService`: `CanEditCourse`, `IsTeacher` (chuyển sang phân quyền theo `MSSV` sau khi map platformUserID qua `bindings`; hỗ trợ fallback Telegram username cũ) |
| `usecases/coursequery` | hiện có | `ActiveCourseService` |
| `usecases/markimport` | hiện có | download + parse + import marks |
| `usecases/marksync` | hiện có | scheduler mark sync 10p |
| `usecases/identity` | **mới** | `BindStart` (kiểm tra roster trước khi sinh OTP, gửi), `BindVerify` (lưu binding), `GetBinding` |
| `usecases/rostersync` | **mới** | download roster CSV → `student` repo |
| `usecases/classsync` | **mới** | enrollment → diff role Discord qua `discord.Bot` |

### 3.3 Delivery

| Gói | Trạng thái |
|---|---|
| `delivery/api` | hiện có (Gin) |
| `delivery/tele` | hiện có + handler `/bind` + sửa `/mark` |
| `delivery/discord` | **mới** (discordgo) — `/bind /profile /mark /create /sync /delete` + middleware auth theo binding |

### 3.4 Infra

| Gói | Trạng thái |
|---|---|
| `infra/mongo` | hiện có + repo mới: `student`, `binding`, `verification` (TTL Date) |
| `infra/http` | hiện có (`SimpleDownloader`) |
| `infra/discord` | **mới** — `discordgo` client implement `discord.Bot` (hỗ trợ rate-limit backoff) |
| `infra/email` | **mới** — SMTP implement `email.Sender` |

## 4. Port (interface)

### 4.1 `domain/discord.Bot`

```go
type Bot interface {
    // Provisioning (trả về ID để lưu DB)
    EnsureRole(ctx context.Context, name string) (roleID string, err error)
    EnsureChannel(ctx context.Context, name string, roleID string) (channelID string, err error)
    DeleteRole(ctx context.Context, roleID string) error
    DeleteChannel(ctx context.Context, channelID string) error

    // Role membership (sử dụng roleID đã lưu)
    AssignRole(ctx context.Context, userID string, roleID string) error
    RemoveRole(ctx context.Context, userID string, roleID string) error
    MembersWithRole(ctx context.Context, roleID string) ([]string, error)
}
```

- Các ID `roleID` và `channelID` được lưu trực tiếp vào collection `courses` trong database sau khi tạo thành công.
- Naming: role = `courseId`; channel = `lowercase(courseId)`.
- **Cơ chế xử lý Rate-Limit:** `infra/discord` bọc client `discordgo` với cơ chế hàng đợi lệnh (command queue) và tự động tạm dừng (sleep) theo header `Retry-After` khi gặp lỗi HTTP 429 từ Discord API, đảm bảo tiến trình scheduler đồng bộ không bị ngắt quãng đột ngột.

### 4.2 `domain/email.Sender`

```go
type Sender interface {
    SendOTP(ctx context.Context, to string, otp string) error
}
```

- `infra/email` implement bằng SMTP (config host/port/username/password/from).

## 5. Phân rã phụ thuộc (Wire)

Mỗi service compose đúng những thứ cần:

- **api** (giữ nguyên): config → mongo client → MarkRepo → handlers → ApiService.
- **fetcher** (mở rộng): + RosterRepo + StudentRepo + `rostersync.Service`. Scheduler chạy cả mark sync và roster sync.
- **tele** (mở rộng): + StudentRepo + BindingRepo + VerificationRepo + `identity.Service` + `email.Sender` (gửi OTP). Handler `/bind` + `/mark` dùng binding.
- **discord** (mới): giống tele + CourseRepo + `discord.Bot` + `classsync.Service` (role-sync scheduler).

> SMTP/Email và Discord là phụ thuộc delivery-side; chỉ `tele` và `discord` cần `email.Sender` và `discord.Bot`.

## 6. Đồ thị luồng dữ liệu (tóm tắt)

```text
Roster CSV ──fetcher──▶ student repo ──▶ identity.BindVerify (email→MSSV)
Class CSV  ──fetcher──▶ mark cache ──▶ enrollment ──▶ classsync ──▶ discord.Bot (role)
                                          │
/mark  ──tele/discord──▶ identity.GetBinding(MSSV) ──▶ mark repo ──▶ reply
```
