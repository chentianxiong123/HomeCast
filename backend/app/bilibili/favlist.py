from pydantic import BaseModel
from app.bilibili.client import BilibiliClient


class FavUpper(BaseModel):
    mid: int = 0
    name: str = ""
    face: str = ""
    followed: bool = False


class FavListInfo(BaseModel):
    id: int = 0
    fid: int = 0
    mid: int = 0
    title: str = ""
    cover: str = ""
    media_count: int = 0
    intro: str = ""
    upper: FavUpper = FavUpper()
    ctime: int = 0
    mtime: int = 0


class FavMedia(BaseModel):
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
    upper: FavUpper = FavUpper()
    link: str = ""


class FavListResult(BaseModel):
    info: FavListInfo = FavListInfo()
    medias: list[FavMedia] = []
    has_more: bool = False


async def get_fav_list(
    client: BilibiliClient, media_id: int, page: int = 1, page_size: int = 20
) -> FavListResult:
    params = {
        "media_id": media_id,
        "pn": page,
        "ps": page_size,
        "platform": "web",
    }
    data = await client.get("/x/v3/fav/resource/list", params=params)

    info_data = data.get("info", {})
    info = FavListInfo(
        id=info_data.get("id", 0),
        fid=info_data.get("fid", 0),
        mid=info_data.get("mid", 0),
        title=info_data.get("title", ""),
        cover=info_data.get("cover", ""),
        media_count=info_data.get("media_count", 0),
        intro=info_data.get("intro", ""),
        upper=FavUpper(**info_data.get("upper", {})),
        ctime=info_data.get("ctime", 0),
        mtime=info_data.get("mtime", 0),
    )

    medias = []
    for m in data.get("medias", []) or []:
        upper_data = m.get("upper", {})
        medias.append(
            FavMedia(
                id=m.get("id", 0),
                type=m.get("type", 0),
                title=m.get("title", ""),
                cover=m.get("cover", ""),
                intro=m.get("intro", ""),
                page=m.get("page", 0),
                duration=m.get("duration", 0),
                bvid=m.get("bvid", ""),
                ctime=m.get("ctime", 0),
                pubtime=m.get("pubtime", 0),
                fav_time=m.get("fav_time", 0),
                upper=FavUpper(mid=upper_data.get("mid", 0), name=upper_data.get("name", "")),
                link=m.get("link", ""),
            )
        )

    return FavListResult(info=info, medias=medias, has_more=data.get("has_more", False))


async def get_fav_list_info(client: BilibiliClient, media_id: int) -> FavListInfo:
    data = await client.get("/x/v3/fav/folder/info", params={"media_id": media_id})
    return FavListInfo(**data)
