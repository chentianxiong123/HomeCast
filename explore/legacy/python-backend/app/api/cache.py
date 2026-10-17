"""缓存管理 API"""

from fastapi import APIRouter
from pathlib import Path
import time
from app.proxy.bvid_cache import bvid_cache
from app.config import get_config

router = APIRouter(prefix="/cache", tags=["cache"])


@router.get("/list")
async def list_cache():
    """获取缓存列表"""
    cache_dir = bvid_cache.cache_dir
    items = []
    
    for meta_file in cache_dir.glob("*.meta"):
        try:
            lines = meta_file.read_text(encoding="utf-8").strip().split("\n")
            if len(lines) >= 4:
                bvid = lines[3]
                mp3_file = cache_dir / f"{bvid}.mp3"
                
                if mp3_file.exists():
                    items.append({
                        "bvid": bvid,
                        "size": int(lines[1]),
                        "size_mb": round(int(lines[1]) / 1024 / 1024, 2),
                        "bitrate": lines[2],
                        "cached_at": float(lines[0]),
                        "cached_at_str": time.strftime(
                            "%Y-%m-%d %H:%M:%S", 
                            time.localtime(float(lines[0]))
                        ),
                    })
        except Exception:
            continue
    
    # 按缓存时间排序（最新的在前）
    items.sort(key=lambda x: x["cached_at"], reverse=True)
    
    info = bvid_cache.info()
    
    return {
        "code": 0,
        "message": "success",
        "data": {
            "items": items,
            "summary": info,
        }
    }


@router.delete("/{bvid}")
async def delete_cache(bvid: str):
    """删除指定缓存"""
    try:
        mp3_path = bvid_cache._cache_path(bvid)
        meta_path = bvid_cache._meta_path(bvid)
        
        deleted = False
        if mp3_path.exists():
            mp3_path.unlink()
            deleted = True
        if meta_path.exists():
            meta_path.unlink()
            deleted = True
        
        if deleted:
            return {"code": 0, "message": f"已删除缓存: {bvid}"}
        else:
            return {"code": 404, "message": f"缓存不存在: {bvid}"}
    except Exception as e:
        return {"code": 500, "message": f"删除失败: {str(e)}"}


@router.delete("/clear/all")
async def clear_all_cache():
    """清空所有缓存"""
    try:
        await bvid_cache.clear()
        return {"code": 0, "message": "已清空所有缓存"}
    except Exception as e:
        return {"code": 500, "message": f"清空失败: {str(e)}"}


@router.post("/create/{bvid}")
async def create_cache(bvid: str):
    """手动创建缓存（后台转码）"""
    try:
        if bvid_cache.exists(bvid):
            return {"code": 200, "message": "缓存已存在", "data": {"bvid": bvid}}
        
        # 后台异步转码
        await bvid_cache.create_async(bvid)
        
        return {
            "code": 0, 
            "message": "已开始后台转码",
            "data": {"bvid": bvid}
        }
    except Exception as e:
        return {"code": 500, "message": f"创建缓存失败: {str(e)}"}
