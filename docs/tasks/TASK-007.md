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

- [ ] 可一次选择并上传多个视频。
- [ ] 各文件有独立 uploading / ready / failed 状态，并能正确展示部分成功响应。
- [ ] 成功视频显示服务端返回的真实时长。
- [ ] 用户可以选择参与混剪的视频。
- [ ] 多个 audio asset 存在时页面始终只有一个当前 `selectedAudioId`。
- [ ] 上传新口播不会要求服务端删除旧资产。
- [ ] API 错误可读，不展示原始后端堆栈。
- [ ] 刷新页面后已有 assets 可恢复。
- [ ] 前端测试与 build 通过，并由 Docker builder 再次执行。

## 验收证据

- Commit：
- 前端测试：
- Build：
- 部分成功 UI：
- Audio 选择语义：
