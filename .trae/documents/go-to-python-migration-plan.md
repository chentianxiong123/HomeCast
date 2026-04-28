# Go 后端迁移 Python + 小爱音箱 + DLNA 投屏 实施计划

## 一、现状分析

### 1.1 现有 Go 后端能力
- B站 API 封装（搜索/视频信息/音频流/收藏夹）
- 音频流代理（解决跨域）
- MySQL + Redis 缓存
- 用户/歌单/播放历史/歌词缓存 数据模型

### 1.2 xiaomusic 的关键能力（可复用）
- **miservice 库**：小米账号登录 + MiNAService API（play_by_url / play_by_music_url）
- **音频代理**：自建 HTTP 服务器，音箱从代理拉取音频流
- **FFmpeg 实时转码**：bilibili CDN 的 mp4/aac → mp3 流式输出
- **设备管理**：小米设备发现、分组、状态轮询

### 1.3 MiAir 的关键能力（可复用）
- **DLNA MediaRenderer**：将小爱音箱伪装成 DLNA 设备
- **SSDP 设备发现** + **SOAP 控制** + **HTTP 媒体代理**
- **MediaBuffer**：预缓冲 + Range 请求 + Seek 支持
- **FFmpeg 转码**：FLAC → WAV（兼容不支持无损的音箱）

### 1.4 关键差异：yt-dlp vs B站 API

| 维度 | xiaomusic (yt-dlp) | 我们要做的 (B站 API) |
|------|---------------------|----------------------|
| 获取音频 | `yt-dlp -f ba -g URL` 提取直链 | 调用 `/x/player/playurl` API |
| 音质 | 不可控，yt-dlp 自动选 | 可选 64K/128K/192K/320K/FLAC |
| 速度 | 慢（需启动进程解析网页） | 快（直接 HTTP 请求） |
| 元数据 | 简陋 | 完整（封面/歌词/作者） |
| 稳定性 | B站改版可能失效 | API 更稳定 |

---

## 二、技术方案

### 2.1 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                  手机 App (Vue3 + Capacitor)                  │
│   🎵 B站音乐  │  📺 视频投屏  │  🔊 音箱控制  │  ⚙ 设置    │
└────────────────────────┬─────────────────────────────────────┘
                         │ REST API + WebSocket
                         ▼
┌──────────────────────────────────────────────────────────────┐
│               Python 后端 (FastAPI)                           │
│                                                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │ ① B站音乐模块 (bilibili-api)                            │ │
│  │    搜索 → 视频信息 → 音频流(320K/FLAC) → 歌词          │ │
│  └────────────────────┬────────────────────────────────────┘ │
│                       │                                      │
│  ┌────────────────────┴────────────────────────────────────┐ │
│  │ ② 小爱音箱推送模块 (复用 xiaomusic)                      │ │
│  │    miservice 登录 → 设备发现 → play_by_music_url        │ │
│  │    音频代理服务器 → FFmpeg 实时转码                      │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │ ③ 视频投屏模块 (m3u8-extractor + ydls + DLNA)           │ │
│  │    Playwright 嗅探 → yt-dlp 提取 → FFmpeg 转码          │ │
│  │    async_upnp_client → DLNA 推送                        │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │ ④ 转码服务 (FFmpeg)                                     │ │
│  │    音频: m4a/flac → mp3 (音箱兼容)                      │ │
│  │    视频: m3u8/ts → mp4 (DLNA 兼容)                      │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### 2.2 转码方案（核心问题）

#### 问题：为什么需要转码？

小爱音箱的固件限制：
- 部分型号不支持 FLAC（L05B/L05C/LX06/L16A）
- 部分型号不支持 MP4/AAC 容器（LX06 等）
- B站 DASH 格式返回的是 m4a 音频，部分音箱不兼容

DLNA 电视的限制：
- 部分电视不支持 m3u8/HLS
- 部分电视不支持 DASH/mpd
- 部分电视不支持 HEVC 编码

#### 方案：FFmpeg 实时转码代理

**参考 xiaomusic 的实现**（已验证可行）：

```python
# 音频转码：m4a/flac → mp3（给音箱）
async def ffmpeg_audio_proxy(url, output_format="mp3", bitrate="128k"):
    cmd = [
        "ffmpeg", "-y",
        "-i", url,           # 输入：B站音频直链
        "-vn",               # 去掉视频
        "-acodec", "libmp3lame",
        "-b:a", bitrate,
        "-f", output_format,
        "pipe:1"             # 输出到 stdout（流式）
    ]
    process = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE
    )
    # 流式返回给音箱
    async for chunk in process.stdout:
        yield chunk
```

```python
# 视频转码：m3u8 → mp4（给 DLNA 电视）
async def ffmpeg_video_proxy(url, output_format="mpegts"):
    cmd = [
        "ffmpeg", "-y",
        "-i", url,           # 输入：m3u8 链接
        "-c:v", "copy",      # 视频不转码（快）
        "-c:a", "aac",       # 音频转 AAC（兼容）
        "-f", output_format,
        "pipe:1"
    ]
    # 如果电视不支持视频编码，改为:
    # "-c:v", "libx264", "-preset", "ultrafast"
```

#### 转码决策表

| 源格式 | 目标设备 | 是否转码 | FFmpeg 参数 |
|--------|----------|----------|-------------|
| B站 320K mp3 | 小爱音箱 | ❌ 不需要 | 直接推送 |
| B站 FLAC | 小爱音箱 | ⚠️ 看型号 | `-acodec libmp3lame -b:a 320k` |
| B站 m4a (DASH) | 小爱音箱 | ✅ 需要 | `-vn -acodec libmp3lame -b:a 128k -f mp3` |
| m3u8/HLS | DLNA 电视 | ⚠️ 看电视 | `-c copy -f mpegts` 或 `-c:v copy -c:a aac` |
| mp4 直链 | DLNA 电视 | ❌ 不需要 | 直接推送 |
| flv | DLNA 电视 | ✅ 需要 | `-c:v copy -c:a aac -f mpegts` |

### 2.3 B站音乐推音箱的完整流程

```
用户点击"推送到小爱音箱"
    │
    ▼
1. 调用 B站 API 获取音频流信息
   GET /x/player/playurl?bvid=xxx&cid=xxx&qn=30280&fnval=16
    │
    ▼
2. 解析返回的 DASH 音频 URL
   优先级: FLAC > 320K > 192K > 128K
   URL 格式: https://xxx.bilivideo.com/xxx.m4a?xxx
    │
    ▼
3. 创建代理 URL（短 token 替代长 URL，避免超出音箱 URL 长度限制）
   http://server:port/proxy/audio?token=abc123
    │
    ▼
4. 推送到音箱
   mina_service.play_by_music_url(device_id, proxy_url, audio_id=xxx)
    │
    ▼
5. 音箱请求代理 URL
   → 后端从 B站 CDN 拉取音频
   → FFmpeg 实时转码（如果需要）
   → 流式返回给音箱
```

### 2.4 视频投屏的完整流程

```
用户输入视频网站 URL
    │
    ▼
1. 判断 URL 类型
   ├── B站视频 → B站 API 提取
   └── 其他网站 → m3u8-extractor 嗅探
       ├── Playwright 打开页面
       ├── 拦截 m3u8/mp4 请求
       ├── uBlock 过滤广告
       └── 提取集数列表
    │
    ▼
2. 返回集数列表给前端
   [{ index: 1, title: "第01集", url: "xxx" }, ...]
    │
    ▼
3. 用户选择集数 + 选择 DLNA 设备
    │
    ▼
4. 创建转码代理 URL
   http://server:port/proxy/video?token=def456
    │
    ▼
5. DLNA 推送
   async_upnp_client → SetAVTransportURI(proxy_url)
   → Play()
    │
    ▼
6. 电视请求代理 URL
   → 后端从源站拉取视频流
   → FFmpeg 实时转码（如果需要）
   → 流式返回给电视
```

---

## 三、项目结构

```
bilibili-music/
├── backend/                          # Python 后端（替换 Go）
│   ├── app/
│   │   ├── __init__.py
│   │   ├── main.py                   # FastAPI 入口
│   │   ├── config.py                 # 配置管理
│   │   ├── api/
│   │   │   ├── router.py             # 路由注册
│   │   │   ├── music.py              # B站音乐 API
│   │   │   ├── favlist.py            # 收藏夹 API
│   │   │   ├── playlist.py           # 歌单 API
│   │   │   ├── speaker.py            # 音箱控制 API
│   │   │   ├── cast.py               # 投屏控制 API
│   │   │   └── sniff.py              # 资源嗅探 API
│   │   ├── service/
│   │   │   ├── music_service.py      # 音乐业务逻辑
│   │   │   ├── speaker_service.py    # 音箱推送逻辑
│   │   │   ├── cast_service.py       # 投屏推送逻辑
│   │   │   └── sniff_service.py      # 嗅探逻辑
│   │   ├── bilibili/
│   │   │   ├── client.py             # B站 API 客户端（从 Go 移植）
│   │   │   ├── audio.py              # 音频流获取
│   │   │   ├── search.py             # 搜索
│   │   │   ├── video.py              # 视频信息
│   │   │   └── favlist.py            # 收藏夹
│   │   ├── speaker/
│   │   │   ├── auth.py               # 小米账号登录（复用 xiaomusic）
│   │   │   ├── device_manager.py     # 设备发现（复用 xiaomusic）
│   │   │   └── mina_service.py       # MiNA API 封装（复用 xiaomusic）
│   │   ├── dlna/
│   │   │   ├── discovery.py          # DLNA 设备发现（async_upnp_client）
│   │   │   └── controller.py         # DLNA 推送控制
│   │   ├── proxy/
│   │   │   ├── audio_proxy.py        # 音频代理 + FFmpeg 转码
│   │   │   ├── video_proxy.py        # 视频代理 + FFmpeg 转码
│   │   │   └── token_store.py        # 短 token → 真实 URL 映射
│   │   ├── sniffer/
│   │   │   ├── extractor.py          # m3u8-extractor 封装
│   │   │   └── episode_parser.py     # 集数解析
│   │   └── model/
│   │       ├── user.py               # 用户模型
│   │       ├── playlist.py           # 歌单模型
│   │       └── history.py            # 播放历史模型
│   ├── configs/
│   │   └── config.yaml               # 配置文件
│   ├── requirements.txt
│   ├── Dockerfile
│   └── docker-compose.yml
│
├── frontend/                         # Vue3 前端（保留，改造）
│   └── ...（现有结构，新增投屏相关组件）
│
├── reference/                        # 参考代码（不直接修改）
│   ├── xiaomusic/                    # xiaomusic 源码
│   └── MiAir/                        # MiAir 源码
│   └── nisheng/                      # 拟声 源码
```

---

## 四、从 Go 移植到 Python 的映射

| Go 模块 | Python 替代 | 说明 |
|---------|-------------|------|
| `gin` | `fastapi` | Web 框架 |
| `gorm` | `sqlalchemy` + `alembic` | ORM + 迁移 |
| `go-redis/v9` | `redis[hiredis]` | Redis 客户端 |
| `zap` | `loguru` | 日志 |
| `viper` | `pydantic-settings` + `yaml` | 配置 |
| `bilibili/client.go` | `httpx` + 直接移植 | B站 API |
| `bilibili/audio.go` | 直接移植逻辑 | 音频流获取 |
| 无 | `miservice` (xiaomusic) | 小米音箱控制 |
| 无 | `async_upnp_client` | DLNA 控制 |
| 无 | `yt-dlp` (Python 库) | 视频提取 |
| 无 | `playwright` | 网页嗅探 |
| 无 | `ffmpeg-python` / subprocess | 转码 |

---

## 五、实施步骤

### Phase 1: Python 后端骨架（替换 Go）

**任务 1.1**: 初始化 Python 项目
- 创建 `backend/app/` 目录结构
- `requirements.txt` 依赖清单
- FastAPI 入口 + 配置管理
- Dockerfile + docker-compose.yml

**任务 1.2**: 移植 B站 API 客户端
- 从 Go `bilibili/client.go` 移植为 Python `httpx` 版本
- 移植搜索 API（`search.go` → `search.py`）
- 移植视频信息 API（`video.go` → `video.py`）
- 移植音频流 API（`audio.go` → `audio.py`）- **关键：保留音质选择逻辑**
- 移植收藏夹 API（`favlist.go` → `favlist.py`）

**任务 1.3**: 移植 HTTP API 层
- 路由注册（`router.go` → `router.py`）
- 音乐处理器（`music.go` → `music.py`）
- 收藏夹处理器（`favlist.go` → `favlist.py`）
- CORS + 日志中间件
- 音频流代理（解决跨域）

**任务 1.4**: 数据库迁移
- SQLAlchemy 模型定义（User/Playlist/PlaylistSong/PlayHistory/LyricsCache）
- Alembic 迁移脚本
- Redis 缓存封装

### Phase 2: 小爱音箱推送

**任务 2.1**: 小米账号集成
- 复用 xiaomusic 的 `miservice` 库
- 实现小米账号登录（`auth.py`）
- 设备发现与列表获取（`device_manager.py`）

**任务 2.2**: 音频代理服务器
- 实现 token → URL 映射（短 token 替代长 URL）
- 音频流代理（从 B站 CDN 拉取 → 流式返回）
- FFmpeg 实时转码（m4a → mp3，参考 xiaomusic 的 `_ffmpeg_mp3_stream`）

**任务 2.3**: 音箱推送 API
- `POST /api/v1/speaker/devices` - 获取设备列表
- `POST /api/v1/speaker/play` - 推送 B站音乐到音箱
- `POST /api/v1/speaker/control` - 播放控制（暂停/停止/音量）
- `GET /api/v1/speaker/status` - 播放状态

**任务 2.4**: B站音乐 → 音箱完整链路
- B站 API 获取音频 URL → 代理转码 → play_by_music_url 推送
- 音质自动选择（根据音箱型号）
- 播放状态轮询 + 自动下一首

### Phase 3: 视频投屏

**任务 3.1**: 资源嗅探
- 集成 m3u8-extractor（Playwright + yt-dlp）
- 实现集数解析（从页面提取第1集/第2集/...）
- 广告过滤（uBlock 集成）

**任务 3.2**: 视频代理 + 转码
- 视频流代理（m3u8 → 电视）
- FFmpeg 实时转码（格式不兼容时）
- Range 请求支持（拖动进度条）

**任务 3.3**: DLNA 推送
- async_upnp_client 设备发现
- SetAVTransportURI + Play 控制
- 播放/暂停/停止/Seek 控制

**任务 3.4**: 投屏 API
- `POST /api/v1/sniff` - 嗅探视频资源
- `GET /api/v1/cast/devices` - 发现 DLNA 设备
- `POST /api/v1/cast/start` - 开始投屏
- `POST /api/v1/cast/control` - 播放控制
- `GET /api/v1/cast/status` - 投屏状态

### Phase 4: 前端改造

**任务 4.1**: 音箱控制 UI
- 设备选择组件
- 推送按钮
- 播放控制条

**任务 4.2**: 投屏 UI
- URL 输入框
- 集数列表
- 设备选择 + 投屏控制

### Phase 5: 部署与优化

**任务 5.1**: Docker 部署
- Python 后端 Dockerfile
- docker-compose.yml（含 FFmpeg）
- NAS 部署文档

**任务 5.2**: 性能优化
- 音频预缓冲（参考 MiAir 的 MediaBuffer）
- 转码缓存（已转码的文件缓存到磁盘）
- 并发控制

---

## 六、依赖清单

```
# Web 框架
fastapi>=0.115.0
uvicorn[standard]>=0.34.0
pydantic>=2.10.0
pydantic-settings>=2.7.0

# 数据库
sqlalchemy>=2.0.0
alembic>=1.14.0
aiomysql>=0.2.0

# 缓存
redis[hiredis]>=5.0.0

# HTTP 客户端
httpx>=0.28.0

# B站 API
bilibili-api-python>=16.0.0    # 可选，或自建

# 小米音箱
miservice-fork>=2.0.0          # xiaomusic 使用的 fork

# DLNA
async-upnp-client>=0.42.0

# 视频嗅探
yt-dlp>=2025.1.0
playwright>=1.49.0

# 转码
ffmpeg-python>=0.2.0           # 可选，或直接 subprocess

# 日志
loguru>=0.7.0

# 工具
pyyaml>=6.0
python-multipart>=0.0.18
websockets>=14.0
```

---

## 七、风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| B站 API 需要登录 Cookie | 高 | 支持用户配置 SESSDATA，或用 wbi 签名 |
| 小爱音箱型号兼容性 | 中 | 参考 xiaomusic 的型号兼容列表，自动选择播放方式 |
| FFmpeg 转码延迟 | 中 | 预缓冲 + copy 模式优先（不转码） |
| Playwright 资源占用 | 高 | 限制并发数，用完即关，或改用 yt-dlp 直接提取 |
| DLNA 设备兼容性 | 中 | 参考 Go2TV 的兼容性处理 |
