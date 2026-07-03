# Migration v1 → v2

> Triển khai v2 song song v1, rủi ro thấp (dữ liệu cộng thêm). Yêu cầu ở `SRS-v2.md`; triển khai ở `deployment.md`.

## 1. Nguyên tắc

- **Cộng thêm, không phá**: v2 thêm collection mới (`students`, `bindings`, `verifications`), mở rộng trường `user.role`. Dữ liệu v1 (`courses`, marks, `users`) giữ nguyên.
- Telegram tiếp tục chạy trong suốt quá trình; chỉ `/mark` thay đổi hành vi (cần bind) ở cutover.
- Discord là service mới, có thể bật/tắt độc lập.

## 2. Điều kiện tiên quyết

- Roster CSV sẵn sàng tại `ROSTER_CSV_URL` (mssv, name, email).
- SMTP creds + Discord bot (token, guild, quyền manage role/channel) đã chuẩn bị (`deployment.md` §5–6).
- DB đã backup.

## 3. Các pha

### Pha 0 — Chuẩn bị (không thay đổi runtime)

1. Chuẩn bị secret Discord/SMTP/roster (`enc.env`/SOPS).
2. Tạo `Dockerfile_discord`, thêm service `discord` vào compose, thêm `discord` vào CI matrix. (Chưa enable service.)

### Pha 1 — Schema (idempotent)

3. Tạo collection mới + index:
   - `mark-settings.students` — unique index `email`.
   - `mark-settings.bindings` — unique `(platform, platform_user_id)`, index `mssv`.
   - `mark-settings.verifications` — TTL index trên `expiry`.
4. Mở rộng `users`: thêm trường `role` (default `student`); dữ liệu cũ `IsTeacher=true` → `role=lecturer`.

```js
// ví dụ migration (Mongo shell)
db.users.updateMany({is_teacher: true}, {$set: {role: "lecturer"}})
db.users.updateMany({role: {$exists: false}}, {$set: {role: "student"}})
db.students.createIndex({email: 1}, {unique: true})
db.bindings.createIndex({"platform":1,"platform_user_id":1}, {unique: true})
db.bindings.createIndex({mssv: 1})
db.verifications.createIndex({expiry: 1}, {expireAfterSeconds: 0})
```

### Pha 2 — Deploy service mới

5. Build + push image `cse-mark-discord` (CI).
6. Up service `discord` (canary): verify bot online, `/bind` thử 1 SV, role được cấp đúng.
7. Up lại `fetcher` (bản mới): roster sync chạy, `students` được populate. Mark sync tiếp tục như cũ.

### Pha 3 — Telegram update + cutover

8. Up lại `tele` (bản mới): có `/bind`, `/mark [courseId]`, `/create`.
9. **Breaking change:** `/mark` giờ yêu cầu bind (không nhận student_id). Thông báo trước cho người dùng.
   - Tuỳ chọn giảm sốc: giữ `/mark <courseId> <studentId>` cũ thêm 1 chu kỳ deprecation, song song với `/mark` mới.
10. Lecturer cũ (`role=lecturer` từ Pha 1) tiếp tục dùng `/create`.

### Pha 4 — Vận hành

11. Bật role-sync scheduler (Discord) — đã bật cùng service discord.
12. Theo dõi log: SMTP deliverability, Discord rate-limit, scheduler mark/roster/role.

## 4. Rollback

- **Telegram**: revert image `tele` về v1 (trả lại `/mark <studentId>`, `/load`). Dữ liệu bindings không ảnh hưởng.
- **Discord**: stop service `discord` (role/channel đã tạo ở lại guild, có thể dọn thủ công).
- **Schema**: collection/index mới để nguyên (an toàn). Trường `role` để nguyên.
- Roster/mark data v1 không bị sửa bởi v2 → an toàn.

## 5. Rủi ro & giảm nhẹ

| Rủi ro | Giảm nhẹ |
|---|---|
| `/mark` Telegram breaking (SV chưa bind) | chu kỳ deprecation + thông báo; `/bind` đơn giản. |
| OTP không tới (spam SMTP) | sender uy tín, test trước; giới hạn lại gửi OTP. |
| Discord rate-limit khi role-sync lớn | batch assign/remove + backoff; `ROLE_SYNC_INTERVAL` điều chỉnh. |
| Trùng tên role/channel | naming cố định theo `courseId`; `EnsureRole/EnsureChannel` idempotent. |
| Roster thiếu email SV | bind báo lỗi rõ; bổ sung roster CSV. |

## 6. Kiểm thử chấp nhận (post-migration)

- [ ] Roster sync populate `students`; email → MSSV đúng.
- [ ] Telegram `/bind` + `/mark` hoạt động.
- [ ] Discord `/bind` cấp đúng role các lớp đang học.
- [ ] `/create` Discord tạo role + channel theo tên; chạy lại không trùng.
- [ ] `/sync` reconcile role đúng (add/remove).
- [ ] Scheduler mark/roster/role chạy không lỗi trong 24h.
