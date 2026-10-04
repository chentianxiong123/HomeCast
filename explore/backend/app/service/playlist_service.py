import json
import os
from pathlib import Path
from pydantic import BaseModel
from app.bilibili.client import BilibiliClient
from app.bilibili import video as bilibili_video
from loguru import logger


class PlaylistItem(BaseModel):
    bvid: str
    title: str
    artist: str
    cover: str
    duration: str
    duration_sec: int = 0


class PlaylistResult(BaseModel):
    list: list[PlaylistItem]


class PlaylistService:
    def __init__(self, client: BilibiliClient, data_dir: str = "data"):
        self.client = client
        self.data_dir = Path(data_dir)
        self.data_dir.mkdir(parents=True, exist_ok=True)
        self.playlist_file = self.data_dir / "playlist.json"
        self._playlist: list[PlaylistItem] = []
        self._load_playlist()

    def _load_playlist(self):
        """从JSON文件加载播放列表"""
        if self.playlist_file.exists():
            try:
                with open(self.playlist_file, "r", encoding="utf-8") as f:
                    data = json.load(f)
                    self._playlist = [PlaylistItem(**item) for item in data]
                logger.info(f"Loaded playlist: {len(self._playlist)} songs")
            except Exception as e:
                logger.error(f"Failed to load playlist: {e}")
                self._playlist = []
        else:
            self._playlist = []

    def _save_playlist(self):
        """保存播放列表到JSON文件"""
        try:
            with open(self.playlist_file, "w", encoding="utf-8") as f:
                json.dump(
                    [item.model_dump() for item in self._playlist],
                    f,
                    ensure_ascii=False,
                    indent=2
                )
            logger.debug(f"Saved playlist: {len(self._playlist)} songs")
        except Exception as e:
            logger.error(f"Failed to save playlist: {e}")

    async def get_playlist(self) -> PlaylistResult:
        return PlaylistResult(list=self._playlist)

    async def add_to_playlist(self, bvid: str) -> PlaylistItem | None:
        # 检查是否已存在
        for item in self._playlist:
            if item.bvid == bvid:
                return item

        try:
            info = await bilibili_video.get_video_info(self.client, bvid)
            item = PlaylistItem(
                bvid=info.bvid,
                title=info.title,
                artist=info.owner.name,
                cover=info.pic,
                duration=self._format_duration(info.duration),
                duration_sec=info.duration,
            )
            self._playlist.append(item)
            self._save_playlist()
            return item
        except Exception as e:
            logger.error(f"Failed to add {bvid} to playlist: {e}")
            return None

    async def remove_from_playlist(self, bvid: str) -> bool:
        for i, item in enumerate(self._playlist):
            if item.bvid == bvid:
                self._playlist.pop(i)
                self._save_playlist()
                return True
        return False

    async def clear_playlist(self) -> None:
        self._playlist.clear()
        self._save_playlist()

    async def reorder_playlist(self, bvids: list[str]) -> PlaylistResult:
        """根据bvid列表重新排序播放列表"""
        bvid_map = {item.bvid: item for item in self._playlist}
        new_list: list[PlaylistItem] = []
        for bvid in bvids:
            if bvid in bvid_map:
                new_list.append(bvid_map[bvid])
        # 保留未在排序列表中的项目
        existing_bvids = set(bvids)
        for item in self._playlist:
            if item.bvid not in existing_bvids:
                new_list.append(item)
        self._playlist = new_list
        self._save_playlist()
        return PlaylistResult(list=self._playlist)

    @staticmethod
    def _format_duration(seconds: int) -> str:
        m = seconds // 60
        s = seconds % 60
        return f"{m}:{s:02d}"
