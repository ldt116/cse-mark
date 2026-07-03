# Hệ thống tra cứu điểm cho Sinh Viên qua Telegram

Tính năng chính:

- Tự động fetch điểm từ link nguồn (CSV) mỗi 10p.
- SV tra cứu điểm thông qua bot Telegram. Không public toàn bộ bảng điểm của SV khác.
- Dễ dàng kiểm soát cột điểm muốn công khai và cần che giấu.

## Screenshots

Tra cứu điểm:

![Tra cứu điểm](https://github.com/ldt116/cse-mark/assets/2363783/dd380a97-db9c-4b3e-aa1a-545e2964c6a0)

Nguồn điểm:

![Nguồn điểm](https://github.com/ldt116/cse-mark/assets/2363783/d71911e8-fbe3-4841-af8d-df524928ffb6)


## Cài đặt

### Tra cứu 

- Mọi user đều có thể tra cứu
- User chat với bot theo cú pháp `/mark <course_id> <user_id>`. Ví dụ:

```
/mark cnpmnc-231 2013307
```

Sẽ tra cứu môn _cnpm-231_ của SV _1913578_ và trả về tuỳ theo quy định muốn tiết lộ của GV. Ví dụ

```
{"_id":"2013307","HT":"NGUYỄN ĐỨC HUY","Kiểm tra - 20%":"7.25","Thi - 60%":"","Thí nghiệm - 20%":"7.8"}
```

### User và permission

- User role Teacher có quyền thêm link 1 bảng điểm vào hệ thống.
- Admin có thể cấp quyền cho người dùng thông qua câu lệnh: `/teacher username` hoặc `/teacher username 0` để gỡ quyền giảng viên.

Lưu ý:
- `username` là Telegram username.
- `0`, `off`, `false`, `f`, `no` được xem là `false`; ngược lại là `true`.

**Lưu ý quan trọng**: 
- Việc sử dụng Telegram Username thuận tiện và dễ tra cứu hơn Telegram Chat Id. Tuy nhiên, Telegram cho phép người dùng __tự đổi username__. Bạn cần tự quản lý rủi ro này nếu một trong những super user "rơi" vào tay người khác.

### Nguồn điểm

- Chỉ user quyền _Teacher_ mới được quyền set nguồn điểm
- Chỉ author của course/admin mới có thể update link của course/xoá course 
- Thiết lập nguồn điểm bằng câu lệnh `/load course_id link`. Ví dụ

```
/load cnpm-231 https://docs.google.com/spreadsheets/d/e/2PACX-1vQzIpK-OjiH1E9yTGLDnqrHEwejEVXYhzmfQGKrdQKkyYHbzJjffcpYN874BcQUsVBvFzkiLEWhaqBd/pub?gid=839733297&single=true&output=csv
```

Sẽ thiết lập bảng điểm link [https://docs.google.com/spreadsheets/d/e/2PACX-1vRvmyeejvTNR7kyDbGT8dJGAOWD-wbe114dUkSgpE1bznX0RPK0XUtofmr3Zjqk9SnWaOZpSp7r01rB/pub?output=csv](https://docs.google.com/spreadsheets/d/e/2PACX-1vQzIpK-OjiH1E9yTGLDnqrHEwejEVXYhzmfQGKrdQKkyYHbzJjffcpYN874BcQUsVBvFzkiLEWhaqBd/pub?gid=839733297&single=true&output=csv) cho môn _cnpm-231_

Lưu ý:

- Hiện tại, chỉ chấp nhận file CSV

Để có thể publish từ Google Spreadsheet, Bạn có thể sử dụng tính năng _File_ -> _Share_ -> _Publish to web_. Trong menu _Web page_, chọn _.csv_; Chọn Publish Sheet tương ứng.

![image](https://github.com/ldt116/cse-mark/assets/2363783/bedbf18b-65be-4761-83ed-8488e950f155)
![image](https://github.com/ldt116/cse-mark/assets/2363783/5c719d81-120e-4ff6-bc8b-93dac74999ff)


#### Parse CSV 

Hệ thống sẽ parse file CSV như sau

- Dòng 1: Cấu hình
  - Cột chứa ID của SV, được đánh dấu là `id`
  - Cột muốn public cho SV, đánh dấu bằng một chuỗi bất kỳ; ví dụ 'x'
  - Cột muốn che giấu, để trống (empty)
- Dòng 2: Headers
  - Các cột này chứa tên cột điểm trong kết quả trả về cho SV
- Dòng 3..n: Danh sách sinh viên và điểm
  

### Cấu hình

- Bảng điểm tự động cập nhật theo nguồn mỗi 10p, trong khoảng thời gian 6-tháng tính từ lần cuối cùng Giảng Viên thêm link vào hệ thống (bằng câu lệnh `/load`)
