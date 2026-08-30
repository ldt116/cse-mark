# Luồng nghiệp vụ v2

> Trình tự các luồng chính. Yêu cầu ở `SRS-v2.md`; kiến trúc ở `architecture.md`.

## 1. Bind (Telegram & Discord)

```text
SV /bind
  │
  ▼
[delivery] hỏi email  ──▶ SV nhập email
  │
  ▼
[identity.BindStart]
  • validate @hcmut.edu.vn
  • tra student repo theo email
       └─ không có → báo lỗi: email chưa có trong roster
  • sinh OTP (độ dài OTP_LEN), tính expiry (OTP_TTL)
  • lưu verification (TTL)
  • email.Sender.SendOTP(email, otp)
  │
  ▼
SV nhập OTP
  │
  ▼
[identity.BindVerify]
  • so khớp OTP + chưa hết hạn
  • lấy MSSV từ verification/email
  • kiểm tra ràng buộc 1:1:1 (MSSV hoặc tài khoản chat chưa được liên kết)
  • upsert binding (platform, platformUserID, MSSV, verified=true)
  │
  ▼ (Discord) [classsync] tính enrollment → gán role các lớp đang học
  ▼ (Telegram) chỉ lưu binding, không có role
```

Bắt lỗi: email không thuộc roster, OTP sai/hết hạn, vượt số lần thử.

## 2. Roster sync (fetcher)

```text
[scheduler, mỗi ROSTER_SYNC_INTERVAL (mặc định 24h)]
  │
  ▼
[rostersync] DownloadCSV(ROSTER_CSV_URL)
  │
  ▼
parse (mssv, name, email) mỗi dòng
  │
  ▼
upsert student repo (theo MSSV); cập nhật name/email
```

Độc lập với mark sync.

## 3. Tạo/cập nhật lớp — `/create <courseId> <csvUrl>`

```text
Admin /create
  │
  ▼
[iam] kiểm quyền Admin
  │
  ▼
[markimport.FetchMarkLinkIntoCourse]
  • upsert course link + metadata đồng bộ
  • DownloadCSV(link) → CleanRawCsvRecords (định dạng 3 dòng)
  • xóa mark cũ → insert mark mới (per-course collection)
  • UpdateCourseRecordCount
  │
  ▼ (Discord) [provisioning]
  • discord.Bot.EnsureRole(courseId)   → roleID
  • discord.Bot.EnsureChannel(lowercase(courseId), roleID) → channelID
  • lưu roleID và channelID vào database discord_mappings
  │
  ▼ (Discord) [classsync] reconcile role ngay
```

> Telegram `/create` dừng sau markimport (không provisioning).

## 4. Mark sync (fetcher, giữ nguyên v1)

```text
[scheduler, mỗi 10 phút]
  │
  ▼
[coursequery] ListActiveCourses (updated_at ≤ 9 tháng, có link)
  │ (cách nhau 1 phút giữa các lớp)
  ▼
[markimport.FetchMarkLinkIntoCourse] cho mỗi lớp
```

Cập nhật mark cache + enrollment (implicit).

## 5. Role sync (discord service)

```text
[scheduler, mỗi ROLE_SYNC_INTERVAL]
  │
  ▼
for mỗi Class:
  • tra cứu discord_mappings lấy discordRoleId
       └─ không có (chưa /create trên Discord) → bỏ qua
  ▼
[classsync]
  • enrolled = MSSV set từ mark cache của lớp
  • map MSSV → Discord userID qua binding (platform=discord, bỏ qua các MSSV chưa bind)
  • current  = discord.Bot.MembersWithRole(discordRoleId)  (sử dụng roleID đã lấy)
  • toAdd    = enrolled_ids \ current
  • toRemove = current \ enrolled_ids
  • AssignRole / RemoveRole (sử dụng discordRoleId)
```

## 6. Tra cứu điểm — `/mark [courseId]`

```text
SV /mark  hoặc  /mark <courseId>
  │
  ▼
[identity.GetBinding] PlatformUserID → MSSV
       └─ chưa bind → yêu cầu /bind
  │
  ▼
không args:
  • duyệt các lớp có MSSV này trong mark cache
  • gom điểm tất cả → render
có courseId:
  • mark repo.GetMark(courseId, MSSV)
  │
  ▼
reply (Discord: ephemeral; Telegram: reply thường)
```

## 7. `/sync <courseId>` (Discord)

```text
Admin /sync
  │
  ▼
[iam] kiểm quyền Admin
  │
  ▼
markimport.FetchMarkLinkIntoCourse (tải lại CSV)
  │
  ▼
classsync reconcile role ngay (như §5 cho 1 lớp)
```

## 8. Telegram `/clear <courseId>`

```text
Admin /clear
  │
  ▼
[iam] kiểm quyền Admin
  │
  ▼
• course.RemoveCourse(courseId)
• mark.RemoveCourseMarks(courseId)  (drop collection)
```

> `/clear` chỉ dọn dữ liệu backend. Role/channel Discord và bản ghi `discord_mappings` không bị xoá tự động trong phạm vi v2 hiện tại; nếu cần, Admin dọn thủ công ngoài hệ thống.

## 9. Tra điểm qua HTTP — `GET /marks` (#44, grade-share)

```text
student app  GET /marks[?course_id=...]
  │  Authorization: Bearer <JWT student app>
  ▼
[middleware Jwt]
  • verify chữ ký EdDSA (chỉ EdDSA) — key resolve theo header kid
    từ JWKS student app (AUTH_JWKS_URL; cache TTL 5m, kid lạ → refresh 1 lần)
  • kiểm iss (AUTH_JWT_ISSUER), aud (AUTH_JWT_AUDIENCE), exp bắt buộc
  • claim sub = MSSV → gin context
       └─ sai → 401 jwt_invalid / 401 jwt_expired
       └─ JWKS lỗi → 503 jwks_unavailable
  │
  ▼
[marksquery] Query(sub, course_id)
  • không course_id: duyệt mọi course, bỏ course SV không có mark doc
  • có course_id: GetMark(courseId, MSSV)
  │
  ▼
200 [{"courseId": "...", "marks": {...}}]   — luôn là mảng, [] khi không có dữ liệu
Cache-Control: no-store
```

Bắt lỗi: 401 `jwt_invalid` / 401 `jwt_expired` / 503 `jwks_unavailable` / 500 `internal_error`.

- Chạy trên binary api **cùng** `/mark` cũ — `/mark` vẫn auth `API_TOKEN`, route và middleware riêng.
- Tham số query MSSV/student **bị bỏ qua** — identity chỉ từ claim `sub`.
- KHÔNG gate email: mapping email→MSSV thuộc contact-stu phía hcmut-util upstream (#146).
- Deploy pairing với student app (hcmut-util #151) — JWKS default `https://student.thuanle.me/.well-known/jwks.json`.
