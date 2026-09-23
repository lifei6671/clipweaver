# TASK-003：本地存储与 FFprobe 媒体探测

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-001（按任务索引中的“开发期 Docker 验收延期例外”允许先行开发；TASK-001 最终仍须补验并 PASS）
- 可并行：可与 TASK-002 并行
- 目标：建立安全的本地资源存储、确定的 stream 选择规则和微秒级媒体时长探测能力。

## 范围

1. 实现 `data/assets/<asset-id>/source.* + meta.json`、`data/tmp/uploads`、`data/tmp/mixes` 存储基础结构。
2. assetId/mixId 使用服务端生成 UUID；进入存储层前按 UUID 解析校验。
3. 原始文件名只作为展示元数据，不参与磁盘路径拼接。
4. manifest 采用临时文件 + rename 原子写入。
5. 实现 FFprobe 命令封装，使用 `exec.CommandContext`，不经过 shell。
6. 多路 stream 固定选择规则：同类型优先 `disposition.default=1` 的最低 index，否则选最低 index。
7. duration 固定优先级：`duration_ts + time_base` → `stream.duration` → `format.duration`。
8. 时间值按十进制/有理数精确解析，并 round-to-nearest 到 `DurationUS`，禁止依赖 float64 累计。
9. 视频至少验证选定 video stream、duration、width、height；音频至少验证选定 audio stream、duration。
10. 应用重启后可从 manifest 读取既有资源，并清理 `data/tmp/uploads` / `data/tmp/mixes` 中孤儿目录。
11. 定义 Probe 超时与稳定错误映射。

## 交付物

- `internal/media/ffprobe.go`
- `internal/storage/local.go`
- 对应单元测试与 FFprobe fixture

## 约束

- 不实现 HTTP 上传接口。
- 不根据扩展名或 MIME Type 判定媒体有效性。
- 不允许用户输入参与最终存储路径拼接。
- 后续 audio 上传允许输入容器存在非音频 stream；本任务只需能选定目标 audio stream。
- v0.1 启动时清理孤儿 tmp，不恢复中断渲染任务。

## 验收条件

- [x] 有效视频可解析选定 stream 的时长、宽高。
- [x] 有效音频可解析选定 audio stream 的真实时长。
- [x] 多路 stream fixture 能验证 default/index 选择规则。
- [x] `duration_ts + time_base`、`stream.duration`、`format.duration` 三层 fallback 均有测试。
- [x] 微秒转换有边界测试并证明使用 round-to-nearest。
- [x] 缺少所需媒体流、duration 非法、FFprobe 失败均返回明确错误。
- [x] manifest 原子写入并可重新读取。
- [x] 模拟应用重启后仍能读取既有资产，并清理孤儿 tmp。
- [x] 危险原始文件名和非法 UUID 不能越出数据目录。
- [x] `go test ./internal/media/... ./internal/storage/... -count=1` 通过。

## 验收证据

- 修改文件：`internal/media/ffprobe.go`、`ffprobe_test.go`、`testdata/*.json`；`internal/storage/local.go`、`local_test.go`；本任务状态和本节证据。
- Commit：484c366d207e429af0404e003322b03514d82a9a（实现提交；2026-09-23 已完成人工 Review 并确认通过）。
- 测试命令：`gofmt -w internal/media/ffprobe.go internal/media/ffprobe_test.go internal/storage/local.go internal/storage/local_test.go`；`go test ./internal/media/... ./internal/storage/... -count=1`；`go test ./internal/domain/... ./internal/media/... ./internal/storage/... -count=1`；`go test ./... -count=1`。GOCACHE 指向临时可写目录。
- 测试结果：gofmt 完成；两条定向 go test 均 PASS；`go test ./... -count=1` exit 1，仅既有 `internal/server.TestStaticPageAndSPAFallback` 在 Windows `TempDir` 清理 `index.html` 时因文件被占用失败，未改动该模块。
- FFprobe fixture/关键输出：`video_default.json` 选 default video index=3、3003000us、1920x1080；`video_lowest.json` 无 default 选 index=2；`audio_mixed.json` 忽略 video/subtitle，选 default audio index=4；`missing_video.json`、`missing_audio.json`、`invalid_duration.json`、`invalid_json.json` 验证稳定错误。命令 runner 测试覆盖参数、超时、取消及进程失败；宿主真实 FFprobe 未运行。
- duration fallback/rounding：`video_default.json` 和 `audio_mixed.json` 验证 duration_ts/time_base 优先；`stream_fallback.json`、`format_fallback.json` 验证逐层回退。`math/big.Rat` 精确计算，0.4/0.5/0.6us 分别舍入为 0/1/1；正好半微秒向上，溢出及舍入为 0 的候选值继续回退。
- tmp 恢复清理：重建 `NewLocal(root)` 后 manifest 可读，`tmp/uploads/*` 与 `tmp/mixes/*` 孤儿目录被清除，两个根目录保留。manifest 通过同目录临时文件、Sync/Close/Rename 更新，持久化内容无 `StoredPath`；读取时从 root + UUID 重建。
- SELF_REVIEW：修复 stream `default` 仅精确等于 1 的选择分支，以及亚微秒/溢出候选值阻断 fallback 的问题；定向测试重跑 PASS。UUID 和固定 `source.bin` 阻断外部路径拼接；已有 asset/source/manifest 符号链接由 `Lstat` 拒绝。Windows 沙箱无创建 symlink 权限，该专项测试 SKIP；本地数据目录需由应用独占管理，未验证并发恶意目录替换。
- Docker builder/Linux 验证：后置于 TASK-001 宿主人工验收，本任务未执行；不进入 TASK-004。
