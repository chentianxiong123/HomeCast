from fastapi import APIRouter
from loguru import logger
from app.api.music import router as music_router, set_music_service
from app.api.favlist import router as favlist_router, set_favlist_service
from app.api.playlist import router as playlist_router, set_playlist_service
from app.api.speaker import router as speaker_router, set_speaker_service
from app.api.cast import router as cast_router, set_cast_service
from app.api.proxy import router as proxy_router
from app.api.cache import router as cache_router
from app.api.sites import router as sites_router
from app.service.music_service import MusicService
from app.service.favlist_service import FavListService
from app.service.playlist_service import PlaylistService
from app.service.speaker_service import SpeakerService
from app.service.cast_service import CastService
from app.bilibili.client import BilibiliClient
from app.speaker.auth import SpeakerAuth
from app.config import get_config

router = APIRouter()


def init_routes() -> APIRouter:
    config = get_config()
    client = BilibiliClient(config.bilibili)

    music_service = MusicService(client)
    favlist_service = FavListService(client)
    playlist_service = PlaylistService(client, data_dir="data")
    set_music_service(music_service)
    set_favlist_service(favlist_service)
    set_playlist_service(playlist_service)

    router.include_router(music_router, prefix="/api/v1")
    router.include_router(favlist_router, prefix="/api/v1")
    router.include_router(playlist_router, prefix="/api/v1")
    router.include_router(proxy_router)

    # 尝试自动登录（优先使用已保存的 Token）
    import os
    import nest_asyncio
    
    # 检查是否有已保存的 Token
    token_file = os.path.join("data", ".mi.token")
    auth_file = os.path.join("data", "auth.json")
    has_saved_token = os.path.exists(token_file) or os.path.exists(auth_file)
    
    # 检查是否有配置（账号密码或 Cookie）
    has_config = config.xiaomi.enable and (
        (config.xiaomi.account and config.xiaomi.password) or config.xiaomi.cookie
    )
    
    if has_config or has_saved_token:
        speaker_auth = SpeakerAuth(
            account=config.xiaomi.account,
            password=config.xiaomi.password,
            cookie=config.xiaomi.cookie,
            conf_path="data",
        )
        
        # 如果有 Token 文件但没有配置，尝试用 Token 登录
        if has_saved_token and not has_config:
            logger.info("Found saved token, trying auto-login...")
        
        # 尝试登录（使用 nest_asyncio 解决事件循环冲突）
        try:
            nest_asyncio.apply()
        except ImportError:
            pass
        
        import asyncio
        loop = asyncio.get_event_loop()
        login_success = loop.run_until_complete(speaker_auth.login())
        if login_success:
            speaker_service = SpeakerService(speaker_auth, client)
            set_speaker_service(speaker_service)
            logger.info("Xiaomi speaker service initialized successfully")
        else:
            logger.error("Xiaomi speaker login failed, service disabled")
            set_speaker_service(None)
    else:
        logger.info("Xiaomi speaker service disabled (no config or saved token)")
        set_speaker_service(None)
    router.include_router(speaker_router, prefix="/api/v1")

    cast_service = CastService()
    set_cast_service(cast_service)
    router.include_router(cast_router, prefix="/api/v1")

    # 缓存管理路由
    router.include_router(cache_router, prefix="/api/v1")

    # 网站管理路由
    router.include_router(sites_router, prefix="/api/v1")

    @router.get("/health")
    async def health():
        return {"status": "ok", "message": "bilibili music api is running"}

    return router
