# ClipWeaver

本地视频混剪 Web 应用：上传视频素材与口播音频，按口播时长生成随机、不重叠的视频切片计划，并使用 FFmpeg 输出 1080×1920 竖屏 MP4。

当前阶段：TASK-001 工程骨架。页面、健康检查和 Docker 构建入口已建立；上传与混剪流程尚未实现。

## 启动与检查

验收机只需 Docker 和 Docker Compose。在仓库根目录运行：

```text
docker compose up --build -d
```

打开 `http://localhost:8080/` 查看占位页面。访问 `http://localhost:8080/api/health` 检查服务、FFmpeg 和 FFprobe；响应中的 `available` 与 `version` 来自运行容器内的实际命令。查看日志运行 `docker compose logs app`，停止运行 `docker compose down`。数据保存在 Compose 的 `app-data` volume 中。

Docker 构建阶段执行 `go test ./...`、`pnpm --dir web test` 和 `pnpm --dir web build`。本阶段的 Docker 运行验收状态以 [TASK-001 证据](docs/tasks/TASK-001.md) 为准。

可配置项通过 Compose 环境变量传入：`LOG_LEVEL`（默认 `INFO`）、`MAX_UPLOAD_MB`（默认 `512`）、`MIX_TIMEOUT`（默认 `10m`）、`MAX_CONCURRENT_MIXES`（默认 `1`）。`APP_ADDR`、`DATA_DIR` 和 `WEB_DIST_DIR` 在 Compose 中固定为容器内路径。当前仅建立配置入口；上传与混剪能力属于后续任务。

构建版本固定为 Go `1.25.14`、Node `22.23.2`、pnpm `10.17.1`、FFmpeg/FFprobe `7.1.1`；Go 与前端应用依赖分别见 `go.mod` / `go.sum` 和 `web/pnpm-lock.yaml`。

- [原始需求基线](docs/requirements-baseline.md)
- [技术设计 v0.1](docs/technical-design.md)
- [开发任务与状态](docs/tasks/README.md)
