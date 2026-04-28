import asyncio
import subprocess
from concurrent.futures import ThreadPoolExecutor
from loguru import logger
from app.config import get_config


_executor = ThreadPoolExecutor(max_workers=2)


def _run_ffmpeg_subprocess(cmd: list[str]) -> subprocess.Popen:
    return subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


async def ffmpeg_audio_transcode(
    url: str,
    output_format: str = "mp3",
    bitrate: str = "128k",
    seek_seconds: float | None = None,
) -> subprocess.Popen:
    """实时流式转码（用于 speaker/DLNA 推送）"""
    config = get_config()
    ffmpeg_path = config.ffmpeg.path

    cmd = [ffmpeg_path, "-y"]

    if seek_seconds is not None:
        cmd.extend(["-ss", str(seek_seconds)])

    # 对于B站URL，尝试直接用 ffmpeg 读取（带headers）
    if "bilibili" in url.lower():
        cmd.extend([
            "-headers", "Referer: https://www.bilibili.com\r\n",
            "-user_agent", (
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/120.0.0.0 Safari/537.36"
            ),
        ])

    cmd.extend([
        "-i", url,
        "-vn",
        "-acodec", "libmp3lame" if output_format == "mp3" else "copy",
        "-b:a", bitrate,
        "-f", output_format,
        "pipe:1",
    ])

    logger.info(f"FFmpeg audio transcode: {url[:80]}... -> {output_format} {bitrate}")

    loop = asyncio.get_event_loop()
    process = await loop.run_in_executor(_executor, _run_ffmpeg_subprocess, cmd)
    return process


async def ffmpeg_video_transcode(
    url: str,
    output_format: str = "mpegts",
    video_codec: str = "copy",
    audio_codec: str = "aac",
    seek_seconds: float | None = None,
) -> subprocess.Popen:
    config = get_config()
    ffmpeg_path = config.ffmpeg.path

    cmd = [ffmpeg_path, "-y"]

    if seek_seconds is not None:
        cmd.extend(["-ss", str(seek_seconds)])

    cmd.extend([
        "-i", url,
        "-c:v", video_codec,
        "-c:a", audio_codec,
        "-f", output_format,
        "pipe:1",
    ])

    logger.info(f"FFmpeg video transcode: {url[:80]}... -> {output_format}")

    loop = asyncio.get_event_loop()
    process = await loop.run_in_executor(_executor, _run_ffmpeg_subprocess, cmd)
    return process


def needs_audio_transcode(mime_type: str, hardware: str = "") -> bool:
    no_flac_models = {"L05B", "L05C", "LX06", "L16A"}
    no_mp4_models = {"LX06"}

    if hardware in no_mp4_models:
        if mime_type in ("audio/mp4", "audio/aac", "audio/x-m4a", "video/mp4"):
            return True

    if hardware in no_flac_models:
        if mime_type in ("audio/flac", "audio/x-flac"):
            return True

    if mime_type in ("audio/mp4", "audio/aac", "audio/x-m4a"):
        return True

    return False


def needs_video_transcode(content_type: str, url: str) -> bool:
    if ".m3u8" in url or "m3u8" in content_type:
        return True
    if content_type in ("video/x-flv", "flv"):
        return True
    return False
