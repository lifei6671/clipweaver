# ClipWeaver

本地视频混剪 Web 应用。上传多个视频和一段口播，选择参与的视频后，应用按口播时长随机排列不重叠的切片，生成可预览、可下载的 1080×1920 无字幕 MP4。

## 环境与启动

宿主机只需 Docker Engine（或 Docker Desktop）和 Docker Compose；无需安装 Go、Node.js、pnpm、Python、FFmpeg、FFprobe 或 curl。首次构建**需要联网**拉取 Docker 基础镜像，并下载 Go 模块、pnpm 包和 Debian 运行依赖。没有数据库、模型下载、密钥或手工初始化步骤；Compose 自动创建持久化数据卷。

在仓库根目录运行：

```sh
docker compose up --build -d
docker compose ps
```

等待 `app` 显示 `healthy`，然后用浏览器打开 <http://localhost:8080/>。若宿主机 8080 端口已占用，先设置 `HOST_PORT` 为可用端口，再执行相同命令，浏览器访问对应端口。容器内端口由 `APP_PORT` 控制；通常无需修改。

数据保存在 Compose 的 `app-data` volume 中。`docker compose down` 不删除该卷，重新运行 `docker compose up -d` 可继续使用已上传素材。请勿在需要保留素材时使用 `down -v`。

## 页面使用

1. 在“视频素材”中一次选取多个视频，等待每项从 `uploading` 变成 `ready`；页面显示服务端探测的时长。勾选要参与本次制作的视频。
2. 在“口播音频”中上传一段音频，等待 `ready`，确认当前选中的口播。历史口播可以保留，但一次制作只选一段。
3. 可填写“随机种子”；对同一批已上传素材作相同选择并使用相同 seed，会得到相同切片计划。留空时由服务端生成。点击“开始混剪”，等待同步请求完成。
4. 在“成片结果”中播放、下载 MP4。失败时查看页面错误提示，调整选择后重新制作；素材不足会明确提示缺少时长，无需重新上传已有素材。

每个已选视频进入候选切片池，**不保证每个已选视频都出现在一次成片中**。v0.1 将视频分成约 3 秒的不重叠候选片段，随机取用至口播目标时长，最后一片可截短；不会循环使用素材区间。输出保持画面比例，空余区域填黑边，口播替换视频原声。

## 在 Docker 内复现验收

以下命令在仓库根目录按顺序执行。`build --no-cache` 会重新执行 Go 测试、前端测试、TypeScript 检查、Vite build 和 Go binary build；所有工具都在 Docker builder/runtime 内。首次启动也可以直接使用上面的 `up --build -d`。

```sh
docker compose build --no-cache
docker compose up -d
docker compose ps
docker compose exec -T app sh /app/scripts/generate-fixtures.sh
docker compose exec -T app sh /app/scripts/e2e.sh
```

fixture 脚本在容器数据卷内生成横屏、竖屏、带 rotation metadata 和素材原声的视频、非整数秒口播、短口播与无效媒体样本。E2E 脚本经运行中的 HTTP API 上传、混剪、预览与下载，并用容器内 FFprobe/FFmpeg 检查实际成片和错误分支。可以再次执行最后一条命令验证重复运行。普通 fixture、E2E 产物和运行期成片留在 volume 中，不进入 Git。

已有实际无字幕成片：[demo/clipweaver-real-sample.mp4](demo/clipweaver-real-sample.mp4)。它由当前实现经真实 HTTP 链路生成，输入是 `material/w_7688313452255266995-hd.mp4`、`material/w_7687915553347915043-hd.mp4`、`material/7671387297355924681-hd.mp4` 与完整口播 `material/7685603214562115578.mp3`，seed 为 `42`。这些素材随仓库提供，可在页面按上述步骤重新制作同一切片计划；不同运行生成的 MP4 文件不承诺逐字节相同。

## 配置与运行管理

Compose 环境变量均可省略；默认值即可完成全部流程。

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `HOST_PORT` | `8080` | 宿主浏览器访问端口 |
| `APP_PORT` | `8080` | 容器监听端口，Compose 同时配置服务和 healthcheck |
| `DATA_DIR` | `/app/data` | 应用数据目录；为保持持久化，应位于挂载的 `/app/data` 内 |
| `LOG_LEVEL` | `INFO` | Go 日志级别 |
| `MAX_UPLOAD_MB` | `512` | 单次上传请求的 body 上限，MiB |
| `MIX_TIMEOUT` | `10m` | 一次同步混剪的最长处理时间，Go duration 格式 |
| `MAX_CONCURRENT_MIXES` | `1` | 同时运行的混剪数量 |

Compose 内部还将 `APP_ADDR` 设为 `:${APP_PORT}`、`WEB_DIST_DIR` 固定为 `/app/web/dist`。修改环境变量后重建或重建容器以使新配置生效。查看状态、日志、停止与重启：

```sh
docker compose ps
docker compose logs --tail=50 app
docker compose down
docker compose up -d
```

## 设计取舍与已知限制

- v0.1 使用本地文件和 JSON manifest 持久化素材与成片，不依赖数据库或外部服务。
- `POST /api/mixes` 同步处理短素材；较长输入可能耗时，未提供后台队列。客户端断线不保证立即取消已开始的渲染；服务端仍受 `MIX_TIMEOUT` 约束。
- 刷新页面会重新加载已上传素材，但不会恢复上一次的勾选、当前口播选择或成片结果展示。已生成文件仍保存在数据卷中。
- 自动字幕**未实现、未验收**。基础无字幕混剪、预览和下载已验证；无需字幕模型或外部密钥。

## 实际验证记录

2026-09-23 的 [TASK-010 Docker E2E 证据](docs/tasks/TASK-010.md)记录：从空数据卷启动后，容器内生成 fixture，连续两轮真实 HTTP E2E 均通过。两轮 9.7 秒样本的实际输出均为 MP4、H.264、1080×1920、yuv420p、30 fps、单一 AAC，视频、音频与容器时长均为 9.700000 秒；rotation、素材原声替换、无效媒体和素材不足分支也通过。`demo/` 的完整口播成片经 FFprobe 得 `Ttarget=59.271813s`、`Tvideo=59.300000s`、`Taudio=59.271995s`、`Tformat=59.300000s`，四项时长误差均小于 100ms；真人尾句已人工试听确认完整。此前的[浏览器流程验收](docs/tasks/TASK-008.md)覆盖制作、播放至尾部、下载落盘及失败后重做。

同日按已提交的本 README 又在独立 Compose project、空数据卷中完成无缓存构建和验收：Go 各测试包通过，前端 31/31 测试与 TypeScript/Vite build 通过，容器 healthy；容器内 fixture 和完整 HTTP E2E 通过。浏览器实际上传三个视频及一段 9.7 秒口播，先看到缺少 6.1 秒素材的错误，再不重传素材制作成功；播放器显示 1080×1920、播放至 9.7 秒结尾且无媒体错误，下载 MP4 落盘后与服务端成片 SHA-256 一致。执行命令与 mixId 见 [TASK-011 验收证据](docs/tasks/TASK-011.md)。

更详细的需求、设计和任务状态见[需求基线](docs/requirements-baseline.md)、[技术设计](docs/technical-design.md)、[任务索引](docs/tasks/README.md)；真实 AI 协作决策见 [AI-NOTES.md](AI-NOTES.md)。
