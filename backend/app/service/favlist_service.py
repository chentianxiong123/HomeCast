from pydantic import BaseModel
from app.bilibili.client import BilibiliClient
from app.bilibili import favlist as bilibili_favlist


class FavUpperResult(BaseModel):
    mid: int = 0
    name: str = ""
    face: str = ""


class FavListInfoResult(BaseModel):
    id: int = 0
    fid: int = 0
    mid: int = 0
    title: str = ""
    cover: str = ""
    media_count: int = 0
    intro: str = ""
    upper: FavUpperResult = FavUpperResult()
    ctime: int = 0
    mtime: int = 0


class FavMediaResult(BaseModel):
    id: int = 0
    type: int = 0
    title: str = ""
    cover: str = ""
    intro: str = ""
    page: int = 0
    duration: int = 0
    bvid: str = ""
    ctime: int = 0
    pubtime: int = 0
    fav_time: int = 0
    upper: FavUpperResult = FavUpperResult()
    link: str = ""


class FavListResult(BaseModel):
    info: FavListInfoResult
    medias: list[FavMediaResult]
    has_more: bool = False


class FavListService:
    def __init__(self, client: BilibiliClient):
        self.client = client

    async def get_fav_list(
        self, media_id: int, page: int = 1, page_size: int = 20
    ) -> FavListResult:
        result = await bilibili_favlist.get_fav_list(
            self.client, media_id, page, page_size
        )
        info = result.info
        return FavListResult(
            info=FavListInfoResult(
                id=info.id,
                fid=info.fid,
                mid=info.mid,
                title=info.title,
                cover=info.cover,
                media_count=info.media_count,
                intro=info.intro,
                upper=FavUpperResult(
                    mid=info.upper.mid,
                    name=info.upper.name,
                    face=info.upper.face,
                ),
                ctime=info.ctime,
                mtime=info.mtime,
            ),
            medias=[
                FavMediaResult(
                    id=m.id,
                    type=m.type,
                    title=m.title,
                    cover=m.cover,
                    intro=m.intro,
                    page=m.page,
                    duration=m.duration,
                    bvid=m.bvid,
                    ctime=m.ctime,
                    pubtime=m.pubtime,
                    fav_time=m.fav_time,
                    upper=FavUpperResult(mid=m.upper.mid, name=m.upper.name),
                    link=m.link,
                )
                for m in result.medias
            ],
            has_more=result.has_more,
        )

    async def get_fav_list_info(self, media_id: int) -> FavListInfoResult:
        info = await bilibili_favlist.get_fav_list_info(self.client, media_id)
        return FavListInfoResult(
            id=info.id,
            fid=info.fid,
            mid=info.mid,
            title=info.title,
            cover=info.cover,
            media_count=info.media_count,
            intro=info.intro,
            upper=FavUpperResult(
                mid=info.upper.mid,
                name=info.upper.name,
                face=info.upper.face,
            ),
            ctime=info.ctime,
            mtime=info.mtime,
        )
