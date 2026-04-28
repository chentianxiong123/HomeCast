# Bilibili Music 前端实施文档

> **原则**: 最小改动，不造轮子，能抄就抄

---

## 一、现有前端改动（最小化）

### 1.1 导航简化
```
当前: [首页] [搜索] [收藏]
改为: [🎵音乐] [📺投屏] [⭐收藏]
```
- 删掉 HomeView（首页），SearchView 就是入口
- 新增 CastView（投屏）

### 1.2 音乐模式改动（只加一个小爱推送）

**现有 SearchView 已经有：**
- ✅ 搜索框 + 搜索结果卡片
- ✅ 点击卡片 → 分P列表弹窗
- ✅ 选择集数 → Web AudioPlayer 播放
- ✅ 底部播放控制栏

**只需加的：**
- 底部播放栏右侧加一个 **「🔊 推送」按钮**
- 点击后弹出设备列表（小爱音箱列表）
- 选择设备 → 推送播放
- 推送后按钮变成 **「⏹ 停止推送」**

```
现有底栏:  [封面] 标题 - 作者    [⏮][▶][⏭]  进度条  [🔊音量]

改为:      [封面] 标题 - 作者    [⏮][▶][⏭]  进度条  [🔊音量] [📢推送▾]
                                                    点击后:
                                                   ┌─────────────┐
                                                   │ 🔊 小爱音箱Pro│ ← 设备列表
                                                   │ 🔊 小爱音箱Play│
                                                   │ 🌐 取消推送   │
                                                   └─────────────┘
```

### 1.3 投屏模式（全新独立页面）

**完全独立的功能，和音乐无关：**
- 输入任意 URL → 嗅探出视频集数 → 选 DLNA 设备投屏

---

## 二、文件清单

### 修改 (3个) - 音乐模式小改
```
src/router/index.ts       - 删HomeView路由, 加CastView
src/views/SearchView.vue  - 加推送按钮逻辑
src/App.vue / 底部播放栏   - 加推送按钮UI
```

### 新建 (6个) - 投屏 + 基础设施
```
src/api/index.ts              API封装
src/stores/player.ts          全局状态(推送状态+设备列表)
src/components/Player/SpeakerPush.vue  推送按钮+设备选择弹窗
src/views/CastView.vue         投屏页面
src/components/Cast/UrlSniffer.vue     URL嗅探
src/components/Cast/EpisodePicker.vue  集数列表
src/components/Cast/DevicePicker.vue   DLNA设备选择
src/components/Cast/CastControls.vue   投屏控制
```

---

## 三、API 接口

```
GET  /api/v1/music/search?keyword=xxx&page=1&page_size=20
GET  /api/v1/music/info/{bvid}
GET  /api/v1/music/audio/{bvid}?quality=30280
GET  /proxy/audio/{token}

GET  /api/v1/speaker/devices        → 小爱设备列表
POST /api/v1/speaker/play           {device_did, bvid, title}  → 推送
POST /api/v1/speaker/control        {device_did, action}

POST /api/v1/cast/sniff             {url}  → {title, episodes[]}
GET  /api/v1/cast/devices           → DLNA设备列表
POST /api/v1/cast/start             {episode_url, device_udn}
POST /api/v1/cast/control           {device_udn, action}
GET  /api/v1/cast/status/{udn}
```

---

## 四、实施顺序

### 第一步: 基础设施 (无UI变化)
1. `src/api/index.ts` - API封装层
2. `src/stores/player.ts` - 全局状态
3. `vite.config.ts` - API代理配置

### 第二步: 音乐模式 (小改)
4. `SpeakerPush.vue` - 推送按钮+设备选择下拉组件
5. 改造底部播放栏 - 集成 SpeakerPush
6. 改造 SearchView - 推送时调用 speaker API
7. 更新路由 - 删 HomeView, 搜索即首页

### 第三步: 投屏页面 (全新)
8. `CastView.vue` - 页面骨架
9. `UrlSniffer.vue` - URL输入+嗅探
10. `EpisodePicker.vue` - 集数表格 (抄xiaomusic歌曲列表)
11. `DevicePicker.vue` - DLNA设备卡片 (抄MiAir device-item结构)
12. `CastControls.vue` - 控制栏 (抄MiAir speaker-card结构)

### 第四步: 联调验证
13. 逐功能前后端验证
14. 错误处理

---

## 五、UI 抄袭映射

| 我们要做的 | 抄谁 | 具体位置 |
|-----------|------|---------|
| 搜索结果展示 | 已有，不改 | - |
| 底部播放栏 | 已有，只加按钮 | - |
| **推送设备下拉** | xiaomusic 设备按钮 | `index.html` L106-127 改为下拉 |
| **集数表格** | xiaomusic 歌曲列表 | `index.html` L162-246 |
| **DLNA设备卡片** | MiAir device-item | `index.html` L128-168 (结构) |
| **投屏状态卡** | MiAir speaker-card | `index.html` L170-194 (结构) |

> 所有配色用 Naive UI 默认亮色主题，只抄布局结构和交互逻辑
