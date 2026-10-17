from fastapi import APIRouter
from pydantic import BaseModel
from app.service.cast_service import CastService

router = APIRouter(prefix="/cast", tags=["cast"])

_cast_service: CastService | None = None


def set_cast_service(service: CastService):
    global _cast_service
    _cast_service = service


def get_cast_service() -> CastService:
    if _cast_service is None:
        raise RuntimeError("CastService not initialized")
    return _cast_service


class SniffRequest(BaseModel):
    url: str


class CastRequest(BaseModel):
    episode_url: str
    device_udn: str
    title: str = "Video"


class PlayUrlRequest(BaseModel):
    url: str
    title: str = "Video"


class ControlRequest(BaseModel):
    device_udn: str
    action: str
    target: str | None = None
    volume: int | None = None


@router.get("/devices")
async def get_devices(target_ip: str = None):
    service = get_cast_service()
    devices = await service.discover_devices(target_ip=target_ip)
    return {
        "code": 0,
        "message": "success",
        "data": devices,
    }


@router.post("/sniff")
async def sniff_video(req: SniffRequest):
    service = get_cast_service()
    return await service.sniff_video(req.url)


@router.post("/play_url")
async def get_play_url(req: PlayUrlRequest):
    service = get_cast_service()
    return await service.play_url(req.url, req.title)


@router.post("/start")
async def start_cast(req: CastRequest):
    print(f"=== CAST API CALLED === url={req.episode_url}, device={req.device_udn}, title={req.title}")
    service = get_cast_service()
    result = await service.cast(req.episode_url, req.device_udn, req.title)
    print(f"=== CAST API RESULT === {result}")
    return result


@router.post("/control")
async def control_cast(req: ControlRequest):
    service = get_cast_service()
    return await service.control(req.device_udn, req.action, req.target, req.volume)


@router.get("/status/{device_udn}")
async def get_status(device_udn: str):
    service = get_cast_service()
    return await service.get_status(device_udn)
