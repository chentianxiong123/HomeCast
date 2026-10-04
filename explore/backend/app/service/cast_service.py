import asyncio
import random
from loguru import logger
from app.dlna.discovery import DLNADiscovery, DLNADevice
from app.dlna.controller import DLNAController
from app.sniffer.extractor import get_extractor
from app.proxy.token_store import token_store
from app.config import get_config


class CastService:
    def __init__(self):
        self.discovery = DLNADiscovery()
        self.controllers: dict[str, DLNAController] = {}
        self.devices: dict[str, DLNADevice] = {}

    def _is_detail_page(self, url: str) -> bool:
        """判断是否为详情页URL"""
        import re
        patterns = [
            r"/vod/",
            r"/voddetail/",
            r"/detail/",
            r"/movie/",
            r"/tv/",
        ]
        return any(re.search(p, url, re.IGNORECASE) for p in patterns)

    async def discover_devices(self, target_ip: str = None) -> list[dict]:
        devices = await self.discovery.search(timeout=5.0, target_ip=target_ip)
        self.devices = {d.udn: d for d in devices}
        return [
            {
                "name": d.name,
                "udn": d.udn,
                "ip": d.ip,
                "port": d.port,
                "device_type": d.device_type,
            }
            for d in devices
        ]

    async def sniff_video(self, url: str) -> dict:
        # 如果是详情页，先检查缓存
        if self._is_detail_page(url):
            from app.api.sites import get_cached_episodes
            cached = get_cached_episodes(url)
            if cached:
                logger.info(f"Returning cached episodes for {url}")
                return {
                    "code": 0,
                    "message": "success",
                    "data": {
                        "title": cached.get("title", ""),
                        "episodes": [],
                        "episodes_list": cached.get("episodes_list", []),
                        "sniff_method": "cache",
                    },
                    "cached": True,
                }

        extractor = get_extractor()
        result = await extractor.sniff(url)

        # 如果是详情页且有集数列表，缓存起来
        if self._is_detail_page(url) and result.episodes_list:
            from app.api.sites import cache_episodes
            cache_episodes(url, result.title, [e.model_dump() for e in result.episodes_list])

        return {
            "code": 0,
            "message": "success",
            "data": {
                "title": result.title,
                "episodes": [e.model_dump() for e in result.episodes],
                "episodes_list": [e.model_dump() for e in result.episodes_list],
                "sniff_method": result.sniff_method,
            },
        }

    async def play_url(self, video_url: str, title: str = "Video") -> dict:
        """获取播放URL，直接返回m3u8或代理URL"""
        try:
            config = get_config()

            # 如果是HLS流，返回m3u8 URL
            if _is_hls_stream(video_url):
                return {
                    "code": 0,
                    "message": "success",
                    "data": {
                        "url": video_url,
                        "type": "application/x-mpegurl",
                        "hls": True,
                    },
                }

            # 非HLS流，使用代理
            is_direct = _is_direct_video_url(video_url)

            if is_direct:
                token = token_store.store(
                    video_url,
                    metadata={
                        "content_type": _guess_content_type(video_url),
                        "original_url": video_url,
                        "referer": _extract_referer(video_url),
                    },
                )
                proxy_url = f"http://{config.server.host}:{config.server.port}/proxy/video/{token}"
                return {
                    "code": 0,
                    "message": "success",
                    "data": {
                        "url": proxy_url,
                        "type": _guess_content_type(video_url),
                        "hls": False,
                    },
                }

            return {"code": 500, "message": "不支持的视频格式"}
        except Exception as e:
            logger.error(f"play_url error: {e}")
            return {"code": 500, "message": str(e)}

    async def cast(self, episode_url: str, device_udn: str, title: str = "Video") -> dict:
        """投屏到DLNA设备"""
        logger.info(f"Cast called: url={episode_url}, device={device_udn}")
        device = self.devices.get(device_udn)
        if not device:
            logger.error(f"Device not found: {device_udn}, available devices: {list(self.devices.keys())}")
            return {"code": 404, "message": "设备未找到，请先刷新设备列表"}

        # 如果不是直接视频URL，先嗅探获取视频URL
        video_url = episode_url
        if not _is_direct_video_url(episode_url) and not _is_hls_stream(episode_url):
            logger.info(f"Not a direct video URL, sniffing: {episode_url}")
            sniff_result = await self._sniff_play_page(episode_url)
            if not sniff_result:
                return {"code": 500, "message": "无法获取视频URL"}
            video_url = sniff_result
            logger.info(f"Sniffed video URL: {video_url}")

        # 获取播放URL
        result = await self.play_url(video_url, title)
        if result["code"] != 0:
            logger.error(f"play_url failed: {result}")
            return result

        stream_url = result["data"]["url"]
        content_type = result["data"]["type"]
        logger.info(f"Stream URL: {stream_url}, type: {content_type}, hls: {result['data'].get('hls')}")

        # 获取或创建控制器
        controller = self.controllers.get(device_udn)
        if not controller:
            controller = DLNAController(device)
            controller.init()
            self.controllers[device_udn] = controller
            logger.info(f"Created new controller for {device.name}")

        # 设置播放地址
        logger.info(f"Calling set_av_transport_uri with URL: {stream_url}")
        success = controller.set_av_transport_uri(stream_url, title=title, content_type=content_type)
        if not success:
            logger.error("set_av_transport_uri failed")
            return {"code": 500, "message": "设置播放地址失败"}

        # 开始播放
        logger.info("Calling play...")
        success = controller.play()
        if not success:
            logger.error("play failed")
            return {"code": 500, "message": "播放启动失败"}

        logger.info(f"Cast started successfully on {device.name}")
        return {"code": 0, "message": "success", "data": {"device_name": device.name}}

    async def _sniff_play_page(self, url: str) -> str | None:
        """嗅探播放页获取视频URL"""
        try:
            extractor = get_extractor()
            result = await extractor.sniff(url)
            if result.episodes and len(result.episodes) > 0:
                return result.episodes[0].url
            return None
        except Exception as e:
            logger.error(f"Sniff play page failed: {e}")
            return None

    async def control(self, device_udn: str, action: str, target: str = None, volume: int = None) -> dict:
        controller = self.controllers.get(device_udn)
        if not controller:
            return {"code": 404, "message": "设备未连接"}

        success = False
        if action == "play":
            success = controller.play()
        elif action == "pause":
            success = controller.pause()
        elif action == "stop":
            success = controller.stop()
        elif action == "seek" and target:
            success = controller.seek(target)
        elif action == "volume" and volume is not None:
            success = controller.set_volume(volume)
        else:
            return {"code": 400, "message": f"未知操作: {action}"}

        return {"code": 0 if success else 500, "message": "success" if success else "操作失败"}

    async def get_status(self, device_udn: str) -> dict:
        controller = self.controllers.get(device_udn)
        if not controller:
            return {"code": 404, "message": "设备未连接"}

        try:
            transport = controller.get_transport_info()
            position = controller.get_position_info()
            return {
                "code": 0,
                "message": "success",
                "data": {
                    "transport": transport,
                    "position": position,
                },
            }
        except Exception as e:
            logger.error(f"get_status error: {e}")
            return {"code": 500, "message": str(e)}


def _is_direct_video_url(url: str) -> bool:
    path = url.split("?")[0].split("#")[0].lower()
    return any(path.endswith(ext) for ext in (".mp4", ".webm", ".mkv", ".avi", ".flv", ".ts", ".mov", ".m3u8", ".mpd"))

def _is_hls_stream(url: str) -> bool:
    path = url.split("?")[0].lower()
    return ".m3u8" in path

def _guess_content_type(url: str) -> str:
    path = url.split("?")[0].lower()
    if ".m3u8" in path: return "application/x-mpegurl"
    if ".mpd" in path: return "application/dash+xml"
    if ".ts" in path: return "video/mp2t"
    if ".flv" in path: return "video/x-flv"
    if ".webm" in path: return "video/webm"
    return "video/mp4"

def _extract_referer(url: str) -> str:
    if "://" in url:
        parts = url.split("://", 1)
        host = parts[1].split("/", 1)[0]
        return f"{parts[0]}://{host}/"
    return ""
