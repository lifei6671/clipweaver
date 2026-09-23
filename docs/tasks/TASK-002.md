# TASK-002：领域模型与确定性 Mix Planner

- 状态：以 [任务索引](README.md) 为准
- 依赖：TASK-001
- 可并行：可与 TASK-003 并行
- 目标：在纯 Go 领域层实现可复现、可证明不重叠的混剪规划算法。

## 范围

1. 定义 `DurationUS int64`，并在 Asset、PlannedClip、MixPlan 的时长字段中统一使用。
2. 定义 Asset、MixRequest、PlannedClip、MixPlan 等领域模型；领域层 seed 保持 `int64`。
3. 实现固定 3 秒候选分片，末尾不足 3 秒的正时长区间仍可作为候选。
4. 构建候选池前按 AssetID 稳定排序。
5. 使用 `rand.New(rand.NewSource(seed))` 创建局部随机源，再执行 Fisher-Yates Shuffle。
6. 按随机顺序消费候选切片，最后一个切片允许按 remaining 截短；remaining 小于 1µs 时不得创建零时长 clip。
7. 素材总时长不足时返回稳定领域错误。
8. 对 MixPlan 执行领域不变量校验。

## 交付物

- `internal/domain/asset.go`
- `internal/domain/mix.go`
- `internal/mixer/planner.go`
- `internal/mixer/planner_test.go`

## 约束

- Planner 不调用 FFmpeg/FFprobe。
- Planner 不访问文件系统、HTTP、数据库。
- 不使用包级全局随机源。
- 不通过“随机 start + 碰撞重试”实现不重叠。
- HTTP 层 seed 字符串化属于 TASK-006；Planner 只接收已经解析成功的 `int64`。
- Go 版本由 Dockerfile 锁定。

## 验收条件

- [ ] 同一视频所有已用区间均不重叠。
- [ ] 同一 AssetID 能在一条 MixPlan 中贡献多个不同 clip。
- [ ] 目标时长 7 秒、固定片长 3 秒时，规划总时长严格为 7 秒，并发生末片截短。
- [ ] 任意成功计划满足 `sum(DurationUS) == TargetDurationUS`。
- [ ] 所有 clip 满足 `DurationUS > 0` 且落在源视频时长范围内。
- [ ] 素材不足时返回 `INSUFFICIENT_VIDEO_DURATION`，不得循环复用候选片段。
- [ ] 相同输入和 seed 连续规划结果完全一致。
- [ ] DifferentSeed 测试使用候选数 >= 2 的固定输入和预先验证的 seed 对，不含概率性偶发失败。
- [ ] `go test ./internal/mixer/... -count=1` 通过，并由后续 Docker builder 全量测试再次覆盖。

## 必须存在的测试

- `TestPlan_NoOverlapPerVideo`
- `TestPlan_SameAssetCanProduceMultipleClips`
- `TestPlan_TruncatesLastClip`
- `TestPlan_TotalDurationEqualsTarget`
- `TestPlan_InsufficientDuration`
- `TestPlan_SameSeedIsDeterministic`
- `TestPlan_DifferentSeedCanChangeOrder`

## 验收证据

- Commit：
- 测试命令：
- 测试结果：
- 关键不变量说明：
