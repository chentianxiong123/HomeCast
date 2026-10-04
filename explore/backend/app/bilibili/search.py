import re
from pydantic import BaseModel
from app.bilibili.client import BilibiliClient


class SearchItem(BaseModel):
    id: int = 0
    bvid: str = ""
    title: str = ""
    desc: str = ""
    duration: str = ""
    pic: str = ""
    link: str = ""
    pubdate: int = 0
    senddate: int = 0
    author: str = ""
    mid: int = 0
    play: int = 0
    review: int = 0
    video_review: int = 0
    favorites: int = 0


class SearchResult(BaseModel):
    seid: str = ""
    page: int = 1
    pagesize: int = 20
    num_results: int = 0
    num_pages: int = 0
    suggest_keyword: str = ""
    result: list[SearchItem] = []


_html_tag_re = re.compile(r"<[^>]+>")


def clean_html(text: str) -> str:
    return _html_tag_re.sub("", text)


async def search(
    client: BilibiliClient, keyword: str, page: int = 1, page_size: int = 20
) -> SearchResult:
    params = {
        "keyword": keyword,
        "search_type": "video",
        "page": page,
        "pagesize": page_size,
    }
    data = await client.get("/x/web-interface/search/type", params=params)
    result = SearchResult(**data)
    for item in result.result:
        item.title = clean_html(item.title)
    return result


async def get_search_suggest(client: BilibiliClient, keyword: str) -> list[str]:
    params = {"term": keyword}
    data = await client.get(
        "https://s.search.bilibili.com/main/suggest", params=params
    )
    if isinstance(data, dict):
        return list(data.values())
    return []
