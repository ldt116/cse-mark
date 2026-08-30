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

## 4. Mark sync (fetcher)

```text
[scheduler, mỗi 10 phút]
  │
  ▼
[coursequery] ListActiveCourses → FindSyncableCourses
  (updated_at ≤ 9 tháng, có link, status ≠ inactive; thiếu status = active)
  │ (cách nhau 1 phút giữa các lớp)
  ▼
[markimport.FetchMarkLinkIntoCourse] cho mỗi lớp
  • DownloadCSVAuthorized(link, GV_PROXY_TOKEN)
       └─ token rỗng → không gửi Authorization (fetch public CSV như cũ)
  │
  ▼
[marksync] classifyFetchErr → hành vi theo lớp lỗi (§4.1), trạng thái course (§4.2)
```

Cập nhật mark cache + enrollment (implicit). Lỗi fetch **không** xoá marks cũ — chỉ lần import thành công mới thay dữ liệu.

### 4.1. Phân lớp lỗi feed

Lỗi download (phiên bản `*downloader.FeedError{Status, Code}`) được `classifyFetchErr` (`internal/usecases/marksync/interactor.go`) ánh xạ vào hành động:

| Lỗi | Lớp | Hành vi |
|---|---|---|
| 401, code `service_token_invalid` | config token | `GV_PROXY_TOKEN` hỏng: log Error (dedupe 1h toàn service), môn đó thử lại sau 1h |
| 403 / 404 (bất kỳ code) | permanent môn đó | Warn → course `stale`, poll chậm 1h/lần, **giữ marks lần import tốt cuối** |
| 410 | grant revoked | course `inactive`: ngừng poll, KHÔNG teardown; marks đóng băng, bot vẫn hiển thị |
| 429, 5xx, lỗi mạng/parse/khác | transient | đếm liên tiếp; đủ 6 lần → Warn "feed unhealthy" (1 lần/streak); thành công reset bộ đếm + auto-heal `stale`→`active` |

### 4.2. Trạng thái course

- `active` — mặc định; course cũ chưa có field `status` vẫn coi là active.
- `stale` — permanent fail (403/404): vẫn trong danh sách poll nhưng bị hạ nhịp còn 1h/lần; probe thành công → auto-heal về `active`.
- `inactive` — grant revoked (410): bị loại khỏi danh sách poll hẳn (`FindSyncableCourses` lọc `status ≠ inactive`); marks đóng băng vẫn đọc được qua bot.
- `/sync <courseId>` (Discord + Telegram, admin — xem §7) kích hoạt lại môn stale/inactive: set `active` **trước** khi fetch lại ngay.

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

## 7. `/sync <courseId>` (Discord & Telegram)

```text
Admin /sync <courseId>
  │
  ▼
[iam] kiểm quyền Admin
  │
  ▼
set course status = active (TRƯỚC khi fetch — poller tiếp tục kể cả khi fetch fail;
  đây là nút kích hoạt lại môn stale/inactive)
  │
  ▼
markimport.FetchMarkLinkIntoCourse (tải lại CSV theo link đã lưu, /create)
  │
  ▼ (Discord) classsync reconcile role ngay (như §5 cho 1 lớp)
```

> Telegram `/sync` dừng sau markimport (không có role).

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
