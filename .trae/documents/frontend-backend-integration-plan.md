# 前后端联调计划 — Bilibili Music

## 一、现状分析

### 后端已就绪的 API（全部在 `backend/app/api/` 下）

| 模块 | 端点 | 方法 | 请求参数 | 返回格式 |
|------|------|------|---------|---------|
| **音乐搜索** | `/api/v1/music/search` | GET | keyword, page, page_size | `{code:0, data:{total, list:[MusicItem]}}` |
| **视频信息** | `/api/v1/music/info/{bvid}` | GET | bvid(path) | `{code:0, data:{bvid,aid,cid,title,...}}` |
| **音频流URL** | `/api/v1/music/audio/{bvid}` | GET | bvid(path), quality | `{code:0, data:{url,quality,size,mime_type}}` |
| **音频代理流** | `/api/v1/music/stream/{bvid}` | GET | bvid(path), quality | StreamingResponse(直接音频) |
| **投屏-设备发现** | `/api/v1/cast/devices` | GET | 无 | `{code:0, data:[{name,udn,ip,port,device_type}]}` |
| **投屏-嗅探** | `/api/v1/cast/sniff` | POST | `{url}` | `{code:0, data:{title,episodes,sniff_method}}` |
| **投屏-开始** | `/api/v1/cast/start` | POST | `{episode_url,device_udn,title}` | `{code:0, data:{proxy_url,device_name}}` |
| **投屏-控制** | `/api/v1/cast/control` | POST | `{device_udn,action,target?,volume?}` | `{code:0}` |
| **投屏-状态** | `/api/v1/cast/status/{udn}` | GET | udn(path) | `{code:0, data:{transport,position}}` |
| **小爱-设备列表** | `/api/v1/speaker/devices` | GET | 无 | `{code:0, data:[{did,name,hardware,device_id}]}` |
| **小爱-推送播放** | `/api/v1/speaker/play` | POST | `{bvid,did,quality}` | `{code:0, data:{proxy_url,...}}` |
| **小爱-控制** | `/api/v1/speaker/control` | POST | `{did,action,volume?}` | `{code:0}` |
| **小爱-状态** | `/api/v1/speaker/status/{did}` | GET | did(path) | `{code:0, data:{...}}` |
| **收藏夹-列表** | `/api/v1/favlist/{mid}` | GET | mid(path), page, page_size | `{code:0, data:{...}}` |
| **收藏夹-详情** | `/api/v1/favlist/info/{mid}` | GET | mid(path) | `{code:0, data:{...}}` |
| **代理-音频** | `/proxy/audio/{token}` | GET | token | 音频流 |
| **代理-视频** | `/proxy/video/{token}` | GET | token | 视频流 |

Vite 代理配置：`/api` → `localhost:28974`，`/proxy` → `localhost:28974`

---

### 已发现的前后端不一致问题（必须修复）

#### P0 — 必须修复（会导致功能完全不可用）

1. **缺少类型定义**
   - 前端从 `@/api` 导入了 `DLNADevice`、`SpeakerDevice`、`Episode`、`SniffResult`，但 `api/index.ts` 中从未 export 这些类型
   - 需要在 `api/index.ts` 中补充这些 interface 定义

2. **request.ts 的 baseURL 错误**
   - `api/request.ts` 第6行：baseURL 默认为 `http://localhost:8080/api/v1`（端口错误，应为 28974 或用相对路径）
   - `api/music.ts` 使用了 request.ts，导致音乐相关请求打到了错误的地址
   - 修复方案：改为相对路径 `/api/v1`，由 Vite proxy 转发

3. **小爱音箱 play 接口字段名不匹配**
   - 前端发送：`{ device_did, bvid, title }`（api/index.ts 第35行）
   - 后端期望：`{ bvid, did, quality }`（speaker_service.py PlayRequest）
   - 字段名完全不同！`device_did` vs `did`

4. **前端 USE_MOCK 全部为 true**
   - `api/index.ts` 第7行：`USE_MOCK = true`
   - `api/music.ts` 第4行：`USE_MOCK = true`
   - 全部需要改为 `false`

5. **mock 数据自动注入播放列表**
   - `stores/player.ts` 第66-81行：`initMockData()` 在模块加载时自动往播放列表塞入 8 首 mock 歌曲
   - 切换真实模式后必须移除或改为条件执行

#### P1 — 需要修复（影响体验）

6. **UrlSniffer 组件自动 emit mock 数据**
   - `UrlSniffer.vue` 第54-69行：onMounted 时自动 emit 一个 MOCK_SNIFF_RESULT
   - 这是给演示用的，真实模式下不应自动触发

7. **core/player.ts 音频 URL 构造问题**
   - 第212行：baseUrl 默认 `http://localhost:8080/api/v1`
   - 应改为使用 Vite proxy 的相对路径 `/api/v1/music/stream/${bvid}`

8. **getAudioProxyUrl 函数路径问题**
   - `api/music.ts` 第60-62行：返回 `${baseUrl}/proxy/audio/${bvid}`
   - 但后端代理路由是 `/proxy/audio/{token}`（token 不是 bvid），这里逻辑不对
   - 实际播放应该走 `/api/v1/music/stream/{bvid}` 端点（后端已有实现）

#### P2 — 可选优化

9. **API 错误处理统一化**
   - 当前各处 catch 只是 return `{code:-1}`，没有 toast 提示用户
   - 应加入全局错误提示机制

10. **Loading 状态管理**
    - 搜索时、嗅探时、投屏启动时都应有 loading 反馈
    - CastControls 的轮询应正确绑定 isCasting 生命周期

---

## 二、联调执行步骤

### Phase 1: 基础设施修复（打通网络层）

#### Step 1.1 — 补充 API 类型定义
**文件**: `frontend/src/api/index.ts`
- 新增导出：
  ```ts
  export interface DLNADevice {
    name: string; udn: string; ip: string; port: number; device_type: string
  }
  export interface SpeakerDevice {
    did: string; name: string; hardware: string; device_id?: string; is_online?: boolean
  }
  export interface Episode {
    index: number; title: string; url: string; duration: number; thumbnail?: string
  }
  export interface SniffResult {
    title: string; sniff_method: string; episodes: Episode[]
  }
  ```

#### Step 1.2 — 修复 request.ts baseURL
**文件**: `frontend/src/api/request.ts`
- 第6行 baseURL 从 `'http://localhost:8080/api/v1'` 改为 `'/api/v1'`（相对路径，走 Vite proxy）

#### Step 1.3 — 关闭所有 MOCK 开关
**文件**:
- `frontend/src/api/index.ts`: `USE_MOCK = false`
- `frontend/src/api/music.ts`: `USE_MOCK = false`

#### Step 1.4 — 移除 mock 自动注入
**文件**: `frontend/src/stores/player.ts`
- 注释掉或删除 `initMockData()` 及其调用（第66-81行）
- 播放列表初始为空，由用户搜索后添加

#### Step 1.5 — 修复 UrlSniffer 自动 mock 触发
**文件**: `frontend/src/components/Cast/UrlSniffer.vue`
- 删除 onMounted 中的 mock 数据 emit（第54-69行）
- 只保留用户手动输入 URL + 点击嗅探按钮后的真实流程

#### Step 1.6 — 修复小爱音箱接口字段名
**文件**: `frontend/src/api/index.ts` speakerApi.play()
- 请求体从 `{ device_did, bvid, title }` 改为 `{ bvid, did: deviceDid, quality: 30280 }`
- 对齐后端 PlayRequest 的字段名

#### Step 1.7 — 修复音频 URL 构造
**文件**: `frontend/src/core/player.ts`
- 第212行改为相对路径：`/api/v1/music/stream/${song.bvid}?quality=${streamInfo.quality}`
- 不再依赖 import.meta.env.VITE_API_BASE_URL

---

### Phase 2: 音乐模块联调

#### Step 2.1 — 搜索功能联调
- SearchView 输入关键词 → 调用 `searchMusic(keyword)` → 显示后端返回的真实结果
- 验证：搜索"周杰伦"能返回真实的B站视频列表
- 关注点：cover 图片是否正常显示（B站图片可能有防盗链）

#### Step 2.2 — 播放功能联调
- 点击歌曲 → 调用 `play(song)` → `getAudioStream(bvid)` → 设置 audio.src 为 `/api/v1/music/stream/bvid`
- 验证：能听到声音，进度条走动，播放/暂停切换正常
- 关注点：B站音频是 mp4 容器+aac 编码，video 元素能否正常播放

#### Step 2.3 — 播放器控制联调
- 进度条拖拽 seek → 调用 `seekToPercent(val)`
- 音量调节 → `setVolume(val)`
- 上一首/下一首 → `next()` / `prev()`
- 播放模式切换 → `setPlayMode(mode)`

---

### Phase 3: 投屏(DLNA)模块联调

#### Step 3.1 — DLNA 设备发现联调
- CastView 加载 → `castApi.getDevices()` → 显示局域网内真实 DLNA 设备
- 点击刷新按钮 → 重新扫描

#### Step 3.2 — URL 嗅探联调
- 用户输入视频 URL → `castApi.sniff(url)` → 后端 3 阶段嗅探 → 返回集数列表
- EpisodePicker 显示真实集数
- 关注点：嗅探耗时可能较长（yt-dlp/Playwright），需显示 loading 状态

#### Step 3.3 — 投屏播放联调
- 选择设备 + 选择集数 → "开始投屏" → `castApi.start(url, udn, title)`
- CastControls 出现 → 开始轮询 `castApi.getStatus(udn)`
- 播放/暂停/停止/seek/音量 → `castApi.control(udn, action, ...)`

---

### Phase 4: 小爱音箱模块联调

#### Step 4.1 — 小爱设备列表联调
- SpeakerPush 打开下拉 → `speakerApi.getDevices()` → 显示真实小爱设备
- 在线/离线状态显示

#### Step 4.2 — 推送播放联调
- 选择设备 → "推送" → `speakerApi.play(did, bvid, title)` → 后端获取音频URL+推送到小爱
- 验证：小爱音箱开始播放当前歌曲

#### Step 4.3 — 小爱控制联调
- 停止推送 → `speakerApi.control(did, 'stop')`

---

### Phase 5: 收藏夹模块联调

#### Step 5.1 — 收藏夹列表联调
- FavlistView 加载 → `favlistApi.getList()` → 显示用户的收藏夹列表
- （注意：收藏夹 API 需要 B站登录 cookie 才能访问个人收藏）

#### Step 5.2 — 收藏夹详情联调
- 点击某个收藏夹 → `favlistApi.getInfo(mid)` → 显示收藏夹信息和媒体列表

---

### Phase 6: 收尾和优化

#### Step 6.1 — 全局错误提示
- API 请求失败时弹出 n-message 提示
- 网络异常友好提示

#### Step 6.2 — Loading 状态完善
- 搜索按钮 loading
- 嗅探按钮 loading + 禁用重复点击
- 投屏启动 loading
- 设备发现 loading

#### Step 6.3 — 边界情况处理
- 无搜索结果时空态展示
- 无 DLNA 设备时提示
- 无小爱设备时提示（未配置小米账号）
- 音频播放失败的 fallback 处理

---

## 三、执行顺序与依赖关系

```
Phase 1 (基础设施)
  ├── 1.1 类型定义     ← 无依赖，先做
  ├── 1.2 baseURL修复   ← 无依赖
  ├── 1.3 关闭MOCK      ← 依赖 1.1, 1.2
  ├── 1.4 移除mock注入  ← 与 1.3 同步
  ├── 1.5 UrlSniffer修复 ← 与 1.3 同步
  ├── 1.6 speaker字段修复 ← 依赖 1.3
  └── 1.7 音频URL修复   ← 依赖 1.2

Phase 2 (音乐联调)       ← 依赖 Phase 1 全部完成
Phase 3 (DLNA投屏联调)    ← 依赖 Phase 1 全部完成
Phase 4 (小爱音箱联调)    ← 依赖 Phase 1 全部完成
Phase 5 (收藏夹联调)      ← 依赖 Phase 1 全部完成
Phase 6 (收尾优化)        ← 依赖 Phase 2~5
```

## 四、验证清单

- [ ] 音乐搜索返回真实数据，封面图正常
- [ ] 点击歌曲能播放出声音，进度条联动
- [ ] 播放/暂停/上/下一首/音量/进度拖拽 都正常
- [ ] DLNA 设备发现能列出局域网设备
- [ ] 输入视频 URL 嗅探能返回集数列表
- [ ] 选择设备和集数后能成功投屏到 TV
- [ ] CastControls 进度条实时更新，播放暂停停止正常
- [ ] 小爱音箱设备列表正常显示
- [ ] 推送歌曲到小爱音箱能播放
- [ ] 收藏夹列表和详情能正常展示
- [ ] 所有 API 错误有友好提示
- [ ] 所有 loading 状态正确显示
