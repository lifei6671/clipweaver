# TASK-004：素材上传与查询 API

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
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

- [x] 一次上传多个有效视频可返回各自 UUID、名称、durationUs 和 ready 状态。
- [x] 同一批视频包含有效/无效文件时返回 HTTP 200，items 内同时存在 ready 与 failed，失败项包含稳定业务错误。
- [x] 无效媒体不生成正式资产，staging 被清理。
- [x] audio 接口拒绝无 audio stream 文件；带额外非音频 stream 但存在可选 audio stream 的容器按设计接受。
- [x] 上传新的 audio asset 不删除已有 audio asset。
- [x] 超过上传上限时返回 HTTP 413 + `UPLOAD_TOO_LARGE`。
- [x] `GET /api/assets` 在应用重启后仍能读取既有资源。
- [x] 原始文件名包含路径字符时不能造成目录穿越。
- [x] API 测试包含多文件部分成功响应结构和 HTTP status 断言。

## 验收证据

- 修改文件：internal/service/asset.go 及测试、internal/httpapi/{handlers,routes,errors}.go 及测试、internal/storage/local.go 及测试、internal/server/app.go 及测试、本任务卡和任务索引。
- Commit：14181956a6958ffdbdbf578d8f4e9e2ad501007c（实现提交；2026-09-23 已完成人工 Review 并确认通过）。
- API 测试命令：设置 GOCACHE=$env:TEMP\clipweaver-go-cache 后，go test ./internal/service/... ./internal/httpapi/... -count=1 PASS；go test ./internal/storage/... ./internal/media/... ./internal/service/... ./internal/httpapi/... -count=1 PASS；go test ./internal/server/... -run '^TestBodyLimitUsesPublicError$' -count=1 PASS；go vet ./internal/storage/... ./internal/media/... ./internal/service/... ./internal/httpapi/... ./internal/server/... PASS。默认 Go 缓存目录在此沙箱中 Access is denied，故使用可写临时缓存。
- 全量测试：go test ./internal/server/... -count=1 和 go test ./... -count=1 均 FAIL，唯一失败为已有 TestStaticPageAndSPAFallback 在 Windows t.TempDir 清理时 index.html 被占用；新增 BodyLimit 真实 HTTP 测试单独 PASS。未将全量测试记为通过。
- 部分成功响应：TestVideoPartialFailureAndUnsafeNames 断言 HTTP 200、items 中 ready + failed、失败 code=INVALID_VIDEO；TestVideoBatchAndDiskReload 断言两个不同 UUID、名称、durationUs 和最终 source.bin/meta.json。
- 无效媒体/staging 清理：TestInvalidMediaAndAudioHistory 覆盖缺视频流、无效尺寸、缺音频流；TestUploadFailureCleansStagingAndFinal 覆盖 copy、probe、promotion、SaveAsset 失败，断言 staging 与正式 assets 均无残留；TestListAssetsOnlyCompleteAndRejectsCorruption 证明未写 manifest 不可查询、损坏 manifest 报错。
- Audio 多资产语义：连续上传两个 audio 后 GET 返回两个 audio；fake prober 模拟包含其他 stream 但可选有效 audio 的容器，DurationUS 取探测结果。
- 重启后查询：TestVideoBatchAndDiskReload 重建 Local store/service 后仍读取两个资产，并断言按 ID 稳定排序。
- BodyLimit：TestBodyLimitUsesPublicError 通过真实 loopback HTTP 超限请求断言 413 + UPLOAD_TOO_LARGE；也断言过大 MaxUploadMB 不被接受。
- SELF_REVIEW：检查 multipart 文件关闭、失败清理、同卷目录 rename、部分成功语义、路径与底层错误不外泄、无包级可变状态。修复无 manifest 的非 UUID 孤儿目录导致 GET 错误、损坏 manifest ID 被误映射为客户端 INVALID_ID 的问题；最终无已知阻塞发现。
- 未验证：真实 FFprobe/媒体样本及 Docker builder 留待后续 Docker 验收；TASK-001 继续 BLOCKED，按开发期例外推进。TASK-004 保持 REVIEW，等待人工复核与 commit 回填。
