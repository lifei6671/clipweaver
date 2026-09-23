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

- [ ] 新环境只安装 Docker 和 Docker Compose，即可按 README 从源码构建和启动。
- [ ] `docker compose build --no-cache` 实际执行并通过 Go tests、frontend tests、frontend build。
- [ ] `docker compose up -d` 后应用 healthy。
- [ ] 首次构建所需联网依赖、配置和初始化步骤均已在 README 说明。
- [ ] 容器内 fixture 生成和 Docker E2E 连续执行至少一次通过，且 TASK-010 的两次稳定性记录仍有效。
- [ ] 浏览器实际完成上传多个视频 → 上传一段口播 → 选择视频 → 发起混剪 → 播放成片 → 下载成片。
- [ ] 浏览器实际触发至少一个失败场景并看到有意义的错误，随后无需重新上传全部素材即可重新制作。
- [ ] clean-room 验收全过程没有临时修改源码、Compose、Dockerfile、配置或测试脚本。
- [ ] README 中所有面向验收人的命令按顺序实际执行成功。
- [ ] AI-NOTES 至少包含 2 个真实案例及证据引用。
- [ ] `git status` 不包含应提交但遗漏的源码/文档，也不包含本机缓存、秘密或普通运行期媒体二进制；显式 `demo/` 交付资产除外。
- [ ] docs/tasks 中所有任务均有最终状态和验收证据。
- [ ] README 的已知限制与最终实现一致。
- [ ] README 能直接定位实际无字幕示例成片（仓库 `demo/` 路径或可访问下载链接）。

## 验收证据

- Commit：
- No-cache build / builder tests：
- 首次构建/初始化 README 核对：
- Docker E2E：
- 浏览器完整流程：
- 失败反馈/重新制作：
- Clean-room 无现场修改：
- README clean-room 验证：
- AI-NOTES 案例：
- 最终 Git status：
