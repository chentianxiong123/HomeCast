import os
import asyncio

import httpx
from fastapi import Request
from fastapi.responses import FileResponse, Response, StreamingResponse
from loguru import logger

from app.proxy.token_store import token_store
from app.proxy.bvid_cache import bvid_cache
from app.proxy.stream_transcoder import stream_transcoder
from app.proxy.transcoder import (
    needs_audio_transcode,
    needs_video_transcode,
)


_BILIBILI_HEADERS = {
    "Referer": "https://www.bilibili.com",
    "Origin": "https://www.bilibili.com",
    "User-Agent": (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/120.0.0.0 Safari/537.36"
    ),
}


async def _stream_from_url(url: str, req_headers: dict | None = None, chunk_size: int = 8192):
    headers = dict(_BILIBILI_HEADERS)
    if "bilibili" not in url.lower():
        headers = {"User-Agent": _BILIBILI_HEADERS["User-Agent"]}
    if req_headers:
        headers.update(req_headers)

    client = httpx.AsyncClient(timeout=60, follow_redirects=True)
    resp = await client.send(
        client.build_request("GET", url, headers=headers),
        stream=True
    )

    resp_headers = dict(resp.headers)
    out_headers = {
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Methods": "GET, HEAD, OPTIONS",
        "Access-Control-Allow-Headers": "*",
        "Access-Control-Expose-Headers": "Content-Length, Content-Range",
    }
    content_type = resp_headers.get("content-type", "audio/mpeg")
    out_headers["content-type"] = content_type

    status_code = resp.status_code

    async def gen():
        try:
            async for chunk in resp.aiter_bytes(chunk_size):
                yield chunk
        except Exception as e:
            logger.warning(f"Stream interrupted: {e}")
        finally:
            await resp.aclose()
            await client.aclose()

    return StreamingResponse(gen(), status_code=status_code, headers=out_headers)


async def proxy_audio_handler(request: Request, token: str):
    data = token_store.get(token)
    if not data:
        return Response(content=b"token expired or invalid", status_code=404)

    url = data["url"]
    metadata = data.get("metadata", {})
    hardware = metadata.get("hardware", "")
    mime_type = metadata.get("mime_type", "")

    # 基于BVID的缓存检查
    bvid = metadata.get("bvid", "")
    if bvid and bvid_cache.exists(bvid):
        cached_path = bvid_cache.get_path(bvid)
        if cached_path:
            logger.debug(f"Serving cached mp3 for bvid={bvid}: {cached_path.name}")
            return FileResponse(
                path=str(cached_path),
                media_type="audio/mpeg",
                headers={"Access-Control-Allow-Origin": "*", "Accept-Ranges": "bytes"},
            )

    if needs_audio_transcode(mime_type, hardware):
        logger.info(f"Audio needs transcode: {mime_type} hardware={hardware}")

        cache_path = bvid_cache._cache_path(bvid) if bvid else None

        async def transcode_stream():
            async for chunk in stream_transcoder.transcode_audio_stream(
                input_url=url,
                output_format="mp3",
                bitrate="64k",
                cache_path=cache_path,
            ):
                yield chunk

        return StreamingResponse(
            transcode_stream(),
            media_type="audio/mpeg",
            headers={
                "Access-Control-Allow-Origin": "*",
                "Accept-Ranges": "none",
            },
        )

    range_header = request.headers.get("range")
    req_headers = {}
    if range_header:
        req_headers["Range"] = range_header

    return await _stream_from_url(url, req_headers)


async def proxy_bvid_audio(request: Request, bvid: str, quality: int = 192):
    """
    音乐流代理端点（对齐桌面项目已验证方案：DASH 音频直转，无 ffmpeg）

    策略：取 B站 DASH 纯音频流(baseUrl)，带 referer 头直接转发。
    WebView/浏览器 audio 元素原生可播 AAC/mp4a，无需转码。
    支持 Range（seek/进度拖动）。
    """
    from app.bilibili.client import BilibiliClient
    from app.bilibili.video import get_video_info
    from app.bilibili.audio import get_best_audio_url
    from app.config import get_config

    config = get_config()
    client = BilibiliClient(config.bilibili)

    try:
        info = await get_video_info(client, bvid)
        stream = await get_best_audio_url(client, bvid, info.cid, quality)
    except Exception as e:
        logger.error(f"Failed to get audio URL for {bvid}: {e}")
        return Response(content=f"Failed to get audio: {e}", status_code=500)

    url = stream.url
    if not url:
        return Response(content="Empty audio URL", status_code=500)

    # DASH 音频直转（B站流带 referer 头可直下，浏览器原生可播）
    range_header = request.headers.get("range")
    req_headers = {}
    if range_header:
        req_headers["Range"] = range_header

    return await _stream_from_url(url, req_headers)
async def proxy_video_handler(request: Request, token: str):
    data = token_store.get(token)
    if not data:
        return Response(content=b"token expired or invalid", status_code=404)

    url = data["url"]
    metadata = data.get("metadata", {})
    content_type = metadata.get("content_type", "")
    referer = metadata.get("referer", "")

    if needs_video_transcode(content_type, url):
        logger.info(f"Video needs transcode: {content_type}")

        async def video_transcode_stream():
            async for chunk in stream_transcoder.transcode_video_stream(
                input_url=url,
                output_format="mpegts",
                video_codec="copy",
                audio_codec="aac",
            ):
                yield chunk

        return StreamingResponse(
            video_transcode_stream(),
            media_type="video/mp2t",
            headers={
                "Access-Control-Allow-Origin": "*",
                "Accept-Ranges": "none",
            },
        )

    range_header = request.headers.get("range")
    req_headers = {}
    if referer:
        req_headers["Referer"] = referer
    if range_header:
        req_headers["Range"] = range_header

    return await _stream_from_url(url, req_headers, chunk_size=65536)
