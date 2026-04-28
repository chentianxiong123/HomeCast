from pydantic import BaseModel
from loguru import logger
from app.speaker.device_manager import SpeakerManager
from app.speaker.auth import SpeakerAuth
from app.bilibili.client import BilibiliClient
from app.bilibili.audio import get_best_audio_url, AUDIO_64K
from app.bilibili.video import get_video_info
from app.proxy.token_store import token_store
from app.config import get_config
import socket


class SpeakerDeviceResult(BaseModel):
    did: str
    name: str
    hardware: str
    device_id: str
    is_online: bool = True


class PlayRequest(BaseModel):
    bvid: str
    did: str
    quality: int = AUDIO_64K


class ControlRequest(BaseModel):
    did: str
    action: str = "pause"
    volume: int | None = None


def _get_local_ip() -> str:
    """获取本地局域网IP地址"""
    try:
        # 方法1：通过UDP连接外部地址获取本地IP
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        try:
            # 方法2：获取主机名对应的IP
            return socket.gethostbyname(socket.gethostname())
        except Exception:
            return "127.0.0.1"


def _get_proxy_base_url() -> str:
    """获取代理URL的基础地址"""
    config = get_config()
    
    # 优先使用配置的 public_url
    if config.server.public_url:
        return config.server.public_url.rstrip('/')
    
    # 否则自动获取局域网IP
    local_ip = _get_local_ip()
    return f"http://{local_ip}:{config.server.port}"


class SpeakerService:
    def __init__(self, auth: SpeakerAuth, bili_client: BilibiliClient):
        self.auth = auth
        self.manager = SpeakerManager(auth)
        self.bili_client = bili_client

    async def get_devices(self) -> list[SpeakerDeviceResult]:
        devices = self.manager.list_devices()
        if not devices:
            devices = await self.manager.refresh_devices()
        return [
            SpeakerDeviceResult(
                did=d.did,
                name=d.name,
                hardware=d.hardware,
                device_id=d.device_id,
                is_online=True,  # miservice 返回的设备默认在线
            )
            for d in devices
        ]

    async def play(self, req: PlayRequest) -> dict:
        config = get_config()

        try:
            video_info = await get_video_info(self.bili_client, req.bvid)
        except Exception as e:
            logger.error(f"Get video info failed: {e}")
            return {"code": 500, "message": f"获取视频信息失败: {e}"}

        try:
            audio_result = await get_best_audio_url(
                self.bili_client, req.bvid, video_info.cid, req.quality
            )
        except Exception as e:
            logger.error(f"Get audio stream failed: {e}")
            return {"code": 500, "message": f"获取音频流失败: {e}"}

        device = self.manager.get_device(req.did)
        hardware = device.hardware if device else ""

        token = token_store.store(
            audio_result.url,
            metadata={
                "mime_type": audio_result.mime_type,
                "hardware": hardware,
                "bvid": req.bvid,
                "title": video_info.title,
            },
        )
        
        # 使用可访问的地址
        proxy_url = f"{_get_proxy_base_url()}/proxy/audio/{token}"
        
        logger.info(f"Playing to speaker: device={req.did}, url={proxy_url}")

        result = await self.manager.play_url(req.did, proxy_url)
        if result is None:
            return {"code": 500, "message": "推送失败，请检查音箱连接"}

        return {
            "code": 0,
            "message": "success",
            "data": {
                "bvid": req.bvid,
                "title": video_info.title,
                "quality": audio_result.quality,
                "proxy_url": proxy_url,
            },
        }

    async def control(self, req: ControlRequest) -> dict:
        if req.action == "pause":
            result = await self.manager.pause(req.did)
        elif req.action == "resume":
            # 恢复播放：重新发送当前URL
            result = await self.manager.resume(req.did)
        elif req.action == "stop":
            result = await self.manager.stop(req.did)
        elif req.action == "volume" and req.volume is not None:
            result = await self.manager.set_volume(req.did, req.volume)
        else:
            return {"code": 400, "message": f"unknown action: {req.action}"}

        if result is None:
            return {"code": 500, "message": "控制失败"}
        return {"code": 0, "message": "success", "data": result}

    async def get_status(self, did: str) -> dict:
        result = await self.manager.get_status(did)
        if result is None:
            return {"code": 500, "message": "获取状态失败"}
        return {"code": 0, "message": "success", "data": result}
