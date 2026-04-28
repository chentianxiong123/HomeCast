from fastapi import APIRouter, Query, Path, Request
from fastapi.responses import Response
from app.service.music_service import MusicService
from app.bilibili.audio import AUDIO_64K
from app.proxy.audio_proxy import proxy_bvid_audio
from app.proxy.bvid_cache import bvid_cache

router = APIRouter(prefix="/music", tags=["music"])

_music_service: MusicService | None = None


def set_music_service(service: MusicService):
    global _music_service
    _music_service = service


def get_music_service() -> MusicService:
    if _music_service is None:
        raise RuntimeError("MusicService not initialized")
    return _music_service


@router.get("/search")
async def search(
    keyword: str = Query(..., description="搜索关键词"),
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=50),
):
    service = get_music_service()
    result = await service.search(keyword, page, page_size)
    return {"code": 0, "message": "success", "data": result.model_dump()}


@router.get("/info/{bvid}")
async def get_video_info(bvid: str = Path(..., description="BV号")):
    service = get_music_service()
    result = await service.get_video_info(bvid)
    return {"code": 0, "message": "success", "data": result.model_dump()}


@router.get("/audio/{bvid}")
async def get_audio_info(
    bvid: str = Path(..., description="BV号"),
    quality: int = Query(AUDIO_64K, description="音质"),
):
    """
    获取音频信息，包括是否有本地缓存

    返回:
    - url: 音频流URL（/api/v1/music/stream/{bvid}）
    - cached: 是否有本地MP3缓存
    - quality: 音质
    """
    service = get_music_service()
    result = await service.get_audio_stream(bvid, quality)

    # 检查是否有本地缓存
    has_cache = bvid_cache.exists(bvid)

    return {
        "code": 0,
        "message": "success",
        "data": {
            "url": f"/api/v1/music/stream/{bvid}?quality={quality}",
            "bvid": bvid,
            "quality": result.quality,
            "size": result.size,
            "mime_type": result.mime_type,
            "codecs": result.codecs,
            "cached": has_cache,
        }
    }


@router.get("/stream/{bvid}")
async def proxy_audio_stream(
    request: Request,
    bvid: str = Path(..., description="BV号"),
    quality: int = Query(AUDIO_64K, description="音质"),
):
    """
    音频流端点

    - 有缓存: 返回本地MP3文件
    - 无缓存: 代理B站原始音频流（MP4容器）
    """
    return await proxy_bvid_audio(request, bvid, quality)
