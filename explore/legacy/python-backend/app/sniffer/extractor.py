import asyncio
import json
import re
from concurrent.futures import ThreadPoolExecutor
from loguru import logger
from pydantic import BaseModel


_executor = ThreadPoolExecutor(max_workers=2)


class VideoEpisode(BaseModel):
    index: int
    title: str
    url: str
    duration: int | None = None
    thumbnail: str | None = None


class SniffResult(BaseModel):
    title: str = ""
    episodes: list[VideoEpisode] = []
    episodes_list: list[VideoEpisode] = []  # 集数列表
    sniff_method: str = ""
    raw_data: dict = {}


_VIDEO_EXTENSIONS = (
    ".mp4", ".webm", ".mkv", ".avi", ".flv",
    ".ts", ".mov", ".wmv", ".mpd",
)

_BILIBILI_PATTERN = re.compile(
    r"(bilibili\.com|b23\.tv)", re.IGNORECASE
)
_BVID_PATTERN = re.compile(r"BV[a-zA-Z0-9]+")


class VideoExtractor:
    def __init__(self, ffmpeg_path: str = "ffmpeg"):
        self.ffmpeg_path = ffmpeg_path

    async def sniff(self, url: str) -> SniffResult:
        logger.info(f"Sniffing: {url}")

        if _BILIBILI_PATTERN.search(url):
            logger.info("Bilibili URL detected, using Bilibili API")
            return await self._try_bilibili_api(url)

        if self._is_detail_page(url):
            logger.info("Detected detail page, parsing episode list only...")
            episodes_list = await self._parse_episode_list(url)
            if episodes_list:
                return SniffResult(
                    title="剧集列表",
                    episodes=[],
                    episodes_list=episodes_list,
                    sniff_method="episode-list",
                )
            return SniffResult(title="", episodes=[])

        if self._is_play_page(url):
            logger.info("Detected play page, sniffing video...")
            return await self._sniff_play_page(url)

        try:
            result = await self._try_ytdlp(url)
            if result and result.episodes:
                return result
        except Exception as e:
            logger.warning(f"yt-dlp failed: {e}")

        logger.info("Falling back to Playwright network sniffing...")
        try:
            result = await self._try_playwright(url)
            if result:
                return result
        except Exception as e:
            logger.warning(f"Playwright sniff failed: {e}")

        return SniffResult(title="", episodes=[])

    def _is_detail_page(self, url: str) -> bool:
        detail_patterns = [
            r"/vod/",
            r"/voddetail/",
            r"/detail/",
            r"/video/\d+",
            r"/movie/",
            r"/tv/",
        ]
        return any(re.search(p, url, re.IGNORECASE) for p in detail_patterns)

    def _is_play_page(self, url: str) -> bool:
        play_patterns = [
            r"/play/",
            r"/vodplay/",
            r"/episode/",
            r"/ep/",
            r"play.*\.html",
        ]
        return any(re.search(p, url, re.IGNORECASE) for p in play_patterns)

    async def _try_bilibili_api(self, url: str) -> SniffResult:
        try:
            from app.bilibili.client import BilibiliClient
            from app.bilibili.video import get_video_info, get_video_pages
            from app.config import get_config

            config = get_config()
            client = BilibiliClient(config.bilibili)

            bvid_match = _BVID_PATTERN.search(url)
            if not bvid_match:
                logger.warning("Cannot extract BVID from URL")
                return SniffResult(title="", episodes=[])

            bvid = bvid_match.group()
            info = await get_video_info(client, bvid)
            pages = await get_video_pages(client, bvid)

            title = info.title

            if not pages:
                return SniffResult(
                    title=title,
                    episodes=[VideoEpisode(
                        index=1,
                        title=title,
                        url=f"https://www.bilibili.com/video/{bvid}",
                        duration=info.duration,
                        thumbnail=info.pic,
                    )],
                    sniff_method="bilibili-api",
                )

            episodes = []
            for idx, page in enumerate(pages, 1):
                part_title = page.part or f"P{idx}"
                if len(pages) > 1:
                    episode_title = f"P{idx} {part_title}"
                else:
                    episode_title = title
                episodes.append(VideoEpisode(
                    index=idx,
                    title=episode_title,
                    url=f"https://www.bilibili.com/video/{bvid}?p={idx}",
                    duration=page.duration,
                    thumbnail=info.pic,
                ))

            return SniffResult(
                title=title,
                episodes=episodes,
                sniff_method="bilibili-api",
            )
        except Exception as e:
            logger.error(f"Bilibili API failed: {e}")
            return SniffResult(title="", episodes=[])

    async def _try_ytdlp(self, url: str) -> SniffResult | None:
        cmd = [
            "yt-dlp",
            "--dump-json",
            "--no-download",
            "--no-check-certificates",
            "--no-warnings",
            "--playlist-items", "1:100",
            url,
        ]

        proc = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=120)

        if proc.returncode != 0:
            err = stderr.decode()[:300] if stderr else "unknown"
            logger.error(f"yt-dlp error: {err}")
            return None

        results = []
        for line in stdout.decode().strip().split("\n"):
            line = line.strip()
            if not line:
                continue
            try:
                results.append(json.loads(line))
            except json.JSONDecodeError:
                continue

        if not results:
            return None

        if len(results) == 1:
            info = results[0]
            if info.get("_type") == "playlist":
                return self._parse_playlist(info)
            return SniffResult(
                title=info.get("title", ""),
                episodes=[VideoEpisode(
                    index=1,
                    title=info.get("title", "视频"),
                    url=info.get("webpage_url") or info.get("url", ""),
                    duration=info.get("duration"),
                    thumbnail=info.get("thumbnail"),
                )],
                sniff_method="yt-dlp",
            )

        playlist_title = results[0].get("playlist_title", results[0].get("title", "播放列表"))
        episodes = []
        for idx, entry in enumerate(results, 1):
            if not entry:
                continue
            episodes.append(VideoEpisode(
                index=idx,
                title=entry.get("title", f"第{idx}集"),
                url=entry.get("webpage_url") or entry.get("url", ""),
                duration=entry.get("duration"),
                thumbnail=entry.get("thumbnail"),
            ))
        return SniffResult(
            title=playlist_title,
            episodes=episodes,
            sniff_method="yt-dlp",
        )

    def _parse_playlist(self, info: dict) -> SniffResult:
        entries = info.get("entries", [])
        episodes = []
        for idx, entry in enumerate(entries, 1):
            if not entry:
                continue
            episodes.append(VideoEpisode(
                index=idx,
                title=entry.get("title", f"第{idx}集"),
                url=entry.get("webpage_url") or entry.get("url", ""),
                duration=entry.get("duration"),
                thumbnail=entry.get("thumbnail"),
            ))
        return SniffResult(
            title=info.get("title", "播放列表"),
            episodes=episodes,
            sniff_method="yt-dlp",
        )

    async def _try_playwright(self, url: str) -> SniffResult | None:
        """第三方网站通用嗅探：已移除无头浏览器(Playwright)。

        无头浏览器 300MB+ 无法打包进手机 APK/轻量部署；
        B站链接走 _try_bilibili_api（纯 API，不受影响）。
        """
        logger.info(f"第三方网站嗅探已禁用(无头浏览器已移除): {url}")
        return None
    async def _parse_episode_list(self, url: str) -> list[VideoEpisode]:
        """第三方详情页集数解析：已移除无头浏览器(Playwright)。

        B站剧集走 _try_bilibili_api（API 直接给 pages），不受影响。
        """
        logger.info(f"第三方详情页集数解析已禁用(无头浏览器已移除): {url}")
        return []
    async def _sniff_play_page(self, url: str) -> SniffResult:
        """第三方播放页嗅探：已移除无头浏览器(Playwright)。"""
        logger.info(f"第三方播放页嗅探已禁用(无头浏览器已移除): {url}")
        return SniffResult(title="", episodes=[])
    async def get_direct_url(self, url: str, quality: str = "best") -> str | None:
        if _BILIBILI_PATTERN.search(url):
            return await self._get_bilibili_direct_url(url)

        cmd = [
            "yt-dlp",
            "-f", quality,
            "-g",
            "--no-check-certificates",
            "--no-warnings",
            url,
        ]

        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=60)
            if proc.returncode != 0:
                logger.error(f"yt-dlp get url error: {stderr.decode()[:200]}")
                return None
            lines = stdout.decode().strip().split("\n")
            return lines[0] if lines else None
        except Exception as e:
            logger.error(f"Get direct url failed: {e}")
            return None

    async def _get_bilibili_direct_url(self, url: str) -> str | None:
        try:
            from app.bilibili.client import BilibiliClient
            from app.bilibili.audio import get_best_audio_url, AUDIO_192K
            from app.bilibili.video import get_video_info, get_video_pages
            from app.config import get_config

            config = get_config()
            client = BilibiliClient(config.bilibili)

            bvid_match = _BVID_PATTERN.search(url)
            if not bvid_match:
                return None

            bvid = bvid_match.group()

            p_match = re.search(r"[?&]p=(\d+)", url)
            page_num = int(p_match.group(1)) if p_match else 1

            pages = await get_video_pages(client, bvid)
            if not pages:
                return None

            page_idx = min(page_num - 1, len(pages) - 1)
            cid = pages[page_idx].cid

            audio_result = await get_best_audio_url(client, bvid, cid, AUDIO_320K)
            if audio_result and audio_result.url:
                return audio_result.url
        except Exception as e:
            logger.error(f"Bilibili direct URL failed: {e}")
        return None


def _dedup(urls: list[str]) -> list[str]:
    seen = set()
    result = []
    for u in urls:
        if u not in seen:
            seen.add(u)
            result.append(u)
    return result


_extractor: VideoExtractor | None = None


def get_extractor() -> VideoExtractor:
    global _extractor
    if _extractor is None:
        _extractor = VideoExtractor()
    return _extractor
