"""
流式转码模块 - 边转边播

核心原理：
1. FFmpeg 从输入源读取，输出到 stdout (pipe)
2. 异步 generator 实时读取 stdout 的 chunk
3. 通过 StreamingResponse 返回给客户端
4. 同时可选地将输出写入缓存文件

适用场景：
- 音乐播放：B站音频 → MP3 实时转码
- 视频投屏：各种格式 → MPEGTS 实时转码
"""

import asyncio
import subprocess
import tempfile
import os
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
from loguru import logger
from app.config import get_config


# 线程池用于执行同步的 subprocess 操作
_executor = ThreadPoolExecutor(max_workers=4, thread_name_prefix="stream_transcoder")

# B站请求头
_BILIBILI_HEADERS = {
    "Referer": "https://www.bilibili.com",
    "Origin": "https://www.bilibili.com",
    "User-Agent": (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/120.0.0.0 Safari/537.36"
    ),
}


class StreamTranscoder:
    """
    流式转码器

    使用 FFmpeg 进行实时转码，输出通过 generator 流式返回
    """

    def __init__(self):
        self._config = get_config()
        self._ffmpeg_path = self._config.ffmpeg.path

    async def transcode_audio_stream(
        self,
        bvid: str,
        cid: int,
        output_format: str = "mp3",
        bitrate: str = "64k",
        sample_rate: int = 44100,
        channels: int = 2,
        cache_path: Path | None = None,
    ):
        """
        音频流式转码 - FFmpeg直接读取B站URL实时转码

        Args:
            bvid: B站视频ID
            cid: 视频CID
            output_format: 输出格式 (mp3, aac, flac 等)
            bitrate: 音频码率 (64k, 128k, 192k 等)
            sample_rate: 采样率
            channels: 声道数
            cache_path: 可选的缓存文件路径，如果提供则同时写入缓存

        Yields:
            bytes: 转码后的音频数据块
        """
        # 实时获取新鲜URL（避免过期）
        from app.bilibili.client import BilibiliClient
        from app.bilibili.audio import get_best_audio_url
        from app.config import get_config

        config = get_config()
        client = BilibiliClient(config.bilibili)

        try:
            stream = await get_best_audio_url(client, bvid, cid, 64)  # 64k quality
            input_url = stream.url
            logger.info(f"Got fresh URL for {bvid}: {input_url[:60]}...")
        except Exception as e:
            logger.error(f"Failed to get audio URL for {bvid}: {e}")
            raise

        # 构建 FFmpeg 命令
        cmd = [
            self._ffmpeg_path,
            "-y",
            "-hide_banner",
            "-loglevel", "error",
            "-headers", "Referer: https://www.bilibili.com\r\n",
            "-user_agent", _BILIBILI_HEADERS["User-Agent"],
            "-i", input_url,
            "-vn",
            "-acodec", self._get_audio_codec(output_format),
            "-b:a", bitrate,
            "-ar", str(sample_rate),
            "-ac", str(channels),
            "-f", output_format,
            "pipe:1",
        ]

        logger.info(f"Stream transcode: {bvid} -> {output_format} {bitrate}")

        # 启动 FFmpeg 进程 - stderr用DEVNULL避免死锁
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,  # 丢弃stderr避免缓冲区满阻塞
            bufsize=8192,
        )

        # 可选的缓存文件
        cache_file = None
        if cache_path:
            try:
                cache_file = open(cache_path, "wb")
                logger.debug(f"Opened cache file: {cache_path}")
            except Exception as e:
                logger.warning(f"Failed to open cache file: {e}")

        try:
            chunk_size = 8192

            while True:
                # 使用 asyncio.to_thread 读取stdout
                chunk = await asyncio.to_thread(
                    process.stdout.read,
                    chunk_size
                )

                if not chunk:
                    break

                if cache_file:
                    await asyncio.to_thread(cache_file.write, chunk)

                yield chunk

        except asyncio.CancelledError:
            logger.info("Stream transcoding cancelled by client")
            # 先关闭文件，再清理进程，最后删除未完成缓存
            if cache_file:
                try:
                    cache_file.close()
                    cache_file = None
                except Exception:
                    pass
            await self._cleanup_process(process)
            # 删除未完成的缓存文件
            if cache_path and cache_path.exists():
                try:
                    cache_path.unlink(missing_ok=True)
                    logger.debug(f"Deleted incomplete cache: {cache_path.name}")
                except Exception as e:
                    logger.warning(f"Failed to delete incomplete cache: {e}")
            raise

        except Exception as e:
            logger.error(f"Stream transcoding error: {e}")
            # 出错时也清理
            if cache_file:
                try:
                    cache_file.close()
                    cache_file = None
                except Exception:
                    pass
            await self._cleanup_process(process)
            if cache_path and cache_path.exists():
                try:
                    cache_path.unlink(missing_ok=True)
                except Exception:
                    pass
            raise

        finally:
            # 正常结束时的清理
            if cache_file:
                try:
                    cache_file.close()
                    if cache_path and cache_path.exists() and cache_path.stat().st_size > 0:
                        logger.info(f"Cache saved: {cache_path.name} ({cache_path.stat().st_size} bytes)")
                    else:
                        cache_path.unlink(missing_ok=True)
                except Exception as e:
                    logger.warning(f"Cache cleanup error: {e}")
            # 确保进程已清理
            await self._cleanup_process(process)

    async def transcode_video_stream(
        self,
        input_url: str,
        output_format: str = "mpegts",
        video_codec: str = "copy",
        audio_codec: str = "aac",
        seek_seconds: float | None = None,
    ):
        """
        视频流式转码（用于投屏）

        Args:
            input_url: 输入视频URL
            output_format: 输出格式 (mpegts, mp4 等)
            video_codec: 视频编码 (copy, libx264 等)
            audio_codec: 音频编码 (aac, copy 等)
            seek_seconds: 可选的跳转时间

        Yields:
            bytes: 转码后的视频数据块
        """
        cmd = [
            self._ffmpeg_path,
            "-y",
            "-hide_banner",
            "-loglevel", "error",
        ]

        if seek_seconds is not None:
            cmd.extend(["-ss", str(seek_seconds)])

        cmd.extend([
            "-i", input_url,
            "-c:v", video_codec,
            "-c:a", audio_codec,
            "-f", output_format,
            "pipe:1",
        ])

        logger.info(f"Video stream transcode: {input_url[:60]}... -> {output_format}")

        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            bufsize=65536,
        )

        try:
            loop = asyncio.get_event_loop()
            chunk_size = 65536  # 视频用更大的块

            while True:
                chunk = await loop.run_in_executor(
                    _executor,
                    process.stdout.read,
                    chunk_size
                )

                if not chunk:
                    break

                yield chunk

        except asyncio.CancelledError:
            logger.info("Video stream transcoding cancelled")
            raise

        except Exception as e:
            logger.error(f"Video stream transcoding error: {e}")
            raise

        finally:
            await self._cleanup_process(process)

    async def _download_bilibili_audio(self, url: str) -> str:
        """下载B站音频到临时文件"""
        import httpx

        fd, temp_path = tempfile.mkstemp(suffix=".m4a")
        os.close(fd)

        def _download():
            with httpx.Client(
                timeout=httpx.Timeout(120.0, connect=30.0),
                follow_redirects=True
            ) as client:
                with client.stream("GET", url, headers=_BILIBILI_HEADERS) as resp:
                    resp.raise_for_status()
                    downloaded = 0
                    with open(temp_path, "wb") as f:
                        for chunk in resp.iter_bytes(chunk_size=65536):
                            if chunk:
                                f.write(chunk)
                                downloaded += len(chunk)
                    return downloaded

        loop = asyncio.get_event_loop()
        downloaded = await loop.run_in_executor(_executor, _download)
        logger.info(f"Downloaded B站 audio: {temp_path} ({downloaded} bytes)")
        return temp_path

    async def _cleanup_process(self, process: subprocess.Popen):
        """清理 FFmpeg 进程"""
        try:
            # 先尝试优雅终止
            process.terminate()
            # 使用 asyncio.to_thread 等待进程结束
            await asyncio.wait_for(
                asyncio.to_thread(process.wait),
                timeout=2.0
            )
        except asyncio.TimeoutError:
            # 强制杀死
            try:
                process.kill()
                await asyncio.to_thread(process.wait)
            except Exception:
                pass
        except Exception as e:
            logger.warning(f"Process cleanup error: {e}")

    @staticmethod
    def _get_audio_codec(output_format: str) -> str:
        """根据输出格式获取对应的音频编码器"""
        codec_map = {
            "mp3": "libmp3lame",
            "aac": "aac",
            "m4a": "aac",
            "flac": "flac",
            "wav": "pcm_s16le",
            "ogg": "libvorbis",
            "opus": "libopus",
        }
        return codec_map.get(output_format, "copy")


# 全局实例
stream_transcoder = StreamTranscoder()
