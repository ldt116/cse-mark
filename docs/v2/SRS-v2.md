# Software Requirements Specification (SRS) — v2

# Hệ thống TL-CSE Mark & Learning Management (Telegram + Discord)

- **Version:** 2.0
- **Ngày:** 2026-07-03
- **Trạng thái:** Bản nháp — chờ review
- **Repo:** `hcmut/cse-mark` (backend Go, clean architecture)
- **Phạm vi:** Mở rộng backend hiện tại (Telegram mark bot) để chạy **song song** hệ thống Discord LMS, trên **cùng một backend**. Không viết lại.

> Tài liệu này là đặc tả yêu cầu (requirements). Thiết kế chi tiết ở `architecture.md`, `data-model.md`, `flows.md`; tham chiếu lệnh ở `commands.md`; cấu hình ở `config-env.md`; triển khai ở `deployment.md`, `migration.md`. Tài liệu hệ thống v1 nằm trong `docs/v1/`.

---

# 1. Giới thiệu

## 1.1 Mục đích

Hệ thống hỗ trợ giảng viên và sinh viên trên **hai nền tảng giao tiếp** (Telegram và Discord), dùng chung một backend dữ liệu học tập:

- **Telegram (v1, giữ lại):** tra cứu điểm nhanh, tải bảng điểm CSV.
- **Discord (v2, mới):** quản lý lớp học đầy đủ — xác thực sinh viên, phân quyền channel tự động, tra cứu điểm.

Hệ thống cung cấp các chức năng:

- Xác thực sinh viên bằng email HCMUT (OTP) — **identity thống nhất cho cả hai nền tảng**.
- Liên kết tài khoản Telegram/Discord với MSSV (bind một lần).
- Đồng bộ danh sách sinh viên (roster) và danh sách lớp học.
- Tự động phân quyền truy cập Discord Channel theo enrollment.
- Tra cứu điểm học tập (Telegram và Discord).
- Đồng bộ dữ liệu từ bảng điểm CSV/Excel.

Telegram và Discord đều là **bề mặt giao tiếp**; toàn bộ dữ liệu học tập được quản lý trong backend và được đồng bộ ra hai nền tảng.

## 1.2 Đối tượng đọc

- Giảng viên / Admin vận hành hệ thống.
- Developer triển khai và bảo trì v2.

---

# 2. Mục tiêu

- Một backend duy nhất phục vụ cả Telegram và Discord.
- Sinh viên **bind một lần** (email → MSSV), dùng cho cả hai nền tảng.
- Không cần quản trị viên cấp quyền channel thủ công (Discord auto role).
- Không yêu cầu sinh viên tự chọn lớp (lấy từ enrollment).
- Dữ liệu lớp học và điểm được đồng bộ tự động từ CSV/Excel.
- **Roster** (danh sách sinh viên chính thức) là nguồn email → MSSV cho việc bind.
- Discord chỉ hiển thị dữ liệu và phân quyền; Excel/CSV là nguồn dữ liệu chính.
- Telegram giữ phạm vi hiện tại (tra cứu điểm), chỉ bổ sung bind.

---

# 3. Kiến trúc tổng thể

## 3.1 Sơ đồ

```text
   Telegram                 Discord Server
      │                          │
      ▼                          ▼
  tele bot                  discord bot
      │                          │
      └──────────┬───────────────┘
                 ▼
        ┌─────────────────┐        ┌──────────────┐
        │  Backend (Go)   │◄──────►│  MongoDB     │
        │  clean arch     │        │  mark-cse    │
        └────────┬────────┘        │  mark-settings│
                 │                  └──────────────┘
        ┌────────┴────────┐
        ▼                 ▼
   api (HTTP)        fetcher (scheduler)
   /healthz /mark    • mark sync (10p)
                     • roster sync (mới)
                            │
                            ▼
                   CSV lớp + Roster CSV
                   (Excel / Google Sheet)
```

## 3.2 Các service (triển khai độc lập, docker-compose)

| Service | Vai trò | Trạng thái v2 |
|---|---|---|
| `api` | HTTP: `GET /healthz`, `GET /mark` | Giữ nguyên |
| `fetcher` | Scheduler: mark sync (10p) **+ roster sync** | Mở rộng |
| `tele` | Telegram bot: tra cứu điểm + bind | Mở rộng (tối thiểu) |
| `discord` | Discord bot + role-sync scheduler | **Mới** |

## 3.3 Lớp hóa clean architecture

Backend tuân thủ clean architecture (xem `docs/v1/DEVELOPMENT.md`). v2 **giữ nguyên các lớp** và bổ sung:

- **Domain (gồm rules):** giữ `course, user, mark, downloader, teleuser`. **Bổ sung** `student`, `binding`, `verification`, và **port** `discord.Bot`, `email.Sender`.
- **Use cases:** giữ `iam, coursequery, markimport, marksync`. **Bổ sung** `identity` (bind/verify OTP), `rostersync` (đồng bộ roster), `classsync` (enrollment → role-sync Discord).
- **Delivery:** giữ `tele`, `api`. **Bổ sung** `discord`.
- **Infra:** giữ `mongo`, `http`. **Bổ sung** `infra/discord` (discordgo), `infra/email` (SMTP), và các mongo repo mới.

## 3.4 Nguyên tắc tái dùng

- Pipeline mark (fetcher → `markimport` → `marksync`) **platform-agnostic**, tái dụng nguyên cho cả Telegram và Discord.
- Identity (bind) và provisioning Discord được thêm dưới dạng **layer mới**, không sửa logic mark hiện có.
- Mọi tích hợp nền tảng cụ thể (Discord API, SMTP) nằm sau **port** (interface) ở domain/use case, implementation ở infra — đúng pattern hiện tại.

---

# 4. Thành phần hệ thống

## 4.1 Telegram (bot, v1)

Giữ nguyên vai trò: giao tiếp, tra cứu điểm. Bổ sung `/bind`. Chi tiết lệnh ở §12.

## 4.2 Discord Server (v2)

Bao gồm các channel:

- Welcome / Rules / Verify (public).
- Announcement.
- Class Channels (mỗi lớp một channel, phân quyền theo role).

Discord chỉ chịu trách nhiệm: giao tiếp, phân quyền, hiển thị thông tin.

## 4.3 Discord Bot (v2 — `cmd/discord`)

Chịu trách nhiệm:

- xác thực sinh viên (OTP qua email),
- bind Discord ↔ MSSV,
- tra cứu điểm (`/mark`),
- `/create` lớp (tạo role + channel theo tên, import CSV),
- `/sync`, `/delete`,
- scheduler đồng bộ role theo enrollment.

## 4.4 Database (MongoDB)

Lưu: Student (roster), Binding, Verification (TTL), Class (= course), User/Account (role), Mark cache, cấu hình. **Database không phải nguồn dữ liệu chính của điểm/roster** — chỉ là cache sau đồng bộ.

## 4.5 Excel / CSV

- **Class CSV:** nguồn điểm chính cho mỗi lớp (Excel publish dạng CSV).
- **Roster CSV:** nguồn email → MSSV (mssv, họ tên, email), do admin duy trì qua URL download độc lập.

---

# 5. Vai trò người dùng

> Không có vai trò Teaching Assistant (TA) hay Giảng viên (Lecturer) trong v2. Hệ thống chỉ gồm 2 vai trò: Admin và Student.

## 5.1 Admin

- Quản lý Bot, cấu hình, secret.
- Tạo / xoá / đồng bộ lớp học và điểm (`/create`, `/sync`, `/delete`, `/clear`).
- Xem log hệ thống.

Cơ chế: **whitelist theo platform UserID** (env config), áp dụng cho cả Telegram và Discord.

## 5.2 Student

- Bind tài khoản (email → MSSV).
- Tra cứu điểm.
- Xem các channel được cấp quyền tự động (Discord).

Cơ chế: Mặc định khi bind thành công.

---

# 6. Luồng sử dụng

## 6.1 Bind (Telegram và Discord — identity thống nhất)

1. SV chạy `/bind`, bot yêu cầu **email HCMUT** (vd `abc@hcmut.edu.vn`).
2. Bot kiểm tra **email** trong danh sách `student` (roster). Nếu email **không có trong roster → báo lỗi và kết thúc ngay**.
3. Nếu email hợp lệ, bot sinh OTP, gửi tới email đó qua `email.Sender`; lưu `verification` (sử dụng kiểu Date cho TTL).
4. SV nhập OTP.
5. Bot kiểm OTP + expiry và thực hiện lưu `binding` (platform + platformUserID ↔ MSSV, verified).

## 6.2 Sau bind — Discord

Bot tính tất cả lớp học của SV (từ enrollment) và cấp toàn bộ role tương ứng (xem §14). SV lập tức thấy các class channel.

## 6.3 Sau bind — Telegram

SV dùng `/mark` (tất cả điểm) hoặc `/mark <courseId>` (một môn), hệ thống tra MSSV từ binding — không cần gõ MSSV.

---

# 7. Mô hình lớp học

Mỗi lớp học (Class) ánh xạ 1-1 với entity `course` hiện có của backend, giữ nguyên cấu trúc v1 để tránh dư thừa dữ liệu:

| Trường | Ý nghĩa |
|---|---|
| `courseId` | Mã lớp (vd `CO2003-L01`, `cnpm-231`) — khóa chính |
| `link` | URL CSV bảng điểm |
| `byUser` | MSSV/UserID chủ sở hữu (Admin tạo) |
| `updatedAt` | Thời điểm đồng bộ gần nhất |
| `recordCnt` | Số dòng điểm |

### Discord Mapping (bảng/collection riêng)

Để tránh thông tin thừa trong Class, phần thông tin liên kết Discord được quản lý thông qua collection riêng (`discord_mappings`) mapping `courseId` -> `discordRoleId` & `discordChannelId`:

| Trường | Ý nghĩa |
|---|---|
| `courseId` | Mã lớp (khóa chính `_id`) |
| `discordRoleId` | ID của Role tương ứng trên Discord Guild (string) |
| `discordChannelId` | ID của Channel tương ứng trên Discord Guild (string) |

> **Quy trình hoạt động:** Khi tạo lớp qua `/create` lần đầu tiên, Discord bot tự resolve role/channel theo tên, lưu lại `discordRoleId` và `discordChannelId` vào collection `discord_mappings`. Các lần sync sau sẽ truy xuất trực tiếp các ID này để làm việc với Discord API.

**Quy ước đặt tên (Discord):**

| Đối tượng | Tên |
|---|---|
| Role | `courseId` nguyên bản (vd `CO2003-L01`) |
| Channel | `courseId` lowercase (vd `co2003-l01`) |

Channel chỉ cho phép role tương ứng truy cập. Admin hệ thống mặc định được cấp quyền truy cập toàn bộ kênh học tập mà không cần gán role lớp học.

---

# 8. Quản lý lớp học — `/create`

Admin tạo/cập nhật lớp:

```text
/create <course-id> <csv-url>
```

Ví dụ:

```text
/create CO2003-L01 https://example.com/co2003.csv
```

Bot thực hiện:

1. Đăng ký/cập nhật `Class` (link, byUser).
2. Tải CSV và import marks (tái dùng `markimport`).
3. **Chỉ Discord:** đảm bảo role + channel tồn tại — tạo theo tên nếu chưa có (idempotent), sau đó lưu thông tin liên kết vào `discord_mappings`.
4. Cập nhật enrollment + (Discord) đồng bộ role.

> `/create` trên Telegram chỉ làm bước 1–2 (không có role/channel). `/create` là dạng đổi tên của `/load` cũ ở Telegram.

---

# 9. Cấu trúc CSV

## 9.1 Class CSV (giữ nguyên định dạng v1)

| Dòng | Nội dung |
|---|---|
| 1 (flags) | `id` = cột MSSV; `x`/bất kỳ = cột public; rỗng = cột ẩn |
| 2 (headers) | Tên cột điểm hiển thị |
| 3..n | Dữ liệu sinh viên + điểm |

Ví dụ:

| id | x | (ẩn) | x |
|----|---|------|---|
| MSSV | Họ tên | Ghi chú nội bộ | Lab 1 |

Bot chỉ yêu cầu cột `id` (MSSV). Các cột còn lại là điểm hiển thị. Cột Email trong class CSV là **tùy chọn** (không dùng cho bind — bind dùng roster).

## 9.2 Roster CSV (mới)

Đúng 3 cột:

| MSSV | Name | Email |
|------|------|-------|

Đây là nguồn chính thức cho **email → MSSV**. Admin duy trì file (Excel/Google Sheet publish CSV), cấu hình URL qua `ROSTER_CSV_URL`. `fetcher` đồng bộ định kỳ (xem §10).

---

# 10. Đồng bộ dữ liệu

## 10.1 Mark sync (giữ nguyên, `fetcher`)

Mỗi 10 phút: với mỗi lớp active (≤ 9 tháng kể từ lần cập nhật cuối), tải Class CSV → parse → cập nhật mark cache → cập nhật enrollment (implicit từ MSSV trong CSV).

## 10.2 Roster sync (mới, `fetcher`)

Lịch định kỳ riêng (**mặc định hàng ngày**, cấu hình qua env): tải Roster CSV → upsert `student` (MSSV, Name, Email). Đây là cơ sở để bind tra email → MSSV. Độc lập với mark sync (10 phút).

## 10.3 Role sync (mới, `discord` service)

Scheduler định kỳ: với mỗi lớp **đã có role Discord** (resolve theo tên), tính enrollment → diff role thành viên → gán/gỡ qua `discord.Bot` (xem §14).

> Việc thêm/rút học phần được phản ánh tự động ở lần đồng bộ tiếp theo (mark sync cập nhật enrollment → role sync cập nhật role).

---

# 11. Phân quyền

## 11.1 System roles

- **Admin** — whitelist config hoặc cấu hình trong database.
- **Student** — mặc định khi bind.

## 11.2 Course roles (Discord)

Mỗi lớp một role (tên = `courseId`). Channel lớp chỉ cho phép role tương ứng.

---

# 12. Các lệnh Bot

## 12.1 Telegram (giữ nguyên + tối thiểu)

| Lệnh | Ai dùng | Chức năng |
|---|---|---|
| `/start` | Guest | Lời chào |
| `/bind` | Guest | email-OTP → bind Telegram-ID ↔ MSSV (mới) |
| `/mark [courseId]` | Đã bind | Không args: **tất cả điểm**; có courseId: điểm môn đó (sửa — dùng MSSV đã bind) |
| `/create <courseId> <csvUrl>` | Admin | Tải link CSV + import marks (đổi tên từ `/load`) |
| `/clear <courseId>` | Admin | Xoá lớp + marks |
| `/my` | Admin | Profile / danh sách lớp quản lý |

## 12.2 Discord (mới)

| Lệnh | Ai dùng | Chức năng |
|---|---|---|
| `/bind` | Guest | email-OTP → bind + tự động cấp role các lớp đang học |
| `/profile` | Student | MSSV, họ tên, email, danh sách lớp, trạng thái bind |
| `/mark [courseId]` | Student | Tất cả điểm hoặc một môn (ephemeral) |
| `/create <courseId> <csvUrl>` | Admin | Import + đảm bảo role/channel tồn tại |
| `/sync <courseId>` | Admin | Tải lại CSV + reconcile role ngay |
| `/delete <courseId>` | Admin | Xoá lớp + marks + archive/xoá channel & role |

> Discord trả kết quả nhạy cảm (điểm) dưới dạng **ephemeral message**.

---

# 13. Mô hình dữ liệu

## Student (roster)

- `MSSV` (khóa)
- `Name`
- `Email`

Nguồn: Roster CSV.

## Binding

- `Platform` (`telegram` | `discord`)
- `PlatformUserID` (Telegram chat ID / Discord user ID)
- `MSSV`
- `Verified` (bool)
- `BoundAt`

Ràng buộc: 
1. Unique theo `(Platform, PlatformUserID)` để một tài khoản chat chỉ liên kết tối đa với một MSSV.
2. Unique theo `(Platform, MSSV)` để đảm bảo quan hệ 1:1:1 (một MSSV chỉ liên kết với tối đa 1 Telegram ID và 1 Discord ID).

## Class (= entity `course`)

- `CourseId` (khóa)
- `Link` (CSV URL)
- `ByUser` (chủ sở hữu - Admin tạo)
- `UpdatedAt`
- `RecordCnt`

## DiscordMapping (mới)

- `CourseId` (khóa chính `_id` kết hợp với Class)
- `DiscordRoleId` (string)
- `DiscordChannelId` (string)

## Enrollment (phái sinh)

- `MSSV` ↔ `CourseId`
- Không lưu collection riêng — phái sinh từ mark cache (các MSSV xuất hiện trong Class CSV).

## Mark cache (giữ nguyên v1)

- Khóa `(CourseId, MSSV)`.
- Mỗi lớp một collection MongoDB, doc `_id` = MSSV, giá trị = JSON grade.

## Verification (tạm thời, TTL)

- `PlatformUserID` (khóa chính `_id`)
- `Email`
- `OTP`
- `Expiry` (kiểu Date) - Thời điểm hết hạn (để phục vụ MongoDB TTL Index)

## User / Account (role)

- `MSSV`/`UserId` (khóa chính `_id`)
- `Role` (`admin` | `student`)
- `GrantedBy`

Cơ chế phân quyền: Quyền Admin được xác thực bằng cách đối chiếu trực tiếp platformUserID của người gửi lệnh với whitelist cấu hình (env `ADMINS` hoặc `DISCORD_ADMIN_IDS`) hoặc trường `Role` trong database `users`. Mặc định người dùng thông thường khi bind thành công sẽ mang vai trò `student`.

---

# 14. Đồng bộ role (Discord)

```text
Enrollment (từ Class CSV / mark cache)
        │
        ▼
MSSV đang học lớp
        │
        ▼
tra Discord UserID qua Binding
        │
        ▼
tra Discord RoleID qua DiscordMapping
        │
        ▼
discord.Bot: gán role cho enrolled, gỡ role cho học viên cũ (bỏ qua Admin)
```

- Role/channel **không được chỉnh sửa thủ công** — bot là nguồn đồng bộ duy nhất.
- Role/channel được **tạo theo tên** khi Admin chạy lệnh `/create` trên Discord, sau đó lưu lại ID trong bảng `discord_mappings`.
- Scheduler role-sync chỉ xử lý các lớp **có bản ghi trong discord_mappings**. Lớp chỉ có trên Telegram (chưa được `/create` hoặc provision trên Discord) không có trong `discord_mappings` → bỏ qua.

---

# 15. Nguyên tắc thiết kế

- Một backend duy nhất cho Telegram và Discord.
- Một lần bind (email → MSSV) cho toàn bộ quá trình học, dùng chung hai nền tảng.
- Một role + một channel cho mỗi lớp (Discord), resolve theo tên `courseId`.
- Một CSV cho mỗi lớp (Class CSV); một roster CSV cho toàn bộ sinh viên.
- Excel/CSV là nguồn dữ liệu chính; backend/Discord chỉ hiển thị và phân quyền.
- Bot chịu trách nhiệm đồng bộ; mọi thay đổi lớp học thực hiện trên Excel/CSV.
- Hỗ trợ sinh viên học nhiều lớp cùng lúc; thêm/rút học phần phản ánh tự động ở lần đồng bộ tiếp theo.
- Tái dùng tối đa pipeline mark hiện có; identity và Discord provisioning là layer mới không phá v1.
- Mọi tích hợp nền tảng (Discord, SMTP) nằm sau port (interface), implementation ở infra.
