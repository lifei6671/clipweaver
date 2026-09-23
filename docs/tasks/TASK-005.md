# TASK-005：FFmpeg 渲染执行器与输出验收

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-002、TASK-003
- 可并行：否
- 目标：把确定的 MixPlan 帧精确渲染为满足完整媒体合同的 1080×1920 MP4，并用 FFprobe 证明满足 100ms 时长要求。

## 范围

1. PlannedClip 的半开区间 `[start,end)` 映射为解码后的 `trim=start=...:end=...`，紧接 `setpts=PTS-STARTPTS`。
2. 禁止使用依赖关键帧的 stream-copy 快速裁剪实现 PlannedClip。
3. 保持 FFmpeg CLI 默认 autorotate，不使用 `-noautorotate`；旋转后的帧再进入 scale/pad。
4. 每片标准化为 1080×1920、30fps、H.264、yuv420p、无音轨，保持原比例并用黑边填充。
5. 按 MixPlan 顺序 concat 为 silent video。
6. mux 阶段只 map silent video 与选定 narration audio，口播先 `asetpts=PTS-STARTPTS`。
7. 视频允许 `tpad=stop=-1:stop_mode=clone` 延长末帧，最终使用显式 `-t <TargetDuration>` 控制硬时长上界，不把 `-shortest` 作为核心对齐机制。
8. 音频编码 AAC，最终 MP4 固定 `-movflags +faststart`。
9. 输出后执行完整 FFprobe Validator。
10. 渲染失败清理临时目录，并保存可诊断日志上下文。

## 交付物

- `internal/media/ffmpeg.go`
- `internal/media/executor.go`
- `internal/media/validator.go`
- FFmpeg 参数构建测试
- 真实 FFmpeg/FFprobe 集成测试

## 约束

- Executor 只能执行 MixPlan，不修改 Planner 的随机和切片决策。
- FFmpeg 使用 `exec.CommandContext`，不通过 shell 拼命令。
- v0.1 优先可诊断性，可以逐片标准化。
- 运行时临时文件必须位于服务端生成的 `data/tmp/mixes/<mix-id>`。
- TargetDurationUS 唯一来源是口播资产上传时冻结的 DurationUS。

## 验收条件

- [x] 精确裁剪参数测试包含 `trim=start:end` 与 `setpts=PTS-STARTPTS`，不存在 stream-copy 切片。
- [ ] 横屏、普通竖屏、rotation metadata 样本均保持正确显示方向和比例，输出 1080×1920。
- [ ] 黑边填充正确，画面不拉伸。
- [ ] 最终容器为 MP4，并使用 `+faststart`。
- [ ] video codec = H.264、pix_fmt = yuv420p、帧率约 30fps。
- [ ] audio codec = AAC，最终只有预期的单一 audio stream。
- [x] mux 参数测试证明只映射 silent video + narration audio，不映射素材原声。
- [ ] `abs(Tvideo-Ttarget) <= 100ms`。
- [ ] `abs(Taudio-Ttarget) <= 100ms`。
- [ ] `abs(Tformat-Ttarget) <= 100ms`。
- [ ] `abs(Tvideo-Taudio) <= 100ms`。
- [ ] 口播末尾人工抽样无明显缺失。
- [x] FFmpeg 失败返回 `FFMPEG_FAILED`；媒体合同不通过返回 `RENDER_VALIDATION_FAILED`。
- [ ] 真实 FFmpeg/FFprobe 集成测试通过。

## 验收证据

- Commit：400d70ecbf4138eb1e21c34136aa840f0cabea8c（实现提交；2026-09-23 已完成人工代码 Review 并允许继续开发；真实媒体/rotation/100ms 实测与人工试听仍后置，故任务状态暂保留 REVIEW）。
- 宿主环境（2026-09-23）：`Get-Command ffmpeg` 与 `Get-Command ffprobe` 均无结果；版本不可获取。未安装软件。
- 集成测试命令：`go test ./internal/media/... -run '^TestExecutorRealFFmpeg$' -count=1 -v` → `SKIP: ffmpeg unavailable`。真实媒体验收未运行。
- 参数/fixture 测试：`go test ./internal/media/... -count=1` PASS；`go test ./internal/domain/... ./internal/media/... -count=1` PASS；`go vet ./internal/media/...` PASS（使用可写的临时 GOCACHE）。覆盖整数微秒裁剪、默认 autorotate、标准化、拼接顺序、显式映射、失败清理、取消，以及 JSON 媒体合同与 100ms 边界。
- 全量测试：`go test ./... -count=1` FAIL：`internal/server/TestStaticPageAndSPAFallback` 在 Windows 清理 `index.html` 时文件仍被占用；其他包通过。此项未归因于 TASK-005 媒体代码。
- FFprobe 输出（format/codec/pix_fmt/fps/width/height/durations）：只有 `testdata/render_valid.json` 合成 fixture（mov,mp4 / h264 / yuv420p / 30/1 / 1080×1920 / video=audio=format=4.5s）；真实输出待后置。
- rotation fixture：未生成、未运行；宿主缺少 FFmpeg/FFprobe，待真实媒体验收。
- 口播尾部人工抽样：未运行，须由人工试听；保持未通过。
- staging 所有权：调用方使用 `Local.NewMixStaging` 提供 `data/tmp/mixes/<mix-id>`；Executor 只清理其 `render-*` 子目录，失败时移除本次新输出，不删除调用方 staging 根目录。
