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
- [ ] 成功后视频可以播放。
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
