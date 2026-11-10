# 状态统一重构：后端 SSOT + Alpine/htmx 组合落地

> 日期：2026-11-10　作者：HomeCast 项目作者
> 定位：**重构设计文档**——不是踩坑记录，是「为什么重构、目标架构、怎么落地、边界与风险」。
> 触发：用户拍板「**全部统一管理，统一走后端管理状态，全嵌入打法，JS 只处理高频前端 UI**」。
> 配套：选型与架构优越性见 `2026-10-17-框架与架构优异性论证.md`（该文已明确前端就 2 个库：htmx + alpine）。

---

## 一句话目标

**业务状态（播放/队列/进度/歌词行/设备/设置）物理上唯一住在 Go 后端（SSOT），前端只发命令 + 订阅一条 SSE 状态流渲染；Alpine.js 只管「临时 UI 态」（面板开合、弹窗、输入中间值、高频拖动反馈），htmx 管权威片段替换。** 回归「业务逻辑一份住 Go 后端」的初心，消除多份状态硬拼接。

---

## ① 为什么重构（现状问题）

### 1.1 症状
- 播放按钮图标与音箱实际状态不符（已修一次）
- 静音偶发「点两次才生效」（已修一次）
- 音箱/投屏区位置错乱
- **状态管理乱到用户喊「一坨屎、错误架构」**

### 1.2 根因：同一个「播放」概念散在 3 处状态
```
【前端 web 播放器】              【前端音箱播放器】              【后端音箱】
song/state/queue/qidx          spkDid/spkState                SpeakerService.playing
playMode/audio(HTMLAudio)      （靠 spkDid 标志拼接）           /progress/devices
        └──────────┬──────────────────────────┬──────────────────────┘
                   靠 spkDid ? 双分支 + SSE 单向同步
```

- `render()`/`toggle()`/`timeupdate` 到处 `spkDid ? ... : ...` 双分支
- 状态源不唯一：web 是 `state`，音箱是 `spkState`，后端是 `progress`
- 无「播放源抽象」，切换靠副作用（`audio.pause()`+`spkDid=did`），易漏易乱

### 1.3 偏离初心
项目文档（`2026-10-17`）核心原则：**服务端是唯一事实源**、业务逻辑一份住后端。但播放状态没这么做——**web 状态留前端（HTMLAudio 必然），音箱状态也学着留前端 + 后端又一份**，成了 3 处副本靠 SSE 推。

---

## ② 目标架构（Alpine + htmx 组合技，社区已验证）

社区组合技（InfoWorld 2025-03 / itechguides / SkillDB 一致）：

| 库 | 负责 |
|----|------|
| **htmx** | 服务端状态：HTTP、校验、持久化、**权威 HTML 替换** |
| **Alpine** | 本地临时 UI 态：菜单、tab、显隐、过渡、输入绑定、**不持久化** |

**核心规则**：状态归属清晰、不双写；htmx 更新含 Alpine 的片段用 `morph` 保留 Alpine 状态。

### 2.1 目标分层

```
┌──────────────────────────────────────────────────────────┐
│  Go 后端 = 唯一状态源（SSOT）                              │
│  PlaybackState{source,bvid,playing,position,duration,     │
│                lyric_line,queue,playMode,spk_devices}     │
│  变更即持久化（SQLite）+ 广播到一条 SSE                     │
└──────────────┬──────────────────────────┬─────────────────┘
               ▲ 命令(POST)               │ 状态流(一个 SSE /api/v1/state/stream)
    ┌──────────┴──────────┐               ▼
    │ 前端（htmx 权威片段） │   ┌────────────────────────────┐
    │ Alpine 管临时 UI 态  │   │ 所有组件从这条流读同一状态渲染 │
    │ 高频：拖动/输入/开合  │   │ 播放器面板/歌词/字幕/挂件/音箱 │
    └─────────────────────┘   └────────────────────────────┘
```

### 2.2 状态归属（谁管什么）

| 状态 | 归属 | 理由 |
|------|------|------|
| 当前歌 bvid/title/artist/cover | **后端** | 业务状态 |
| 播放/暂停 playing | **后端** | 业务状态 |
| 进度 position/duration | **后端**（web 位置前端**上报**） | 唯一来源，组件一致 |
| 歌词当前行 lyric_line | **后端** | 已是后端（lyric-state SSE） |
| 队列 queue/playMode | **后端** | 业务状态，持久化 |
| 来源 web/speaker + 音箱设备/进度 | **后端** | 音箱进度已后端（零小米云） |
| 设置（音质/音量/主题） | **后端**或前端持久化 | 音量/音质可留前端 localStorage（设备偏好，非业务状态） |
| 面板开合/弹窗/tab/输入中间值 | **Alpine 临时态** | 不持久化，纯 UI |
| 拖动进度条即时反馈 | **Alpine 临时态** | 高频，本地即时，最终以 SSE 为准 |
| web 音频位置（timeupdate） | **前端上报后端** | HTMLAudio 是浏览器对象，物理无法放后端，但作为**输入**上报，非状态源 |

### 2.3 关键：web 音频位置的唯一现实约束
HTMLAudio 的 `currentTime` 是浏览器对象，物理上无法住后端。处理：
- 前端 `timeupdate` **低频上报**（500ms~1s）到 `POST /api/v1/state/progress`
- 后端统一存进 `PlaybackState.position` 并广播
- 前端本地只保留「最近一次上报值」用于拖动条即时响应（**临时 UI 态，Alpine**）
- 不做业务状态，只做高频 UI 反馈

---

## ③ 落地步骤（分 4 阶段，每阶段可独立验证）

### 阶段 1：后端建统一状态源（PlaybackState + 命令入口 + 一条 SSE）
- 新建 `go/internal/service/playback/playback.go`：
  - `type State struct { Source, BVID, Title, Artist, Cover, Playing, Position, Duration, LyricLine string/int; Queue []Song; PlayMode string; SpkDevices []SpeakerDevice }`
  - 方法：`Play(song) / Pause() / Resume() / Seek(pos) / Next() / Prev() / SetMode(m) / SetQueue(q) / SwitchSpeaker(did) / SwitchWeb()`
  - 每个变更 → `bcast()` 广播到 `/api/v1/state/stream`（SSE，单连接/多订阅）
  - 变更即持久化（关键状态写 SQLite `state` 表，启动恢复）
- 新建 `go/internal/api/state.go`：`GET /api/v1/state`（当前快照）+ `GET /api/v1/state/stream`（SSE）+ `POST /api/v1/state/progress`（web 位置上报）
- **验证**：curl 发命令 → SSE 收到广播 → 快照正确

### 阶段 2：前端接状态流（纯渲染，删业务状态）
- 引入 `alpine.min.js`（已在 `go/web/assets/`，shell.html 加 `<script>`）
- 新建 `go/web/assets/state.js`：`EventSource('/api/v1/state/stream')` → `Alpine.store('playback', ...)` 更新
- `player.js` **删业务状态**（`song/state/queue/qidx/playMode/spkDid/spkState`），只保留：
  - HTMLAudio（物理播放器，执行 play/pause/seek）
  - 命令转发（点击 → POST 后端）
  - `timeupdate` 上报进度
  - 渲染从 `Alpine.store('playback')` 派生
- **验证**：点播放 → 后端广播 → 面板/歌词/字幕同步更新，无双分支

### 阶段 3：音箱/挂件/字幕接入同一状态流
- 音箱进度从 `SpeakerService.progress` 并入 `PlaybackState`（或后端统一广播）
- 挂件（widget）/字幕从状态流读，不再各自订阅
- **验证**：投送音箱 → 面板图标/进度/字幕全走一条流

### 阶段 4：持久化 + 迁移
- `state` 表存队列/播放模式/上次歌曲，启动恢复
- 旧 `localStorage hc:song/hc:queue` 数据迁移到后端（一次性）
- **验证**：重启恢复队列/上次播放

---

## ④ 边界与取舍（诚实）

| 项 | 决定 | 理由 |
|----|------|------|
| Alpine 只管临时 UI 态 | 不存业务状态 | 避免双写（社区规则） |
| web 位置上报而非后端轮询 | 低频上报 | HTMLAudio 物理约束 |
| 音量/音质/主题 | 可留前端 localStorage | 设备偏好，非业务状态，跨端不同属合理 |
| htmx 权威片段 | 列表/表单/搜索/设置 | 服务端渲染舒适区 |
| 是否引入 Alpine | **是**（已在 assets，文档已选型） | 组合技社区验证，补上当初没落地的 |
| SSE 单流 or 多流 | **一条 `/api/v1/state/stream`** | 消除多点订阅同步 |
| 歌词行 | 并入状态流 | 已是后端 lyric-state，统一 |

## ⑤ 风险与缓解

| 风险 | 缓解 |
|------|------|
| 重构大，player.js 基本重写状态层 | 分 4 阶段，每阶段可独立验证，UI 不动 |
| 命令→后端→SSE→渲染有网络往返延迟 | web 高频反馈走 Alpine 临时态（乐观），业务态以 SSE 为准 |
| htmx 替换片段擦 Alpine 状态 | 用 `hx-swap="morph"` 或 Alpine 数据存 `$store`（片段外） |
| 音箱进度与 web 位置双源 | 统一进 `PlaybackState.position`，source 区分 |
| 迁移 localStorage 丢数据 | 阶段 4 一次性迁移，保留旧值兜底 |

## ⑥ 结论

- **根治**：业务状态物理唯一住后端，前端无副本 → 无同步问题
- **组合技**：Alpine（临时 UI 态）+ htmx（权威片段），社区验证，且项目文档早已选型
- **分 4 阶段**：每阶段可验证，风险可控，UI 不变
- **回归初心**：业务逻辑一份住 Go 后端，前端只渲染 + 高频 UI
