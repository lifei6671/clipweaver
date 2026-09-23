# TASK-006：Mix 编排服务与 HTTP API

- 状态：以 [任务索引](README.md) 为准
- 依赖：TASK-002、TASK-004、TASK-005
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

- [ ] 未选视频返回 HTTP 400 + `NO_VIDEO_SELECTED`。
- [ ] 缺少口播返回 HTTP 400 + `AUDIO_REQUIRED`。
- [ ] 非法 UUID 返回 HTTP 400 + `INVALID_ID`，不能进入路径拼接；合法但不存在的资产返回 HTTP 404 + `ASSET_NOT_FOUND`。
- [ ] 素材不足返回 HTTP 422 + `INSUFFICIENT_VIDEO_DURATION`，信息包含缺少时长。
- [ ] seed 大于 JavaScript 安全整数时仍能以字符串往返并准确恢复 int64。
- [ ] seed 非法或越界字符串返回 HTTP 400 + `INVALID_SEED`。
- [ ] 相同输入和同一 seed 字符串可得到语义相同的 plan。
- [ ] 并发超过上限返回 HTTP 429 + `MIX_BUSY`。
- [ ] 渲染超时取消 FFmpeg Context，并返回 HTTP 504 + `MIX_TIMEOUT`。
- [ ] 成功响应包含 mixId、seed 字符串、durationUs、previewUrl、downloadUrl。
- [ ] preview 支持 Range 请求并能被浏览器 video 使用。
- [ ] download 返回 attachment。
- [ ] FFprobe/FFmpeg/Validator 内部失败统一映射 500，并不泄露完整 stderr。
- [ ] API 集成测试通过。

## 验收证据

- Commit：
- API 测试命令：
- seed 大整数往返：
- 并发/超时：
- 成功创建：
- 素材不足：
- Range：
