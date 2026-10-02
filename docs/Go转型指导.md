# homecast 转型 Go + Wails 指导文档

> 2026-10-02 · 方案文档，进度随动手更新（不编造未做步骤）

## 背景 / 为什么转

- 目标形态：**给爸妈的安卓 App**——后端必须在 App 内本地跑（不能远程后端），WebView 显示界面。
- 否决掉的路线：
  - **Node/Bun**：Bun 官方不支持安卓目标；Node 只有老旧的 nodejs-mobile（Node 12/16 时代 API），Fastify 新特性受限。**死路。**
  - **Python(Chaquopy)**：能嵌进 APK（官方 Gradle 插件，Python 3.11），但 APK 笨重（解释器+依赖几十 MB）、启动慢、打包配置折腾。
- 选定：**Go 重写后端 + Wails 壳**。理由：
  - 安卓嵌入：Go 官方支持交叉编译 `.so` + JNI，壳里起本地服务；体积几 MB、毫秒启动。
  - 并发：B站多请求 / DASH 流代理天然适合 goroutine。
  - **其他项目已定 Go**，这块当打基础。
  - Wails = Go + 系统 WebView（不打包 Chromium），一套 Go 代码 + 一套 Vue 前端，出桌面 / 安卓 / 服务器三个出口。

## 技术栈（查证过）

| 层 | 选型 | 状态 |
|---|---|---|
| 语言 | Go 1.26（本机已装） | ✅ |
| 框架 | Wails v2（桌面）| ✅ 稳定生产可用 |
| 安卓 | Wails v3（iOS/Android）| ⚠️ **experimental**，v3 目前 beta.21/23（GitHub releases 实测），API 稳定但官方明确"生产慎用" |
| 前端 | Vue + TS 不动 | ✅ 现有 frontend/ |
| HTTP | 标准库 net/http（Go 1.22+ 路由增强）或 chi | 视需要 |
| 数据 | 本地 JSON（收藏/播放列表），对齐桌面那套 | ✅ |

**风险提示**：安卓 APK 建议等 Wails v3 转正再出正式版；**桌面版先用 v2 打基础验证全部链路**。

## 目录结构（在 homecast 仓库内，不新建仓库）

```
~/homecast
├── backend/          # Python 版（保留作对照，验证 Go 版逐端点对齐后再退役）
├── frontend/         # Vue 前端（不动）
├── go/               # ★ Go 后端（新）
│   ├── cmd/server/   # 入口（HTTP 服务器 + 静态托管）
│   └── internal/
│       ├── bilibili/ # B站 client / search / audio / video
│       ├── service/  # music / fav / lyric
│       ├── api/      # 路由 + 端点（对齐 /api/v1）
│       └── config/
└── docs/             # 本文档
```

Wails 壳后面以 `go/wails/` 引入（v2 桌面先行），不并入 cmd/server。

## 迁移进度（2026-10-02 实测）

- ✅ 阶段1 搜索 + DASH 流（duration/字节数对齐 Python）
- ✅ 阶段2 收藏 + 歌词（网易云）
- ✅ 阶段3 播放列表
- ✅ 阶段4 cast/DLNA 投屏（goupnp 库）+ token 流代理
- ✅ 音箱：找到 Go 库 lsongdev/miservice-go（Player 全套），QR 登录手写协议实测生成成功
- ✅ sites 站点管理 + 集缓存
- ✅ Wails v2 桌面壳（阶段5）：desktop/ 单二进制内嵌全后端+前端；webkit2gtk-4.1 构建
  （Debian 13 用 4.1 而非 4.0！wails build -tags webkit2_41）
- ✅ 桌面歌词挂件：desktop/widget（cgo+GTK3 移植 lyric_widget.py）：
  前端上报 /widget/state → 后端内存 → 挂件轮询快照；X11 真穿透+悬停控制条+锁定小锁
- ⏳ Wails v3 安卓 APK（阶段6，等转正）
- ⏳ 静态托管（Go 托管 frontend/dist 单端口，阶段5 前置）

## 迁移顺序（分阶段，每阶段可独立验证）

1. **阶段 1（当前）**：Go 骨架 + B站 client + 搜索 + DASH 音频流直转
   - 对齐端点：`GET /api/v1/music/search`、`GET /api/v1/music/stream/{bvid}?quality=`
   - 验证：搜索 duration_sec、流可播（对照 Python 版 28974）
2. **阶段 2**：收藏（/api/v1/fav/*）、歌词（网易云源）
3. **阶段 3**：播放列表、音质全局切换
4. **阶段 4**：音箱认证 / DLNA / 投屏（最贵，最后）
5. **阶段 5**：Wails v2 桌面壳（前端接入）+ 静态托管单服务
6. **阶段 6**：Wails v3 安卓 APK（等转正）

## API 兼容清单（Python → Go 逐端点对齐）

| 端点 | 说明 | 状态 |
|---|---|---|
| GET /api/v1/music/search?keyword=&page=&page_size= | B站搜索 → {total, list:[MusicItem]} | 阶段1 |
| GET /api/v1/music/stream/{bvid}?quality= | DASH 音频直转（带 referer，支持 Range） | 阶段1 |
| GET/POST/DELETE /api/v1/fav/* | 本地 JSON 收藏（去重置顶） | 阶段2 |
| GET /api/v1/music/lyric | 网易云歌词 | 阶段2 |
| GET /api/v1/music/info/{bvid} | 视频信息（cid/时长） | 阶段1 内部用 |
| 音箱/DLNA/投屏 | 阶段4 | 待定 |

## 关键细节备忘（从 Python 版提炼，Go 翻译时必须保持）

1. **B站请求头**（client）：UA + `Referer` + `Origin: https://search.bilibili.com` + `Cookie: buvid3=<uuid4>`（每次启动随机）→ 防风控。
2. **搜索**：`/x/web-interface/search/type?search_type=video&keyword=&page=&pagesize=`；title 要剥 HTML 标签（`<em class="keyword">` 高亮标签）；`duration` 形如 `3:43` / `1:02:33` → `duration_sec`。
3. **音质 qn 码（桌面项目验证的真实码）**：64k=**30216**、128k=**30232**、192k=**30280**（免费最高）、FLAC=30250。优先级 `[192k,128k,64k,FLAC]`，默认 192k。**没有 320k。**
4. **取流**：`/x/player/playurl?bvid=&cid=&qn=&fnval=16&fourk=1` —— **fnval=16 纯 DASH；绝不加 platform=html5/mobisel/highbit**（那些会强制 durl 视频模式，是曾经"流是视频不是音频"的元凶）。cid 先经 `/x/web-interface/view?bvid=` 拿。
5. **DASH 直转**：取 `dash.audio[qn].baseUrl`（或 backup_url[0]），**带 Referer: https://www.bilibili.com 头直接转发流**，不转码不缓存；支持 Range（seek）；WebView 的 audio 原生可播 AAC。
6. **收藏**：本地 JSON `~/.config/homecast/favorites.json`，同 bvid 去重置顶；**写盘 ensure_ascii 语义**（B站标题可能含孤儿代理项 \ud800，Go 里 json.Marshal 默认转义 OK，但解码/读入时要防 invalid surrogate 报错）。
7. **歌词**：网易云免费源 `https://music.163.com/api/song/lyric?id=` / `search/get/web?`，headers 要 Referer；找不到歌词返回 `code:0,data:null`（前端拦截器按 code!=0 弹错，0 才是业务正常）。
8. **响应格式**：一律 `{code:0,message:"success",data:...}`，前端 request.ts 拦截器依赖。

## 坑与风险

- Wails v3 移动端 experimental：**别赌**，桌面 v2 先行。
- Go 里 JSON 对 B站非法代理项：读入用 `json.Decoder`（UseNumber 可选），写出时 `json.Marshal` 默认会替换非法字符，注意收藏文件读写要稳。
- 音质切换是**全局**（不按歌记忆），前端 localStorage 存 `bilibili-music-quality`，后端只是透传 quality 参数。
- 桌面项目（~sh/music/music.py）仍 Python，**不统一**，别误伤。
