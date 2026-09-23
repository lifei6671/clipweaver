# TASK-011：README、AI 协作记录与最终交付

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-010
- 可并行：否
- 目标：把已验证实现整理为面试官在只有 Docker/Docker Compose 的新环境中可独立复现的最终提交。

## 范围

1. 完善根 README：
   - 项目说明；
   - 唯一宿主前置条件：Docker + Docker Compose；
   - `docker compose up --build -d` 启动；
   - 页面使用流程；
   - 容器内 fixture/E2E 执行方式；
   - 测试与构建在 Docker builder 中执行的说明；
   - 日志查看；
   - 停止与重启；
   - 关键假设与设计取舍；
   - 已知限制；
   - 自动字幕状态；
   - 实际完整验证结果。
2. 编写 `AI-NOTES.md`。
3. 记录 2～3 个真实发生的 AI 协作决策/纠偏案例，关联测试、日志或提交。
4. 检查仓库不存在密钥、本机路径、普通运行期 fixture/output 和无关 IDE/缓存文件；显式 `demo/` 交付资产除外。
5. 在 clean-room 口径下重新执行 Docker build + 容器内 E2E，并用浏览器实际完成上传 → 选择 → 混剪 → 播放 → 下载流程。
6. README 明确说明首次构建允许联网下载依赖，并列出全部必需配置、环境变量和初始化步骤。
7. README 已知限制至少写明：同步短素材模型、客户端断线不保证立即取消渲染、页面刷新恢复 assets 但不恢复上一次 mix 结果展示、v0.1 不实现字幕。
8. AI-NOTES 的 2～3 个案例必须说明方案来源、接受/调整/否决内容、判断依据、验证方式，并关联提交/测试/日志或必要的关键对话节选。
9. 更新任务索引，将全部已通过任务标记为 `PASS`。

## 交付物

- `README.md`
- `AI-NOTES.md`
- 实际无字幕示例成片（`demo/` 文件或可访问下载链接）
- 可复现示例成片生成说明
- 完整验收记录
- 干净的 Git 提交历史

## 约束

- AI-NOTES 只记录实际发生的过程，不编造错误、否决或纠偏。
- README 中未验证项目必须明确标记。
- 自动字幕未实现时直接说明未实现及基础流程不受影响。
- 题目要求实际无字幕成片，最终必须提供真实文件或可访问下载链接。普通运行期 output 不提交；若选择随源码提供，应放入明确的 `demo/` 目录，并确认体积与分发许可。
- 最终验收不得要求宿主机运行 `go test`、pnpm 或 FFprobe。
- clean-room 验收只能按照已提交 README 操作；从开始构建到完整流程验证结束，不允许临时修改源码、Docker 配置、脚本或补充未记录的手工修复。

## 验收条件

- [x] 新环境只安装 Docker 和 Docker Compose，即可按 README 从源码构建和启动。
- [x] `docker compose build --no-cache` 实际执行并通过 Go tests、frontend tests、frontend build。
- [x] `docker compose up -d` 后应用 healthy。
- [x] 首次构建所需联网依赖、配置和初始化步骤均已在 README 说明。
- [x] 容器内 fixture 生成和 Docker E2E 连续执行至少一次通过，且 TASK-010 的两次稳定性记录仍有效。
- [x] 浏览器实际完成上传多个视频 → 上传一段口播 → 选择视频 → 发起混剪 → 播放成片 → 下载成片。
- [x] 浏览器实际触发至少一个失败场景并看到有意义的错误，随后无需重新上传全部素材即可重新制作。
- [x] clean-room 验收全过程没有临时修改源码、Compose、Dockerfile、配置或测试脚本。
- [x] README 中所有面向验收人的命令按顺序实际执行成功。
- [x] AI-NOTES 至少包含 2 个真实案例及证据引用。
- [x] `git status` 不包含应提交但遗漏的源码/文档，也不包含本机缓存、秘密或普通运行期媒体二进制；显式 `demo/` 交付资产除外。
- [ ] docs/tasks 中所有任务均有最终状态和验收证据。
- [x] README 的已知限制与最终实现一致。
- [x] README 能直接定位实际无字幕示例成片（仓库 `demo/` 路径或可访问下载链接）。

## 验收证据

- Commit：README、AI-NOTES 和本任务进入 `IN_PROGRESS` 的任务索引已先以 `9afce5fa6e2234661fac849db17584ae948bb004` 提交；下述验收后补充的 README/任务证据随本次文档变更提交。人工 Review 尚未进行，本任务保持 `REVIEW`；不把 `REVIEW` 当作 `PASS`。
- No-cache build / builder tests：2026-09-23，在独立 project `clipweaver-task011-clean` 开始前 `docker volume inspect clipweaver-task011-clean_app-data` 返回 `no such volume`。在已提交 README 的源码状态执行 `docker compose -p clipweaver-task011-clean build --no-cache --progress plain`，退出 0；完整日志为本机 `%TEMP%/clipweaver-task011-clean-build.log`。前端 Vitest 31/31 PASS，`tsc --noEmit && vite build` PASS；Go builder `go test ./...` 的六个测试包均 PASS、两个包无测试文件，`CGO_ENABLED=0 go build` PASS，runtime 镜像构建 PASS。宿主未运行 Go、Node、pnpm、FFmpeg 或 FFprobe。
- 首次构建/初始化 README 核对：README 已列出仅需 Docker/Compose、首次允许联网拉取镜像及 Go/pnpm/Debian 依赖、全部七个可选 Compose 环境变量、内部 `APP_ADDR`/`WEB_DIST_DIR`、自动创建数据卷且无需手工初始化。宿主 8080 已被其他进程占用，依 README 的 `HOST_PORT` 说明使用 18092；`docker compose -p clipweaver-task011-clean up --build -d` 退出 0，新建项目容器、网络和空 volume。`ps` 显示 `Up (healthy)`，容器内 `/api/health` 为 200、`status=ok`、FFmpeg/FFprobe 均为 7.1.1。
- Docker E2E：`docker compose -p clipweaver-task011-clean exec -T app sh /app/scripts/generate-fixtures.sh` 退出 0，fixture 只在新 volume 内生成。`docker compose -p clipweaver-task011-clean exec -T app sh /app/scripts/e2e.sh` 退出 0，证据目录 `/app/data/e2e/run.R06chG`；经 HTTP 上传 4 个视频、2 个音频，主 mixId `097de348-f7f0-40d8-a3ca-78aabaafbd13`、9.7 秒成片为 H.264/1080×1920/yuv420p/30fps + 单 AAC，video/audio/format 均 9.700000 秒，四项误差均 0。rotation 源 metadata=90，成片上/下 YAVG=41/81；素材原声替换、无效媒体 `FFPROBE_FAILED`、不足时 HTTP 422 / `missingDurationUs=6100000` 均通过。TASK-010 的两次连续 E2E 记录保持有效。
- 浏览器完整流程：Codex in-app 真实浏览器打开 `http://localhost:18092/`，一次选择三个由容器内 FFmpeg 生成并临时复制到宿主 `%TEMP%` 的视频 `portrait.mp4`、`landscape.mp4`、`with-source-audio.mp4`，看到 `uploading → ready` 和 3.6/4.2/4.5 秒服务端时长；上传 9.7 秒 `narration.wav`，单一当前口播被选中。seed `42`，提交成功后的 mixId `720ea68d-5fb2-4719-bb7a-9db971bdad60`；制作中按钮禁用并显示“制作中”。浏览器 video 的 metadata 为 1080×1920、duration 9.7 秒、`error=null`，从开头实际播放，`currentTime` 增长到 7.398937 秒，再到 9.7 秒且 `ended=true`。点击“下载 MP4”触发浏览器 download 事件，下载件落盘 1,383,698 bytes，其 SHA-256 `d7324adaec436a9ef2ed7f5df2f801ad285ad105c8af3062f76e895277156148` 与容器服务端 output 一致；容器内 FFprobe 得 H.264 1080×1920、AAC、video/audio/format 均 9.700000 秒。浏览器 Console error 列表为空。
- 失败反馈/重新制作：浏览器只勾选 3.6 秒 `portrait.mp4` 配 9.7 秒口播，页面明确显示“所选视频素材时长不足，还缺少 6.1 秒视频素材”；重新勾选刚上传的另外两个视频后，不重新上传即可成功制作上述成片。
- Clean-room 无现场修改：从无缓存构建开始到浏览器完成上传、失败重做、播放和下载，未修改源码、Compose、Dockerfile、配置或验收脚本；期间 `git status --short` 只有用户要求保留、不参与本轮的原始题目 PDF 为 untracked。该宿主机还安装有其他开发工具，但本次构建、服务、媒体探测、fixture 及 HTTP E2E 只使用 Docker/Compose；浏览器测试所用短素材仅由容器生成并复制到宿主临时目录，不依赖宿主 FFmpeg。
- README clean-room 验证：按已提交 README 分别执行 `build --no-cache`、`up --build -d`、`ps`、容器内 fixture/E2E、`logs --tail=50 app`、`down`、`up -d`，均退出 0；`down` 后 named volume 未删除，重新 `up -d` 后容器再次 healthy，`GET /api/assets` 仍能读取先前上传资产。浏览器刷新后 assets 恢复，选中状态及上次 mix 结果展示不恢复，与 README 已知限制一致。初次容器日志已保存在本机 `%TEMP%/clipweaver-task011-clean-app.log`。
- AI-NOTES 案例：上传 BodyLimit 测试客户端时序（`TestBodyLimitUsesPublicError`、提交 `62abe001`）；长口播音轨缺尾与 video-finalize/simple mux（提交 `19f97e4`、TASK-005）；rotation fixture metadata 与像素方向验证（TASK-010、提交 `9696f87`）。均写明方案来源、调整与验证依据。
- 最终 Git status：clean-room 完成时仅有用户要求本阶段不处理的原始题目 PDF 为 untracked；本次 README/任务证据提交后也只保留该用户文件。仓库未跟踪普通运行期 fixture/output、缓存或密钥；明确的 `demo/` 成片与用户允许分发的 `material/` 是交付资产。唯一未勾选项待 TASK-011 人工 Review 后更新最终状态。
