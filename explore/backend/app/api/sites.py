from fastapi import APIRouter
from pydantic import BaseModel
from loguru import logger
import json
import os
import hashlib

router = APIRouter(prefix="/sites", tags=["sites"])

SITES_FILE = "data/sites.json"
EPISODES_CACHE_FILE = "data/episodes_cache.json"

class Site(BaseModel):
    name: str
    url: str
    site_type: str = "video"

class EpisodesCache(BaseModel):
    detail_url: str
    detail_url_hash: str
    title: str
    episodes_list: list
    cached_at: float

def get_url_hash(url: str) -> str:
    return hashlib.md5(url.encode()).hexdigest()

def load_sites():
    if os.path.exists(SITES_FILE):
        try:
            with open(SITES_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            pass
    return {"sites": [], "preset": [
        {"name": "MoMoVOD-真情", "url": "https://momovod.app/vod/466165.html", "site_type": "detail"},
    ]}

def save_sites(data: dict):
    os.makedirs(os.path.dirname(SITES_FILE), exist_ok=True)
    with open(SITES_FILE, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

def load_episodes_cache() -> dict:
    if os.path.exists(EPISODES_CACHE_FILE):
        try:
            with open(EPISODES_CACHE_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            pass
    return {}

def save_episodes_cache(cache: dict):
    os.makedirs(os.path.dirname(EPISODES_CACHE_FILE), exist_ok=True)
    with open(EPISODES_CACHE_FILE, "w", encoding="utf-8") as f:
        json.dump(cache, f, ensure_ascii=False, indent=2)

def cache_episodes(detail_url: str, title: str, episodes_list: list):
    cache = load_episodes_cache()
    url_hash = get_url_hash(detail_url)
    import time
    cache[url_hash] = {
        "detail_url": detail_url,
        "detail_url_hash": url_hash,
        "title": title,
        "episodes_list": episodes_list,
        "cached_at": time.time(),
    }
    save_episodes_cache(cache)
    logger.info(f"Cached {len(episodes_list)} episodes for {detail_url}")

def get_cached_episodes(detail_url: str) -> dict | None:
    cache = load_episodes_cache()
    url_hash = get_url_hash(detail_url)
    return cache.get(url_hash)

@router.get("/list")
async def list_sites():
    data = load_sites()
    return {"code": 0, "data": data}

@router.post("/add")
async def add_site(site: Site):
    data = load_sites()
    for s in data["sites"]:
        if s["url"] == site.url:
            return {"code": 400, "message": "网站已存在"}
    data["sites"].append(site.model_dump())
    save_sites(data)
    return {"code": 0, "message": "添加成功"}

@router.post("/remove")
async def remove_site(url: str):
    data = load_sites()
    data["sites"] = [s for s in data["sites"] if s["url"] != url]
    save_sites(data)
    return {"code": 0, "message": "删除成功"}

@router.get("/episodes/{detail_url:path}")
async def get_cached_episodes_api(detail_url: str):
    cached = get_cached_episodes(detail_url)
    if cached:
        return {"code": 0, "data": cached, "cached": True}
    return {"code": 404, "message": "缓存未找到"}

@router.post("/cache_episodes")
async def cache_episodes_api(detail_url: str, title: str, episodes_list: list):
    cache_episodes(detail_url, title, episodes_list)
    return {"code": 0, "message": "缓存成功"}
