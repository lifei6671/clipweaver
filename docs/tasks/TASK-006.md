# TASK-006：Mix 编排服务与 HTTP API

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-002、TASK-004、TASK-005（按任务索引中的开发期例外，TASK-005 已完成人工代码 Review，真实媒体验收后置，允许本任务先行开发）
- 可并行：否
- 目标：把资产读取、Planner、Executor、Validator 串成受超时和并发保护的同步混剪 API。

## 范围

1. 实现 Mix application service。
2. 校验 videoIds、audioId、UUID 格式及资产类型。
3. HTTP JSON 中 seed 固定为十进制字符串；未提供时服务端生成，提供时严格解析为 int64，响应再次字符串化。
4. 调用 Planner 生成 MixPlan，并把 seed/plan 持久化。
5. 调用 Executor 生成 output.mp4，成功响应只能发生在 Validator PASS 后。
6. 保存 mix meta 和最终状态。
7. `POST /api/mixes` 使用可配置 `MIX_TIMEOUT` + `context.WithTimeout`。
8. 服务端使用简单 in-flight semaphore；v0.1 默认最大并发为 1，超限返回 `MIX_BUSY`。
9. 应用优雅退出时取消在途渲染；客户端断线不承诺立即取消 FFmpeg。
10. 实现 `GET /api/mixes/:id/file`，支持浏览器 Range。
11. 实现 `GET /api/mixes/:id/download`。
12. 完整实现错误码到 HTTP status 的映射。

## 交付物

- Mix service
- Mix storage/meta
- Mix HTTP handlers/routes
- timeout/semaphore 生命周期管理
- API tests

## 约束

- v0.1 使用同步请求，不引入队列或后台 Worker。
- 一次请求失败后允许使用同一组资产重新制作。
- Fiber/fasthttp 请求 Context 不作为“客户端断线立即 cancel”的契约。
- `durationUs` 保持 JSON number；seed 使用 JSON string。

## 验收条件

- [x] 未选视频返回 HTTP 400 + `NO_VIDEO_SELECTED`。
- [x] 缺少口播返回 HTTP 400 + `AUDIO_REQUIRED`。
- [x] 非法 UUID 返回 HTTP 400 + `INVALID_ID`，不能进入路径拼接；合法但不存在的资产返回 HTTP 404 + `ASSET_NOT_FOUND`。
- [x] 素材不足返回 HTTP 422 + `INSUFFICIENT_VIDEO_DURATION`，信息包含缺少时长。
- [x] seed 大于 JavaScript 安全整数时仍能以字符串往返并准确恢复 int64。
- [x] seed 非法或越界字符串返回 HTTP 400 + `INVALID_SEED`。
- [x] 相同输入和同一 seed 字符串可得到语义相同的 plan。
- [x] 并发超过上限返回 HTTP 429 + `MIX_BUSY`。
- [x] 渲染超时取消 FFmpeg Context，并返回 HTTP 504 + `MIX_TIMEOUT`。
- [x] 成功响应包含 mixId、seed 字符串、durationUs、previewUrl、downloadUrl。
- [ ] preview 支持 Range 请求并能被浏览器 video 使用（Range HTTP 合同已通过；真实媒体浏览器播放待 TASK-005 集成验收）。
- [x] download 返回 attachment。
- [x] FFprobe/FFmpeg/Validator 内部失败统一映射 500，并不泄露完整 stderr。
- [x] API 集成测试通过。

## 验收证据

- Commit：3880770183f6654f19bef74cd37bc1ef1cd39b9b（实现提交；2026-09-23 已完成人工代码 Review 并允许继续开发；真实媒体浏览器播放仍后置，故任务状态暂保留 REVIEW）。
- 关键文件：`internal/service/mix.go`、`internal/httpapi/mixes.go`、`internal/httpapi/errors.go`、`internal/storage/local.go`、`internal/domain/mix.go`、`internal/server/app.go`、`cmd/server/main.go` 及相应测试。
- API 测试命令：`go test ./internal/service/... ./internal/httpapi/... ./internal/storage/... -count=1 -timeout=60s` PASS；`go test ./internal/domain/... ./internal/mixer/... ./internal/media/... ./internal/service/... ./internal/httpapi/... ./internal/storage/... -count=1 -timeout=60s` PASS；`go test -race ./internal/service/... ./internal/httpapi/... -count=1 -timeout=90s` PASS；相关包 `go vet` PASS。均使用工作区可写 `GOCACHE`。
- server 接线测试：`go test ./internal/server/... ./cmd/server/... -run 'TestLoadConfig|TestBodyLimitUsesPublicError|TestMixRoutesRegistered' -count=1 -timeout=60s` PASS。
- seed 大整数往返：`9007199254740993` 与 `MaxInt64` 请求/响应/磁盘 meta 精确一致；省略 seed 的注入值 `MinInt64` 精确返回；非法字符串、溢出和 JSON number 均为 400。
- 并发/超时/shutdown：阻塞 fake executor 时第二请求立即 429 且未进入执行器；释放后再次请求成功。超时 HTTP 504，fake 观察到 `ctx.Done()`；service `Close()` 取消在途渲染且不误报 timeout。
- 成功创建：fake executor 执行前确认 `meta.json` 为 rendering、`plan.json` 已完整持久化；执行后响应五项核心字段、meta 为 completed、临时目录清理；重建 Local 后仍可预览/下载。
- 素材不足：HTTP 422，`details.missingDurationUs=4700000`；Planner 不复用候选片段。
- Range：无 Range 为 200；`0-3`、`4-`、`-3` 为 206 且 body、Content-Range、Content-Length 正确；越界、多段和非法 Range 为 416；download 为 attachment。
- 失败清理：FFprobe/FFmpeg/Validator fake 失败返回稳定 500 且响应无 stderr/路径；meta failed、plan 保留、半成品 output 与 staging 清理，同组资产可重试。
- 未验证：无真实 FFmpeg/FFprobe 媒体输出及浏览器播放实测，本任务按 fake executor 验证编排和 Range HTTP 合同；TASK-005 真实媒体验收仍为 REVIEW。`go test ./... -count=1 -timeout=60s` FAIL，仅 `internal/server.TestStaticPageAndSPAFallback` 在 Windows 清理 TempDir 时因 `index.html` 被占用失败；其余包通过，未扩修该既有问题。Docker 验收仍按 TASK-001 例外后置。
