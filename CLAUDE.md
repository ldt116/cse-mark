# Project rules

## Không commit tài liệu dùng một lần

Không commit các tài liệu chỉ dùng một lần (superpowers plans, task briefs, implementer/review reports, review packages) vào git.

- Viết chúng vào scratch đã gitignore: `.superpowers/plans/…`, `.superpowers/sdd/…` (cả `.superpowers/` đã nằm trong `.gitignore`).
- Git chỉ chứa code + tài liệu bền: specs, SRS, kiến trúc, deployment/rollout checklist.
- PR chứa code + durable docs, không chứa plan/brief/report.

## Bảo mật log

Không log secrets (`ROSTER_CSV_URL`, tokens, `SMTP_*`, …) nguyên văn — chỉ log host.

## CI deps gate

CI chạy `go mod tidy` + `git diff --exit-code`; `go.sum` bị gitignore — chỉ `go.mod` được kiểm tra, đừng commit `go.sum`.
