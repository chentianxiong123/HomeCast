from fastapi import APIRouter, Query, Path, Request
from fastapi.responses import Response
from app.service.music_service import MusicService
from app.bilibili.audio import AUDIO_192K
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
    quality: int = Query(AUDIO_192K, description="音质"),
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
    quality: int = Query(AUDIO_192K, description="音质"),
):
    """音频流端点（DASH 直转，无 ffmpeg/无缓存）"""
    return await proxy_bvid_audio(request, bvid, quality)


# ── 歌词（对齐桌面播放器：网易云 API，免费无需登录） ──

@router.get("/lyric")
async def get_lyric(
    keyword: str = Query(..., description="歌名关键字"),
    sid: int | None = Query(None, description="指定网易云歌曲id"),
):
    """按歌名自动取第一个有词的歌词；或按 sid 精确取"""
    from app.service.lyric_service import get_lyric_lines

    result = await get_lyric_lines(keyword, sid)
    if not result:
        # 找不到歌词是正常业务（很多歌无词），code=0 避免前端拦截器当错误弹框
        return {"code": 0, "message": "success", "data": None}
    return {"code": 0, "message": "success", "data": result}


@router.get("/lyric/candidates")
async def lyric_candidates(
    keyword: str = Query(..., description="歌名关键字"),
    limit: int = Query(8, ge=1, le=20),
):
    """搜索歌词候选列表（只含有词的），前端可弹窗选择"""
    from app.service.lyric_service import search_candidates

    cands = await search_candidates(keyword, limit)
    return {"code": 0, "message": "success", "data": cands}
