# TASK-001：工程骨架与运行基线

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
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

- [x] Docker builder 日志证明 `go test ./...` 实际执行并通过。
- [x] Docker builder 日志证明前端测试实际执行并通过。
- [x] Docker builder 日志证明 `pnpm --dir web build` 实际执行并通过。
- [x] `docker compose config` 通过。
- [x] `docker compose build --no-cache` 通过。
- [x] `docker compose up -d` 后 `GET /api/health` 返回 200。
- [x] health 结果证明 FFmpeg 与 FFprobe 在运行容器中可执行。
- [x] 浏览器访问根路径能够加载 React 页面。
- [x] Dockerfile/lockfile 能明确确认 Go、Node、pnpm 和 FFmpeg/FFprobe 的版本来源。
- [x] 仓库没有生成物、运行期 data、密钥或本机绝对路径。

## 验收证据

- 开发顺序决策（2026-09-23）：人工确认先继续业务代码开发，Docker 调试统一后置；本任务继续保持 `BLOCKED`，未通过项不得视为通过。按任务索引中的开发期例外，TASK-002～TASK-008 可先推进；TASK-009 开始前必须补齐本任务全部 Docker 验收并标记 `PASS`。

- 修改文件：go.mod、go.sum、cmd/server/main.go、internal/server/{app,config}.go 及测试、web/ 的 React/TypeScript/Vite/Ant Design 源码与 pnpm-lock.yaml、Dockerfile、docker-compose.yml、.dockerignore、.gitignore、README.md、docs/tasks/README.md、本任务卡。
- Commit：624fcd705c3c74a3118e87134609fd8acc7790e8（实现与 TASK-002 同批基线提交；本任务状态仍为 BLOCKED，Docker 验收尚未完成）。
- 依赖锁文件生成：go mod tidy 退出码 0；corepack pnpm install --lockfile-only --ignore-scripts 退出码 0，使用 pnpm 10.17.1。两项仅用于生成锁文件，不计为 Linux 测试。
- Compose 配置：docker compose config 退出码 0；CLI 同时提示宿主 Docker config.json 无访问权限。
- Docker build：docker compose build --no-cache 退出码 1；拉取镜像前报 error listing credentials - A specified logon session does not exist。此前 docker version --format '{{.Server.Version}}' 退出码 1，npipe:////./pipe/docker_engine 不存在；常见路径未找到 Docker Desktop 可执行文件。
- Go tests（builder）：NOT_RUN；Docker build 未进入 builder。
- Frontend tests/build（builder）：NOT_RUN；Docker build 未进入 builder。
- Compose up / Health / 根页面：NOT_RUN；Docker runner 不可用，没有启动容器。
- 版本锁定：golang:1.25.14-bookworm、node:22.23.2-bookworm-slim、Corepack pnpm@10.17.1、mwader/static-ffmpeg:7.1.1、debian:12.12-slim；应用依赖见 Go 和 pnpm 锁文件。以上是源码配置，尚未通过 Docker 构建验证。
- 阻塞原因与影响：当前环境无法完成 TASK-001 的 Docker builder 测试、构建、启动、运行容器 health 与浏览器验收，故不能进入 REVIEW。恢复条件：可用 Docker daemon/runner、Docker Compose 与正常镜像凭据；随后重新执行 docker compose config、docker compose build --no-cache、docker compose up -d、health 与根页面验证，最后 docker compose down。
- 本次续验（2026-09-23）：docker version 退出码 1；CLI 提示无法读取 Docker config.json（Access is denied），且 npipe:////./pipe/docker_engine 不存在，未获得 Server 信息。Docker daemon 和镜像凭据未恢复；本次未运行 docker compose config、build、up、health、根页面或 down，前次 Compose config 通过的记录不视为本次运行验收。
- 本次 runner 发现（2026-09-23）：docker context ls 退出码 1，.docker/contexts/meta 拒绝访问；docker context show 退出码 0，显示 default；无 DOCKER* 环境变量。Get-Process 发现 Docker Desktop、com.docker.backend、com.docker.build、com.docker.sailor、docker-agent；Get-Service 未发现 Docker 服务（仅 Microsoft/NVIDIA 的无关 container 服务）。Docker 命名管道存在，但临时空 DOCKER_CONFIG 下的 docker context ls 仅列出 default，docker version 连接 docker_engine 为 permission denied；显式连接 dockerDesktopLinuxEngine、docker_engine_linux、dockerDesktopEngine 的 docker version 均为 permission denied（退出码 1）。对 .docker/config.json 的 Test-Path、Get-Item、Get-Acl、icacls 均被拒绝访问，无法确认文件存在性或读取 ACL；.docker/contexts/meta 的 icacls 亦拒绝访问。没有可用 Server/context，因此本次未运行 Compose 构建和运行验收，状态继续 BLOCKED。
- 本次恢复验收（2026-09-23）：用户在正常 Windows PowerShell 报告 desktop-linux 的 Docker Desktop 4.91.0 / Engine 29.8.0（linux/amd64）可用；Agent 执行上下文中 docker context show 退出码 0 但只显示 default，且 Docker config.json 拒绝访问。docker --context desktop-linux version 退出码 1，因 desktop-linux/meta.json Access is denied 无法解析 endpoint；docker --context desktop-linux compose version 退出码 0，显示 Compose v5.5.1，但仅证明插件存在。进程级 docker --host npipe:////./pipe/dockerDesktopLinuxEngine version 退出码 1，连接 API 为 permission denied。阻塞定位为 Agent 执行上下文对 context 元数据和 Docker 管道的权限限制；未在 Agent 会话执行 Compose config/build/up、builder 测试、health、根页面或 down，不能引用用户宿主结果充当验收通过。状态继续 BLOCKED，Commit pending。
- 执行身份根因补充（2026-09-23）：Agent PowerShell 的 whoami 为 lifeilin\codexsandboxonline（CodexSandboxUsers），Medium 完整性，SessionId 1；父链为 pwsh → codex-command-runner → codex。环境变量 USERNAME/USERPROFILE 仍指向 lifei，但不代表实际 token 用户；普通用户文件 .gitconfig 可读取，.docker 元数据与 Docker named pipe 被拒。Docker Desktop/后端亦在 SessionId 1，其 Owner 因访问限制未能读取。证据指向独立沙箱账号的访问边界；未发现低完整性或 AppContainer 迹象，无法从当前进程命令行/环境变量确认具体沙箱启动开关。

### ISSUE-001 修复与 TASK-001 Docker 复验（2026-09-23）

- ISSUE-001：**技术阻塞已关闭，人工 Review 已通过**。仅修改 `internal/server/app_test.go`：构造实际超过 1 MiB 限额的 multipart 请求，将完整 HTTP 请求预装进内存连接，调用实际 fasthttp `ServeConn`，读取其已写出的响应并断言 HTTP 413 与 `error.code=UPLOAD_TOO_LARGE`；随后同一 app 仍可响应 `/api/missing`。没有改动生产 BodyLimit、错误映射或上传上限。
- 根因：fasthttp 在 `Content-Length > BodyLimit` 时先拒绝、写 413 并关闭连接；原测试的 Go HTTP 客户端仍在写 body，`Do` 可能先返回 `write: connection reset by peer`。Fiber `app.Test` 同样先返回 `ServeConn` 的 body-limit 错误，不读取已写出的 413，因此本测试用预装完整请求的内存连接读取真实公共响应。另用运行镜像的实际 HTTP multipart 请求验证网络端契约。
- 定向测试：宿主 `go test ./internal/server -run '^TestBodyLimitUsesPublicError$' -count=20` 退出码 0，20 次 PASS；Linux `golang:1.25.14-bookworm` 容器执行同一命令，退出码 0，20 次 PASS。
- 宿主全量测试：`go test ./... -count=1` 退出码 1；本次 ISSUE-001 用例通过，唯一失败为既有 `TestStaticPageAndSPAFallback` 在 Windows 清理 `TempDir/index.html` 时报告文件占用。该用例不在 ISSUE-001 范围内，未修改。Docker/Linux builder 的完整 Go 测试通过。
- 正式构建：`docker compose -p clipweaver-issue001 build --no-cache --progress plain` 退出码 0，最终代码复建日志显示前端 31/31 tests PASS、`tsc --noEmit && vite build` PASS、`RUN go test ./...` PASS（含 `internal/server`）、`CGO_ENABLED=0 go build` PASS、runtime image `clipweaver-issue001-app` 构建完成。
- Compose 运行：`docker compose -p clipweaver-issue001 config --quiet` 退出码 0；`up -d` 退出码 0，最终镜像 `up -d --force-recreate` 退出码 0；`ps` 显示 app `Up (healthy)`。容器内 `GET http://127.0.0.1:8080/api/health` 返回 200、`status=ok`、FFmpeg 和 FFprobe 均为 7.1.1。
- 根页面：Compose 容器内 `GET /` 返回 200 和 React 入口 HTML。宿主 8080 同时被其他进程占用，因此使用同一运行镜像的独立容器映射 18080 浏览器访问；页面实际显示视频上传、口播选择和混剪控件，容器日志记录 `/`、JS、CSS、`/api/assets` 均为 200；不将宿主 8080 浏览器结果算作 Compose 证据。
- 实际超限 HTTP：同一正式运行镜像的独立限额容器设置 `MAX_UPLOAD_MB=1`，用容器内 `dd` 生成 2 MiB 文件、`curl -F file=@/tmp/oversize.bin` 上传 `/api/assets/audio`，收到 `HTTP/1.1 413 Request Entity Too Large` 和 `error.code=UPLOAD_TOO_LARGE`；随后 `GET /api/health` 为 200、`status=ok`。
- 版本与交付文件：Dockerfile 锁定 Go 1.25.14、Node 22.23.2、pnpm 10.17.1、FFmpeg/FFprobe 7.1.1；`go.mod`、`go.sum`、`web/pnpm-lock.yaml`、Dockerfile、Compose、`.dockerignore` 均已跟踪。`git ls-files` 检查未见构建生成目录或运行期 data；本次检查的交付配置未见本机绝对路径或密钥。用户未跟踪的 `material/` 和最终验收报告未处理。
- 最终收口：TASK-001 自身验收条件均已形成运行证据；ISSUE-001 实现及修复提交为 `62abe001a307f69355595f2d1ec40f9e508adbaa`，人工 Review 已通过。TASK-001 由 `REVIEW` 转为 `PASS`。以上历史 BLOCKED、Docker 权限问题和修复前失败记录保持原样，作为实际开发过程证据。
