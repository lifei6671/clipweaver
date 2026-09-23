# TASK-009：Docker 交付与运行配置

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-006、TASK-008
- 可并行：否
- 目标：把完整前后端、测试构建链和 FFmpeg 运行环境封装为验收机只需 Docker/Docker Compose 即可使用的单容器应用。

## 范围

1. 完成多阶段 Dockerfile：Node/pnpm builder、Go builder、runtime。
2. Node builder 通过 Corepack 使用锁定 pnpm，实际运行前端测试与 build。
3. Go builder 实际运行 `go test ./...` 后再构建 server binary。
4. runtime 安装并固定可复现的 FFmpeg/FFprobe 来源，同时包含 E2E 所需 `sh`、`curl`；正式 fixture/E2E scripts 由依赖本任务的 TASK-010 实现并复制进 runtime。
5. Go binary 同时提供 API 与前端静态资源。
6. Docker Compose 仅包含一个业务服务 `app`。
7. 持久化 data volume，并确认容器运行用户具有读写权限。
8. 配置 healthcheck。
9. 支持环境变量覆盖监听端口、数据目录、上传上限、日志级别、`MIX_TIMEOUT` 和最大并发数。
10. 完善 `.dockerignore`，排除 data、node_modules、临时产物等。
11. 容器重启后已上传资产和已生成 mix 仍可访问；启动时孤儿 tmp 可被清理。

## 交付物

- `Dockerfile`
- `docker-compose.yml`
- `.dockerignore`
- 供 TASK-010 纳入 scripts 的单容器 runtime
- 运行配置文档片段

## 约束

- 不要求宿主机安装 Go、Node、pnpm、FFmpeg、FFprobe、curl。
- 不引入 Nginx、数据库或额外业务服务。
- 镜像内不能包含开发机绝对路径和本地秘密。
- 前端 Docker 构建只使用 pnpm，不允许 npm/yarn 锁文件并存。

## 验收条件

- [x] `docker compose config` 通过。
- [x] `docker compose build --no-cache` 通过，日志明确包含 Go tests、frontend tests 与 frontend build PASS。
- [x] `docker compose up -d` 后 healthcheck 为 healthy。
- [x] 浏览器可打开前端。
- [x] 容器内 FFmpeg、FFprobe、curl、sh 可执行；正式 E2E scripts 在 TASK-010 实现和验收。
- [x] data volume 持久化有效：重启容器后资源仍存在。
- [x] 约定运行用户对 data volume 具备所需读写权限。
- [x] 启动时可以清理上次崩溃留下的 orphan tmp。
- [x] `docker compose down` 可正常停止并触发应用优雅退出。
- [x] 构建/运行过程不依赖宿主机 Go/Node/pnpm/FFmpeg/curl。

## 验收证据

- Commit：`053cb63e76b3cd0d259113a69a1857b337058731`；Dockerfile、Compose、`.dockerignore` 和本任务验收证据已提交，人工 Review 已通过，状态为 `PASS`。
- Compose config：`docker compose -p clipweaver-task009 config --quiet` 退出 0。覆盖环境变量 `HOST_PORT=19085`、`APP_PORT=19086`、`DATA_DIR=/app/data/override`、`LOG_LEVEL=DEBUG`、`MAX_UPLOAD_MB=64`、`MIX_TIMEOUT=2m`、`MAX_CONCURRENT_MIXES=3` 后，解析结果分别为 host `19085` → container `19086`、`APP_ADDR=:19086`、目标数据目录和所有配置值；healthcheck 指向 `127.0.0.1:19086`。
- No-cache build：`docker compose -p clipweaver-task009 build --no-cache --progress plain` 退出 0，生成 `clipweaver-task009-app:latest`；完整日志保存在本机 `%TEMP%/clipweaver-task009-build.log`。Builder 均只在 Docker 内运行，没有使用宿主 Go/Node/pnpm/FFmpeg/curl；`.dockerignore` 排除 `data/`、`material/`、已安装依赖与已有前端 dist。
- Builder tests：前端 Vitest `31 passed / 0 failed`，`tsc --noEmit && vite build` PASS，Go builder `go test ./...` 全部包 PASS，`CGO_ENABLED=0 go build` PASS。Go builder 现含 FFmpeg/FFprobe 7.1.1；另以 `docker build --target go-builder` 后执行 `go test ./internal/media -run '^TestExecutorRealFFmpeg$' -count=1 -v`，该用例 PASS、无 SKIP，视频/音频/format 均为 4.500000 秒。
- Health：独立 Compose project `clipweaver-task009` 用 `HOST_PORT=18085`、`APP_PORT=18086`、`DATA_DIR=/app/data/task009` 执行 `up -d --no-build`；`ps` 显示单一 app 容器 `healthy`，外部 `GET /api/health` 返回 200，FFmpeg 与 FFprobe 均报告 7.1.1。Edge 实际打开 `http://127.0.0.1:18085/`，页面显示标题、已持久化素材及上传/混剪控件。
- Runtime tools：容器内 `id` 为 uid/gid `10001(clipweaver)`；`command -v ffmpeg ffprobe curl sh` 均有结果。容器内 FFmpeg 生成 3.000000 秒视频和 1.500000 秒音频，容器内 curl 上传返回两个 ready asset；外部真实 HTTP `POST /api/mixes` 返回 200 / completed，`mixId=c6b37eaa-0697-4eac-825b-9f14748747e2`。
- Volume 权限/重启：非 root 进程对 `/app/data/task009` 的 `test -w` 通过，真实上传和 mix 写入成功。`docker compose restart app` 后 `GET /api/assets` 仍返回 2 个素材，mix 文件仍可访问，成片 SHA-256 前后均为 `36587916412a53af644b092ed49584a9beee370a575813b2c4d80d7b104659bc`。再执行 `docker compose stop -t 15 app`，容器退出码 0；`docker compose down` 后 named volume `clipweaver-task009_app-data` 仍存在，`up -d --no-build` 创建新容器后再次 healthy、2 个素材仍可查询、成片哈希仍一致。最终从运行中执行 `docker compose down -t 15` 退出 0，volume 保留。
- Orphan tmp：重启前由容器内非 root 用户在 `/app/data/task009/tmp/uploads/orphan-task009` 与 `tmp/mixes/orphan-task009` 写入不完整文件；重启后两目录均不存在，既有资产和 mix 未被删除。

## 运行配置片段（供 TASK-011 整合进最终 README）

- 默认 `docker compose up --build -d` 使用宿主与容器端口 8080、容器数据目录 `/app/data`，仅需宿主 Docker / Docker Compose；首次构建允许联网拉取基础镜像和 Go/pnpm 依赖。
- `HOST_PORT` 控制宿主映射端口，`APP_PORT` 同时控制容器监听端口和 healthcheck，二者默认 8080。可用不同 `HOST_PORT` 与已有开发服务并存。
- `DATA_DIR` 默认 `/app/data`；覆盖时应选择 `/app/data` 内的子目录，以沿用 `app-data` named volume 的持久化和非 root 写权限。`LOG_LEVEL`、`MAX_UPLOAD_MB`、`MIX_TIMEOUT`、`MAX_CONCURRENT_MIXES` 可按 Compose 中的默认值覆盖。
- TASK-010 将增加 `scripts/generate-fixtures.sh`、`scripts/e2e.sh` 并复制进 runtime；TASK-011 再按最终 README 做完整 clean-room 验收。本任务的手工合成媒体仅证明 volume/容器恢复能力，不充当 TASK-010 正式 fixture 交付。

阶段划分：原任务卡把尚未实现的 `scripts/generate-fixtures.sh` / `scripts/e2e.sh` 列为 TASK-009 的 PASS 前置条件，但这两份脚本明确属于 TASK-010，且 TASK-010 依赖 TASK-009。此处仅调整任务执行顺序；原始需求中的容器内脚本、Docker-only E2E 和最终交付标准保持不变，由 TASK-010 / TASK-011 验收。
