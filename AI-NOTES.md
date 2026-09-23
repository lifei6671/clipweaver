# AI 协作记录

以下只记录开发中实际发生的决策与纠偏。最终判断依据是运行结果和媒体产物，而非 AI 对代码的描述。

## 1. 上传超限的测试客户端时序

- **方案来源**：用户指出 `TestBodyLimitUsesPublicError` 在 Docker/Linux 中的 `connection reset by peer` 与 Go `net/http` 客户端持续写入大请求、fasthttp 提前拒绝之间可能存在时序冲突，并要求保留公共 413 契约。AI 沿此方向分析和修改测试。
- **接受与调整**：保留生产 BodyLimit、错误映射和 Docker builder 的 `go test ./...`；将测试改为预装完整 HTTP 请求的内存连接，通过实际 fasthttp `ServeConn` 读取响应。不接受把 reset 当作测试通过的做法。
- **判断与验证**：原网络测试有时先得到写入错误，未证明客户端收到业务错误。修正后，宿主与 Linux 容器内定向测试各连续 20 次通过；正式镜像的真实超限 multipart HTTP 请求返回 413 / `UPLOAD_TOO_LARGE`，后续 health 仍为 200。见提交 `62abe001a307f69355595f2d1ec40f9e508adbaa`、[TASK-001 证据](docs/tasks/TASK-001.md)及 `internal/server/app_test.go` 的 `TestBodyLimitUsesPublicError`。

## 2. 长口播音轨提前结束

- **方案来源**：真实 59.27 秒口播的成片仅有 41.062993 秒 AAC，用户要求先保留“原口播 → PCM/WAV 连续时间轴”，再将视频 `tpad` 与最终音频 mux 拆成独立阶段。AI 依据失败产物和中间文件实现、验证这一改动。
- **接受与调整**：接受 `narration-normalize → video-finalize → simple mux`。最终 mux 只映射已定长视频与标准化 WAV，视频 copy、音频编码 AAC；没有用音频 loop、`apad`、`-shortest` 或放宽 Validator 掩盖缺尾。
- **判断与验证**：单独 WAV 可解码至约 59.272 秒，故继续调口播标准化参数缺乏依据。新 Docker 镜像经真实 HTTP 上传原始 MP3 和视频后成片成功；独立 FFprobe 得目标 59.271813 秒、视频 59.300000 秒、AAC 59.271995 秒、容器 59.300000 秒，四项误差均小于 100ms。用户又试听确认真人最后一句完整、尾部音乐未截断。见提交 `19f97e4d4a41480aa4e5b07280cc38ebc5277adc`、[TASK-005 回归证据](docs/tasks/TASK-005.md)及 `TestFFmpegArgumentContract`、`TestExecutorRealFFmpeg`。

## 3. rotation fixture 的真实方向验证

- **方案来源**：TASK-010 的容器内 fixture 起初尝试写入 `rotate` metadata，但 FFprobe 未见 display matrix。AI 根据实际探测输出改用 FFmpeg `-display_rotation:v:0 90`，并加强 E2E 的方向断言。
- **接受与调整**：接受可重复生成的 rotation 素材与像素抽样验证；没有把命令行参数或输出尺寸本身视为方向正确的证据。
- **判断与验证**：源画面左红右蓝，写入 display matrix=90 后，真实 HTTP 混剪成片应上蓝下红。E2E 从实际画面抽样，上/下 YAVG 分别为 41/81，并断言上下均非黑且颜色顺序正确；连续两轮 Docker E2E 通过。见提交 `9696f87e11f8be4dec5f2525d050a059c5dccaf4`、[TASK-010 证据](docs/tasks/TASK-010.md)及 `scripts/e2e.sh`。
