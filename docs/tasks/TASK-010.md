# TASK-010：可复现测试素材与 Docker E2E

- 状态：以 [任务索引](README.md) 为准
- 依赖：TASK-009
- 可并行：否
- 目标：在“宿主机只有 Docker + Docker Compose”的前提下，用容器内可重复生成的素材验证完整真实业务链路。

## 范围

1. `scripts/generate-fixtures.sh` 在 app 容器内调用 FFmpeg 内置源生成测试素材。
2. fixture 至少包含：
   - 横屏视频；
   - 普通竖屏/其他宽高比视频；
   - 带 rotation metadata 的手机竖屏样本；
   - 带素材原声音轨的视频；
   - 非整数秒口播，例如 9.7 秒；
   - 无效媒体样本；
   - 可触发素材不足的组合。
3. `scripts/e2e.sh` 在 app 容器内使用 curl 调真实 HTTP API。
4. E2E 完成 health → 上传视频 → 上传口播 → 创建 mix → 下载 → FFprobe assertions。
5. FFprobe 断言完整媒体合同：MP4、1080×1920、H.264、yuv420p、约 30fps、AAC、时长约束。
6. 验证无效媒体与素材不足错误分支。
7. 普通运行期 fixture/output 进入 Git ignore；同时生成一份与最终代码一致的真实无字幕示例成片，供 TASK-011 以 `demo/` 文件或可访问下载链接方式交付。

## 标准执行方式

```text
docker compose up --build -d
docker compose exec app sh /app/scripts/generate-fixtures.sh
docker compose exec app sh /app/scripts/e2e.sh
```

宿主机不要求 bash、curl、Go、Node、pnpm、FFmpeg 或 FFprobe。

## 交付物

- `scripts/generate-fixtures.sh`
- `scripts/e2e.sh`
- 生成产物目录的 ignore 规则
- 实际无字幕示例成片
- README 可复现说明

## 约束

- E2E 必须调用运行中的 HTTP API，不能直接调用 Go 内部函数。
- 不使用无法公开分发的第三方素材。
- 成片验证必须读取真实 FFprobe 输出。
- 测试不能依赖宿主机媒体工具。
- rotation fixture 必须能证明 autorotate + scale/pad 后显示方向正确。

## 验收条件

- [ ] 从空 data 开始执行 `docker compose up --build -d` 成功。
- [ ] 容器内 fixture 脚本生成全部素材。
- [ ] E2E 完成上传 → 选择 → 混剪 → 下载。
- [ ] 下载结果满足 MP4 / H.264 / yuv420p / 1080×1920 / ≈30fps / AAC。
- [ ] `abs(Tvideo-Ttarget) <= 100ms`。
- [ ] `abs(Taudio-Ttarget) <= 100ms`。
- [ ] `abs(Tformat-Ttarget) <= 100ms`。
- [ ] `abs(Tvideo-Taudio) <= 100ms`。
- [ ] rotation 样本输出方向与比例正确。
- [ ] 带素材原声音轨的输入最终只输出预期单一 AAC 音轨。
- [ ] 无效媒体返回明确错误。
- [ ] 素材不足返回明确错误且没有伪成功成片。
- [ ] 整套 E2E 连续执行至少两次均通过。
- [ ] Git 中不包含普通运行期 fixture/output 二进制；若示例成片最终选择随源码提交，只允许明确的 `demo/` 交付资产进入版本库。

## 验收证据

- Commit：
- Docker 启动：
- Fixture：
- E2E run #1：
- E2E run #2：
- FFprobe 关键输出：
- rotation：
- 失败分支：
- 实际示例成片路径/下载交付候选：
