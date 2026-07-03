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
Lecturer /create
  │
  ▼
[iam] kiểm quyền Lecturer (và sở hữu nếu sửa lớp có sẵn)
  │
  ▼
[markimport.FetchMarkLinkIntoCourse]
  • UpdateCourseLink(courseId, link, owner)
  • DownloadCSV(link) → CleanRawCsvRecords (định dạng 3 dòng)
  • xóa mark cũ → insert mark mới (per-course collection)
  • UpdateCourseRecordCount
  │
  ▼ (Discord) [provisioning]
  • discord.Bot.EnsureRole(courseId)   → roleID
  • discord.Bot.EnsureChannel(lowercase(courseId), roleID)
  • (chỉ tạo nếu chưa có theo tên — idempotent)
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
  • discord.Bot tìm role theo tên = courseId
       └─ không có → bỏ qua (chưa /create trên Discord)
  ▼
[classsync]
  • enrolled = MSSV set từ mark cache của lớp
  • map MSSV → Discord userID qua binding (platform=discord)
  • current  = discord.Bot.MembersWithRole(courseId)
  • toAdd    = enrolled_ids \ current
  • toRemove = current \ enrolled_ids
  • AssignRole / RemoveRole
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
Lecturer /sync
  │
  ▼
[iam] kiểm quyền sở hữu lớp
  │
  ▼
markimport.FetchMarkLinkIntoCourse (tải lại CSV)
  │
  ▼
classsync reconcile role ngay (như §5 cho 1 lớp)
```

## 8. `/delete <courseId>`

```text
Lecturer/Admin /delete
  │
  ▼
[iam] kiểm quyền
  │
  ▼
• course.RemoveCourse(courseId)
• mark.RemoveCourseMarks(courseId)  (drop collection)
  │
  ▼ (Discord)
  • discord.Bot.DeleteChannel(lowercase(courseId))
  • discord.Bot.DeleteRole(courseId)
```

> Telegram `/clear <courseId>` (v1) thực hiện phần DB (xóa lớp + marks), không có Discord.
