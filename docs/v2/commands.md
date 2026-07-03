# Tham chiếu lệnh v2

> Toàn bộ lệnh Telegram và Discord. Yêu cầu ở `SRS-v2.md` §12.

Quy ước: `<bắt buộc>` `[tùy chọn]`. Trả lời nhạy cảm (điểm) ở Discord là **ephemeral**.

## Telegram

| Lệnh | Ai | Hành vi |
|---|---|---|
| `/start` | Guest | Lời chào, hướng dẫn `/bind`. |
| `/bind` | Guest | Bind email→MSSV (xem `flows.md` §1). |
| `/mark` | Đã bind | Tổng điểm mọi lớp đang học của MSSV đã bind. |
| `/mark <courseId>` | Đã bind | Điểm của một môn. |
| `/create <courseId> <csvUrl>` | Admin | Tạo/cập nhật lớp + import marks (đổi tên từ `/load`). |
| `/clear <courseId>` | Admin | Xoá lớp + marks. |
| `/my` | Admin | Profile + danh sách toàn bộ lớp trong hệ thống. |

### Telegram — ví dụ

```text
/bind
> Nhập email HCMUT: abc@hcmut.edu.vn
> Đã gửi OTP. Nhập mã: 123456
✅ Đã liên kết với MSSV 2212345.

/mark CO2003-L01
Lab 1   10
Lab 2    9
...

/create CO2003-L01 https://example.com/co2003.csv
✅ Đã nhập 45 dòng điểm.
```

### Telegram — lỗi thường

- `/mark` khi chưa bind → "Chưa xác thực, dùng /bind".
- `/create` không phải Admin → UnauthorizedError.
- `courseId` sai định dạng → ArgValueMismatchError.

## Discord

| Lệnh | Ai | Hành vi |
|---|---|---|
| `/bind` | Guest | Bind email→MSSV + tự động cấp role các lớp đang học. |
| `/profile` | Student | MSSV, họ tên, email, danh sách lớp, trạng thái bind (ephemeral). |
| `/mark` | Student | Tổng điểm mọi lớp (ephemeral). |
| `/mark <courseId>` | Student | Điểm một môn (ephemeral). |
| `/create <courseId> <csvUrl>` | Admin | Import + đảm bảo role/channel tồn tại (tạo theo tên nếu thiếu, lưu vào `discord_mappings`). |
| `/sync <courseId>` | Admin | Tải lại CSV + reconcile role ngay. |

### Discord — ví dụ

```text
/bind
> Nhập email HCMUT: abc@hcmut.edu.vn
> Nhập OTP: 123456
✅ Liên kết MSSV 2212345. Đã cấp role: CO2003-L01, CO1007-L02.

/mark CO2003-L01   (ephemeral — chỉ mình SV thấy)
CO2003-L01
Lab 1   10
...
```

### Discord — lỗi thường

- `/bind` email không thuộc roster → "Email chưa có trong danh sách sinh viên".
- OTP sai/hết hạn → yêu cầu gửi lại.
- `/create` role/channel trùng tên đã tồn tại → tái sử dụng (idempotent), không lỗi.
- `discord.Bot` rate-limit → scheduler retry/backoff (xem `architecture.md`).

## Bảng so sánh nhanh

| Chức năng | Telegram | Discord |
|---|---|---|
| Bind email→MSSV | `/bind` | `/bind` |
| Xem thông tin | `/my` (admin) | `/profile` (student) |
| Tra điểm | `/mark [courseId]` | `/mark [courseId]` |
| Tạo/cập lớp | `/create` | `/create` (+provision) |
| Sync ngay | — | `/sync` |
| Dọn dữ liệu lớp | `/clear` | — |
| Quản role | — | tự động (role-sync) |
