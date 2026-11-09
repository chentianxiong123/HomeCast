# 小程序 mp-html 渲染方案（已废弃，代码留档于此）

> 2026-11-09 探索记录。**结论：废弃**。代码移到 `explore/mp-html/` 留档，壳撤回 `web-view` 版本。

## ① 目标与动机

小程序壳不用 web-view（壳感重），改用 **mp-html 渲染**（原生感）——后端（Go，本地 HTTP）吐动态 HTML 片段 → 小程序 mp-html 渲 → 500ms 高频刷新。逻辑全在 Go 后端，小程序唯一重写点：InnerAudioContext 播放器 + linktap 事件分发。这是"第三条路"（web-view 之外）。

## ② 资源拓扑

- **mp-html 组件**：`components/mp-html/`（dist/mp-weixin 直接拷贝，零 npm/零构建）
- **后端吐 HTML 端点**：`/hx/search-mp`（真实 B站搜索卡片）、`/hx/player-bar`（播放条）、`/hx/lyric-panel`（12 行歌词当前行高亮）
- **小程序侧**：mp-html 渲 + 500ms `setData` 刷新 + linktap 分发（action://play-pause 等）
- **音频**：`wx.createInnerAudioContext()` 播 `/api/v1/music/stream/{bvid}`（后端代理 B站流）

## ③ 工具链清单

| 工具 | 版本 | 用途 |
|---|---|---|
| mp-html | dist/mp-weixin（拷贝） | 渲染 HTML 片段（不支持 JS/onclick） |
| 微信开发者工具 | 2.02.2609250 | 模拟器编译/预览 |
| Go 后端 | 本地 HTTP（Mac 28976） | 吐动态 HTML + 代理流 |

## ④ 成功路径（已验证部分）

- **mp-html 高频刷新流畅**：播放条（节点少）+ 12 行歌词（节点多）同时 500ms setData → 模拟器实测**不卡**（`pages/exp/` 实验）。
- **搜索真实**：`/hx/search-mp` 返回真实 B站卡片（href 包 `https://play.homecast/?bvid=...`）。
- **音频生死**：`InnerAudioContext` 播真实 B站 mp4（`/api/v1/music/stream/{bvid}`，1.2M `video/mp4`）→ **模拟器有声音**（生死实验过，但音量/音质待优化，真机未验）。

## ⑤ 踩坑记录（每条：现象 → 根因 → 解法）

1. **mp-html 不执行 JS**（`<script>`/`onclick`/`hx-post` 全不执行）→ 后端片段纯展示，交互由壳逻辑层重接（linktap 分发）。
2. **mp-html 事件名是 `linktap`**（非 `link`）→ 绑 `bindlinktap`。
3. **`href` 含 `://` 默认复制链接** → `copy-link="{{false}}"` 关掉。
4. **自定义协议 `hc-play://` 不被当链接** → 改 `https://play.homecast/?bvid=...`。
5. **mp4 渐进式 duration 未加载** → 微信 `InnerAudioContext.duration` 为 0 → 进度只显示 `/s`，改用 currentTime + 后端时长。
6. **真机连后端**：手机连 Mac 局域网 IP（`192.168.31.12:28976`，不是 127.0.0.1）→ "打开调试"绕过域名校验。

## ⑥ 验证成果

- mp-html 高频刷新（播放条 + 12 行歌词 500ms）→ **模拟器流畅不卡**。
- 搜索 → 真实 B站卡片 mp-html 渲。
- 音频 → 模拟器有声音（生死过）。

## ⑦ 决策记录

- **选 mp-html（B 方案）**：原生感、后端吐 HTML、逻辑一份住 Go。
- **但功能受限**：mp-html 无拖拽进度条（只能点跳）、无 SSE（小程序无 EventSource，用轮询）、后台锁屏（微信特有才支持）、真机连后端（局域网 IP）。
- **废弃（用户决定）**：web-view（A 方案）仍是最优——一行 `<web-view>` 加载 H5 整站，零重写、JS 全跑、导航/播放器/歌词全有。
- **音频生死**：模拟器有声（过），但音质（B站 mp4 音轨低质）+ 真机未验——不构成废弃主因，废弃主因是**功能受限 + web-view 更优**。

## ⑧ 未解之谜 / 下一步岔路

- 真机音频（InnerAudioContext 播 B站流）——模拟器有声但真机未验。
- 音质（B站 mp4 音轨）——若走 B 需换高质量源。
- **下一步（主线）**：回 web-view 版本，真机扫码（miniprogram-ci + 正式账号）验证。
