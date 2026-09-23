# ClipWeaver 最终真实验收报告

> 依据：`docs/Go 全栈笔试题 2026-09-21.pdf`（3 页，最高优先级）、`docs/requirements-baseline.md`、`docs/technical-design.md`、TASK-001～011。原题没有独立的执行耗时上限；其中“0.1 秒”是成片音视频与口播的时长误差。仅采集证据，未修改产品实现。

## 1. 验收环境

| 项目 | 实测值 |
| -- | -- |
| 时间 | 2026-09-23 16:46～17:15，Asia/Shanghai |
| commit / branch | `dacc3530d25e148b90cde64c74333e7bdbf42b05` / `master` |
| 开始前工作树 | 仅 `?? docs/Go 全栈笔试题 2026-09-21.pdf`；该原题 PDF 未在 Git 中跟踪 |
| 操作系统 | Windows 11 专业版 `10.0.26200`；Docker Desktop Linux Engine `linux/amd64` |
| Docker | Client / Engine `29.8.0`，Docker Desktop `4.91.0` |
| Compose | `v5.5.1` |
| 正式验收入口 | `docker compose -p clipweaver-accept-0923 config`、`docker compose -p clipweaver-accept-0923 build --no-cache --progress plain`、`docker compose -p clipweaver-accept-0923 up --build -d`；独立 project/volume，不做全局清理 |

仓库内没有 `AGENTS.md` 或 `AGENTS.override.md`；本轮遵守用户消息给出的全局 AGENTS 规则。验收期间根目录又陆续出现未跟踪的 `TikVideo.App_*` / `[DLPanda.com]*` 媒体文件，来源未确认；这些文件不在开始前工作树中，本轮未读取、修改或用作 fixture。本助手仅新增本报告，未改动任何已跟踪文件。

正式 Compose 构建始终没有产出运行镜像。为继续验证功能，另以当前源码只读挂载到 `golang:1.25.14-bookworm` 容器，放入 Docker 构建所得 Web dist 和 `mwader/static-ffmpeg:7.1.1` 的二进制，以独立 volume `clipweaver-accept-0923-diagdata`、诊断容器 `clipweaver-accept-0923-diagnostic` 和宿主端口 `18080` 运行。HTTP 客户端为同一独立 Docker 网络上的另一容器；浏览器访问 `127.0.0.1:18080`。下文将此标为**诊断环境**，它不能证明 Dockerfile、Compose、非 root 用户或 README 一键交付通过。宿主原有 Go 进程占用 `8080`，其 FFmpeg 为 9.0.1；所有有效诊断 HTTP/浏览器证据均隔离到 `18080` 或 Docker 网络内的诊断容器 `8080`。

## 2. 原始需求追踪矩阵

下表在运行测试前建立并在本轮结束时回填。实现映射写在“原始要求”列；`PASS` 对功能项表示诊断环境中真实 HTTP/媒体/浏览器已验证，不能替代 R01/R18/R24/R25 的正式交付验收。`BLOCKED` 表示无法达到原题要求的证据强度。

| ID | 原始要求 | 验证方式 | 证据 | 结果 |
| -- | ---- | ---- | -- | -- |
| R01 | 本地 Web 完整上传→选择→混剪→预览→下载；`web/src/App.tsx`、`internal/httpapi/routes.go` | 正式 Compose 中浏览器完成全链 | 诊断浏览器全链成功，正式 Compose 无容器 | BLOCKED |
| R02 | 经实际验证且按 README 构建使用，自备素材/依赖 | 仅按交付命令从源码构建、运行、复验 | 无缓存 build 与 README 式 up 均退出 1；缺脚本 | FAIL |
| R03 | 冻结的 Go+Fiber、React+TS+AntD | 源码/依赖和构建检查 | `go.mod`、`web/package.json`；前端 build PASS，诊断 Go build PASS | PASS |
| R04 | 本地 FFmpeg；Docker/Compose；存储自选 | 容器中检查媒体工具和外部依赖 | 诊断容器 FFmpeg/FFprobe 7.1.1 可执行；本地 volume manifest；正式交付另见 R18 | PASS |
| R05 | 多视频上传；一次混剪仅一段口播 | 真实 multipart/JSON HTTP 和浏览器 | 2 视频一批 ready；单 `audioId` 请求完成，浏览器单选 | PASS |
| R06 | 名称、真实时长、上传/校验状态、视频选择 | 浏览器实际上传、选择、刷新 | 6 秒视频和 9.7/4.5 秒口播按服务端数据展示；uploading→ready/failed | PASS |
| R07 | 无效媒体有清楚错误 | 无效二进制文件 API/浏览器 | 浏览器 `invalid.bin` 显示 `failed`、`媒体探测失败，请检查文件`；后续请求正常 | PASS |
| R08 | 随机切片，同视频多片、区间不重叠 | 真正 HTTP mix 后读取 plan，核对源区间；Planner 测试 | seed 42 的 9.7 秒计划含每视频两片；区间 `[0,3)`、`[3,6)` 或截短，无重叠 | PASS |
| R09 | 口播真实时长目标，末片截短；不足明确报错、不循环 | 0.3/5.9/9.7 秒真媒体和不足 HTTP | 末片 0.7/2.9 秒；单 6 秒视频配 9.7 秒口播返回 422、缺 3.7 秒 | PASS |
| R10 | 1080×1920 MP4，比例和黑边 | 下载文件 FFprobe、FFmpeg 解码及像素探针 | H.264/yuv420p/30fps；横屏上边 Y=16、中心红色 Y=81，竖屏/旋转样本顶部非黑 | PASS |
| R11 | 口播替换素材原声；音视频各与口播差 ≤0.1 秒；无明显语音缺尾 | 成片 FFprobe、音频频谱和尾部检测；真实语音试听 | 合成音 0.3/5.9/9.7 秒误差均 0；输出频谱约 889Hz 对应口播 880Hz、非素材 330Hz；尾部有声，但无真实语音试听 | BLOCKED |
| R12 | 页面播放/下载、失败原因、保留素材重做 | 真实浏览器操作 | `<video>` 读到 1080×1920、4.5 秒且播放进度 4.10 秒；下载事件；缺 3.7 秒错误后重做 9.7 秒成功 | PASS |
| R13 | 短素材可同步处理，无队列要求 | HTTP 单请求/静态依赖检查 | `POST /api/mixes` 200 直接返回 completed；Compose 仅单服务 | PASS |
| R14 | 字幕可不实现，但 README 须说明状态 | 对照 PDF 第 2 页和 README | 未实现字幕；README 未说明字幕实现/验证状态 | FAIL |
| R15 | 完整前后端源码及运行/测试文件 | `git ls-files`、交付文件检查 | 核心前后端源码在 Git；正式 fixture/E2E 脚本及 AI-NOTES 缺失 | FAIL |
| R16 | 核心规则自动测试：不重复、末片截短、总时长、不足、可复现 | 检查 `internal/mixer/planner_test.go` 并运行 Go 测试 | 对应 9 个顶层 Planner 测试存在；Go 容器中通过 | PASS |
| R17 | Dockerfile、`.dockerignore`、Compose 文件 | 文件检查、`docker compose config` | 三文件已跟踪；config 退出 0；构建失败另见 R18 | PASS |
| R18 | 仅 Docker/Compose 可 `up --build -d` | 独立 project、无缓存构建和真实启动 | no-cache build 退出 1；up 两次退出 1；无 app 容器/health | FAIL |
| R19 | 首构可联网；必要配置/初始化步骤在 README | README 与实际运行/验收入口对照 | README 没有最终 fixture/E2E 初始化或完整验收步骤，仍写骨架阶段 | FAIL |
| R20 | README 启动、使用、测试、假设、取舍、限制、日志、停止、重启、真实验证 | 按 PDF 第 2 页逐项审读 | 有启动/日志/停止/部分配置；缺最终使用/测试、已知限制、实际验证、字幕状态等 | FAIL |
| R21 | 自制/可分发短素材或脚本及实际无字幕成片/链接 | Git 和 README 检查 | 无 `scripts/generate-fixtures.sh`、`demo/`、README 下载链接；诊断产物仅在私有 volume | FAIL |
| R22 | `AI-NOTES.md` 记录 2～3 个真实案例 | 文件/案例/证据检查 | 文件不存在 | FAIL |
| R23 | 登录/任务系统/转场等非必需 | 基础流程依赖检查 | 诊断流程不要求这些能力或外部数据库 | PASS |
| R24 | 按 README 从源码构建，短素材验证功能、成片与失败反馈，现场不改代码 | 严格执行交付入口 | 无现场代码改动；但交付入口 build/up 失败，后续只能诊断容器替代 | FAIL |
| R25 | 示例成片和单测不能替代完整可运行流程 | 要求正式交付的 Docker→UI→媒体→失败链 | 诊断链完成，正式 Compose 链在构建阶段中断 | FAIL |

## 3. Docker 干净环境验收

| 实际命令/步骤 | 结果 | 关键证据 |
| -- | -- | -- |
| `docker compose -p clipweaver-accept-0923 config` | PASS，exit 0 | 仅 `app` 服务；独立网络 `clipweaver-accept-0923_default`、独立 volume `clipweaver-accept-0923_app-data` |
| `docker compose -p clipweaver-accept-0923 build --no-cache --progress plain` | FAIL，exit 1 | 前端 `31 passed`、`tsc --noEmit && vite build` 完成；`Dockerfile:16 RUN go test ./...` 中 `TestBodyLimitUsesPublicError` 报 `write: connection reset by peer` |
| `docker compose -p clipweaver-accept-0923 up --build -d` | FAIL，exit 1，两次 | 同一 Go 用例失败；`docker compose ... ps` 为空，无正式镜像/容器/health |
| 仓库文档声称的 `docker compose exec app sh /app/scripts/generate-fixtures.sh` 和 `.../e2e.sh` | BLOCKED | `app` 无法启动；仓库没有 `scripts/`，Dockerfile 也没有复制脚本的指令 |

原始日志保存在本机临时目录 `C:\Users\lifei\AppData\Local\Temp\clipweaver-accept-0923-build.log`、`clipweaver-accept-0923-up.log`、`clipweaver-accept-0923-up2.log`；上述首个失败和退出码也在本报告中完整记录。没有执行 `docker system prune`、全局 volume 清理或触碰其他 Docker 项目。

为采集不受构建闸门阻塞的功能证据，诊断环境执行了 `docker build --target web-builder`（PASS）、在 Docker 内复制前端 dist 和 FFmpeg/FFprobe 7.1.1、`go run ./cmd/server`。诊断容器的 `/api/health` 为 200，两个媒体工具均报告 7.1.1。它运行在 root 身份，故**不能**证明正式 Dockerfile 的 `USER clipweaver` 对 volume 有写权限。诊断容器重启后 11 个既有 asset 仍由 `GET /api/assets` 返回、既有 mix 下载 200 / 119268 字节，预置的 `/app/data/tmp/uploads/acceptance-orphan` 被清理；这仅证明相同源码在诊断 volume 下的恢复行为。完成取证后诊断容器已停止，独立 volume 留存供人工复核。正式 Compose 的健康检查、非 root 权限、运行工具、优雅退出和 README 一键运行均未达到可验证状态。

## 4. 自动化测试

| 命令/环境 | PASS / FAIL / SKIP | 退出码及关键输出 |
| -- | -- | -- |
| Dockerfile `RUN pnpm --dir web test`，无缓存 builder | 31 / 0 / 0 | 0；Vitest `Test Files 1 passed`、`Tests 31 passed`。测试使用 jsdom/模拟 API，不是浏览器真实 HTTP |
| Dockerfile `RUN pnpm --dir web build` | typecheck 和 Vite build PASS | 0；`tsc --noEmit && vite build`，有大于 500 kB chunk 警告 |
| Dockerfile `RUN go test ./...`，无缓存 builder | 包级汇总：`internal/server` FAIL，其余已执行包 PASS；builder 输出不含顶层用例总数 | 1；首个失败 `TestBodyLimitUsesPublicError`，`app_test.go:40` 连接重置 |
| `docker run ... golang:1.25.14-bookworm go test ./internal/server -run '^TestBodyLimitUsesPublicError$' -count=2 -v` | 0 / 2 / 0 | 1；两次都在 `app_test.go:40` 连接重置 |
| `docker run ... golang:1.25.14-bookworm go test ./... -json -count=1`，无 FFmpeg | 98 / 0 / 1 | 0；单独一次全套通过，显示失败具有波动性；`TestExecutorRealFFmpeg` 为唯一 SKIP：`ffmpeg unavailable` |
| `docker exec ... go test ./... -json -count=1`，有 FFmpeg 7.1.1 的诊断容器 | 98 / 1 / 0 | 1；唯一失败仍是 `TestBodyLimitUsesPublicError`；`TestExecutorRealFFmpeg` PASS，实际输出 video/audio/format 均 4,500,000µs |
| `docker exec ... go vet ./...` | PASS | 0 |
| `docker exec ... go build -o /tmp/clipweaver-acceptance-server ./cmd/server` | PASS | 0；生成 12 MB 二进制；正式 Dockerfile 因前一测试失败未执行自身 go build |
| 项目 E2E、fixture 脚本、lint/CI | 未运行 | 仓库无 `scripts/`、`.github/` 或 lint 命令；缺失的 E2E 影响原题完整流程验收 |

JSON 测试事件日志为本机临时目录中的 `clipweaver-accept-0923-builder-go-tests.jsonl` 与 `clipweaver-accept-0923-go-tests.jsonl`。核心规则测试在 `internal/mixer/planner_test.go` 覆盖不重叠、同一 AssetID 多片、末片截短、总时长、不足、同/异 seed 等。`internal/media/executor_integration_test.go:17` 在找不到 FFmpeg 时直接 `t.Skip`，而 Dockerfile 的 Go builder 在复制媒体工具阶段之前运行测试，所以交付构建中的该核心真实媒体测试会跳过；本轮以诊断容器补做，但并未改变正式构建闸门。

## 5. API 黑盒验收

以下均从独立 Docker 客户端容器向诊断服务发 HTTP，不调用 Go 内部函数；媒体由容器内 FFmpeg 生成。JSON 响应在本机临时目录 `clipweaver-accept-0923/` 留存。主要请求/结果如下：

| 输入 | 预期 | 实际 |
| -- | -- | -- |
| `POST /api/assets/videos`，横屏 6 秒、竖屏 6 秒及 7 字节无效文件，同批 multipart | 两个成功、一个独立失败 | HTTP 200；前两项 `ready`、`durationUs=6000000`，无效项 `failed/FFPROBE_FAILED` |
| `POST /api/assets/audio`，9.7/5.9/4.5/0.3 秒 AAC | 探测实际时长 | 四次 `ready`，分别返回 9700000/5900000/4500000/300000µs |
| `POST /api/mixes`，两段 6 秒视频、9.7 秒口播、seed `"42"` | completed、9.7 秒及可用 URL | HTTP 200，ID `08ee9fdb-eb2c-43e9-b42a-21afcaecfb2f`，`durationUs=9700000`，seed 字符串 `"42"`，预览/下载 URL 可用 |
| 单 6 秒视频 + 9.7 秒口播 | 素材不足，不复用 | HTTP 422 `INSUFFICIENT_VIDEO_DURATION`，`missingDurationUs=3700000`，服务仍 healthy |
| 空体、JSON `null`、`{}` | 明确参数错误 | 依次 HTTP 400 `INVALID_REQUEST`、400 `INVALID_REQUEST`、400 `NO_VIDEO_SELECTED` |
| seed `"abc"`、JSON number、`"9223372036854775808"` | 拒绝非法/越界 seed | 均 HTTP 400 `INVALID_SEED` |
| seed `"9223372036854775807"`、省略 seed | 精确往返/服务端生成 | 均 HTTP 200；前者响应原字符串，后者返回 `"-7546165348675801580"` |
| 缺口播、随机不存在的视频 UUID、随机不存在的 mix 文件 | 错误结构稳定 | 400 `AUDIO_REQUIRED`、404 `ASSET_NOT_FOUND`、404 `MIX_NOT_FOUND` |
| 成片预览 `Range: bytes=0-3` / 越界 Range | 可播放分段/明确越界 | 206、`Content-Range: bytes 0-3/119268`、长度 4；416、`Content-Range: bytes */119268` |
| 1 MiB 上传上限的第二诊断服务接收 2 MiB 文件 | 413 且服务存活 | 独立 curl 收到 413 `UPLOAD_TOO_LARGE`；随后 health 200。与 Go 测试客户端连接重置的失败同时成立 |

重复请求和 seed 的多次真实执行见 §8；异常后的健康响应见 §11。正式 Compose 没有成功启动，故本节属于隔离诊断证据。

## 6. 媒体输出验收

fixture 全部在诊断容器中调用 FFmpeg lavfi 生成，并存在独立 Docker volume；宿主未用 FFmpeg。样本包括横屏红色 6 秒且含 330Hz 素材原声、竖屏蓝色 6 秒、30 秒横屏、前 3 秒红/后 3 秒白的时间标记视频、带 display matrix `rotation=90` 的视频、0.3/4.5/5.9/9.7 秒 880Hz 口播和无效文件。**仓库没有正式生成脚本，这组临时素材不算原题演示材料交付。**

对 mix `08ee9fdb-eb2c-43e9-b42a-21afcaecfb2f`，独立客户端 HTTP 下载后 SHA-256 与服务端成片相同：`031f973420bf0668491a94578a4ff880bc794521530434992da6a9a171f3c7a4`。实际下载文件经 `ffprobe -show_entries stream=... -show_entries format=... -of json` 检查：MP4 容器，视频 1080×1920 / H.264 / yuv420p / `30/1` / 291 帧 / 9.700 秒，唯一音轨 AAC / 9.700 秒，容器 9.700 秒。`ffmpeg -i download97.mp4 -f null -` 解码退出 0。旋转源编码为 320×180、display matrix 为 90°，其 0.3 秒成片仍为 1080×1920，画面顶部 Y 均值 81（非黑），说明实际 autorotate 后铺满竖屏。

横屏片段第 3.1 秒画面顶部 Y 均值 16（黑）、中央 Y 均值 81（红）；竖屏片段第 0.1 秒顶部 Y 均值 41（蓝，非黑），与比例保持/黑边策略吻合。9.7 秒输出中心区域逐帧 Y 值在 3.000/6.000/9.000 秒按蓝→红→蓝→红切换；标记视频 seed 1 在 3.000 秒从红切到白，最后一帧 PTS 5.866667 秒加 1/30 秒帧长为 5.900 秒，匹配源区间 `[0,3)` 后 `[3,5.9)`。

声音内容检查：1 秒处素材原声频谱质心约 340Hz，上传口播约 888Hz，成片约 889Hz；成片只含一个 AAC 音轨。9.6～9.7 秒尾部，成片平均电平 -20.6dB、输入口播 -21.1dB，表明合成音尾段没有静音截断。它不能代替真实语音尾部人工试听。

## 7. 时间精度验收

实际媒体由 FFprobe 读取时长，色块转场由成片解码后逐帧亮度测得；不是程序内部计算值。下面误差为绝对值：

| Case | Expected | Actual | Error |
| ---- | -------: | -----: | ----: |
| 0.3 秒口播 → 视频/音频/容器终点 | 0.300s | 0.300/0.300/0.300s | 0ms |
| 5.9 秒口播 → 视频/音频/容器终点 | 5.900s | 5.900/5.900/5.900s | 0ms |
| 9.7 秒口播 → 视频/音频/容器终点 | 9.700s | 9.700/9.700/9.700s | 0ms |
| 30 秒源配 9.7 秒口播 → 成片终点 | 9.700s | 9.700s（三类时长） | 0ms |
| 9.7 秒片段 1 起点 / 终点 | 0.000 / 3.000s | 0.000 / 3.000s | 0 / 0ms |
| 相邻片段转场 | 6.000 / 9.000s | 6.000 / 9.000s | 0 / 0ms |
| 5.9 秒标记源：从前 3 秒片段切到靠近源末尾的 `[3,5.9)` | 3.000s | 3.000s（帧 90） | 0ms |
| 5.9 秒标记源：末片终点 | 5.900s | 最后一帧 PTS 5.866667s + 1/30s | <0.001ms（显示精度内） |

**本轮代表样本最大实测误差：0ms（FFprobe 六位小数及 30fps 帧边界精度内）。**样本为合成恒定/分段色块，不推断任意复杂源媒体都为 0ms；原题约束为 ≤100ms。

## 8. 确定性 / Seed 验收

同一组两个 6 秒视频 + 9.7 秒口播，经真实 HTTP 渲染执行：

| seed | mix ID | plan 的源片顺序（视频 ID 简记） | 末片 |
| -- | -- | -- | -- |
| 42（初次） | `08ee9fdb-...` | 竖 `[3,6)` → 横 `[0,3)` → 竖 `[0,3)` → 横 `[3,3.7)` | 0.7s |
| 42（再试 1） | `9cc55db1-...` | 与初次完全一致 | 0.7s |
| 42（再试 2） | `3e047fe4-...` | 与初次完全一致 | 0.7s |
| 43 | `1662887a-...` | 竖 `[0,3)` → 横 `[0,3)` → 横 `[3,6)` → 竖 `[3,3.7)` | 0.7s |
| 44 | `12c53e79-...` | 横 `[0,3)` → 竖 `[0,3)` → 竖 `[3,6)` → 横 `[3,3.7)` | 0.7s |

三次 seed 42 的 `plan.json`（Seed、TargetDurationUS、ClipDurationUS、Clips 全字段）字面一致；43、44 则改变顺序及源时间点。所有计划片长总和 9.7 秒，且同 AssetID 的区间不重叠。实际媒体在 seed 42 的 3/6/9 秒呈相同转场。原始响应和计划保存在本机临时目录 `clipweaver-accept-0923/seed-runs.json`。

## 9. 前端 E2E 验收

使用真实浏览器访问诊断容器 `http://127.0.0.1:18080/`：

1. 页面打开，初始资产列表由 HTTP 恢复；未选择素材时“开始混剪”禁用。
2. 文件选择器一次上传两个真实 MP4；观察到两个 `uploading`，随后均为 `ready`、各 6 秒并默认选中。上传 4.5 秒口播时观察到 `uploading`，完成后只有新口播被选中。
3. 输入 seed `42`，点击“开始混剪”：按钮禁用并显示“制作中”；随后出现 4.5 秒结果、seed 42、`<video controls>` 和“下载 MP4”。视频元素 `readyState=4`、尺寸 1080×1920、duration 4.5；以浏览器 Space 播放后 `paused=false`、`currentTime=4.097746`，无 media error。下载链接点击触发浏览器 download 事件。
4. 只保留一段 6 秒视频，改选 9.7 秒口播后点击“重新制作”：页面提示“所选视频素材时长不足，还缺少 3.7 秒视频素材”，原成片仍可访问。重新勾选另一段视频后不重传素材即可重做，结果更新为 9.7 秒。
5. 刷新后资产列表恢复，已选数量回到 0，上次成片不再显示；这与冻结设计一致。另上传 `invalid.bin`，行状态为 `failed`，提示“媒体探测失败，请检查文件”。浏览器控制台 `error` 日志为 0。

这些是真实页面及 HTTP 服务交互；因正式 Compose 无法启动，不能计作最终 Docker 交付的浏览器 E2E PASS。

## 10. 性能验收

原题未给出 planner/handler/HTTP 耗时 `<100ms` 要求；0.1 秒是**成片时长精度**。仍以诊断容器中两个 6 秒视频、4.5 秒口播、seed 42 的 `POST /api/mixes` 记录稳定运行数据。计时使用独立 curl 客户端的 `%{time_total}`，从开始 HTTP 传输到完整响应，包含服务端规划、FFmpeg 渲染、FFprobe 验证和网络传输；不含每次 `docker run` 容器启动，也不是纯 planner 时间。两次 warm-up：1.009076、0.894524 秒。十次正式样本（均 HTTP 200，秒）：

`0.879570, 0.806310, 0.809853, 0.811562, 0.814949, 0.838135, 0.854982, 0.829702, 0.806946, 0.812111`

| min | max | mean | median | p95（Type-7 线性插值，n=10） |
| --: | --: | --: | --: | --: |
| 0.806310s | 0.879570s | 0.826412s | 0.813530s | 0.868505s |

正式 Docker 冷构建/启动未成功，故无法测量正式交付冷启动；诊断 `go run` 编译启动与上述稳定请求不是同一计时边界。样本原始记录在本机临时目录 `clipweaver-accept-0923/performance.json`。

## 11. 异常路径验收

空体/null、未选视频/口播、非法/溢出 seed、缺失视频/mix、越界 Range、无效音视频和素材不足的输入、HTTP 状态与错误结构见 §5；错误后 `/api/health` 仍 200，后续有效混剪可继续。重复 seed 42 请求三次均 completed，无 panic/服务退出。用 `MAX_UPLOAD_MB=1` 运行第二诊断服务，2 MiB multipart 请求收到 `413 UPLOAD_TOO_LARGE`，前后 health 均 200。浏览器错误后不重传即可重新制作。服务依赖尚未 ready 的精确启动时序及正式容器退出/恢复路径未在交付环境中验证，因为 Docker 镜像没有构建成功。

## 12. 发现的问题

### ISSUE-001

- 严重程度：**Blocker**；分类：测试缺陷导致 Docker 交付阻塞。
- 对应原始要求：PDF 第 3 页“仅 Docker/Compose 从源码 `up --build -d`”“验证最终提交完整流程”。
- 复现步骤：独立 project 执行 `docker compose build --no-cache`，再执行两次 `docker compose up --build -d`；Go 基础镜像定向运行 `go test ./internal/server -run '^TestBodyLimitUsesPublicError$' -count=2 -v`。
- Expected：Go builder 测试通过、镜像构建并启动。
- Actual：三次 Compose 构建均在 `Dockerfile:16` 的 `go test ./...` 退出 1；定向用例连续两次连接重置。另一次相同 Go 镜像完整测试为 98 PASS/1 SKIP，说明存在波动，不能按一次通过判定稳定。实际 curl 对超限上传得到 413 且服务存活。
- 证据：构建日志 `TestBodyLimitUsesPublicError`、`internal/server/app_test.go:40`、`write: connection reset by peer`；`docker compose ps` 为空；限额诊断 HTTP 为 413 `UPLOAD_TOO_LARGE`。
- 初步定位：`internal/server/app_test.go:16-51` 的 Go `http.DefaultClient` 上传 1MiB+1 字节请求，与 Fiber body limit 提前拒绝/关闭连接的时序交互；不将实际 HTTP 413 路径误判为必然产品失败。
- 是否影响最终交付：**是，直接阻断一键构建和正式端到端验收。**

### ISSUE-002

- 严重程度：**Major**；分类：Docker/交付缺陷。
- 对应原始要求：PDF 第 3 页自备可复现短素材或脚本、仅 Docker/Compose 环境完成验收；冻结 TASK-010。
- 复现步骤：`git ls-files scripts`、查看 Dockerfile 与任务文档中的标准 E2E 命令。
- Expected：有正式 `scripts/generate-fixtures.sh`、`scripts/e2e.sh`，运行镜像包含它们，可容器内生成素材并调用真实 API。
- Actual：仓库无 `scripts/`，Dockerfile 未复制脚本；文档给出的 `/app/scripts/...` 无法成立。本轮临时 Docker fixture 不能代替交付文件。
- 证据：`git ls-files` 无脚本，`docs/tasks/TASK-010.md:31-32` 要求执行这些路径，Dockerfile 仅复制 server 与 Web dist。
- 初步定位：TASK-010 尚为 TODO，交付物未落盘。
- 是否影响最终交付：**是，评审方无法按文档复现媒体 E2E。**

### ISSUE-003

- 严重程度：**Major**；分类：测试覆盖/验收缺陷。
- 对应原始要求：PDF 第 2～3 页要求实际媒体结果和完整可运行流程；冻结技术方案 §15、TASK-005/TASK-009/TASK-010。
- 复现步骤：在 Dockerfile 的 Go builder 等价 `golang:1.25.14-bookworm` 镜像执行 `go test ./internal/media -run '^TestExecutorRealFFmpeg$' -count=1 -v`。
- Expected：正式自动化/容器 E2E 至少有一道真实 FFmpeg 成片验证闸门。
- Actual：`SKIP: ffmpeg unavailable`；Dockerfile 在 Go builder 测试阶段尚未引入 FFmpeg，且 ISSUE-002 的运行期 E2E 脚本缺失。诊断容器另行安装 FFmpeg 后该集成测试 PASS，但不属于交付构建闸门。
- 证据：`internal/media/executor_integration_test.go:15-20`、`Dockerfile:10-19`；Go 基础镜像定向测试 SKIP，诊断容器同测试输出 video/audio/format 各 4,500,000µs。
- 初步定位：构建阶段顺序及 `t.Skip` 条件，缺少后续交付 E2E 补位。
- 是否影响最终交付：**是，现有 Docker builder 的测试结果无法证明真实媒体要求。**

### ISSUE-004

- 严重程度：**Major**；分类：文档缺陷。
- 对应原始要求：PDF 第 2 页 README 的使用/测试/假设/取舍/限制/实际验证/未验证、字幕状态、日志/停止/重启说明。
- 复现步骤：阅读根 README 并与 PDF、任务卡及现有前后端代码对照。
- Expected：README 指导最终版本从源码运行、完整操作与验证，明确字幕未实现和已知限制。
- Actual：仍写“当前阶段：TASK-001 工程骨架”“上传与混剪流程尚未实现”“占位页面”；未给出最终页面使用、容器 fixture/E2E、真实完成记录、字幕状态及完整限制。
- 证据：`README.md:5-20`；与已存在的 Mix API/Web UI 及 PDF 交付清单冲突。原题优先，不能以 README 降低验收标准。
- 初步定位：TASK-011 尚为 TODO，README 未更新到最终交付状态。
- 是否影响最终交付：**是，评审方无法仅按 README 完成验收。**

### ISSUE-005

- 严重程度：**Major**；分类：演示材料交付缺陷。
- 对应原始要求：PDF 第 3 页必须有由最终版本实际生成的无字幕成片，可随源码或提供可访问链接。
- 复现步骤：检查 Git 文件、`demo/`、README 链接。
- Expected：正式可交付 MP4 或可访问下载链接，且来源可核查。
- Actual：无 `demo/`，README 无链接；诊断容器 volume 中的成片不是源码/链接交付。
- 证据：`git ls-files` 无 demo 媒体，`README.md` 无成片定位信息。
- 初步定位：TASK-010/011 尚未完成。
- 是否影响最终交付：**是，原题显式交付项缺失。**

### ISSUE-006

- 严重程度：**Major**；分类：文档交付缺陷。
- 对应原始要求：PDF 第 3 页 `AI-NOTES.md`，2～3 个真实决策/纠偏案例及依据和验证。
- 复现步骤：`git ls-files AI-NOTES.md`、检查仓库根目录。
- Expected：存在可人工复核的 AI 协作记录。
- Actual：`AI-NOTES.md` 不存在。
- 证据：`git ls-files` 无此文件，`Test-Path AI-NOTES.md` 为 False。
- 初步定位：TASK-011 尚未完成。
- 是否影响最终交付：**是，原题显式交付项缺失。**

## 13. 未覆盖项

1. 正式 Compose 镜像/容器健康、容器内 `/app/scripts`、非 root `clipweaver` 用户写 volume、正式容器重启/优雅退出：ISSUE-001 阻断；诊断容器不能替代。
2. 真实人声口播末尾“无明显语音缺尾”的主观试听：本轮自制 880Hz 合成音仅能验证末尾有信号，不能等同真实语音。
3. 正式仓库 fixture/E2E 连续两次：ISSUE-002；临时手工生成素材不算该交付脚本验收。
4. 原题未规定处理耗时阈值，故性能数据只陈述实测，不作虚构的 `<100ms` 通过判定；正式冷启动因镜像未生成而不可测。

## 14. 最终验收状态

**FAIL**

Blocker **1**、Major **5**、Minor **0**。导致 FAIL：**ISSUE-001、ISSUE-002、ISSUE-003、ISSUE-004、ISSUE-005、ISSUE-006**。诊断环境证明核心功能在部分合成样本上可运行，但原题要求的是按 README 从源码以 Docker/Compose 完整交付；该链路未通过，且多项显式交付物缺失。未修改产品代码，未执行 commit/push。
