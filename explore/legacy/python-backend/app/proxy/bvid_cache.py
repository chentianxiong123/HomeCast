"""
基于BVID的音频缓存系统

解决B站URL每次都不同的问题，使用BVID作为缓存标识。
转码逻辑在 audio_proxy.py 中通过 stream_transcoder 实时完成
"""

import asyncio
import time
from pathlib import Path
from loguru import logger
from app.config import get_config


class BvidAudioCache:
    """基于BVID的音频缓存"""

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
        # 记录正在转码中的BVID，避免重复转码
        self._transcoding: set[str] = set()

    def _cache_path(self, bvid: str) -> Path:
        return self.cache_dir / f"{bvid}.mp3"

    def _meta_path(self, bvid: str) -> Path:
        return self.cache_dir / f"{bvid}.meta"

    def exists(self, bvid: str) -> bool:
        """检查BVID是否有缓存"""
        mp3_path = self._cache_path(bvid)
        return mp3_path.exists() and mp3_path.stat().st_size > 0

    def get_path(self, bvid: str) -> Path | None:
        """获取缓存文件路径（如果存在）"""
        mp3_path = self._cache_path(bvid)
        if mp3_path.exists() and mp3_path.stat().st_size > 0:
            self._touch_meta(bvid)
            return mp3_path
        return None

    async def get_or_create(
        self,
        bvid: str,
        input_url: str = "",
        bitrate: str = "64k",
    ) -> Path | None:
        """
        获取缓存，如果不存在则创建（使用 stream_transcoder 实时转码）

        Args:
            bvid: B站视频ID
            input_url: B站音频URL（用于转码）
            bitrate: 目标码率

        返回缓存路径（如果转码成功）或None
        """
        from app.proxy.stream_transcoder import stream_transcoder

        # 先检查是否已有缓存
        existing = self.get_path(bvid)
        if existing:
            return existing

        # 检查是否正在转码中
        if bvid in self._transcoding:
            logger.debug(f"Transcoding in progress for {bvid}, waiting...")
            # 等待转码完成（最多等60秒）
            for _ in range(60):
                await asyncio.sleep(1)
                existing = self.get_path(bvid)
                if existing:
                    return existing
            logger.warning(f"Timeout waiting for transcoding: {bvid}")
            return None

        if not input_url:
            logger.error(f"No input URL provided for {bvid}")
            return None

        # 开始转码
        async with self._lock:
            # 双重检查
            existing = self.get_path(bvid)
            if existing:
                return existing

            if bvid in self._transcoding:
                return None  # 另一个任务正在处理

            self._transcoding.add(bvid)

        try:
            logger.info(f"Starting transcode for {bvid}...")
            cache_path = self._cache_path(bvid)

            # 使用 stream_transcoder 下载并转码
            async for chunk in stream_transcoder.transcode_audio_stream(
                input_url=input_url,
                output_format="mp3",
                bitrate=bitrate,
                cache_path=cache_path,
            ):
                pass  # 只需要等待转码完成，数据已写入缓存文件

            if cache_path.exists() and cache_path.stat().st_size > 0:
                file_size = cache_path.stat().st_size
                self._save_meta(bvid, file_size, bitrate)
                logger.info(f"Transcode complete for {bvid}: {file_size} bytes")
                await self._evict_if_needed()
                return cache_path
            else:
                logger.error(f"Transcode failed for {bvid}")
                return None

        except Exception as e:
            logger.error(f"Transcode error for {bvid}: {e}")
            return None

        finally:
            self._transcoding.discard(bvid)

    def create_async(
        self,
        bvid: str,
        input_url: str = "",
        bitrate: str = "64k",
    ):
        """
        异步创建缓存（不等待结果）

        Args:
            bvid: B站视频ID
            input_url: B站音频URL（用于转码）
            bitrate: 目标码率
        """
        if self.exists(bvid) or bvid in self._transcoding:
            return
        if not input_url:
            logger.error(f"create_async: No input URL for {bvid}")
            return

        asyncio.create_task(self._do_transcode(bvid, input_url, bitrate))

    async def _do_transcode(self, bvid: str, input_url: str, bitrate: str):
        """执行转码（内部方法，使用 stream_transcoder）"""
        from app.proxy.stream_transcoder import stream_transcoder

        async with self._lock:
            if self.exists(bvid) or bvid in self._transcoding:
                return
            self._transcoding.add(bvid)

        try:
            logger.info(f"Background transcode for {bvid}...")
            cache_path = self._cache_path(bvid)

            # 使用 stream_transcoder 下载并转码
            async for chunk in stream_transcoder.transcode_audio_stream(
                input_url=input_url,
                output_format="mp3",
                bitrate=bitrate,
                cache_path=cache_path,
            ):
                pass  # 数据已写入缓存文件

            if cache_path.exists() and cache_path.stat().st_size > 0:
                file_size = cache_path.stat().st_size
                self._save_meta(bvid, file_size, bitrate)
                logger.info(f"Background transcode complete for {bvid}: {file_size} bytes")
                await self._evict_if_needed()
            else:
                logger.error(f"Background transcode failed for {bvid}")

        except Exception as e:
            logger.error(f"Background transcode error for {bvid}: {e}")

        finally:
            self._transcoding.discard(bvid)

    def _save_meta(self, bvid: str, size: int, bitrate: str):
        meta_path = self._meta_path(bvid)
        with open(meta_path, "w") as f:
            f.write(f"{time.time()}\n{size}\n{bitrate}\n{bvid}\n")

    def _touch_meta(self, bvid: str):
        meta_path = self._meta_path(bvid)
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
                    bvid = meta_file.stem
                    entries.append((atime, size, bvid))
            except (ValueError, IndexError):
                continue

        entries.sort(key=lambda x: x[0])
        freed = 0
        target = total - int(self.max_size_bytes * 0.7)
        for atime, size, bvid in entries:
            if freed >= target:
                break
            mp3_p = self._cache_path(bvid)
            meta_p = self._meta_path(bvid)
            mp3_p.unlink(missing_ok=True)
            meta_p.unlink(missing_ok=True)
            freed += size
            logger.debug(f"Evicted cache: {bvid} ({size} bytes)")

    async def clear(self):
        """清空缓存，等待转码任务完成"""
        # 等待所有转码任务完成
        if self._transcoding:
            logger.info(f"Waiting for {len(self._transcoding)} transcoding tasks to complete...")
            for _ in range(30):  # 最多等30秒
                if not self._transcoding:
                    break
                await asyncio.sleep(1)
            self._transcoding.clear()

        # 删除缓存文件
        deleted = 0
        failed = 0
        for f in list(self.cache_dir.glob("*.mp3")) + list(self.cache_dir.glob("*.meta")):
            try:
                f.unlink()
                deleted += 1
            except Exception as e:
                failed += 1
                logger.warning(f"Failed to delete {f}: {e}")
        logger.info(f"BVID audio cache cleared: {deleted} deleted, {failed} failed")

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


# 全局实例
bvid_cache = BvidAudioCache()
