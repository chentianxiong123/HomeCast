from fastapi import APIRouter, Query, Path
from app.service.favlist_service import FavListService

router = APIRouter(prefix="/favlist", tags=["favlist"])

_favlist_service: FavListService | None = None


def set_favlist_service(service: FavListService):
    global _favlist_service
    _favlist_service = service


def get_favlist_service() -> FavListService:
    if _favlist_service is None:
        raise RuntimeError("FavListService not initialized")
    return _favlist_service


@router.get("/info/{media_id}")
async def get_fav_list_info(media_id: int = Path(..., description="收藏夹ID")):
    service = get_favlist_service()
    result = await service.get_fav_list_info(media_id)
    return {"code": 0, "message": "success", "data": result.model_dump()}


@router.get("/{media_id}")
async def get_fav_list(
    media_id: int = Path(..., description="收藏夹ID"),
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=50),
):
    service = get_favlist_service()
    result = await service.get_fav_list(media_id, page, page_size)
    return {"code": 0, "message": "success", "data": result.model_dump()}
