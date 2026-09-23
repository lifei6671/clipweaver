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
6. 口播独立标准化为 PCM/WAV 连续时间轴；`video-finalize` 只处理 silent video，最终 mux 只 map final-video 与标准化 narration，不映射素材原声。
7. `video-finalize` 使用 `tpad=stop=-1:stop_mode=clone` 延长末帧；视频阶段和最终 mux 均使用显式 `-t <TargetDuration>` 控制硬时长上界，不把 `-shortest` 作为核心对齐机制。
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
- [x] 横屏、普通竖屏、rotation metadata 样本均保持正确显示方向和比例，输出 1080×1920。
- [x] 黑边填充正确，画面不拉伸。
- [x] 最终容器为 MP4，并使用 `+faststart`。
- [x] video codec = H.264、pix_fmt = yuv420p、帧率约 30fps。
- [x] audio codec = AAC，最终只有预期的单一 audio stream。
- [x] 参数测试证明 `video-finalize` 仅映射 silent video，最终 mux 仅映射 final-video + 标准化 narration，不映射素材原声。
- [x] `abs(Tvideo-Ttarget) <= 100ms`。
- [x] `abs(Taudio-Ttarget) <= 100ms`。
- [x] `abs(Tformat-Ttarget) <= 100ms`。
- [x] `abs(Tvideo-Taudio) <= 100ms`。
- [x] 口播末尾人工抽样无明显缺失。
- [x] FFmpeg 失败返回 `FFMPEG_FAILED`；媒体合同不通过返回 `RENDER_VALIDATION_FAILED`。
- [x] 真实 FFmpeg/FFprobe 集成测试通过。

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

### 2026-09-23 Docker 真实媒体联合验收（当前状态：BLOCKED）

- 环境：正式镜像 `clipweaver-issue001-app:latest`，独立 Compose project `clipweaver-real-media`，端口 18081，容器 healthy。`material/` 只读挂载给 FFprobe；原文件未修改。以下时长为容器内 `ffprobe` 的视频/音频 stream duration（秒）。8 个原始 MP4 均为竖屏、均自带 AAC 音轨，均无 rotation/display matrix：

| 文件 | 视频 codec | 宽×高 | fps | 视频时长 | 音频时长 |
| --- | --- | --- | --- | ---: | ---: |
| `7624563971110620403.mp4` | HEVC | 1440×2560 | 20200/337 | 16.850000 | 16.832993 |
| `7629753801633876323-hd.mp4` | H.264 | 1080×1920 | 30 | 16.100000 | 16.068005 |
| `7637724846634148454.mp4` | HEVC | 1900×3378 | 60 | 13.566667 | 13.559002 |
| `7671387297355924681-hd.mp4` | H.264 | 1080×1920 | 30 | 19.100000 | 19.061995 |
| `7679394532500536201.mp4` | HEVC | 1900×3378 | 60 | 8.233333 | 8.218005 |
| `7681245340850422643-hd.mp4` | H.264 | 1080×1920 | 30 | 28.233333 | 28.187007 |
| `7684871670289046154-hd.mp4` | H.264 | 1080×1920 | 30 | 7.466667 | 7.428934 |
| `7686157174385692111.mp4` | H.264 | 1080×1920 | 30 | 17.866667 | 17.856009 |

- `7685603214562115578.mp3`：MP3 音频 stream 59.271812 秒、format 59.271813 秒；容器内完整解码到 59.271837 秒。是否为真人语言口播、尾句语义是否完整，自动化无法听觉确认，不能据此标记人工试听 PASS。
- 横屏样本来自只读原素材 `7681245340850422643-hd.mp4` 的 5 秒横向裁切副本（1080×608），并非声称 `material/` 自带横屏。普通竖屏使用原素材 `7684871670289046154-hd.mp4`；rotation 样本在横屏副本上用容器 FFmpeg `-display_rotation:v:0 90` 生成，FFprobe 确认 Display Matrix rotation=90。三者经真实 HTTP 混剪得到 1080×1920、H.264、yuv420p、30/1 fps、单一 AAC 音轨，抽帧可解码并检查方向/比例。横屏成片 1 秒帧的顶部/中部/底部 200 像素带 `YAVG=16 / 120.996 / 16`，实际画面上下为黑边；竖屏与 rotation 抽帧无异常拉伸。短样本 MP4 atom 顺序 `ftyp → moov → mdat`，证明 `+faststart` 实际生效。三个短成片均使用原 MP3 的 4 秒派生副本，仅用于独立媒体路径验证，不代表完整原 MP3 验收。
- 短样本示例：`mixId=6caab01d-3599-4d47-97f8-e962b0b71060`，`Ttarget=4.048980s`，`Tvideo=4.066667s`，`Taudio=4.000000s`，`Tformat=4.066667s`；四项误差依次为 17.687ms、48.980ms、17.687ms、66.667ms。输入视频自带 AAC，而成片只有一条 AAC；mux 显式映射所选口播，不映射素材原声。
- Linux 真集成：在只读挂载仓库的 `golang:1.25.14-bookworm` + 正式镜像内 FFmpeg/FFprobe 临时测试容器执行 `go test ./internal/media -run '^TestExecutorRealFFmpeg$' -count=1 -v`，PASS、无 SKIP；测试输出 `video_us=4500000 audio_us=4500000 format_us=4500000`。
- **ISSUE-REAL-001（Major，阻塞 PASS）**：浏览器上传的三个真实视频 `7681245340850422643-hd.mp4`、`7671387297355924681-hd.mp4`、`7629753801633876323-hd.mp4`（合计 63.433333 秒）与完整 `7685603214562115578.mp3`，seed `42`，`POST /api/mixes` 稳定返回 HTTP 500 / `RENDER_VALIDATION_FAILED`；浏览器同一请求及再次直接 HTTP 请求均复现。首次/重试 mixId 分别为 `60580531-19d2-4550-b51a-1d1185cd6eb2`、`490aa28f-7bb9-4d7f-817a-93b190ab9d93`，保留 `meta.json`、`plan.json`，服务后续 health 为 200。隔离卷中抢在失败清理前保留的真实输出 `/app/data/acceptance/failed-output.mp4`（SHA-256 `58f75be0ccde3b06b5532e2ac9ea28a549d4ba4734322ced545953f1dce6b93`）可由 FFmpeg 完整解码，FFprobe 结果：视频 H.264 1080×1920 yuv420p 30fps、仅一条 AAC，`Ttarget=59.271813s`、`Tvideo=59.300000s`、`Taudio=41.062993s`、`Tformat=59.300000s`；四项误差分别为 **28.187ms、18208.820ms、28.187ms、18237.007ms**，音轨比目标短约 18.21 秒。上传资产与原 MP3 的 SHA-256 一致，源 MP3 完整解码约 59.272 秒，排除原文件被上传截断。再增加第四个真实视频后仍返回 500；捕获对应 silent video 后，按当前 `muxArgs` 重放可得到仅 23.670 秒 AAC 音轨，说明问题范围在长口播的 mux/时间戳链路，尚未断言最终根因。对应代码范围 `internal/media/ffmpeg.go` mux 参数与 `internal/media/validator.go`；本轮未改生产代码。保留的日志、FFprobe JSON、输入派生副本与失败 MP4 位于本机 `%TEMP%/clipweaver-real-media-evidence/`，同一失败 MP4 也在独立 Compose 卷 `/app/data/acceptance/`。
- ISSUE-REAL-001 稳定复现输入：上传上列三个原始 MP4 与完整原始 MP3 后，按 `failed-mix-meta.json` 的资产 ID 执行 `POST /api/mixes`，`Content-Type: application/json`，请求体 `{"videoIds":["88b04f61-1a45-4244-86e1-b614ecbce8b0","338a24ed-3857-407d-a402-eb49c026f6d0","371657c1-d9d0-4a88-9837-609506545fa8"],"audioId":"6c388cc5-5eb8-4c1d-a909-bb3bfdabf185","seed":"42"}`。预期 200 且四项时长误差均 ≤100ms；实际 500，业务码 `RENDER_VALIDATION_FAILED`。复核文件还包括 `%TEMP%/clipweaver-real-media-evidence/failed-mix-meta.json`、`failed-mix-plan.json`、`compose-app.log` 与 `failed-output.ffprobe.json`；这些是隔离环境证据，不是仓库交付物。
- 阻塞影响与恢复条件：完整原 MP3 成片违反原题 100ms 媒体合同，无法访问成功成片，尾部人工试听也无法完成。待人工决定修复后，须对完整原 MP3 重做真实 HTTP/FFprobe 四项误差及真人尾句试听；本轮不得以短样本成功替代。历史宿主 SKIP 与 Docker 权限记录仍为当时事实。

### 2026-09-23 ISSUE-REAL-001 第二阶段修复（当前状态：BLOCKED）

- 保留已有 `narration-normalize → narration.wav`；Executor 在 `concat` 后固定执行 `narration-normalize → video-finalize → mux → Validator`。`video-finalize` 仅处理 `silent.mp4` 的视频 `tpad`、目标时长与 H.264 输出；最终 mux 仅映射 `final-video.mp4` 的视频和 `narration.wav` 的音频，视频 copy、音频 AAC 编码，不再使用 `filter_complex`。原口播与素材音轨均不直接进入最终 mux。
- 本地验证：`go test ./internal/media -skip '^TestExecutorRealFFmpeg$' -count=1` PASS，`go vet ./internal/media` PASS；定向参数、顺序及 narration-normalize/video-finalize/mux 失败清理测试 PASS。将当前源码交叉编译为 Linux 测试二进制，在已有 FFmpeg/FFprobe 运行容器执行 `TestExecutorRealFFmpeg`，4.5 秒合成素材测试 PASS、无 SKIP，三项时长均为 4.500000 秒；该合成测试不代表完整原 MP3 已通过真实媒体回归。
- 下一轮须在新 Docker 镜像中，用完整原始 `7685603214562115578.mp3` 与原始视频重新走 HTTP 链路；在失败清理前从 `data/tmp/mixes/<mix-id>/render-*/` 保留并分别 FFprobe `silent.mp4`、`narration.wav`、`final-video.mp4`，同时检查最终 `output.mp4`。四项 100ms 时长合同与真人尾句试听尚未完成，TASK-005 保持 `BLOCKED`。

### 2026-09-23 原始口播 Docker 回归（当前状态：REVIEW）

- 从当前未提交源码执行 `docker compose -p clipweaver-video-finalize-regression build --no-cache --progress plain`，Docker builder 的 `go test ./...`、前端 31 项测试、TypeScript/Vite build 与 Go binary build 均 PASS；独立 Compose project 使用端口 18083、新 volume，容器 healthy，FFmpeg/FFprobe 7.1.1、`GET /api/health` 200。构建日志在 `%TEMP%/clipweaver-video-finalize-regression-evidence/docker-build.log`。
- 经真实 HTTP 上传三个原始视频 `7681245340850422643-hd.mp4`、`7671387297355924681-hd.mp4`、`7629753801633876323-hd.mp4` 及完整原始 `7685603214562115578.mp3`（只读使用，未改原件）；seed `"42"` 的 `POST /api/mixes` 返回 200 / completed，`mixId=89e25790-545a-4ff4-8d67-d58a313340b7`、`durationUs=59271813`，包含 previewUrl/downloadUrl。先前稳定复现的 `RENDER_VALIDATION_FAILED` 未再出现，服务后续仍 healthy。
- 在 Executor 清理前于独立 volume 建立硬链接保留中间文件，容器内独立 FFprobe：`silent.mp4` 仅 H.264 视频，59.300000 秒；`narration.wav` 仅 PCM 音频，59.271837 秒；`final-video.mp4` 仅 H.264 视频，59.300000 秒；`output.mp4` 为 1080×1920、H.264、yuv420p、30/1 fps，只有一条 AAC 音轨，`Tvideo=59.300000s`、`Taudio=59.271995s`、`Tformat=59.300000s`。相对冻结 `Ttarget=59.271813s`，四项误差依次为 **28.187ms、0.182ms、28.187ms、28.005ms**，全部 ≤100ms；下载成片 SHA-256 与服务端 output 一致，FFmpeg 视频和音频全片解码 exit 0。中间文件、output、meta、plan、日志保存在 `%TEMP%/clipweaver-video-finalize-regression-evidence/`，Docker volume `/app/data/acceptance/` 也保留四个媒体阶段产物。
- 原 MP3 与最终成片在从 54.271813 秒至结尾的音轨中均有约 5 秒实际音频样本；`volumedetect` 均为 `max_volume=-0.6 dB`，均值分别为 -14.5/-14.6 dB。参数与阶段证据无音频 loop、`apad` 或素材原声映射；这些信号证据不等同于真人尾句语义试听。`ISSUE-REAL-001` 的 **时长与音轨技术缺陷已关闭**；真人尾句仍待人工试听，修复实现仍未提交，故 TASK-005 由 `BLOCKED` 转为 `REVIEW`，不标 `PASS`。上方旧失败和“下一轮须”记录是当时历史状态。
- 2026-09-23 用户人工试听确认：原 MP3 包含真人口播，最后一句完整；成片尾部有音乐，未被截断。因此“口播末尾人工抽样无明显缺失”验收项通过。此确认发生在上方“仍待人工试听”的历史记录之后；本次媒体流水线修复尚未提交（Commit：`pending`），且新修改仍待人工代码 Review，按任务索引的 PASS 规则保持 `REVIEW`。
