# TASK-004：素材上传与查询 API

- 状态：以 [任务索引](README.md) 为准
- 依赖：TASK-003
- 可并行：完成后 TASK-007 可提前开始
- 目标：通过 HTTP 暴露视频、口播上传和资产查询能力，并以 staging + 原子 promotion 保证失败不留下半成品。

## 范围

1. `POST /api/assets/videos` 支持 multipart 多视频上传。
2. 每个文件先写入服务端生成的 `data/tmp/uploads/<upload-id>/`，再执行 FFprobe/校验；成功后原子 promotion 到正式资产目录，失败删除 staging。
3. 多视频上传允许部分成功：整个 multipart 请求成功解析时返回 HTTP 200，每个 `items[*]` 独立返回 `ready/failed`。
4. `POST /api/assets/audio` 一次创建一个 audio asset；服务端允许存在多个 audio asset，不维护全局默认口播，也不自动删除旧 audio。
5. `GET /api/assets` 返回全部可用资产元数据。
6. 建立统一 API error envelope 与基础 HTTP status 映射。
7. 上传 BodyLimit 可配置；请求级超限返回 413 `UPLOAD_TOO_LARGE`。
8. API 返回的时长全部来自 TASK-003 的 FFprobe/DurationUS 规则。

## 交付物

- Asset application service
- Fiber handlers/routes
- API error mapper
- Handler/API tests

## 约束

- 不实现混剪。
- 不把完整 FFmpeg/FFprobe stderr 直接返回前端。
- 不自研 multipart parser；使用 Fiber/fasthttp 的 multipart 能力 + 明确 BodyLimit + staging copy。
- 原始文件名不参与正式或临时目录定位。

## 验收条件

- [ ] 一次上传多个有效视频可返回各自 UUID、名称、durationUs 和 ready 状态。
- [ ] 同一批视频包含有效/无效文件时返回 HTTP 200，items 内同时存在 ready 与 failed，失败项包含稳定业务错误。
- [ ] 无效媒体不生成正式资产，staging 被清理。
- [ ] audio 接口拒绝无 audio stream 文件；带额外非音频 stream 但存在可选 audio stream 的容器按设计接受。
- [ ] 上传新的 audio asset 不删除已有 audio asset。
- [ ] 超过上传上限时返回 HTTP 413 + `UPLOAD_TOO_LARGE`。
- [ ] `GET /api/assets` 在应用重启后仍能读取既有资源。
- [ ] 原始文件名包含路径字符时不能造成目录穿越。
- [ ] API 测试包含多文件部分成功响应结构和 HTTP status 断言。

## 验收证据

- Commit：
- API 测试命令：
- 部分成功响应：
- 无效媒体/staging 清理：
- Audio 多资产语义：
- 重启后查询：
