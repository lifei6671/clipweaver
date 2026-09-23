# ClipWeaver 技术设计 v0.1

> 状态：开发基线  
> 目标：完成“本地视频混剪工具”笔试题基础要求，并保证从源码可通过 Docker Compose 一键构建、运行和验证。  
> 范围：基础混剪流程；自动字幕暂不纳入 v0.1。
>
> 原始需求与不可削弱的验收边界见 [requirements-baseline.md](requirements-baseline.md)。若本文、任务卡或实现与需求基线冲突，以需求基线为准，相关任务必须先进入 `BLOCKED` 并完成修正。

## 1. 目标与验收边界

ClipWeaver 是一个本地运行的 Web 应用。用户上传多段视频素材和一段口播音频，选择参与混剪的视频后发起制作，系统按照口播真实时长规划不重叠视频切片，随机排列并拼接为 1080×1920 竖屏 MP4，以口播替换素材原声，完成后支持网页预览和下载。

v0.1 必须满足：

1. 支持一次上传多个视频和一段口播音频。
2. 展示素材名称、媒体时长和上传状态。
3. 使用 FFprobe 校验真实媒体流，不能只依赖扩展名或 MIME Type。
4. 用户可以选择本次参与混剪的视频。
5. 同一视频允许使用多个切片，同一次制作中使用区间不能重叠。
6. 使用确定性随机策略；给定相同素材元信息、目标时长、参数和 seed 时必须生成相同 MixPlan。
7. 成片目标时长由口播实际时长决定，最后一个规划切片允许截短。
8. 所选视频可用总时长不足时立即返回明确错误，不循环使用素材补足。
9. 成片输出为 1080×1920 MP4，保持原视频比例，剩余区域使用黑边填充。
10. 素材原声全部移除，只保留上传口播。
11. 最终输出通过 FFprobe 二次验收；成片 video stream、audio stream、container 三个时长分别与口播目标时长的差值均不得超过 100ms，同时 video/audio 之间的差值不得超过 100ms，并避免明显语音缺尾。
12. 成片可在线播放和下载；失败时展示稳定错误码与用户可理解的信息，并允许重新制作。
13. 使用本地 FFmpeg/FFprobe，应用整体通过 Docker 与 Docker Compose 交付。
14. 从只安装 Docker 和 Docker Compose 的新环境执行 `docker compose up --build -d` 可以构建并运行。
15. 提供核心规则自动化测试、真实 FFmpeg 集成测试、演示素材生成脚本、README 和 AI-NOTES.md。

v0.1 明确不包含：

- 用户登录与权限系统
- 任务管理系统
- 任务队列、Redis、消息中间件
- 转场
- 字幕编辑
- 专业时间线
- 云存储
- 高并发
- 在线部署
- 多租户
- 自动字幕
- 数据库

其中登录、任务管理系统、转场、字幕编辑、专业时间线、云存储、高并发和线上部署属于原题明确不要求项；数据库、多租户、自动字幕属于 ClipWeaver v0.1 主动裁剪范围。

## 2. 设计原则

### 2.1 规划与执行分离

核心链路固定为：

```text
Upload
  ↓
FFprobe
  ↓
Mix Planner
  ↓
MixPlan
  ↓
FFmpeg Executor
  ↓
FFprobe Validator
  ↓
Preview / Download
```

Mix Planner 只处理纯业务数据，不调用 FFmpeg、不访问 HTTP、不写文件。  
FFmpeg Executor 不参与随机选择和切片策略，只执行已经冻结的 MixPlan。

这样可以使最重要的混剪规则在没有真实媒体文件的情况下完成确定性单元测试，并让 FFmpeg 问题与业务规划问题独立定位。

### 2.2 时间统一使用整数微秒

领域层禁止使用 float64 表达时间。

```go
type DurationUS int64
```

所有开始时间、结束时间和持续时间统一为微秒：

```text
1s = 1_000_000us
```

领域结构体中的时长字段统一使用 `DurationUS`，不得退回裸 `float64` 秒值。

FFprobe 的时长进入领域层时采用固定解析优先级：

1. 选定 stream 存在有效 `duration_ts + time_base` 时优先使用；
2. 否则使用选定 stream 的 `duration`；
3. 再否则使用 `format.duration`；
4. 仍无法得到正时长则判定媒体无效。

十进制秒数或 `time_base` 必须按十进制/有理数精确解析，再四舍五入到最近微秒（round-to-nearest microsecond）；禁止先转换为 `float64` 再累计计算。

只有调用 FFmpeg 或向前端序列化展示值时才转换表示形式，避免浮点累计误差影响 100ms 验收边界。

### 2.3 随机必须可复现

每次混剪具有显式 seed：

- 领域层 seed 使用 `int64`。
- HTTP JSON 边界的 seed 使用十进制字符串，避免 JavaScript Number 超过安全整数范围后丢精度。
- API 未提供 seed 时，由服务端生成。
- API 提供 seed 字符串时，在 HTTP boundary 严格解析为 `int64`，非法或越界值返回 400。
- MixPlan、mix 元数据和日志中都保存同一逻辑 seed；HTTP 响应再次序列化为字符串。
- Planner 使用 `rand.New(rand.NewSource(seed))` 创建局部随机源，再自行执行 Fisher-Yates Shuffle。
- Go 版本由 Dockerfile 锁定；禁止使用包级全局随机源驱动 Planner。

### 2.4 本地单用户优先

题目按短素材设计并允许同步处理，因此 v0.1 使用同步请求模型。  
`POST /api/mixes` 在请求内完成规划、转码、拼接、音频替换和验收；前端在请求期间展示“制作中”。

不引入后台任务和轮询协议。

同步模型同时冻结以下运行边界：

- 渲染总超时可配置，例如 `MIX_TIMEOUT`，并通过 `context.WithTimeout` 传递给 FFmpeg `CommandContext`。
- 服务端设置简单的 in-flight semaphore；v0.1 默认最大并发混剪数为 1，超限返回 `MIX_BUSY`。
- 应用优雅退出时主动取消在途渲染 Context。
- Fiber/fasthttp 的请求 Context 不作为“客户端断线即取消 FFmpeg”的保证；v0.1 依赖显式渲染超时和服务关闭取消。

## 3. 技术栈

### 后端

- Go
- Fiber
- 标准库 `os/exec` 调用 FFmpeg / FFprobe
- 标准库 JSON 作为元数据格式

### 前端

- React
- TypeScript
- Ant Design
- pnpm + Corepack
- 单页应用，不引入复杂路由状态

### 媒体

- FFmpeg / FFprobe
- H.264 + AAC
- MP4
- yuv420p
- 1080×1920
- 固定输出帧率：30fps

### 交付

- 多阶段 Dockerfile
- Docker Compose
- 单业务容器
- 挂载本地 data volume 保存上传文件与成片

Go、Node、pnpm、FFmpeg/FFprobe 与应用依赖的精确版本在 TASK-001 / TASK-009 中锁定；前端包管理器统一使用 pnpm，不允许宿主机、Docker 与文档使用不同包管理器。最终验收环境不依赖宿主机 Go、Node、pnpm、FFmpeg、FFprobe 或 curl。

## 4. 总体架构

```text
┌──────────────────────────────┐
│ React + TypeScript + AntD    │
│ Upload / Select / Mix / View │
└──────────────┬───────────────┘
               │ HTTP
┌──────────────▼───────────────┐
│ Go + Fiber                   │
│                              │
│  HTTP Handlers               │
│       │                      │
│  Application Service         │
│   ├─ Asset Service           │
│   └─ Mix Service             │
│       │                      │
│  ┌────▼───────────────────┐  │
│  │ Domain / Mix Planner   │  │
│  └────┬───────────────────┘  │
│       │ MixPlan              │
│  ┌────▼───────────────────┐  │
│  │ Media                  │  │
│  │ FFprobe / FFmpeg       │  │
│  │ Executor / Validator   │  │
│  └────┬───────────────────┘  │
│       │                      │
│  Local File Storage          │
└──────────────────────────────┘
```

依赖方向：

```text
HTTP → Application → Domain
             │
             ├→ Storage
             └→ Media

Domain 不依赖 HTTP / Storage / Media
```

## 5. 领域模型

### 5.1 Asset

```go
type AssetKind string

const (
    AssetKindVideo AssetKind = "video"
    AssetKindAudio AssetKind = "audio"
)

type Asset struct {
    ID         string
    Kind       AssetKind
    Name       string
    StoredPath string
    DurationUS DurationUS
    Width      int
    Height     int
    CreatedAt  time.Time
}
```

### 5.2 MixRequest

```go
type MixRequest struct {
    VideoIDs []string
    AudioID  string
    Seed     *int64
}
```

### 5.3 PlannedClip

采用半开区间 `[StartUS, StartUS + DurationUS)`：

```go
type PlannedClip struct {
    AssetID    string
    StartUS    DurationUS
    DurationUS DurationUS
}
```

### 5.4 MixPlan

```go
type MixPlan struct {
    Seed             int64
    TargetDurationUS DurationUS
    ClipDurationUS   DurationUS
    Clips            []PlannedClip
}
```

必须保持的领域不变量：

1. `TargetDurationUS > 0`。
2. `len(Clips) > 0`。
3. 每个 clip 的 `StartUS >= 0` 且 `DurationUS > 0`；最后一次截短后的 remaining 必须至少为 1µs，否则不得生成零时长 clip。
4. 每个 clip 都落在对应源视频实际时长内。
5. 同一 AssetID 的任意两个 clip 时间区间不重叠。
6. `sum(Clips.DurationUS) == TargetDurationUS`。
7. Planner 不重复使用同一个候选区间。

## 6. 混剪规划算法

### 6.1 基线策略

v0.1 固定为：

```text
固定分片
  ↓
候选池
  ↓
Seeded Shuffle
  ↓
顺序消费
  ↓
最后一片按剩余时长截短
```

默认 `clipDurationUS = 3_000_000`。

例如 10 秒视频拆分为：

```text
[0,3)
[3,6)
[6,9)
[9,10)
```

所有候选区间在构建时已经互斥，因此 Planner 无需运行时进行随机区间碰撞检测，也天然满足“不重叠”要求。

### 6.2 算法步骤

输入：

- 已选视频及其真实 DurationUS
- 口播 TargetDurationUS
- ClipDurationUS
- Seed

步骤：

1. 校验至少选择一个视频。
2. 校验口播时长大于 0。
3. 计算所有选中视频可用总时长。
4. 若可用总时长小于目标时长，返回 `INSUFFICIENT_VIDEO_DURATION`。
5. 按固定时长将每个视频完整划分为互不重叠候选片段，末尾不足固定长度的区间也进入候选池。
6. 使用 `rand.New(rand.NewSource(seed))` 创建局部随机源，并对候选池执行 Fisher-Yates Shuffle。
7. 按打乱后的顺序消费候选片段。
8. 若某候选片段长度大于剩余目标时长，只取其前 `remaining` 微秒并结束。
9. 构造 MixPlan 并执行领域不变量自检。
10. 持久化 seed 和 MixPlan 后进入 FFmpeg 阶段。

### 6.3 确定性要求

为避免调用方传入视频 ID 顺序影响结果，进入 Planner 后先按 AssetID 稳定排序，再建立候选池。

给定相同：

- 视频 ID 与时长
- 口播时长
- clipDurationUS
- seed

MixPlan 必须完全一致。

## 7. 上传与媒体探测

### 7.1 安全边界

- 后端生成 Asset ID 和实际存储文件名。
- 原始文件名仅作为展示元数据，不能参与磁盘路径拼接。
- 不根据扩展名判断媒体有效性。
- FFmpeg/FFprobe 使用 `exec.CommandContext`，不经过 shell。
- 临时路径全部由服务端生成。
- 上传大小上限通过配置提供。
- Probe 和 Render 设置 Context 超时。
- assetId / mixId 均由服务端生成 UUID；HTTP 路径参数进入存储层前必须按 UUID 解析校验，不能直接拼接路径。
- 上传文件先写入服务端生成的 `data/tmp/uploads/<upload-id>/` staging 路径；Probe 与校验成功后再原子 rename/promotion 到正式 asset 目录，失败时删除 staging。
- 使用 Fiber 上传体积上限与 multipart 临时文件机制即可，v0.1 不自研 multipart 流式解析器。

### 7.2 Stream 选择与视频校验

FFprobe 解析多路 stream 时使用固定规则：同类型 stream 中优先选择 `disposition.default=1` 的最低 index stream；不存在 default 时选择最低 index stream。

视频资产至少验证：

- 存在可选定的 video stream；
- duration > 0；
- width > 0；
- height > 0。

真实手机视频可能使用编码宽高 + rotation/display matrix 表示显示方向。Executor 依赖 FFmpeg CLI 默认 autorotate，在转码 filter 链进入 scale/pad 前应用显示旋转；实现禁止使用 `-noautorotate`。

### 7.3 音频校验与口播目标时长

音频资产至少验证：

- 存在可选定的 audio stream；
- duration > 0。

`POST /api/assets/audio` 的语义是“创建 audio asset”。容器中允许存在非音频 stream，但后续 Mix 只使用按上述规则选定的 audio stream，其余 stream 忽略。

口播上传成功时，将选定 audio stream 按 §2.2 规则解析得到的时长冻结为该资产的 `DurationUS`；它是后续 `TargetDurationUS` 的唯一来源。

前端展示的媒体时长统一使用服务端 FFprobe 结果。

## 8. 本地存储

不使用数据库，采用“一资源一目录 + manifest”：

```text
data/
├─ assets/
│  └─ <asset-id>/
│     ├─ source.bin
│     └─ meta.json
├─ mixes/
│  └─ <mix-id>/
│     ├─ meta.json
│     ├─ plan.json
│     └─ output.mp4
└─ tmp/
   ├─ uploads/
   │  └─ <upload-id>/
   └─ mixes/
      └─ <mix-id>/
         ├─ clips/
         └─ concat.txt
```

约束：

- manifest 采用临时文件 + rename 原子写入。
- 上传、混剪完成或失败后清理对应 tmp 目录。
- 应用启动时扫描 `data/tmp/uploads` 与 `data/tmp/mixes` 并清理孤儿目录；v0.1 不恢复中断中的渲染任务。
- assets 与 mixes 通过 Docker volume 持久化。
- 应用重启后可以依据 manifest 访问既有文件，不依赖纯内存索引。
- Docker 运行用户必须对挂载 data volume 具备读写权限，并在 TASK-009 验收。
- v0.1 支持单容器、单进程，不解决多进程并发写入。

## 9. FFmpeg 执行流水线

### 9.1 阶段 A：标准化切片

每个 PlannedClip 独立生成临时无声视频。Planner 的半开区间 `[start, end)` 必须映射为解码后的帧精确 filter 裁剪：

```text
FFmpeg default autorotate
  ↓
trim=start=<start>:end=<end>
  ↓
setpts=PTS-STARTPTS
  ↓
scale=1080:1920:force_original_aspect_ratio=decrease
  ↓
pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black
  ↓
fps=30
  ↓
format=yuv420p
```

禁止使用依赖关键帧的 stream-copy 快速裁剪来实现 PlannedClip；所有用于成片的 clip 都经过解码、filter 与重新编码。

统一输出：

- 1080×1920
- 30fps
- H.264
- yuv420p
- 无音轨

v0.1 优先可诊断性，不追求单条巨大 `filter_complex`。

### 9.2 阶段 B：拼接

生成 concat 清单，按照 MixPlan 顺序拼接为 `silent.mp4`。

所有临时片段先统一编码参数，再使用 concat demuxer。实现阶段必须通过集成测试验证时间戳和末片时长。

### 9.3 阶段 C：替换口播

将 `silent.mp4` 与上传口播合并：

- 仅映射 silent 视频的 video stream。
- 仅映射口播选定的 audio stream。
- 原素材音轨不进入最终输出。
- 口播音频进入 mux 前执行 `asetpts=PTS-STARTPTS`。
- 视频端允许使用 `tpad=stop=-1:stop_mode=clone` 让最后一帧继续覆盖尾部，避免视频先结束截断口播。
- 最终输出使用显式 `-t <TargetDuration>` 作为硬时长上界；不把 `-shortest` 作为核心对齐机制。
- 音频统一编码 AAC。
- MP4 固定添加 `-movflags +faststart`，保证浏览器预览时 moov 索引前置。
- 逻辑 MixPlan 始终保持精确 `TargetDurationUS`，tpad 仅用于渲染容错，不回写 Planner。

## 10. 输出验收

FFmpeg 退出码为 0 只表示命令成功，不能直接标记业务成功。

生成后必须再次 FFprobe，并按照与上传 probe 相同的 stream 选择和 duration 解析规则得到：

```text
Ttarget = 源口播资产冻结的 TargetDurationUS
Tvideo  = 成片选定 video stream duration
Taudio  = 成片选定 audio stream duration
Tformat = 成片 container format duration
```

Validator 必须断言：

1. 文件存在且大小 > 0，容器为 MP4。
2. 恰好存在预期的主 video/audio 输出流，且可按固定规则选定。
3. video codec = H.264。
4. width = 1080，height = 1920。
5. pixel format = yuv420p。
6. 帧率约为 30fps；实现可按有理数解析 `avg_frame_rate` / `r_frame_rate` 并允许极小表示误差。
7. audio codec = AAC。
8. `abs(Tvideo - Ttarget) <= 100ms`。
9. `abs(Taudio - Ttarget) <= 100ms`。
10. `abs(Tformat - Ttarget) <= 100ms`。
11. `abs(Tvideo - Taudio) <= 100ms`。

FFmpeg 命令构建测试还必须证明 mux 阶段只映射 silent video 与 narration audio，不映射素材原声；集成测试使用带独立音轨的视频素材验证最终只存在一个 AAC 音轨。

任一断言不满足时返回 `RENDER_VALIDATION_FAILED`。

最终交付前还需要人工抽样确认口播尾部无明显缺失，并在 README 记录实际验证结果。

## 11. HTTP API

统一前缀：`/api`。

### GET /api/health

返回应用健康状态和 FFmpeg/FFprobe 可用状态。

### POST /api/assets/videos

- multipart/form-data
- 支持多个视频
- 单文件独立 staging、probe、校验和 promotion
- 返回成功资产和失败项
- 前端展示 uploading / ready / failed
- 请求成功解析后统一返回 HTTP 200，每个文件在 `items` 内独立给出 `ready/failed` 状态

混合成功/失败示例：

```json
{
  "items": [
    {
      "filename": "a.mp4",
      "status": "ready",
      "asset": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "kind": "video",
        "name": "a.mp4",
        "durationUs": 8000000,
        "width": 1920,
        "height": 1080
      }
    },
    {
      "filename": "bad.mp4",
      "status": "failed",
      "error": {
        "code": "INVALID_MEDIA",
        "message": "未检测到有效视频流"
      }
    }
  ]
}
```

### POST /api/assets/audio

- multipart/form-data
- 一次创建一个口播 audio asset
- 服务端允许历史上存在多个 audio asset；“替换口播”只表示前端把当前 `selectedAudioId` 切换到新上传资产，不维护全局默认口播，也不自动删除旧资产

### GET /api/assets

返回本地可用资产元数据，例如：

```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "kind": "video",
      "name": "a.mp4",
      "durationUs": 8000000,
      "width": 1920,
      "height": 1080
    },
    {
      "id": "1c6cde7d-1f8c-4d18-b2bd-490cc7fb4216",
      "kind": "audio",
      "name": "narration.wav",
      "durationUs": 9700000,
      "width": 0,
      "height": 0
    }
  ]
}
```

`durationUs` 对短素材使用 JSON number；它在本题范围远小于 JavaScript 安全整数上限。

### POST /api/mixes

请求：

```json
{
  "videoIds": [
    "550e8400-e29b-41d4-a716-446655440000",
    "1b7b48ac-2747-4a55-8e2a-1e8b6b5f27ab"
  ],
  "audioId": "1c6cde7d-1f8c-4d18-b2bd-490cc7fb4216",
  "seed": "9527"
}
```

`seed` 可选；JSON 中固定使用十进制字符串。未提供时服务端生成，响应仍以字符串返回。

同步成功响应：

```json
{
  "id": "20bf0d09-8aa2-4cf8-84fb-2f9abcc07833",
  "status": "completed",
  "seed": "9527",
  "durationUs": 9700000,
  "previewUrl": "/api/mixes/20bf0d09-8aa2-4cf8-84fb-2f9abcc07833/file",
  "downloadUrl": "/api/mixes/20bf0d09-8aa2-4cf8-84fb-2f9abcc07833/download"
}
```

### GET /api/mixes/:id/file

- `Content-Type: video/mp4`
- 支持浏览器 Range
- 默认 inline

### GET /api/mixes/:id/download

- `Content-Type: video/mp4`
- `Content-Disposition: attachment`

## 12. 错误模型

统一结构：

```json
{
  "error": {
    "code": "INSUFFICIENT_VIDEO_DURATION",
    "message": "所选视频可用总时长不足，至少还缺少 4.4 秒素材。",
    "details": {}
  }
}
```

首版稳定错误码及 HTTP 映射：

| code | HTTP |
|---|---:|
| `INVALID_MEDIA` / `INVALID_VIDEO` / `INVALID_AUDIO` | 400 |
| `INVALID_ID` / `INVALID_SEED` | 400 |
| `NO_VIDEO_SELECTED` / `AUDIO_REQUIRED` | 400 |
| `ASSET_NOT_FOUND` | 404 |
| `UPLOAD_TOO_LARGE` | 413 |
| `INSUFFICIENT_VIDEO_DURATION` | 422 |
| `MIX_BUSY` | 429 |
| `MIX_TIMEOUT` | 504 |
| `FFPROBE_FAILED` / `FFMPEG_FAILED` / `RENDER_VALIDATION_FAILED` | 500 |
| `INTERNAL_ERROR` | 500 |

多视频上传中单文件媒体校验失败属于 `items[*].status=failed`，整个 multipart 请求成功解析时仍返回 200；请求级错误（例如 BodyLimit）按上表返回。

前端只展示稳定业务信息；完整 FFmpeg stderr 进入服务端日志。

## 13. 日志

至少记录：

- requestId
- mixId
- assetId（适用时）
- seed（混剪时）
- phase：probe / plan / normalize / concat / mux / validate
- elapsedMs
- errorCode

禁止记录上传文件二进制内容。

## 14. 前端交互

v0.1 使用单页面：

```text
视频素材
  ├─ 上传多个视频
  ├─ 名称 / 时长 / 状态
  └─ Checkbox 选择

口播音频
  ├─ 上传 / 选择当前 selectedAudioId
  └─ 名称 / 时长

操作区
  └─ 开始混剪

结果区
  ├─ <video controls>
  ├─ 重新制作
  └─ 下载 MP4
```

交互约束：

- 混剪中禁止重复提交。
- 服务端错误通过 Alert/Message 明确展示。
- 素材不足时展示缺少的可用时长。
- 失败后保留素材选择，可直接重新制作。
- 不增加登录、复杂导航和重量级状态管理。
- 页面刷新后恢复 assets，但 v0.1 不恢复上一次页面中的 mix 结果展示；这属于 README 已知限制。

## 15. 自动化测试

### 15.1 Planner 单元测试

必须至少包含：

- `TestPlan_NoOverlapPerVideo`
- `TestPlan_TruncatesLastClip`
- `TestPlan_TotalDurationEqualsTarget`
- `TestPlan_InsufficientDuration`
- `TestPlan_SameSeedIsDeterministic`
- `TestPlan_DifferentSeedCanChangeOrder`
- `TestPlan_SameAssetCanProduceMultipleClips`

`DifferentSeed` 的测试输入必须保证候选片段数 >= 2，并使用预先验证的 seed 对，避免概率性偶发失败。

其中不重叠检查对同一 AssetID 的任意两个半开区间验证：

```text
end1 <= start2 || end2 <= start1
```

测试重点验证性质，不依赖随机结果中某个固定片段恰好排在某一位置。

### 15.2 FFprobe / Media 单元测试

覆盖：

- JSON 解析
- 多路 stream 的 default/index 选择规则
- `duration_ts + time_base → stream.duration → format.duration` fallback
- 十进制/有理数 round-to-nearest 微秒
- 缺 video/audio stream
- duration 无效
- duration → 微秒转换

### 15.3 FFmpeg 集成测试

使用 FFmpeg 内置源生成可公开复现素材：

- `testsrc`
- `color`
- `sine`

至少包含多个不同比例/时长视频、一段带素材原声音轨的视频、一个带 rotation metadata 的手机竖屏样本，以及一段非整数秒口播，例如 9.7s。

执行完整 Executor 后用 FFprobe 断言：

- MP4
- 1080×1920
- H.264
- yuv420p
- 约 30fps
- 单一 AAC 成片音轨
- video/audio/container 三种时长分别与 TargetDurationUS 差值 <= 100ms
- video/audio 互差 <= 100ms
- rotation 样本保持正确显示方向和宽高比

### 15.4 Docker E2E 冒烟测试

最终验收机只假设存在 Docker 与 Docker Compose。fixture 生成、HTTP 调用与 FFprobe 断言都在容器内执行，不要求宿主机安装 bash、curl、Go、Node、pnpm、FFmpeg 或 FFprobe。

标准流程：

```text
docker compose up --build -d
  ↓
docker compose exec app sh /app/scripts/generate-fixtures.sh
  ↓
docker compose exec app sh /app/scripts/e2e.sh
      ├─ health
      ├─ upload videos
      ├─ upload audio
      ├─ create mix
      ├─ download
      └─ ffprobe assertions
```

只有完整 Docker 链路通过，最终版本才算可交付。

题目要求提供“实际生成的无字幕成片”。运行期普通 output/fixture 仍保持 Git ignore；最终交付必须另外提供一份与最终代码一致的真实示例成片，可选择：

- 作为明确的 `demo/` 交付资产随源码提供；或
- 在 README 提供可访问的下载链接。

不能只用“可复现脚本”替代这份实际成片。

## 16. Docker 设计

单业务容器，多阶段构建：

```text
Node builder
   ↓ Corepack + pinned pnpm
   ↓ frontend tests
   ↓ React build

Go builder
   ↓ go test ./...
   ↓ server binary

Runtime
   ├─ ffmpeg / ffprobe
   ├─ Go binary
   └─ React static files
```

Go 服务同时提供：

- `/api/*`
- React 静态资源
- SPA fallback

避免额外 Nginx、跨容器 CORS 和多服务网络配置。

Compose 至少包含：

- 单 app service
- data volume
- healthcheck
- 显式端口
- 可配置数据目录、上传大小、日志级别、混剪总超时和最大并发数
- runtime 镜像包含最终 E2E 所需的 `sh`、`curl`、FFmpeg/FFprobe 与 scripts
- builder 阶段执行 Go/前端测试，因此“干净验收环境”不要求宿主机安装 Go/Node/pnpm

## 17. 建议目录结构

```text
clipweaver/
├─ cmd/
│  └─ server/
│     └─ main.go
├─ internal/
│  ├─ domain/
│  │  ├─ asset.go
│  │  └─ mix.go
│  ├─ mixer/
│  │  ├─ planner.go
│  │  └─ planner_test.go
│  ├─ media/
│  │  ├─ ffprobe.go
│  │  ├─ ffmpeg.go
│  │  ├─ executor.go
│  │  └─ validator.go
│  ├─ storage/
│  │  └─ local.go
│  ├─ service/
│  │  ├─ asset.go
│  │  └─ mix.go
│  └─ httpapi/
│     ├─ handlers.go
│     ├─ errors.go
│     └─ routes.go
├─ web/
│  ├─ src/
│  └─ package.json
├─ scripts/
│  ├─ generate-fixtures.sh
│  └─ e2e.sh
├─ testdata/
├─ docs/
│  ├─ technical-design.md
│  └─ tasks/
│     ├─ README.md
│     └─ TASK-001～TASK-011 任务文档
├─ Dockerfile
├─ docker-compose.yml
├─ .dockerignore
├─ README.md
└─ AI-NOTES.md
```

运行期 `data/` 不进入 Git。

## 18. 开发任务

开发任务的状态、依赖关系和验收规则统一维护在 [任务索引](tasks/README.md)。任务已经拆到可以单独下发 Agent、单独记录状态并单独验收的粒度：

1. [TASK-001：工程骨架与运行基线](tasks/TASK-001.md)
2. [TASK-002：领域模型与确定性 Mix Planner](tasks/TASK-002.md)
3. [TASK-003：本地存储与 FFprobe 媒体探测](tasks/TASK-003.md)
4. [TASK-004：素材上传与查询 API](tasks/TASK-004.md)
5. [TASK-005：FFmpeg 渲染执行器与输出验收](tasks/TASK-005.md)
6. [TASK-006：Mix 编排服务与 HTTP API](tasks/TASK-006.md)
7. [TASK-007：前端素材上传与选择流程](tasks/TASK-007.md)
8. [TASK-008：前端混剪、预览与下载流程](tasks/TASK-008.md)
9. [TASK-009：Docker 交付与运行配置](tasks/TASK-009.md)
10. [TASK-010：可复现测试素材与 Docker E2E](tasks/TASK-010.md)
11. [TASK-011：README、AI 协作记录与最终交付](tasks/TASK-011.md)

每个任务卡必须包含明确依赖、实施范围、交付物、验收条件和验收证据。任务只有在依赖全部通过且自身验收证据完整后才能标记为 `PASS`。

## 19. AI 协作记录

AI 可以生成代码、测试、文档和命令，但每个阶段必须由可观察证据验收。

开发过程中保留真实发生的 2～3 个关键决策或纠偏到 `AI-NOTES.md`：

- 背景
- AI 建议
- 最终采用方案
- 采用依据
- 对应测试 / 日志 / 提交

不为满足记录要求编造错误或纠偏。

## 20. 完成定义

一个开发阶段只有同时满足以下条件才算完成：

1. 代码已落盘。
2. 对应自动化测试实际执行通过。
3. 涉及媒体输出时，真实 FFmpeg/FFprobe 集成验证通过。
4. 错误分支有可观察结果。
5. Docker 行为未被破坏。
6. 文档与实现保持一致。
7. 最终阶段实际执行完整 Docker E2E。
8. [requirements-baseline.md](requirements-baseline.md) 中适用于 v0.1 的全部原始要求均已有实现与验收证据。
9. clean-room 最终验收期间没有临时修改源码、Docker 配置或测试脚本。

最终交付判断依据是从源码在干净 Docker 环境完成：

```text
上传 → 选择 → 混剪 → 预览/下载 → FFprobe 验收
```

单元测试和示例成片都不能替代这条完整流程。
