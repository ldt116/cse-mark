# Triển khai v2

> Docker Compose, image, secret, CI. Yêu cầu ở `SRS-v2.md`; config ở `config-env.md`.

## 1. Tổng quan

4 service chạy độc lập trong `docker-compose.yml` (`network_mode: host` như v1):

| Service | Build | Image (registry `public/`) |
|---|---|---|
| api | `Dockerfile_api` | `cse-mark-api` |
| lf (fetcher) | `Dockerfile_fetcher` | `cse-mark-fetcher` |
| tele | `Dockerfile_tele` | `cse-mark-tele` |
| **discord** (mới) | `Dockerfile_discord` | **`cse-mark-discord`** |

## 2. Thêm service `discord`

### 2.1 `Dockerfile_discord` (theo pattern `Dockerfile_tele`)

```dockerfile
# BUILD
FROM golang:1.24-alpine AS build
WORKDIR /app
COPY . .
RUN go mod tidy
WORKDIR /app/cmd/discord
RUN go build -o discordbot
# RUN IMAGE
FROM alpine
WORKDIR /app
COPY --from=build /app/cmd/discord/discordbot .
CMD ["./discordbot"]
```

### 2.2 Mục trong `docker-compose.yml`

```yaml
  discord:
    build:
      context: .
      dockerfile: Dockerfile_discord
    env_file:
      - .env
    environment:
      - TZ=Asia/Ho_Chi_Minh
    restart: always
    network_mode: host
```

`fetcher` và `tele` giữ nguyên (chỉ code nội tại thay đổi). `api` giữ nguyên.

## 3. CI — thêm image thứ 4

`.gitea/workflows/build-docker.yml` hiện build `api/fetcher/tele` qua matrix `service`. **Thêm `discord`** vào danh sách service:

```yaml
        service:
          - api
          - fetcher
          - tele
          - discord   # mới
```

→ sinh thêm image `git.thuanle.me/public/cse-mark-discord` (amd64+arm64 + manifest `:latest`).

## 4. Secret

- Registry: `DOCKER_USERNAME` / `DOCKER_PASSWORD` (Actions secret — đã có).
- Ứng dụng: `enc.env` mã hoá bằng **SOPS** (`config-env.md` §8). Khi deploy, decrypt thành `.env` (compose `env_file: .env`).
- Mới cần cấp: `DISCORD_TOKEN`, `DISCORD_GUILD_ID`, `SMTP_HOST/USERNAME/PASSWORD/FROM`, `ROSTER_CSV_URL`.

## 5. Discord bot — chuẩn bị

1. Tạo ứng dụng + bot tại Discord Developer Portal, lấy `DISCORD_TOKEN`.
2. Mời bot vào server (guild) với `DISCORD_GUILD_ID`; quyền: **Manage Roles, Manage Channels, Send Messages, Add Reactions** (cần manage role/channel để provisioning).
3. **Thứ tự role**: role của bot phải nằm **cao hơn** các class role để có quyền gán/gỡ.
4. Lưu `DISCORD_ADMIN_IDS` (user ID Admin).

## 6. SMTP — chuẩn bị

Cấu hình SMTP sender (host/port/user/pass/from). HCMUT email là Google Workspace nên gửi tới được qua SMTP thường; cần sender có uy tín để không vào spam. Test gửi OTP tới `@hcmut.edu.vn`.

## 7. Runner Gitea Actions

Giữ `amd64`, `arm64`, `linux`. Không thay đổi.

## 8. Lệnh triển khai

```bash
# decrypt secret (SOPS) -> .env
sops -d enc.env > .env

# build & up
docker compose build
docker compose up -d

# kiểm tra
docker compose ps
curl -fsS http://localhost:8080/healthz
```

Discord bot online + Telegram bot đáp lệnh `/bind` → triển khai thành công.
