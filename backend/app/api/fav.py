"""收藏夹 API（本地收藏，对齐桌面播放器：去重置顶 + ☆/★ toggle）"""
from fastapi import APIRouter, Path
from pydantic import BaseModel
from app.service.fav_service import load_favs, save_fav, remove_fav, clear_favs

router = APIRouter(prefix="/fav", tags=["fav"])


class FavEntry(BaseModel):
    bvid: str
    title: str = ""
    artist: str = ""
    cover: str = ""
    duration: str = ""
    duration_sec: int = 0
    play_count: int = 0


@router.get("/list")
async def get_favs():
    """收藏列表（新的在前）"""
    return {"code": 0, "message": "success", "data": load_favs()}


@router.post("/add")
async def add_fav(entry: FavEntry):
    """收藏一首（同 bvid 去重，插到最前）"""
    data = save_fav(entry.model_dump())
    return {"code": 0, "message": "success", "data": data}


@router.delete("/{bvid}")
async def del_fav(bvid: str = Path(..., description="BV号")):
    """取消收藏"""
    data = remove_fav(bvid)
    return {"code": 0, "message": "success", "data": data}


@router.post("/clear")
async def clear_all():
    """清空全部收藏"""
    clear_favs()
    return {"code": 0, "message": "success", "data": []}
