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
        try:
            from playwright.sync_api import sync_playwright
        except ImportError:
            logger.error("playwright not installed, run: pip install playwright && playwright install chromium")
            return None

        def _sniff_in_thread():
            m3u8_urls = []
            video_urls = []
            page_title = ""

            with sync_playwright() as p:
                browser = p.chromium.launch(headless=True)
                context = browser.new_context(
                    user_agent=(
                        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                        "AppleWebKit/537.36 (KHTML, like Gecko) "
                        "Chrome/120.0.0.0 Safari/537.36"
                    ),
                )

                page = context.new_page()

                def handle_request(request):
                    req_url = request.url
                    path = req_url.split("?")[0].split("#")[0]
                    if ".m3u8" in path:
                        if not any(x in path for x in ["hls", "ts/", "/1000k/", "/500k/", "/2000k/"]):
                            if not path.endswith(".ts"):
                                m3u8_urls.append(req_url)
                    elif any(path.endswith(ext) or (ext + "/") in path for ext in _VIDEO_EXTENSIONS):
                        video_urls.append(req_url)

                page.on("request", handle_request)

                try:
                    page.goto(url, wait_until="domcontentloaded", timeout=30000)
                except Exception as e:
                    logger.warning(f"Page load partial: {e}")

                page.wait_for_timeout(5000)

                try:
                    page_title = page.title()
                except Exception:
                    page_title = ""

                try:
                    content = page.content()
                    m3u8_pattern = r'(https?://[^\s"\'>]+\.m3u8(?:[?#][^\s"\'>]*)?)'
                    for match in re.findall(m3u8_pattern, content):
                        if not any(x in match for x in ["hls", "ts/", "/1000k/", "/500k/", "/2000k/"]):
                            if not match.endswith(".ts"):
                                if match not in m3u8_urls:
                                    m3u8_urls.append(match)

                    exts = "|".join(re.escape(e) for e in _VIDEO_EXTENSIONS)
                    video_pattern = r'(https?://[^\s"\'>]+(?:' + exts + r')(?:/[^\s"\'>]*)?(?:\?[^\s"\'>]*)?)'
                    for match in re.findall(video_pattern, content):
                        if match not in video_urls:
                            video_urls.append(match)
                except Exception as e:
                    logger.warning(f"Page content extraction failed: {e}")

                browser.close()

            m3u8_urls = _dedup(m3u8_urls)
            video_urls = _dedup(video_urls)
            video_urls = [u for u in video_urls if not u.endswith(".ts")]

            all_urls = m3u8_urls + video_urls
            if not all_urls:
                return None

            episodes = []
            for idx, vurl in enumerate(all_urls, 1):
                short = vurl.split("?")[0].split("/")[-1][:40]
                episodes.append(VideoEpisode(
                    index=idx,
                    title=f"视频流 {idx} ({short})",
                    url=vurl,
                ))

            return SniffResult(
                title=page_title or "网络嗅探结果",
                episodes=episodes,
                sniff_method="playwright",
            )

        loop = asyncio.get_event_loop()
        try:
            result = await loop.run_in_executor(_executor, _sniff_in_thread)
            return result
        except Exception as e:
            logger.error(f"Playwright sniff error: {e}")
            return None

    async def _parse_episode_list(self, url: str) -> list[VideoEpisode]:
        """解析视频详情页，提取集数列表 - 通用方法"""
        try:
            from playwright.sync_api import sync_playwright
        except ImportError:
            return []
        import time
        from urllib.parse import urlparse, urljoin

        def _parse_in_thread():
            episodes = []
            parsed_url = urlparse(url)
            base_url = f"{parsed_url.scheme}://{parsed_url.netloc}"

            t0 = time.time()
            logger.info(f"[Parse] Step 1: Launching browser...")

            with sync_playwright() as p:
                t1 = time.time()
                logger.info(f"[Parse] Step 1 done in {t1-t0:.1f}s")
                logger.info(f"[Parse] Step 2: Creating context and page...")
                browser = p.chromium.launch(headless=True)
                context = browser.new_context(
                    user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
                )
                page = context.new_page()
                t2 = time.time()
                logger.info(f"[Parse] Step 2 done in {t2-t1:.1f}s")

                try:
                    logger.info(f"[Parse] Step 3: Navigating to {url}...")
                    page.goto(url, wait_until="domcontentloaded", timeout=30000)
                    t3 = time.time()
                    logger.info(f"[Parse] Step 3 (page load) done in {t3-t2:.1f}s")

                    logger.info(f"[Parse] Step 4: Waiting for dynamic content...")
                    page.wait_for_timeout(2000)
                    t4 = time.time()
                    logger.info(f"[Parse] Step 4 (wait) done in {t4-t3:.1f}s")

                    logger.info(f"[Parse] Step 5: Batch extracting links with JavaScript...")
                    t5 = time.time()
                    
                    links_data = page.evaluate("""
                        () => {
                            const links = document.querySelectorAll('a');
                            return Array.from(links).map(a => ({
                                text: a.textContent.trim(),
                                href: a.getAttribute('href') || ''
                            }));
                        }
                    """)
                    t6 = time.time()
                    logger.info(f"[Parse] Step 5 done in {t6-t5:.1f}s, found {len(links_data)} links")

                    logger.info(f"[Parse] Step 6: Finding episode list (li > a)...")
                    
                    # 直接查找 li > a 结构中的集数
                    li_links = page.evaluate("""
                        () => {
                            const results = [];
                            const lis = document.querySelectorAll('li');
                            lis.forEach(li => {
                                const a = li.querySelector('a');
                                if (a) {
                                    const text = a.textContent.trim();
                                    const href = a.getAttribute('href') || '';
                                    if (text && href) {
                                        results.push({ text, href });
                                    }
                                }
                            });
                            return results;
                        }
                    """)
                    
                    # 直接从 li > a 取集数，只处理播放链接
                    for item in li_links:
                        text = item['text']
                        href = item['href']

                        # 只取播放页链接
                        if not ('/play/' in href or '/vodplay/' in href):
                            continue

                        # 只取包含"集"字的（过滤"立即播放"等按钮）
                        if '集' not in text:
                            continue

                        full_url = base_url + href if href.startswith('/') else href
                        episodes.append(VideoEpisode(
                            index=len(episodes) + 1,
                            title=text,
                            url=full_url,
                        ))

                    t7 = time.time()
                    logger.info(f"[Parse] Step 6 done in {t7-t6:.1f}s, matched {len(episodes)} episodes")

                    seen = set()
                    unique_episodes = []
                    episodes.sort(key=lambda x: (x.index, x.url))
                    for ep in episodes:
                        if ep.index not in seen:
                            seen.add(ep.index)
                            unique_episodes.append(ep)

                    unique_episodes.sort(key=lambda x: x.index)

                    t8 = time.time()
                    logger.info(f"[Parse] Step 7 (dedup/sort) done in {t8-t7:.1f}s, unique: {len(unique_episodes)}")

                except Exception as e:
                    logger.warning(f"Episode list parse error: {e}")

                browser.close()
                t9 = time.time()
                logger.info(f"[Parse] Total time: {t9-t0:.1f}s")

            return unique_episodes

        loop = asyncio.get_event_loop()
        try:
            result = await loop.run_in_executor(_executor, _parse_in_thread)
            return result or []
        except Exception as e:
            logger.warning(f"Episode list parse failed: {e}")
            return []

    async def _sniff_play_page(self, url: str) -> SniffResult:
        """嗅探 momovod 播放页的视频 URL"""
        try:
            from playwright.sync_api import sync_playwright
        except ImportError:
            logger.error("playwright not installed")
            return SniffResult(title="", episodes=[])

        def _sniff_in_thread():
            m3u8_urls = []
            video_urls = []
            page_title = ""

            with sync_playwright() as p:
                browser = p.chromium.launch(headless=True)
                context = browser.new_context(
                    user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
                )
                page = context.new_page()

                def handle_request(request):
                    req_url = request.url
                    path = req_url.split("?")[0].split("#")[0]
                    if ".m3u8" in path:
                        # 过滤掉 segment 文件，只保留主 m3u8
                        if not any(x in path for x in ["hls", "ts/", "/1000k/", "/500k/", "/2000k/"]):
                            if not path.endswith(".ts"):
                                m3u8_urls.append(req_url)
                    elif any(path.endswith(ext) or (ext + "/") in path for ext in _VIDEO_EXTENSIONS):
                        video_urls.append(req_url)

                page.on("request", handle_request)

                try:
                    page.goto(url, wait_until="domcontentloaded", timeout=30000)
                    page.wait_for_timeout(5000)  # 等待视频加载
                    page_title = page.title()
                except Exception as e:
                    logger.warning(f"Page load partial: {e}")

                # 尝试从页面内容中提取 m3u8
                try:
                    content = page.content()
                    m3u8_pattern = r'(https?://[^\s"\'>]+\.m3u8(?:[?#][^\s"\'>]*)?)'
                    for match in re.findall(m3u8_pattern, content):
                        if not any(x in match for x in ["hls", "ts/", "/1000k/", "/500k/", "/2000k/"]):
                            if not match.endswith(".ts"):
                                if match not in m3u8_urls:
                                    m3u8_urls.append(match)
                except Exception as e:
                    logger.warning(f"Content extraction failed: {e}")

                browser.close()

            m3u8_urls = _dedup(m3u8_urls)
            video_urls = _dedup(video_urls)
            video_urls = [u for u in video_urls if not u.endswith(".ts")]

            all_urls = m3u8_urls + video_urls
            if not all_urls:
                return SniffResult(title=page_title or "播放页", episodes=[])

            episodes = []
            for idx, vurl in enumerate(all_urls, 1):
                short = vurl.split("?")[0].split("/")[-1][:40]
                episodes.append(VideoEpisode(
                    index=idx,
                    title=f"视频流 {idx}",
                    url=vurl,
                ))

            return SniffResult(
                title=page_title or "播放页",
                episodes=episodes,
                sniff_method="playwright-play",
            )

        loop = asyncio.get_event_loop()
        try:
            result = await loop.run_in_executor(_executor, _sniff_in_thread)
            return result or SniffResult(title="", episodes=[])
        except Exception as e:
            logger.error(f"Play page sniff error: {e}")
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
            from app.bilibili.audio import get_best_audio_url, AUDIO_320K
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
