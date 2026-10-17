from pydantic import BaseModel
from app.bilibili.client import BilibiliClient


class OwnerInfo(BaseModel):
    mid: int = 0
    name: str = ""
    face: str = ""


class VideoStat(BaseModel):
    view: int = 0
    danmaku: int = 0
    reply: int = 0
    favorite: int = 0
    coin: int = 0
    share: int = 0
    like: int = 0


class VideoInfo(BaseModel):
    bvid: str = ""
    aid: int = 0
    cid: int = 0
    title: str = ""
    desc: str = ""
    pic: str = ""
    duration: int = 0
    pubdate: int = 0
    owner: OwnerInfo = OwnerInfo()
    stat: VideoStat = VideoStat()


class PageInfo(BaseModel):
    cid: int = 0
    page: int = 0
    part: str = ""
    duration: int = 0


async def get_video_info(client: BilibiliClient, bvid: str) -> VideoInfo:
    data = await client.get("/x/web-interface/view", params={"bvid": bvid})
    return VideoInfo(**data)


async def get_video_info_by_aid(client: BilibiliClient, aid: int) -> VideoInfo:
    data = await client.get("/x/web-interface/view", params={"aid": aid})
    return VideoInfo(**data)


async def get_video_pages(client: BilibiliClient, bvid: str) -> list[PageInfo]:
    data = await client.get("/x/player/pagelist", params={"bvid": bvid})
    return [PageInfo(**p) for p in data]
