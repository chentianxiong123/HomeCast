from pydantic import BaseModel
from app.bilibili.client import BilibiliClient


AUDIO_64K = 64
AUDIO_128K = 128
AUDIO_192K = 192
AUDIO_320K = 30280
AUDIO_FLAC = 30250

QUALITY_PRIORITY = [AUDIO_FLAC, AUDIO_320K, AUDIO_192K, AUDIO_128K, AUDIO_64K]


def get_lowest_quality() -> int:
    return AUDIO_64K


def get_preferred_quality() -> int:
    return AUDIO_64K


class DashAudioItem(BaseModel):
    id: int
    base_url: str = ""
    backup_url: list[str] = []
    bandwidth: int = 0
    mime_type: str = ""
    codecs: str = ""
    size: int = 0


class DashInfo(BaseModel):
    duration: int = 0
    audio: list[DashAudioItem] = []


class AudioURLInfo(BaseModel):
    url: str = ""
    size: int = 0
    quality: int = 0


class AudioStreamInfo(BaseModel):
    quality: int = 0
    format: str = ""
    timelength: int = 0
    accept_format: str = ""
    accept_description: list[str] = []
    accept_quality: list[int] = []
    dash: DashInfo | None = None
    durl: list[AudioURLInfo] = []


class AudioStreamResult(BaseModel):
    url: str
    quality: int
    size: int = 0
    mime_type: str = ""
    codecs: str = ""


async def get_audio_stream(
    client: BilibiliClient, bvid: str, cid: int, quality: int = AUDIO_64K
) -> AudioStreamInfo:
    params = {
        "bvid": bvid,
        "cid": cid,
        "qn": quality,
        "fnval": 16,
        "fourk": 1,
        "platform": "html5",
        "mobisel": 1,
        "highbit": 1,
    }
    data = await client.get("/x/player/playurl", params=params)
    return AudioStreamInfo(**data)


async def get_best_audio_url(
    client: BilibiliClient, bvid: str, cid: int, prefer_quality: int = AUDIO_64K
) -> AudioStreamResult:
    """获取最佳音频URL

    策略：优先使用 durl（可直接下载的URL），fallback 到 dash
    因为 dash 的 base_url 有签名限制，后端直接访问会403
    """
    stream = await get_audio_stream(client, bvid, cid, prefer_quality)

    # 优先使用 durl（可直接下载，无403问题）
    if stream.durl:
        audio = stream.durl[0]
        return AudioStreamResult(
            url=audio.url,
            quality=prefer_quality,
            size=audio.size,
        )

    # fallback 到 dash
    if stream.dash and stream.dash.audio:
        audio_map = {a.id: a for a in stream.dash.audio}
        if prefer_quality in audio_map:
            best = audio_map[prefer_quality]
        else:
            for q in QUALITY_PRIORITY:
                if q in audio_map:
                    best = audio_map[q]
                    break
            else:
                best = stream.dash.audio[0]
        return AudioStreamResult(
            url=best.base_url,
            quality=best.id,
            size=best.size,
            mime_type=best.mime_type,
            codecs=best.codecs,
        )

    raise ValueError("no audio stream available")


async def get_available_qualities(
    client: BilibiliClient, bvid: str, cid: int
) -> list[dict]:
    stream = await get_audio_stream(client, bvid, cid, AUDIO_FLAC)
    result = []
    if stream.dash and stream.dash.audio:
        for a in stream.dash.audio:
            result.append(
                {
                    "quality": a.id,
                    "bandwidth": a.bandwidth,
                    "mime_type": a.mime_type,
                    "codecs": a.codecs,
                    "size": a.size,
                }
            )
    return result
