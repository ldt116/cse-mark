# Issue Workflow

Repo này dùng labels theo 4 nhóm:

- `area:*` cho phần hệ thống bị ảnh hưởng
- `topic:*` cho luồng nghiệp vụ chính
- `bug` / `enhancement` / `task` / `wontfix` cho loại công việc
- `status:*` cho trạng thái xử lý

## Quy tắc gắn label

- Mỗi issue chỉ nên có `1` label `status:*` tại một thời điểm.
- Mỗi issue nên có `1` label loại công việc: `bug`, `enhancement`, `task`, hoặc `wontfix`.
- Mỗi issue nên có ít nhất `1` label `area:*`.
- Thêm `topic:*` khi issue liên quan đến luồng nghiệp vụ rõ ràng như bind, sync, auth, hoặc deploy.

## Ý nghĩa `status:*`

- `status:refinement`: Chưa đủ rõ yêu cầu, cần làm rõ scope, acceptance criteria, hoặc cách tiếp cận.
- `status:ready`: Đã rõ yêu cầu, đã triage xong, có thể đưa vào implementation.
- `status:implementing`: Đang có người làm.
- `status:in-review`: Đã có branch/PR và đang chờ review.
- `status:approved`: Review xong, có thể merge khi policy cho phép.
- `status:blocked`: Tạm dừng vì phụ thuộc bên ngoài, thiếu quyết định, hoặc gặp vấn đề truy cập/hệ thống.
- `status:done`: Công việc đã merge, đã đóng, hoặc đã kết thúc.

## Luồng đề xuất

Luồng chính:

`status:refinement` -> `status:ready` -> `status:implementing` -> `status:in-review` -> `status:approved` -> `status:done`

Nhanh hơn khi issue đã rõ ngay từ đầu:

`status:ready` -> `status:implementing` -> `status:in-review` -> `status:approved` -> `status:done`

Khi gặp chặn:

`status:implementing` -> `status:blocked` -> `status:implementing`

## Đóng issue không làm

Nếu issue được đóng mà không có ý định implement, gắn `wontfix`.

- Có thể giữ `status:refinement` nếu issue đóng sớm sau khi triage.
- Hoặc đổi sang `status:done` nếu muốn thể hiện đây là trạng thái kết thúc.

Điều quan trọng là lý do đóng issue phải rõ trong comment hoặc mô tả issue.
