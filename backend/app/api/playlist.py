from fastapi import APIRouter, Path
from app.service.playlist_service import PlaylistService

router = APIRouter(prefix="/playlist", tags=["playlist"])

_playlist_service: PlaylistService | None = None


def set_playlist_service(service: PlaylistService):
    global _playlist_service
    _playlist_service = service


def get_playlist_service() -> PlaylistService:
    if _playlist_service is None:
        raise RuntimeError("PlaylistService not initialized")
    return _playlist_service


@router.get("")
async def get_playlist():
    service = get_playlist_service()
    result = await service.get_playlist()
    return {"code": 0, "message": "success", "data": result.model_dump()}


@router.post("/add/{bvid}")
async def add_to_playlist(bvid: str = Path(..., description="BV号")):
    service = get_playlist_service()
    item = await service.add_to_playlist(bvid)
    if item:
        return {"code": 0, "message": "success", "data": item.model_dump()}
    return {"code": 1, "message": "failed to add song", "data": None}


@router.post("/remove/{bvid}")
async def remove_from_playlist(bvid: str = Path(..., description="BV号")):
    service = get_playlist_service()
    success = await service.remove_from_playlist(bvid)
    return {"code": 0 if success else 1, "message": "success" if success else "not found", "data": success}


@router.post("/clear")
async def clear_playlist():
    service = get_playlist_service()
    await service.clear_playlist()
    return {"code": 0, "message": "success", "data": True}
