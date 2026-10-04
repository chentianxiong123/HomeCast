"""歌词服务（对齐桌面音乐播放器方案）

桌面版验证过的免费方案：
- 网易云 API 搜索歌名 → 歌曲候选
- 网易云 /api/song/lyric 取 LRC 文本（无需登录/无需 cookie）
- LRC 解析 → [(秒, 行)]，过滤作词/作曲等元信息行
"""
import re
from loguru import logger
import httpx

# 过滤的元信息行开头（桌面版同款）
_SKIP_PREFIXES = (
    "作词", "作曲", "编曲", "制作", "录音", "混音", "监制",
    "OP:", "SP:", "和声", "配唱",
)

_SEARCH_URL = "https://music.163.com/api/search/get/web"
_LYRIC_URL = "https://music.163.com/api/song/lyric"

_HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/120.0.0.0 Safari/537.36"
    ),
    "Referer": "https://music.163.com/",
}


def parse_lrc(text: str) -> list[tuple[float, str]]:
    """LRC 文本 → [(秒, 行)]，按时间升序；无词返回空列表"""
    out = []
    for ln in text.splitlines():
        m = re.match(r"\[(\d+):(\d+(?:\.\d+)?)\]", ln.strip())
        if m:
            t = int(m.group(1)) * 60 + float(m.group(2))
            txt = ln.strip()[m.end():].strip()
            if txt and not txt.startswith(_SKIP_PREFIXES):
                out.append((t, txt))
    out.sort()
    return out


async def fetch_lyric(sid: int) -> str:
    """网易云歌词 LRC 文本（sid=歌曲id）；无词返回空串"""
    try:
        async with httpx.AsyncClient(timeout=15, headers=_HEADERS) as c:
            r = await c.get(_LYRIC_URL, params={"id": sid, "lv": 1, "kv": 1, "tv": -1})
            j = r.json()
            if j.get("code") == 200:
                return j.get("lrc", {}).get("lyric", "") or ""
    except Exception as e:
        logger.warning(f"fetch_lyric failed sid={sid}: {e}")
    return ""


async def search_candidates(keyword: str, limit: int = 8) -> list[dict]:
    """搜歌名 → 候选列表（只留有歌词的），[{id,name,artist,dur,lines}]"""
    try:
        async with httpx.AsyncClient(timeout=15, headers=_HEADERS) as c:
            r = await c.get(_SEARCH_URL, params={"s": keyword, "type": 1, "limit": limit, "offset": 0})
            songs = r.json().get("result", {}).get("songs", []) or []
    except Exception as e:
        logger.warning(f"lyric search failed: {e}")
        return []

    out = []
    for song in songs:
        sid = song.get("id")
        if not sid:
            continue
        text = await fetch_lyric(sid)
        lines = parse_lrc(text)
        if len(lines) >= 3:
            artists = song.get("artists") or []
            out.append({
                "id": sid,
                "name": song.get("name", ""),
                "artist": artists[0].get("name", "") if artists else "",
                "dur": (song.get("duration") or 0) // 1000,
                "lines": lines,
            })
    return out


async def get_lyric_lines(keyword: str, sid: int | None = None) -> dict | None:
    """按歌名自动取第一个有词的歌词；或按 sid 精确取。

    返回 {id, name, artist, lines:[[t,line],...]} 或 None
    """
    if sid is not None:
        text = await fetch_lyric(sid)
        lines = parse_lrc(text)
        if lines:
            return {"id": sid, "name": "", "artist": "", "lines": lines}
        return None
    cands = await search_candidates(keyword)
    if cands:
        first = cands[0]
        return {
            "id": first["id"],
            "name": first["name"],
            "artist": first["artist"],
            "lines": first["lines"],
        }
    return None
