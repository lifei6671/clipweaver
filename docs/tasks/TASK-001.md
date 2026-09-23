# TASK-001：工程骨架与运行基线

- 状态：以 [任务索引](README.md) 为准
- 依赖：无
- 可并行：否
- 目标：建立后续所有任务共享的 Go、React、测试和 Docker-only 验收基线。

## 范围

1. 初始化 Go module，建立 `cmd/server` 与 `internal` 基础目录。
2. 使用 Fiber 提供最小 HTTP 服务和 `GET /api/health`。
3. 初始化 `web/`：React + TypeScript + Vite + Ant Design。
4. 前端包管理器统一为 pnpm，通过 Corepack 使用锁定版本并提交 `pnpm-lock.yaml`。
5. Go 服务能够在生产构建中托管前端静态资源和 SPA fallback。
6. 建立前后端最小测试框架。
7. 建立基础配置入口：监听地址、数据目录、日志级别、上传上限、混剪超时、最大并发等。
8. 建立基础日志和 requestId 中间件。
9. 提供多阶段 Docker 构建骨架，锁定 Go、Node、pnpm 和 FFmpeg/FFprobe 来源。
10. Docker builder 阶段实际执行 Go 测试、前端测试和前端 build；运行镜像可执行 FFmpeg/FFprobe 并提供 health。

## 交付物

- `go.mod` / `go.sum`
- `cmd/server/main.go`
- 后端基础包结构
- `web/package.json`、`pnpm-lock.yaml`
- React 首屏占位页面
- 基础 `Dockerfile` / `docker-compose.yml`
- 最小单元测试与构建入口

## 约束

- 不实现素材上传、Planner、混剪和业务 UI。
- 不引入数据库、Redis、任务队列。
- 前后端生产环境同源，不额外引入 Nginx。
- Go、Node、pnpm、FFmpeg/FFprobe 与应用依赖必须锁版本。
- 最终验收机只假设安装 Docker 与 Docker Compose，不依赖宿主 Go、Node、pnpm、FFmpeg、FFprobe 或 curl。

## 验收条件

- [ ] Docker builder 日志证明 `go test ./...` 实际执行并通过。
- [ ] Docker builder 日志证明前端测试实际执行并通过。
- [ ] Docker builder 日志证明 `pnpm --dir web build` 实际执行并通过。
- [ ] `docker compose config` 通过。
- [ ] `docker compose build --no-cache` 通过。
- [ ] `docker compose up -d` 后 `GET /api/health` 返回 200。
- [ ] health 结果证明 FFmpeg 与 FFprobe 在运行容器中可执行。
- [ ] 浏览器访问根路径能够加载 React 页面。
- [ ] Dockerfile/lockfile 能明确确认 Go、Node、pnpm 和 FFmpeg/FFprobe 的版本来源。
- [ ] 仓库没有生成物、运行期 data、密钥或本机绝对路径。

## 验收证据

- Commit：
- Docker build：
- Go tests（builder）：
- Frontend tests/build（builder）：
- Health：
- 版本锁定：
- 备注：
