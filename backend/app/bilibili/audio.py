from pydantic import BaseModel
from app.bilibili.client import BilibiliClient


AUDIO_64K = 30216   # 64k（桌面项目已验证的 B站真实 qn 码）
AUDIO_128K = 30232  # 128k
AUDIO_192K = 30280  # 192k（免费最高）
AUDIO_FLAC = 30250  # FLAC

# 音质优先级：与桌面音乐项目一致（192k 优先，免费最高）
QUALITY_PRIORITY = [AUDIO_192K, AUDIO_128K, AUDIO_64K, AUDIO_FLAC]


def get_lowest_quality() -> int:
    return AUDIO_64K


def get_preferred_quality() -> int:
    return AUDIO_192K   # 桌面项目 PREFERRED_QN，默认 192k


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
    client: BilibiliClient, bvid: str, cid: int, quality: int = AUDIO_192K
) -> AudioStreamInfo:
    # 桌面项目已验证方案：纯 DASH 音频模式（fnval=16）。
    # 不加 platform=html5 / highbit=1 / mobisel（会强制 durl 视频模式）
    params = {
        "bvid": bvid,
        "cid": cid,
        "qn": quality,
        "fnval": 16,
        "fourk": 1,
    }
    data = await client.get("/x/player/playurl", params=params)
    return AudioStreamInfo(**data)


async def get_best_audio_url(
    client: BilibiliClient, bvid: str, cid: int, prefer_quality: int = AUDIO_192K
) -> AudioStreamResult:
    """获取音频 URL（对齐桌面项目：DASH 纯音频优先）"""
    stream = await get_audio_stream(client, bvid, cid, prefer_quality)

    # 优先 dash.audio（桌面项目主路径）
    if stream.dash and stream.dash.audio:
        by_id = {a.id: a for a in stream.dash.audio}
        for qn in [prefer_quality] + QUALITY_PRIORITY:
            if qn in by_id:
                a = by_id[qn]
                url = a.base_url or (a.backup_url[0] if a.backup_url else "")
                if url:
                    return AudioStreamResult(
                        url=url,
                        quality=qn,
                        size=a.size,
                        mime_type=a.mime_type,
                        codecs=a.codecs,
                    )
        a = stream.dash.audio[0]
        return AudioStreamResult(
            url=a.base_url or (a.backup_url[0] if a.backup_url else ""),
            quality=a.id,
            size=a.size,
            mime_type=a.mime_type,
            codecs=a.codecs,
        )

    # fallback：durl（老接口格式，一般不会出现）
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
