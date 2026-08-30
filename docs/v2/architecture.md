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
| API | `cmd/api` | HTTP (Gin) | `GET /healthz`, `GET /mark` (giữ nguyên) + `GET /marks` (mới #44 — JWT student app, xem §7) |
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
| `domain/course` | hiện có | entity Class, `Repository`, `Rules` |
| `domain/discordmapping` | **mới** | entity DiscordMapping `Model{CourseId, DiscordRoleId, DiscordChannelId}`, `Repository` |
| `domain/user` | hiện có (legacy v1) | không tham gia phân quyền v2 |
| `domain/mark` | hiện có | `Repository` (per-course) |
| `domain/downloader` | hiện có | `Repository.DownloadCSV` |
| `domain/teleuser` | hiện có | validation |
| `domain/student` | **mới** | `Model{MSSV,Name,Email}`, `Repository` |
| `domain/binding` | **mới** | `Model{Platform,PlatformUserID,MSSV,Verified,BoundAt}`, `Repository` (index unique `platform + mssv` và `platform + platform_user_id`) |
| `domain/verification` | **mới** | `Model{PlatformUserID,Email,OTP,Expiry time.Time}`, `Repository` (TTL qua kiểu Date) |
| `domain/discord` | **mới (port)** | interface `Bot` (xem §4) |
| `domain/email` | **mới (port)** | interface `Sender` (xem §4) |
| `domain/jwks` | **mới (#44, port)** | `Repository.SigningKey(kid)` — resolve Ed25519 public key từ JWKS student app (`ErrUnavailable`, `ErrUnknownKid`) |

### 3.2 Use cases

| Gói | Trạng thái | Vai trò |
|---|---|---|
| `usecases/iam` | **mở rộng** | `AuthzService`: `IsAdmin` (kiểm tra whitelist config theo platform UserID); các thao tác quản trị dùng chung check này |
| `usecases/coursequery` | hiện có | `ActiveCourseService` |
| `usecases/markimport` | hiện có | download + parse + import marks |
| `usecases/marksync` | hiện có | scheduler mark sync 10p |
| `usecases/identity` | **mới** | `BindStart` (kiểm tra roster trước khi sinh OTP, gửi), `BindVerify` (lưu binding), `GetBinding` |
| `usecases/rostersync` | **mới** | download roster CSV → `student` repo |
| `usecases/assertion` | **mới (#44)** | verify JWT EdDSA (iss/aud/exp bắt buộc) qua JWKS, trả `sub` = MSSV |
| `usecases/marksquery` | **mới (#44)** | tra điểm 1 SV theo MSSV across courses từ mark cache |
| `usecases/classsync` | **mới** | enrollment → diff role Discord qua `discord.Bot` |

### 3.3 Delivery

| Gói | Trạng thái |
|---|---|
| `delivery/api` | hiện có (Gin) + `GET /marks` với middleware `Jwt` riêng (mới #44) |
| `delivery/tele` | hiện có + handler `/bind` + sửa `/mark` |
| `delivery/discord` | **mới** (discordgo) — `/bind /profile /mark /create /sync` + middleware auth theo binding và admin whitelist |

### 3.4 Infra

| Gói | Trạng thái |
|---|---|
| `infra/mongo` | hiện có + repo mới: `student`, `binding`, `verification` (TTL Date), `discord_mapping` |
| `infra/http` | hiện có (`SimpleDownloader`) + `JwksClient` (mới #44 — cache TTL 5m, kid lạ refresh 1 lần) |
| `infra/discord` | **mới** — `discordgo` client implement `discord.Bot` (hỗ trợ rate-limit backoff) |
| `infra/email` | **mới** — SMTP implement `email.Sender` |

## 4. Port (interface)

### 4.1 `domain/discord.Bot`

```go
type Bot interface {
    // Provisioning (trả về ID để lưu DB)
    EnsureRole(ctx context.Context, name string) (roleID string, err error)
    EnsureChannel(ctx context.Context, name string, roleID string) (channelID string, err error)

    // Role membership (sử dụng roleID đã lưu)
    AssignRole(ctx context.Context, userID string, roleID string) error
    RemoveRole(ctx context.Context, userID string, roleID string) error
    MembersWithRole(ctx context.Context, roleID string) ([]string, error)
}
```

- Các ID `roleID` và `channelID` được lưu trực tiếp vào collection `discord_mappings` trong database sau khi tạo thành công.
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

- **api** (mở rộng #44): config → mongo client → CourseRepo + MarkRepo → handlers → ApiService; thêm `http.JwksClient` (`AUTH_JWKS_URL`, TTL/timeout const) → `assertion.Service` (iss/aud từ config) → middleware `Jwt` + `marksquery.Service` cho `GET /marks`.
- **fetcher** (mở rộng): + RosterRepo + StudentRepo + `rostersync.Service`. Scheduler chạy cả mark sync và roster sync.
- **tele** (mở rộng): + StudentRepo + BindingRepo + VerificationRepo + `identity.Service` + `email.Sender` (gửi OTP). Handler `/bind` + `/mark` dùng binding.
- **discord** (mới): giống tele + CourseRepo + DiscordMappingRepo + `discord.Bot` + `classsync.Service` (role-sync scheduler).

> SMTP/Email và Discord là phụ thuộc delivery-side; chỉ `tele` và `discord` cần `email.Sender` và `discord.Bot`.

## 6. Đồ thị luồng dữ liệu (tóm tắt)

```text
Roster CSV ──fetcher──▶ student repo ──▶ identity.BindStart/BindVerify
Class CSV  ──fetcher──▶ mark cache ──▶ enrollment ──▶ classsync ──▶ discord.Bot (role)
                                          │
/mark  ──tele/discord──▶ identity.GetBinding(PlatformUserID) ──▶ mark repo ──▶ reply
/marks ──api(JWT #44)─▶ assertion.Verify(sub=MSSV) ──▶ marksquery ──▶ mark repo ──▶ JSON
```

## 7. `GET /marks` — tra điểm cho student app (mới, #44)

Endpoint trên binary api (`cmd/api`), chạy **cùng** `/mark` cũ: `/mark` vẫn auth bằng static token (`API_TOKEN`), nguyên vẹn; `/marks` dùng middleware `Jwt` riêng — auth bằng JWT của student app.

- **Auth:** header `Authorization: Bearer <JWT>`; verify chữ ký **EdDSA** (whitelist chỉ EdDSA), key resolve theo header `kid` từ JWKS của student app (`AUTH_JWKS_URL`); kiểm `iss` (`AUTH_JWT_ISSUER`), `aud` (`AUTH_JWT_AUDIENCE`), `exp` bắt buộc. Claim `sub` = MSSV — identity duy nhất, lấy từ context, không nhận từ query.
- **Response `200`:** luôn là mảng `[{"courseId":"...","marks":{...}}]`; `[]` khi không có dữ liệu (MSSV lạ, `course_id` lạ, SV không có mark doc của môn đó). Header `Cache-Control: no-store`. Query `course_id` tuỳ chọn lọc 1 môn; tham số MSSV/student **bị bỏ qua**.
- **KHÔNG gate email** — mapping email→MSSV thuộc contact-stu upstream (hcmut-util #146); api chỉ tin claim `sub`.
- **Lỗi:**

| HTTP | `error` | Khi nào |
|---|---|---|
| 401 | `jwt_invalid` | thiếu/sai prefix `Bearer`, chữ ký/iss/aud/alg sai, kid lạ, thiếu `sub` |
| 401 | `jwt_expired` | claim `exp` đã quá hạn |
| 503 | `jwks_unavailable` | không fetch/parse được JWKS endpoint |
| 500 | `internal_error` | lỗi đọc mark cache không mong muốn |

- **Deploy pairing:** JWKS default trỏ student app (hcmut-util #151) — `https://student.thuanle.me/.well-known/jwks.json`.
