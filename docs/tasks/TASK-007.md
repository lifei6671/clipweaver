# TASK-007：前端素材上传与选择流程

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 依赖：TASK-004
- 可并行：可在 TASK-005/TASK-006 开发期间推进
- 目标：完成用户进入页面后到“具备可提交混剪输入”为止的素材工作流。

## 范围

1. 多视频选择与上传。
2. 每个视频展示名称、时长、上传/校验状态。
3. 上传成功视频支持勾选/取消参与本次混剪。
4. 单次上传一段口播 audio asset；页面通过 `selectedAudioId` 选择当前口播。
5. 上传新口播后默认把 `selectedAudioId` 切换到新资产，但旧 audio asset 仍可存在于服务端。
6. 口播展示名称、真实时长和状态。
7. 无效媒体、部分成功、上传失败、文件过大等错误明确展示。
8. 页面刷新后通过 `GET /api/assets` 恢复已有资产列表；当前选择重新建立，不实现账户级持久化。

## 交付物

- React 资产上传组件
- 素材列表与选择状态
- Audio uploader / selectedAudioId
- API client/types
- 前端组件/交互测试

## 约束

- 不实现混剪结果区。
- 不引入重量级全局状态管理，除非出现明确必要性。
- 不自行推断媒体时长，展示服务端数据。
- 前端不得把 int64 seed 当作 Number 处理；seed 属于 TASK-008/006 的字符串协议。

## 验收条件

- [x] 可一次选择并上传多个视频。
- [x] 各文件有独立 uploading / ready / failed 状态，并能正确展示部分成功响应。
- [x] 成功视频显示服务端返回的真实时长。
- [x] 用户可以选择参与混剪的视频。
- [x] 多个 audio asset 存在时页面始终只有一个当前 `selectedAudioId`。
- [x] 上传新口播不会要求服务端删除旧资产。
- [x] API 错误可读，不展示原始后端堆栈。
- [x] 刷新页面后已有 assets 可恢复。
- [ ] 前端测试与 build 通过，并由 Docker builder 再次执行。

## 验收证据

- Commit：566cf5bfe095d3f068d8c324d19b7c62d32c742c（实现提交；2026-09-23 已完成人工代码 Review 并允许继续开发；Docker builder 与真实浏览器/实际媒体交互仍后置，故任务状态暂保留 REVIEW）。
- 前端测试：`corepack pnpm test`（cwd=`web`，pnpm 10.17.1），PASS，13/13；覆盖批量单请求、同名映射、部分成功、错误、刷新、音频单选和并发批次。
- Build：`corepack pnpm build`（cwd=`web`），PASS，`tsc --noEmit` 与 Vite build 完成；Vite 有大于 500 kB 的 chunk 提示。
- 部分成功 UI：测试断言同批 ready/failed 分别显示，失败项不可选，仅 ready 默认选中；`durationUs` 使用服务端值。
- Audio 选择语义：测试断言恢复后无默认选中，切换时唯一选中，新上传自动切换且旧项保留，失败不改变旧选择；没有 DELETE 请求。

- 修改文件：`web/src/App.tsx`、`App.test.tsx`、`api.ts`、`types.ts`、`utils/formatDuration.ts`；本任务状态与证据回填在 `docs/tasks/README.md`、`TASK-007.md`。
- 待验证：Docker builder 再次执行前端测试与 build；真实浏览器与实际媒体文件的交互验收后置。因此组合验收项保持未勾选，状态保持 `REVIEW`。
