from pydantic import BaseModel
from loguru import logger
from app.speaker.auth import SpeakerAuth


NEED_USE_PLAY_MUSIC_API = {
    "X08C", "X08E", "X8F", "X4B", "LX05", "OH2", "OH2P", "X6A",
}


class SpeakerDevice(BaseModel):
    device_id: str
    hardware: str
    did: str
    name: str


class SpeakerManager:
    def __init__(self, auth: SpeakerAuth):
        self.auth = auth
        self.devices: dict[str, SpeakerDevice] = {}
        # 保存当前播放信息用于恢复
        self._playing_urls: dict[str, str] = {}  # did -> url

    async def refresh_devices(self) -> list[SpeakerDevice]:
        raw_devices = await self.auth.get_device_list()
        self.devices = {}
        result = []
        for d in raw_devices:
            device = SpeakerDevice(**d)
            self.devices[device.did] = device
            result.append(device)
        logger.info(f"Found {len(result)} xiaomi speaker devices")
        return result

    def get_device(self, did: str) -> SpeakerDevice | None:
        return self.devices.get(did)

    def list_devices(self) -> list[SpeakerDevice]:
        return list(self.devices.values())

    async def play_url(
        self, did: str, url: str, audio_id: str | None = None
    ) -> dict | None:
        device = self.get_device(did)
        if not device:
            logger.error(f"Device not found: {did}")
            return None

        # 保存当前播放的URL用于恢复
        self._playing_urls[did] = url

        if audio_id:
            return await self.auth.play_by_music_url(
                device.device_id, url, audio_id
            )

        if device.hardware in NEED_USE_PLAY_MUSIC_API:
            return await self.auth.play_by_music_url(device.device_id, url)

        return await self.auth.play_by_url(device.device_id, url)

    async def pause(self, did: str) -> dict | None:
        device = self.get_device(did)
        if not device:
            return None
        return await self.auth.pause(device.device_id)

    async def stop(self, did: str) -> dict | None:
        device = self.get_device(did)
        if not device:
            return None
        # 清除保存的URL
        if did in self._playing_urls:
            del self._playing_urls[did]
        await self.auth.pause(device.device_id)
        return await self.auth.stop(device.device_id)

    async def resume(self, did: str) -> dict | None:
        """恢复播放（重新发送之前的URL）"""
        url = self._playing_urls.get(did)
        if not url:
            logger.warning(f"No saved URL to resume for {did}")
            return None

        device = self.get_device(did)
        if not device:
            return None

        if device.hardware in NEED_USE_PLAY_MUSIC_API:
            return await self.auth.play_by_music_url(device.device_id, url)

        return await self.auth.play_by_url(device.device_id, url)

    async def get_status(self, did: str) -> dict | None:
        device = self.get_device(did)
        if not device:
            return None
        return await self.auth.get_status(device.device_id)

    async def get_volume(self, did: str) -> int | None:
        """获取音量"""
        device = self.get_device(did)
        if not device:
            return None
        return await self.auth.get_volume(device.device_id)

    async def set_volume(self, did: str, volume: int) -> dict | None:
        device = self.get_device(did)
        if not device:
            return None
        return await self.auth.set_volume(device.device_id, volume)
