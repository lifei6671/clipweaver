# TASK-008：前端混剪、预览与下载流程

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-006、TASK-007（按任务索引中的开发期例外，两项均已完成人工代码 Review，真实媒体/Docker 浏览器验收后置，允许本任务先行开发）
- 可并行：否
- 目标：完成用户从已选素材发起混剪到预览、下载和失败重试的完整页面流程。

## 范围

1. “开始混剪”按钮根据输入完整性控制可用状态。
2. 调用 `POST /api/mixes`。
3. 请求期间展示明确制作中状态并禁止重复提交。
4. 成功后使用 previewUrl 展示 HTML5 video。
5. 提供下载 MP4 操作。
6. 提供重新制作操作，保留当前资产和视频选择。
7. 显示服务端业务错误，包括素材不足的缺少时长。
8. 处理网络失败、服务端失败和重试。

## 交付物

- Mix action/state UI
- Result/preview/download UI
- 错误与重试交互
- 前端测试

## 约束

- v0.1 不增加任务历史列表、进度轮询或后台通知。
- 同步请求期间仅展示制作中，不虚构 FFmpeg 百分比进度。
- 不增加字幕开关。

## 验收条件

- [x] 没有已选视频或口播时不能提交。
- [x] 制作期间无法重复点击创建多个请求。
- [x] 成功后视频可以播放。
- [x] 下载按钮请求 downloadUrl。
- [x] 失败信息明确可读。
- [x] 失败后素材与选择仍保留，可直接重新制作。
- [x] 素材不足错误包含缺少时长。
- [x] 前端测试与 build 通过。

## 验收证据

- Commit：pending（本任务不执行 git add/commit/push）。
- 前端测试：`corepack pnpm test`（`web/` 下），31/31 PASS；含 seed 字符串精度、同步防重入、URL/下载语义、错误与重试、并发和卸载 abort。
- Build：`corepack pnpm build`（`web/` 下），`tsc --noEmit && vite build` PASS；有 bundle size warning。
- 成功流程截图/说明：jsdom 验证 `<video controls src=previewUrl>`、duration、服务端 seed 和 `downloadUrl` 链接；未用真实 MP4 在浏览器播放，故“成功后视频可以播放”暂不勾选。真实浏览器播放与 Docker builder 复验后置。
- 失败重试截图/说明：jsdom 验证素材不足缺少 4.7 秒、MIX_BUSY/MIX_TIMEOUT/INVALID_SEED/渲染与网络失败文案；失败保留素材选择及上一次成片，重试成功后替换结果。

### 2026-09-23 真实浏览器联合验收（当前状态：BLOCKED）

- 在正式运行镜像的独立 Compose project `clipweaver-real-media` 上，Codex 内置浏览器实际上传四个原始 `material/` 视频和完整 `7685603214562115578.mp3`。只选 7.466667 秒视频发起混剪时，页面显示“所选视频素材时长不足，还缺少 51.8 秒视频素材”；资产与选择仍保留。随后不重新上传，勾选合计 63.433333 秒的三个视频并设 seed `42`，点击“开始混剪”；请求期间页面显示“制作中”，按钮 `disabled=true`，不能重复提交。同步请求完成后页面显示“成片校验失败，请重试”；服务端 HTTP 500 / `RENDER_VALIDATION_FAILED`，同输入另一次 HTTP 请求稳定复现。增加第四个原始视频重试也失败；具体媒体证据与初步定位见 TASK-005 的 `ISSUE-REAL-001`。后续 health 为 200。
- 为继续独立验收前端交互，用原始视频内容裁切出的 5 秒横屏副本、2 秒不足时长副本及原 MP3 的约 4 秒副本测试。Edge 页面选择 2 秒视频与短音频时明确显示“还缺少 2 秒”；无需重新上传，取消 2 秒视频并选择 5 秒视频后重新制作成功。Edge `<video>` metadata `readyState=4`、`videoWidth=1080`、`videoHeight=1920`、`duration=4.066667s`、`media.error=null`；实际点击播放后 `paused=false`、`currentTime` 增长，最终 `currentTime=duration`、`ended=true`。浏览器 Console 无 error。该短样本成功证明 UI 播放能力，但不是完整原 MP3 的成功链，故不勾选其最后一个真实交付验收项。
- 页面“下载 MP4”链接指向 `/api/mixes/:id/download`；Edge 实际点击后 Compose 日志记录对应 GET 200，服务端 HTTP 响应为 `video/mp4` + attachment，容器内下载与服务端 output SHA-256 一致。浏览器自动化接口未捕获 download event，用户下载目录也未观察到该新文件，因此只记录“点击与 HTTP 下载请求”通过，不声称浏览器文件已落盘。浏览器实际落盘这一层仍待复验。
- 阻塞影响与恢复条件：TASK-005 的完整原 MP3 音轨偏差超过 100ms，使用户要求的真实素材完整浏览器制作→播放→下载流程无法完成；TASK-006 亦未达到 PASS。待人工决定修复 `ISSUE-REAL-001` 后，用同一完整原 MP3 重做成片、播放至末尾并验证真实下载落盘；本轮不改生产代码，TASK-008 保持 BLOCKED。原有 jsdom 证据仍为开发期记录。

### 2026-09-23 完整原口播 Edge 回归（当前状态：REVIEW）

- 在当前未提交源码 `--no-cache` 构建的独立 Compose project `clipweaver-video-finalize-regression` 上，Edge 页面从服务端恢复三个已上传原始视频及完整原 MP3；勾选三段视频和当前口播，seed 输入 `42`，点击“开始混剪”。请求期间显示“制作中”，提交按钮禁用；成功显示 `mixId=51e94a49-b5e9-4798-9531-9040c7e858cc` 的完整 59.3 秒成片与下载链接。
- Edge `<video>` 实测 `readyState=4`、`videoWidth=1080`、`videoHeight=1920`、`duration=59.3s`、`media.error=null`；实际点击播放后 `currentTime` 增至 8.360733 秒。使用视频控件 seek 至 54.556 秒并继续播放，最终 `currentTime=duration=59.3s`、`ended=true`、`media.error=null`；浏览器 Console 无 error。
- 点击“下载 MP4”捕获真实浏览器 download event；`C:\Users\lifei\Downloads\clipweaver-51e94a49-b5e9-4798-9531-9040c7e858cc.mp4` 实际落盘 64879433 字节，SHA-256 `a6ddbff918f641a4965d04e4cd33c1b5cd20ade02b3c84d20148deae33e2e1ab` 与服务端成片一致。旧记录中的“未捕获下载/未落盘”是当时短样本浏览器证据边界，现已由完整原口播成片补验。
- 同一页面取消两段视频，仅保留 28.233333 秒视频后点击制作，页面显示“还缺少 31 秒”；服务返回 HTTP 422 / `INSUFFICIENT_VIDEO_DURATION`、`missingDurationUs=31038480`。原素材和口播保持 ready，重新勾选足够视频后点击“重新制作”，再次成功显示 `mixId=38fc9511-3886-4d22-b2e1-f0d6d4d8c905`，新预览 metadata 仍为 1080×1920、59.3 秒、`media.error=null`。本任务的真实浏览器功能验收项已补齐；TASK-006 依赖 TASK-005 的人工尾句验收仍为 `REVIEW`，且本任务实现提交仍为 `pending`，按状态规则 TASK-008 由 `BLOCKED` 转为 `REVIEW`，不标 `PASS`。
