from fastapi import APIRouter, Request
from app.proxy.audio_proxy import proxy_audio_handler, proxy_video_handler

router = APIRouter(tags=["proxy"])


@router.get("/proxy/audio/{token}")
async def proxy_audio(request: Request, token: str):
    return await proxy_audio_handler(request, token)


@router.get("/proxy/video/{token}")
async def proxy_video(request: Request, token: str):
    return await proxy_video_handler(request, token)
