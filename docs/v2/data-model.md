# Mô hình dữ liệu v2

> Thực thể, collection MongoDB, index, và quy ước đặt tên. Yêu cầu ở `SRS-v2.md` §13.

## 1. Thực thể (entity)

### Student (roster)

| Trường | Kiểu | bson | Ghi chú |
|---|---|---|---|
| MSSV | string | `_id` | khóa |
| Name | string | `name` | |
| Email | string | `email` | `@hcmut.edu.vn` |

Nguồn: Roster CSV. Email unique ngầm (mỗi email ↔ 1 MSSV).

### Binding

| Trường | Kiểu | bson | Ghi chú |
|---|---|---|---|
| Platform | string | `platform` | `telegram` \| `discord` |
| PlatformUserID | string | `platform_user_id` | TG chat ID / Discord user ID (string) |
| MSSV | string | `mssv` | |
| Verified | bool | `verified` | |
| BoundAt | int64 | `bound_at` | unix |

Unique: `(platform, platform_user_id)`. Một MSSV có thể có binding trên cả 2 platform.

### Class (= entity `course`, giữ nguyên v1)

| Trường | Kiểu | bson |
|---|---|---|
| CourseId | string | `_id` (`course`) |
| Link | string | `link` |
| ByTeleId/ByUser | int64/string | `by_id` / `by_user` |
| UpdatedAt | int64 | `updated_at` |
| RecordCnt | int64 | `record_cnt` |

> Không thêm trường Discord nào. Quy ước đặt tên Discord role/channel ở §4.

### Enrollment (phái sinh, không collection riêng)

- `MSSV` ↔ `CourseId`.
- Phái sinh từ **mark cache** của lớp: tập các MSSV có document trong collection của lớp đó.

### Mark cache (giữ nguyên v1)

- Mỗi lớp = 1 collection trong DB `mark-cse`, tên collection = `courseId`.
- Document: `_id` = MSSV; các trường còn lại = cột điểm (public) theo header.

### Verification (TTL)

| Trường | Kiểu | bson |
|---|---|---|
| PlatformUserID | string | `_id` (hoặc `platform_user_id`) |
| Email | string | `email` |
| OTP | string | `otp` |
| Expiry | int64 | `expiry` |

Tự xoá qua **TTL index** (`expireAfterSeconds`).

### User / Account (mở rộng từ `user` v1)

| Trường | Kiểu | bson |
|---|---|---|
| UserId/MSSV | string | `_id` (`user_id`) |
| Role | string | `role` | `admin` \| `lecturer` \| `student` |
| GrantedBy | string | `granted_by` |

> Tương đương `user.Model{IsTeacher}` v1 nhưng tổng quát hóa thành `Role`.

## 2. CSDL / Collection MongoDB

| DB | Collection | Entity | Ghi chú |
|---|---|---|---|
| `mark-cse` | `<courseId>` (mỗi lớp 1 collection) | Mark cache | v1 |
| `mark-settings` | `courses` | Class | v1 (`DB_SETTINGS_COURSES`) |
| `mark-settings` | `users` | User/Account | v1 (`DB_SETTINGS_USERS`), mở rộng role |
| `mark-settings` | `students` | Student | **mới** |
| `mark-settings` | `bindings` | Binding | **mới** |
| `mark-settings` | `verifications` | Verification | **mới** (TTL) |

> Tên DB/collection cấu hình qua env (xem `config-env.md`).

## 3. Index

v1 hiện không có index ngoài `_id`. v2 thêm:

- `students`: unique index trên `email` (tra email → MSSV khi bind).
- `bindings`: unique index trên `(platform, platform_user_id)`; index trên `mssv` (liệt kê binding của 1 MSSV).
- `verifications`: **TTL index** trên `expiry` (tự xoá OTP hết hạn).
- `courses`: index trên `updated_at` (đã dùng ngầm bởi `FindCoursesUpdatedAfter`).
- mark collections: index `_id` (MSSV) — đủ cho `GetMark(courseId, studentId)`.

## 4. Quy ước đặt tên (Discord)

Discord role/channel được resolve **theo tên** (không lưu ID):

| Đối tượng | Quy tắc | Ví dụ (`courseId=CO2003-L01`) |
|---|---|---|
| Role | `courseId` nguyên bản | `CO2003-L01` |
| Channel | `lowercase(courseId)` | `co2003-l01` |

Ràng buộc: `courseId` phải hợp lệ làm tên channel (lowercase, không khoảng trắng). Quy tắc validate `courseId` hiện có (`course.Rules.IsValidCourseId`) đảm bảo dạng `[a-zA-Z][a-zA-Z0-9-]+`; channel dùng lowercase + giữ hyphen.

## 5. Mối quan hệ

```text
Student(MSSV) ──email── bind flow
Binding(MSSV, platform, platformUserID)
Class(CourseId) ──mark cache──▶ Enrollment(MSSV set)
/mark: PlatformUserID ──Binding──▶ MSSV ──▶ mark cache (theo CourseId)
role-sync: Enrollment ──Binding──▶ platformUserID ──▶ discord.Bot
```
