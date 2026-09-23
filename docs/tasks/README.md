# ClipWeaver 开发任务

本文档是开发任务状态的唯一入口。所有任务首先受 [原始需求基线](../requirements-baseline.md) 约束，再受 [technical-design.md](../technical-design.md) 约束；任务卡只负责定义可执行范围、依赖、交付物和验收证据。任务卡不得削弱需求基线。

## 状态模型

每个任务只能处于以下状态之一：

- `TODO`：依赖尚未全部通过，或尚未开始。
- `IN_PROGRESS`：依赖均已 `PASS`，当前正在实施。
- `BLOCKED`：实施过程中遇到明确阻塞，必须在任务卡“验收证据”末尾追加阻塞原因、影响范围和恢复条件。
- `REVIEW`：实现已完成，验收命令已执行，等待复核。
- `PASS`：所有验收项均通过，证据已记录，可以作为后续任务的前置条件。

状态流转：

```text
TODO → IN_PROGRESS → REVIEW → PASS
          │            │
          └→ BLOCKED ←─┘
```

只有满足任务卡中的全部“验收条件”，并记录对应证据后，任务才能标记为 `PASS`。不能用“代码已写完”“看起来能跑”替代验收。

### 开发期 Docker 验收延期例外（2026-09-23）

当前 TASK-001 的实现骨架已落盘，唯一未闭环项为 Codex Windows 沙箱无法访问宿主 Docker Desktop，宿主用户已确认 `desktop-linux` Docker Server 正常。经人工确认，开发阶段允许先推进 TASK-002～TASK-008，TASK-001 继续保持 `BLOCKED`，不得伪造为 `PASS`。

该例外只改变开发顺序，不改变最终验收标准：

- TASK-002～TASK-008 可以基于已落盘的 TASK-001 工程骨架推进，并各自按任务卡完成宿主可执行测试与人工 Review。
- TASK-001 的 Docker builder、Compose 启动、health、FFmpeg/FFprobe 与根页面验收仍必须补齐。
- TASK-009 是硬门禁；开始 TASK-009 前，TASK-001 必须补齐全部 Docker 验收并标记 `PASS`。
- TASK-010 / TASK-011 的 clean-room Docker E2E 与最终交付要求保持不变。
- 除上述 TASK-001 Docker 权限阻塞外，其他依赖仍遵守“前置任务必须 PASS”规则。

## 状态总表

| ID | 任务 | 依赖 | 状态 |
|---|---|---|---|
| [TASK-001](TASK-001.md) | 工程骨架与运行基线 | - | BLOCKED |
| [TASK-002](TASK-002.md) | 领域模型与确定性 Mix Planner | TASK-001 | PASS |
| [TASK-003](TASK-003.md) | 本地存储与 FFprobe 媒体探测 | TASK-001 | PASS |
| [TASK-004](TASK-004.md) | 素材上传与查询 API | TASK-003 | PASS |
| [TASK-005](TASK-005.md) | FFmpeg 渲染执行器与输出验收 | TASK-002, TASK-003 | REVIEW |
| [TASK-006](TASK-006.md) | Mix 编排服务与 HTTP API | TASK-002, TASK-004, TASK-005 | TODO |
| [TASK-007](TASK-007.md) | 前端素材上传与选择流程 | TASK-004 | TODO |
| [TASK-008](TASK-008.md) | 前端混剪、预览与下载流程 | TASK-006, TASK-007 | TODO |
| [TASK-009](TASK-009.md) | Docker 交付与运行配置 | TASK-006, TASK-008 | TODO |
| [TASK-010](TASK-010.md) | 可复现测试素材与 Docker E2E | TASK-009 | TODO |
| [TASK-011](TASK-011.md) | README、AI 协作记录与最终交付 | TASK-010 | TODO |

## 推荐推进顺序

```mermaid
flowchart TD
    T001[TASK-001 工程骨架] --> T002[TASK-002 Mix Planner]
    T001 --> T003[TASK-003 存储与 FFprobe]
    T003 --> T004[TASK-004 素材 API]
    T002 --> T005[TASK-005 FFmpeg Executor]
    T003 --> T005
    T002 --> T006[TASK-006 Mix API]
    T004 --> T006
    T005 --> T006
    T004 --> T007[TASK-007 前端素材流程]
    T006 --> T008[TASK-008 前端混剪流程]
    T007 --> T008
    T006 --> T009[TASK-009 Docker 交付]
    T008 --> T009
    T009 --> T010[TASK-010 Docker E2E]
    T010 --> T011[TASK-011 最终交付]
```

TASK-002 与 TASK-003 可以并行；TASK-007 在 TASK-004 通过后即可推进，不必等待媒体渲染链路。

## 标准验收命令约定

最终验收环境只要求 Docker 与 Docker Compose。Go、Node、pnpm、FFmpeg、FFprobe、curl 等依赖必须由 Docker builder/runtime 提供。

稳定验收入口：

```text
docker compose config
docker compose build --no-cache
docker compose up -d
docker compose exec app sh /app/scripts/generate-fixtures.sh
docker compose exec app sh /app/scripts/e2e.sh
docker compose down
```

其中 Docker build 阶段必须实际执行 `go test ./...`、前端测试与前端 build；这些命令可以作为开发机快速反馈入口，但不能成为最终验收机的宿主机依赖。

如果实现阶段必须改变这些入口，应先更新任务卡和 README，再继续推进，避免验收命令与实现漂移。

## 每个任务必须记录的验收证据

任务进入 `REVIEW` 前，任务卡中的“验收证据”至少写入：

1. 实际修改的关键文件或目录。
2. 实际执行的测试/构建/验证命令。
3. 命令结果摘要；失败后修复过的，应记录最终通过结果。
4. 对应 Git commit hash；尚未提交时写 `pending`，标记 `PASS` 前必须补齐。
5. 涉及媒体输出的任务，记录 FFprobe 的关键输出或断言结果。
6. 涉及 Docker 的任务，记录实际 Compose 构建和运行结果。

## 任务边界规则

- 一个任务只解决任务卡列出的范围，不顺带扩展功能。
- 每次开始任务前必须阅读 `../requirements-baseline.md`；实现、测试或任务卡与需求基线冲突时，任务必须转为 `BLOCKED`。
- 技术设计可以比原题更严格，但不得通过实现便利性降低原题的功能、交付或验收要求。
- 发现设计缺口时先更新技术设计或任务卡，再继续实现。
- 后续任务不得依赖尚未 `PASS` 的前置任务；仅适用上文“开发期 Docker 验收延期例外”时，允许 TASK-002～TASK-008 暂时基于 TASK-001 已落盘骨架推进。
- 验收项必须可以通过命令、自动化测试、HTTP 响应、FFprobe 输出或明确 UI 操作复现。
- 自动字幕不属于 v0.1 基础任务。
