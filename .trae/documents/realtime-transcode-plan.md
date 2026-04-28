# 音频实时转码推流方案

## 问题分析

当前 `/api/v1/music/stream/{bvid}` 直接代理B站原始MP4流给前端，浏览器报 `Format error`：
- B站返回的是**完整视频MP4**（包含视频轨道+音频轨道），不是纯音频
- `moov` box 高达163KB，`mdat` 50MB+，浏览器加载困难
- `<video>` 元素播放这种"音频伪装成视频"的MP4，格式兼容性差

## 方案选择

### 方案A：实时转码推流（推荐 ✅）

后端用 FFmpeg 从B站URL实时读取 → 转码为纯MP3 → 边转边推给前端 → 同时写入缓存文件

**优点：**
- 第一次播放就能听到MP3（延迟仅2-3秒FFmpeg启动时间）
- MP3格式浏览器100%兼容，用 `<audio>` 元素即可
- 转码同时写入缓存，下次直接返回缓存文件
- 不需要额外后台任务

**技术可行性：**
- `stream_transcoder.py` 已有完整实现：FFmpeg stdout → async generator → StreamingResponse
- `_download_bilibili_audio()` 方法已能下载B站durl音频
- 唯一需要改的是：`proxy_bvid_audio()` 调用 `stream_transcoder.transcode_audio_stream()` 而不是 `_stream_from_url()`

### 方案B：代理原始流 + 后台缓存

第一次代理B站原始MP4给前端，后台同时下载+转码缓存MP3

**缺点：**
- 第一次播放仍然有格式问题（浏览器Format error）
- 需要两套逻辑：代理流 + 后台缓存
- 用户体验差：第一次播放可能失败

**结论：不采用**，因为第一次播放体验无法保证

## 实施计划（方案A）

### Step 1: 修改 `audio_proxy.py::proxy_bvid_audio()`

当前逻辑：
1. 有缓存 → 返回缓存MP3 ✅
2. 无缓存 → 代理B站原始MP4流 ❌（前端Format error）
3. 触发后台转码 ❌（用yt-dlp，会412）

改为：
1. 有缓存 → 返回缓存MP3 ✅（不变）
2. 无缓存 → **实时转码推流**：FFmpeg读取B站URL → MP3 → StreamingResponse
3. 转码同时写入缓存文件（下次直接返回缓存）

### Step 2: 修改 `stream_transcoder.py::transcode_audio_stream()`

当前逻辑：先 `_download_bilibili_audio()` 下载完整文件到临时路径，再FFmpeg读取临时文件转码

改为：
- 对于B站URL，**不再先下载整个文件**
- 直接让FFmpeg从B站URL读取（带 `-headers` 和 `-user_agent` 参数）
- FFmpeg边读边转码，输出MP3到stdout
- 后端从stdout读取chunk，同时yield给HTTP响应和写入缓存文件

关键FFmpeg命令：
```
ffmpeg -y -hide_banner -loglevel error
  -headers "Referer: https://www.bilibili.com\r\n"
  -user_agent "Mozilla/5.0 ..."
  -i <B站durl>
  -vn -acodec libmp3lame -b:a 64k -ar 44100 -ac 2
  -f mp3 pipe:1
```

### Step 3: 修改 `bvid_cache.py`

当前 `create_async()` 使用 `transcode_bilibili_audio_to_file()` → 依赖 yt-dlp（会412）

改为：
- `create_async()` 改为调用 `stream_transcoder.transcode_audio_stream()` 并写入缓存文件
- 或者直接在 `proxy_bvid_audio()` 的实时转码过程中写入缓存，不再需要单独的后台任务

### Step 4: 修改前端 `player.ts`

- 使用 `<audio>` 元素（不再是 `<video>`）
- MP3格式所有浏览器都支持，不需要video元素
- 移除 `initMediaElement(true)`，改为 `initMediaElement(false)`

### Step 5: 清理不再需要的代码

- `transcoder.py` 中的 `_download_with_ytdlp_sync()` 可移除（yt-dlp方案已废弃）
- `transcoder.py` 中的 `transcode_bilibili_audio_to_file()` 可移除
- `bvid_cache.py` 中对 `transcoder.py` 的依赖改为对 `stream_transcoder.py` 的依赖

## 文件修改清单

| 文件 | 修改内容 |
|------|---------|
| `backend/app/proxy/audio_proxy.py` | `proxy_bvid_audio()` 改为调用实时转码 |
| `backend/app/proxy/stream_transcoder.py` | `transcode_audio_stream()` 改为FFmpeg直接读B站URL |
| `backend/app/proxy/bvid_cache.py` | `create_async()` 改用 stream_transcoder |
| `backend/app/proxy/transcoder.py` | 清理yt-dlp相关代码 |
| `frontend/src/core/player.ts` | 改用 `<audio>` 元素播放MP3 |

## 数据流

```
第一次播放:
前端 → GET /api/v1/music/stream/BVxxx?quality=64
后端 → 检查缓存(无) → 获取B站durl URL
后端 → FFmpeg -i <durl> -vn -acodec libmp3lame -b:a 64k -f mp3 pipe:1
后端 → 读取stdout chunk → 同时yield给HTTP响应 + 写入缓存文件
前端 → <audio> 播放MP3流 ✅

第二次播放:
前端 → GET /api/v1/music/stream/BVxxx?quality=64
后端 → 检查缓存(有) → 直接返回 {bvid}.mp3 文件
前端 → <audio> 播放MP3 ✅
```
