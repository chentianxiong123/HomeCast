# HomeCast

<p align="center">
  <b>🏠 家庭影音投屏平台</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go" alt="Go">
  <img src="https://img.shields.io/badge/htmx-2.x-3366CC?logo=htmx" alt="htmx">
  <img src="https://img.shields.io/badge/SQLite-内嵌-003B57?logo=sqlite" alt="SQLite">
  <img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License">
</p>

---

## 🧭 架构演进：Python 探索 → Go 实现

本项目的核心技术路线是 **「Python 探索、Go 实现」**：

- **`explore/`（探索目录，历史只读）** —— 收纳两条探索路径：
  - `explore/backend/`（Python/FastAPI）：早期用 Python 快速验证玩法（B站音乐、DLNA 投屏、音箱、嗅探），逻辑成熟后由 Go 正式实现。
  - `explore/android/`（手搓 WebView 壳）：安卓壳的早期实现（纯 Go 服务端二进制 + 极简 WebView + 零 Gradle 手搓 APK），已验证稳定但已切到 Wails v3，保留作兜底参考。
- **`go/` 是正式实现** —— 单二进制、全部内嵌（htmx + Tailwind CDN + SQLite，无 Node 构建链、无依赖安装），一个端口跑全部。
- **`wails3/` 是安卓主线壳** —— Wails v3 壳（Go 编 libwails.so 进 APK + WebView），homecast 全路由挂 AssetOptions.Handler，框架管生命周期。
- 探索版保留在仓库内**只读参考**，不参与维护；新改动只做 Go 版。
- Python 端沉淀的接口语义（`/api/v1/*` 前缀、`{code,message,data}` 信封）已 1:1 对齐进 Go 实现，前端无感知切换。

---

## 简介

**HomeCast** 是一个家庭影音投屏平台，集成了 Bilibili 音乐播放、DLNA 视频投屏、小米音箱控制等功能。通过简洁的 Web 界面或 Android APP，轻松管理和播放你的音乐与视频内容。

### ✨ 核心亮点

- 🎧 **Bilibili 音乐** - 搜索、收藏、播放 Bilibili 音频内容
- 📺 **DLNA 投屏** - 自动发现局域网设备，一键投屏视频
- 🔍 **智能嗅探** - 自动解析视频网站，提取播放链接
- 📱 **跨平台** - Web + Android APP，随时随地使用
- 🔊 **小米音箱** - 支持小米智能音箱音乐推送

---

## 功能模块

### 🎵 音乐播放
| 功能 | 描述 |
|-----|------|
| 音乐搜索 | 搜索 Bilibili 视频音频 |
| 收藏管理 | 同步 Bilibili 收藏夹 |
| 播放列表 | 创建和管理本地歌单 |
| 歌词显示 | 实时歌词滚动 |
| 音频代理 | 解决跨域播放问题 |

### 📺 视频投屏
| 功能 | 描述 |
|-----|------|
| 设备发现 | 自动搜索局域网 DLNA 设备 |
| 视频嗅探 | 智能解析视频网站播放链接 |
| 集数提取 | 自动识别剧集列表 |
| 播放控制 | 播放/暂停/停止/进度/音量 |
| 常用网站 | 保存常用视频网站 |

### 🔊 小米音箱
| 功能 | 描述 |
|-----|------|
| 账号登录 | 密码/Cookie/二维码登录 |
| 设备管理 | 查看和管理小米音箱设备 |
| 音乐推送 | 将音乐推送到音箱播放 |

---

## 技术栈

### ✅ 当前正式版（Go）
- **Go 1.26** - 单二进制，全内嵌（模板/静态资源/SQLite）
- **htmx** - 页面交互（无状态优先，服务端渲染片段）
- **SQLite**（modernc.org/sqlite 纯 Go 无 CGO） - 本地持久化
- **Tailwind CDN** - 样式（本地化资源，无构建链）
- **测试**：Go 单测/E2E 全覆盖（`go test ./...` + Playwright 真浏览器冒烟）

### 🧪 探索版（Python · 历史只读，不维护）

<table>
<tr>
<td width="50%">

### 后端
- **Python 3.12**
- **FastAPI** - 高性能异步框架
- **Playwright** - 视频嗅探
- **yt-dlp** - 视频解析
- **async-upnp-client** - DLNA 协议

</td>
<td width="50%">

### 前端
- **Vue 3** - 渐进式框架
- **TypeScript** - 类型安全
- **Naive UI** - 组件库
- **Vite** - 构建工具
- **Capacitor** - Android 打包

</td>
</tr>
</table>

---

## 快速开始

### 环境要求
- Python 3.12+
- Node.js 18+
- FFmpeg（可选，用于音频转码）

### 🚀 后端启动

```bash
cd explore/backend

# 安装依赖
pip install -r requirements.txt

# 安装 Playwright 浏览器
playwright install chromium

# 启动服务
uvicorn app.main:app --host 0.0.0.0 --port 28974 --reload
```

### 💻 前端启动

```bash
cd frontend

# 安装依赖
npm install

# 开发模式
npm run dev

# 构建生产版本
npm run build
```

### 📱 Android APP

```bash
cd frontend

# 构建
npm run build

# 同步到 Android
npx cap sync android

# 用 Android Studio 打开
npx cap open android
```

---

## 项目结构

```
homecast/
├── go/                        # 正式实现（单二进制，htmx + Tailwind CDN + SQLite）
│   ├── cmd/server/            # 服务入口
│   ├── web/                   # 页面/模板/静态资源
│   └── internal/              # api / service / store / bilibili / dlna / speaker
│
├── wails3/                    # 安卓主线壳（Wails v3：Go 编 .so + WebView）
├── desktop/                   # 桌面壳（Wails v2）
├── explore/                   # 探索目录（历史只读）
│   ├── backend/               # Python/FastAPI 探索版
│   └── android/               # 手搓 WebView 壳（安卓早期实现，兜底参考）
├── scripts/                   # E2E 冒烟等脚本
├── docs/                      # 文档/记录
└── README.md
```

---

## API 接口

| 接口 | 方法 | 说明 |
|-----|------|------|
| `/api/v1/music/search` | GET | 搜索音乐 |
| `/api/v1/music/play` | GET | 获取播放链接 |
| `/api/v1/cast/devices` | GET | 获取 DLNA 设备 |
| `/api/v1/cast/sniff` | POST | 嗅探视频 |
| `/api/v1/cast/start` | POST | 开始投屏 |
| `/api/v1/cast/control` | POST | 投屏控制 |
| `/api/v1/speaker/devices` | GET | 获取小米音箱 |
| `/api/v1/favlist/sync` | POST | 同步收藏夹 |

---

## 配置说明

### 后端配置 (`explore/backend/configs/config.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 28974

bilibili:
  cookie: ""  # Bilibili Cookie（可选）

xiaomi:
  enable: false
  account: ""
  password: ""
```

---

## 注意事项

1. **DLNA 投屏** - 需要手机/电脑与投屏设备在同一局域网
2. **视频嗅探** - 需要安装 Playwright 浏览器
3. **音频代理** - 用于解决 Bilibili 跨域限制
4. **小米音箱** - 需要小米账号登录

---

## 开发计划

- [ ] iOS APP 支持
- [ ] 更多视频网站支持
- [ ] 播放历史记录
- [ ] 歌词同步优化
- [ ] PWA 支持

---

## License

[MIT](LICENSE)

---

<p align="center">
  Made with ❤️ by HomeCast Team
</p>
