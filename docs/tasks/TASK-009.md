# TASK-009：Docker 交付与运行配置

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-006、TASK-008
- 可并行：否
- 目标：把完整前后端、测试构建链和 FFmpeg 运行环境封装为验收机只需 Docker/Docker Compose 即可使用的单容器应用。

## 范围

1. 完成多阶段 Dockerfile：Node/pnpm builder、Go builder、runtime。
2. Node builder 通过 Corepack 使用锁定 pnpm，实际运行前端测试与 build。
3. Go builder 实际运行 `go test ./...` 后再构建 server binary。
4. runtime 安装并固定可复现的 FFmpeg/FFprobe 来源，同时包含 E2E 所需 `sh`、`curl` 与 scripts。
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
- 容器内 scripts
- 运行配置文档片段

## 约束

- 不要求宿主机安装 Go、Node、pnpm、FFmpeg、FFprobe、curl。
- 不引入 Nginx、数据库或额外业务服务。
- 镜像内不能包含开发机绝对路径和本地秘密。
- 前端 Docker 构建只使用 pnpm，不允许 npm/yarn 锁文件并存。

## 验收条件

- [ ] `docker compose config` 通过。
- [ ] `docker compose build --no-cache` 通过，日志明确包含 Go tests、frontend tests 与 frontend build PASS。
- [ ] `docker compose up -d` 后 healthcheck 为 healthy。
- [ ] 浏览器可打开前端。
- [ ] 容器内 FFmpeg、FFprobe、curl、sh 和 E2E scripts 可执行。
- [ ] data volume 持久化有效：重启容器后资源仍存在。
- [ ] 约定运行用户对 data volume 具备所需读写权限。
- [ ] 启动时可以清理上次崩溃留下的 orphan tmp。
- [ ] `docker compose down` 可正常停止并触发应用优雅退出。
- [ ] 构建/运行过程不依赖宿主机 Go/Node/pnpm/FFmpeg/curl。

## 验收证据

- Commit：
- Compose config：
- No-cache build：
- Builder tests：
- Health：
- Runtime tools：
- Volume 权限/重启：
- Orphan tmp：
