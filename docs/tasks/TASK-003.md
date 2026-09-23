# TASK-003：本地存储与 FFprobe 媒体探测

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-001
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

- [ ] 有效视频可解析选定 stream 的时长、宽高。
- [ ] 有效音频可解析选定 audio stream 的真实时长。
- [ ] 多路 stream fixture 能验证 default/index 选择规则。
- [ ] `duration_ts + time_base`、`stream.duration`、`format.duration` 三层 fallback 均有测试。
- [ ] 微秒转换有边界测试并证明使用 round-to-nearest。
- [ ] 缺少所需媒体流、duration 非法、FFprobe 失败均返回明确错误。
- [ ] manifest 原子写入并可重新读取。
- [ ] 模拟应用重启后仍能读取既有资产，并清理孤儿 tmp。
- [ ] 危险原始文件名和非法 UUID 不能越出数据目录。
- [ ] `go test ./internal/media/... ./internal/storage/... -count=1` 通过。

## 验收证据

- Commit：
- 测试命令：
- 测试结果：
- FFprobe fixture/关键输出：
- duration fallback/rounding：
- tmp 恢复清理：
