# TASK-010：可复现测试素材与 Docker E2E

- 状态：以 [任务索引](README.md) 为准
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
- 需求基线：[requirements-baseline.md](../requirements-baseline.md)，不可削弱
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
- 供 TASK-011 整合进最终 README 的可复现命令与验收结果

## 约束

- E2E 必须调用运行中的 HTTP API，不能直接调用 Go 内部函数。
- 不使用无法公开分发的第三方素材。
- 成片验证必须读取真实 FFprobe 输出。
- 测试不能依赖宿主机媒体工具。
- rotation fixture 必须能证明 autorotate + scale/pad 后显示方向正确。

## 验收条件

- [x] 从空 data 开始执行 `docker compose up --build -d` 成功。
- [x] 容器内 fixture 脚本生成全部素材。
- [x] E2E 完成上传 → 选择 → 混剪 → 下载。
- [x] 下载结果满足 MP4 / H.264 / yuv420p / 1080×1920 / ≈30fps / AAC。
- [x] `abs(Tvideo-Ttarget) <= 100ms`。
- [x] `abs(Taudio-Ttarget) <= 100ms`。
- [x] `abs(Tformat-Ttarget) <= 100ms`。
- [x] `abs(Tvideo-Taudio) <= 100ms`。
- [x] rotation 样本输出方向与比例正确。
- [x] 带素材原声音轨的输入最终只输出预期单一 AAC 音轨。
- [x] 无效媒体返回明确错误。
- [x] 素材不足返回明确错误且没有伪成功成片。
- [x] 整套 E2E 连续执行至少两次均通过。
- [x] Git 中不包含普通运行期 fixture/output 二进制；若示例成片最终选择随源码提交，只允许明确的 `demo/` 交付资产进入版本库。

## 验收证据

- Commit：`9696f87e11f8be4dec5f2525d050a059c5dccaf4`；实现、真实 Docker E2E 与示例成片已提交，人工 Review 已通过，全部验收条件满足，状态为 `PASS`。
- Docker 启动：最终脚本收紧 rotation 颜色顺序断言后，执行 `docker volume inspect clipweaver-task010-final_app-data`，确认该 volume 不存在；设置 `HOST_PORT=18089` 后执行 `docker compose -p clipweaver-task010-final up --build -d`，退出 0，新建单 app 容器、网络和空 named volume。后续正式 fixture、HTTP、FFprobe 操作均在容器中完成，不使用宿主机媒体工具。容器以 uid/gid 10001 运行，脚本位于 `/app/scripts/` 且 mode 755，`sh -n` 两脚本均通过。
- Fixture：`docker compose -p clipweaver-task010-final exec -T app sh /app/scripts/generate-fixtures.sh` 退出 0，生成 4.2 秒横屏、3.6 秒普通竖屏、4.5 秒自带 1200 Hz AAC 原声视频、3.7 秒 rotation 视频、9.7 秒非整数秒 WAV 口播、2.3 秒短 WAV 和无效 MP4 文本样本。rotation 样本编码尺寸 640×360，FFprobe display matrix rotation=90；首次试生成使用 `rotate` metadata 未写入 display matrix，已改用 FFmpeg `-display_rotation:v:0 90` 并从新卷重验。生成目录 `/app/data/fixtures/` 位于已有 Git ignore 的 `data/` 对应 volume 中。
- E2E run #1：`docker compose -p clipweaver-task010-final exec -T app sh /app/scripts/e2e.sh` 退出 0，证据目录 `/app/data/e2e/run.hFAyTg`；4 个视频、2 个音频均经真实 HTTP 上传，seed `42`，主 mixId `37227820-ef65-4dcf-ad24-9eb31bd72c6a`，HTTP 200/completed，下载 MP4、Range 预览 206 均成功。
- E2E run #2：相同命令紧接再运行，退出 0，证据目录 `/app/data/e2e/run.PbQNvA`；主 mixId `81cc8709-417c-40b9-93c9-7d350c39cdda`，全部断言再次通过。两次均独立创建资产和 mix，未复用第一轮结果。
- FFprobe 关键输出：两轮主成片均为 MP4、恰好一条 H.264 video（1080×1920、yuv420p、30/1 fps）和一条 AAC audio、无字幕流。HTTP 音频资产冻结 `Ttarget=9.700000s`；独立 FFprobe 得 `Tvideo=Taudio=Tformat=9.700000s`，四项误差均 `0ms`。E2E 脚本逐项解析真实下载文件的 FFprobe stream/container duration，断言各误差 ≤100ms，而非只依赖服务内 Validator。
- rotation：专门将只含 rotation metadata 的横向编码样本作为唯一视频经真实 HTTP 混剪，输出 1080×1920；源画面左红右蓝，metadata=90 后实际输出上蓝下红，抽样画面上、下中央 YAVG 分别为 41、81。脚本同时断言上下非黑及蓝在红上，能够识别未旋转或反向旋转。另将仅含 1200 Hz 素材原声的视频与 440 Hz 短口播混剪，成片只有一条 AAC；带通检测成片 440 Hz 为 −21.1 dB、1200 Hz 为 −52.1 dB，支持原声未混入的判断。
- 失败分支：无效文本伪装 MP4 的上传 HTTP 200、单项 `failed/FFPROBE_FAILED`；仅选 3.6 秒竖屏视频配 9.7 秒口播的混剪 HTTP 422、`INSUFFICIENT_VIDEO_DURATION`、`missingDurationUs=6100000`，无 completed 伪成功响应。
- 实际示例成片路径/下载交付候选：`demo/clipweaver-real-sample.mp4`，大小 29,284,239 bytes，SHA-256 `8edcebcc103c8ea88de8cf6f663d9f082ad09263f6852dc676cb2770096bf44c`。通过只读挂载的用户允许分发素材，真实 HTTP 上传两段新增横屏 `w_7688313452255266995-hd.mp4`、`w_7687915553347915043-hd.mp4`，一段竖屏 `7671387297355924681-hd.mp4` 和完整原始口播 `7685603214562115578.mp3`；seed `42`，mixId `b7d914ef-55ba-4ea7-bf16-490b810492a2`。三段视频都出现在服务保存的实际 plan 中。经 HTTP 下载后由容器内 FFprobe 确认 MP4、H.264、1080×1920、yuv420p、30/1 fps、单一 AAC、无字幕流，FFmpeg 全片解码退出 0；`Ttarget=59.271813s`、`Tvideo=59.300000s`、`Taudio=59.271995s`、`Tformat=59.300000s`，四项误差依次为 28.187ms、0.182ms、28.187ms、28.005ms。新横屏画面抽样的顶部/中部/底部 YAVG 为 16/132.199/16，证明实际黑边。容器下载件与仓库候选文件的 SHA-256 一致。普通运行期 fixture/E2E 下载件只在 Docker volume 的 `/app/data/`，已由现有 `.gitignore` 的 `data/` 规则覆盖；仓库内仅明确 `demo/` 候选进入交付范围。
