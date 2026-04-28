# HomeCast

<p align="center">
  <b>🏠 家庭影音投屏平台</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Vue-3.x-4FC08D?logo=vue.js" alt="Vue 3">
  <img src="https://img.shields.io/badge/FastAPI-0.100+-009688?logo=fastapi" alt="FastAPI">
  <img src="https://img.shields.io/badge/Python-3.12+-3776AB?logo=python" alt="Python">
  <img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License">
</p>

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
cd backend

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
├── backend/                    # 后端服务
│   ├── app/
│   │   ├── api/               # API 路由
│   │   ├── bilibili/          # Bilibili API
│   │   ├── dlna/              # DLNA 投屏
│   │   ├── proxy/             # 音频代理
│   │   ├── service/           # 业务服务
│   │   ├── sniffer/           # 视频嗅探
│   │   └── speaker/           # 小米音箱
│   ├── data/                  # 数据存储
│   └── requirements.txt
│
├── frontend/                   # 前端应用
│   ├── src/
│   │   ├── api/               # API 调用
│   │   ├── components/        # 组件
│   │   ├── views/             # 页面
│   │   └── stores/            # 状态管理
│   ├── android/               # Android APP
│   └── package.json
│
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

### 后端配置 (`backend/configs/config.yaml`)

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
