"""收藏夹服务（对齐桌面音乐播放器那套：本地 JSON 持久化）

桌面逻辑（~sh/music/music.py）：
- 收藏条目 [{bvid,title,artist,duration,duration_sec,cover}]
- 同 bvid 去重，新的插到最前
- ensure_ascii=True 写盘（B站标题可能含非法字符，False 会写盘抛错）
"""
from pathlib import Path
import json
import sys

FAV_FILE = Path.home() / ".config" / "homecast" / "favorites.json"


def load_favs() -> list[dict]:
    """读取收藏（新的在前）"""
    try:
        if FAV_FILE.exists():
            with open(FAV_FILE, encoding="utf-8") as f:
                data = json.load(f)
            if isinstance(data, list):
                return data
    except Exception:
        pass
    return []


def save_fav(entry: dict) -> list[dict]:
    """收藏一条：同 bvid 去重，插到最前"""
    try:
        FAV_FILE.parent.mkdir(parents=True, exist_ok=True)
        favs = load_favs()
        favs = [f for f in favs if f.get("bvid") != entry.get("bvid")]
        favs.insert(0, entry)
        with open(FAV_FILE, "w", encoding="utf-8") as f:
            json.dump(favs, f, ensure_ascii=True)
        return favs
    except Exception as e:
        print(f"[收藏保存失败] {e}", file=sys.stderr)
        return load_favs()


def remove_fav(bvid: str) -> list[dict]:
    """取消收藏"""
    try:
        favs = load_favs()
        favs = [f for f in favs if f.get("bvid") != bvid]
        with open(FAV_FILE, "w", encoding="utf-8") as f:
            json.dump(favs, f, ensure_ascii=True)
        return favs
    except Exception as e:
        print(f"[收藏删除失败] {e}", file=sys.stderr)
        return load_favs()


def clear_favs() -> None:
    """清空全部收藏"""
    try:
        FAV_FILE.parent.mkdir(parents=True, exist_ok=True)
        with open(FAV_FILE, "w", encoding="utf-8") as f:
            json.dump([], f, ensure_ascii=True)
    except Exception as e:
        print(f"[收藏清空失败] {e}", file=sys.stderr)
