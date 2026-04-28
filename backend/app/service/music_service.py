from pydantic import BaseModel
from app.bilibili.client import BilibiliClient
from app.bilibili import search as bilibili_search
from app.bilibili import video as bilibili_video
from app.bilibili import audio as bilibili_audio
from app.bilibili.audio import AUDIO_64K


class MusicItem(BaseModel):
    bvid: str
    title: str
    artist: str
    cover: str
    duration: str
    duration_sec: int = 0
    play_count: int = 0


class MusicSearchResult(BaseModel):
    total: int
    list: list[MusicItem]


class VideoInfoResult(BaseModel):
    bvid: str
    aid: int = 0
    cid: int = 0
    title: str
    desc: str = ""
    cover: str = ""
    duration: int = 0
    artist: str = ""
    artist_id: int = 0


class AudioStreamResult(BaseModel):
    url: str
    quality: int
    size: int = 0
    mime_type: str = ""
    codecs: str = ""


class MusicService:
    def __init__(self, client: BilibiliClient):
        self.client = client

    async def search(
        self, keyword: str, page: int = 1, page_size: int = 20
    ) -> MusicSearchResult:
        result = await bilibili_search.search(self.client, keyword, page, page_size)
        items = []
        for item in result.result:
            items.append(
                MusicItem(
                    bvid=item.bvid,
                    title=item.title,
                    artist=item.author,
                    cover=item.pic,
                    duration=item.duration,
                    play_count=item.play,
                )
            )
        return MusicSearchResult(total=result.num_results, list=items)

    async def get_video_info(self, bvid: str) -> VideoInfoResult:
        info = await bilibili_video.get_video_info(self.client, bvid)
        return VideoInfoResult(
            bvid=info.bvid,
            aid=info.aid,
            cid=info.cid,
            title=info.title,
            desc=info.desc,
            cover=info.pic,
            duration=info.duration,
            artist=info.owner.name,
            artist_id=info.owner.mid,
        )

    async def get_audio_stream(
        self, bvid: str, quality: int = AUDIO_64K
    ) -> AudioStreamResult:
        info = await bilibili_video.get_video_info(self.client, bvid)
        result = await bilibili_audio.get_best_audio_url(
            self.client, bvid, info.cid, quality
        )
        return AudioStreamResult(
            url=result.url,
            quality=result.quality,
            size=result.size,
            mime_type=result.mime_type,
            codecs=result.codecs,
        )

    async def get_audio_proxy_url(self, bvid: str, quality: int = AUDIO_64K) -> str:
        return f"/api/v1/music/stream/{bvid}?quality={quality}"
