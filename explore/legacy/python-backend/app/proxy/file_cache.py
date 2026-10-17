import asyncio
import hashlib
import os
import time
from pathlib import Path
from loguru import logger

from app.proxy.transcoder import transcode_bilibili_audio_to_file
from app.config import get_config


class AudioFileCache:
    def __init__(self, cache_dir: str | None = None, max_size_mb: int = 500):
        if cache_dir is None:
            try:
                cfg = get_config()
                cache_dir = cfg.cache.dir
                max_size_mb = cfg.cache.max_size_mb
            except Exception:
                pass
        self.cache_dir = Path(cache_dir or "data/audio_cache")
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self.max_size_bytes = max_size_mb * 1024 * 1024
        self._lock = asyncio.Lock()

    @staticmethod
    def _url_hash(url: str) -> str:
        return hashlib.md5(url.encode()).hexdigest()

    def _cache_path(self, url_hash: str) -> Path:
        return self.cache_dir / f"{url_hash}.mp3"

    def _meta_path(self, url_hash: str) -> Path:
        return self.cache_dir / f"{url_hash}.meta"

    async def get(self, url: str) -> Path | None:
        h = self._url_hash(url)
        mp3_path = self._cache_path(h)
        if mp3_path.exists() and mp3_path.stat().st_size > 0:
            self._touch_meta(h)
            logger.debug(f"Cache hit: {h[:8]}... ({mp3_path.stat().st_size} bytes)")
            return mp3_path
        return None

    async def get_or_transcode(
        self,
        url: str,
        source_headers: dict | None = None,
        bitrate: str = "64k",
    ) -> Path | None:
        h = self._url_hash(url)
        mp3_path = self._cache_path(h)

        cached = await self.get(url)
        if cached:
            return cached

        async with self._lock:
            double_check = await self.get(url)
            if double_check:
                return double_check

            logger.info(f"Cache miss, transcoding: {url[:80]}... -> mp3 {bitrate}")
            try:
                success = await transcode_bilibili_audio_to_file(
                    url,
                    output_path=str(mp3_path),
                    bitrate=bitrate,
                )

                if not success:
                    mp3_path.unlink(missing_ok=True)
                    logger.error(f"FFmpeg transcode failed for {url[:80]}")
                    return None

                file_size = mp3_path.stat().st_size
                self._save_meta(h, url, file_size, bitrate)
                logger.info(f"Transcoded & cached: {h[:8]}... ({file_size} bytes, {bitrate})")

                await self._evict_if_needed()

                return mp3_path

            except Exception as e:
                logger.error(f"Transcode error for {url[:80]}: {e}")
                mp3_path.unlink(missing_ok=True)
                return None

    def _save_meta(self, h: str, url: str, size: int, bitrate: str):
        meta_path = self._meta_path(h)
        with open(meta_path, "w") as f:
            f.write(f"{time.time()}\n{size}\n{bitrate}\n{url}\n")

    def _touch_meta(self, h: str):
        meta_path = self._meta_path(h)
        if meta_path.exists():
            mtime = time.time()
            lines = meta_path.read_text(encoding="utf-8").split("\n")
            if len(lines) >= 1:
                lines[0] = str(mtime)
                meta_path.write_text("\n".join(lines), encoding="utf-8")

    async def _evict_if_needed(self):
        total = sum(
            f.stat().st_size
            for f in self.cache_dir.glob("*.mp3")
            if f.exists()
        )
        if total <= self.max_size_bytes:
            return

        entries = []
        for meta_file in self.cache_dir.glob("*.meta"):
            try:
                lines = meta_file.read_text(encoding="utf-8").split("\n")
                if len(lines) >= 2:
                    atime = float(lines[0])
                    size = int(lines[1])
                    h = meta_file.stem
                    entries.append((atime, size, h))
            except (ValueError, IndexError):
                continue

        entries.sort(key=lambda x: x[0])
        freed = 0
        target = total - int(self.max_size_bytes * 0.7)
        for atime, size, h in entries:
            if freed >= target:
                break
            mp3_p = self._cache_path(h)
            meta_p = self._meta_path(h)
            mp3_p.unlink(missing_ok=True)
            meta_p.unlink(missing_ok=True)
            freed += size
            logger.debug(f"Evicted cache: {h[:8]}... ({size} bytes)")

    def clear(self):
        for f in list(self.cache_dir.glob("*.mp3")) + list(self.cache_dir.glob("*.meta")):
            f.unlink(missing_ok=True)
        logger.info("Audio cache cleared")

    def info(self) -> dict:
        files = list(self.cache_dir.glob("*.mp3"))
        total_size = sum(f.stat().st_size for f in files if f.exists())
        return {
            "count": len(files),
            "total_bytes": total_size,
            "total_mb": round(total_size / 1024 / 1024, 2),
            "max_mb": round(self.max_size_bytes / 1024 / 1024, 2),
            "dir": str(self.cache_dir),
        }


audio_cache = AudioFileCache()
