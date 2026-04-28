"""小米账号认证管理器

参考 XiaoMusic 和 MiAir 项目的实现
"""

import json
import os
from loguru import logger
from aiohttp import ClientSession


class SpeakerAuth:
    """小米账号认证管理器

    支持两种登录方式：
    1. 账号密码登录
    2. Cookie 登录（userId + passToken）
    """

    def __init__(
        self,
        account: str = "",
        password: str = "",
        cookie: str = "",
        token_path: str = ".mi.token",
        conf_path: str = "data",
    ):
        self.account = account
        self.password = password
        self.cookie = cookie
        self.token_path = token_path
        self.conf_path = conf_path
        self.mi_token_home = os.path.join(conf_path, ".mi.token")

        self.mina_service = None
        self.miio_service = None
        self.mi_account = None
        self.mi_session = None
        self._logged_in = False
        self.device_id = self._generate_device_id()

    def _generate_device_id(self) -> str:
        """生成随机设备ID"""
        import random
        import string
        return "".join(random.choices(string.ascii_uppercase + string.digits, k=16))

    async def init_all_data(self):
        """初始化所有数据（参考 XiaoMusic）"""
        is_need_login = await self.need_login()
        is_can_login = await self.can_login()

        if is_need_login and is_can_login:
            logger.info("尝试登录小米账号...")
            await self.login()
        else:
            logger.info(f"可能已登录 need_login={is_need_login} can_login={is_can_login}")

    async def can_login(self) -> bool:
        """检查是否可以登录"""
        if self.account and self.password:
            return True
        if self.cookie:
            return True
        if os.path.isfile(self.mi_token_home):
            return True
        logger.warning("没有账号密码或 Cookie，无法登录")
        return False

    async def need_login(self) -> bool:
        """检查是否需要登录"""
        if self.mina_service is None:
            return True
        try:
            # 尝试调用设备列表接口检查登录状态
            await self.mina_service.device_list()
            return False
        except Exception as e:
            logger.warning(f"登录状态检查失败，需要重新登录: {e}")
            return True

    async def login(self) -> bool:
        """登录小米账号

        流程：
        1. 创建 MiAccount 对象
        2. 设置 token（如果有 Cookie 或 token 文件）
        3. 调用 login("micoapi")
        4. 创建 MiNAService 和 MiIOService
        """
        try:
            from miservice import MiAccount, MiNAService, MiIOService

            # 创建 session
            if self.mi_session is None or self.mi_session.closed:
                self.mi_session = ClientSession()

            # 创建 MiAccount
            self.mi_account = MiAccount(
                self.mi_session,
                self.account,
                self.password,
                str(self.mi_token_home),
            )

            # 设置 token（如果有 Cookie 或已有 token 文件）
            self._set_token()

            # 执行登录
            await self.mi_account.login("micoapi")

            # 创建服务
            self.mina_service = MiNAService(self.mi_account)
            self.miio_service = MiIOService(self.mi_account)
            self._logged_in = True

            logger.info(f"小米账号登录成功: {self.account or 'Cookie登录'}")
            return True

        except Exception as e:
            logger.error(f"小米账号登录失败: {e}")
            self.mina_service = None
            self.miio_service = None
            self._logged_in = False
            return False

    def _set_token(self):
        """设置 token 到 account（参考 XiaoMusic）"""
        # 1. 优先从 auth.json 读取
        auth_path = os.path.join(self.conf_path, "auth.json")
        if os.path.isfile(auth_path):
            try:
                with open(auth_path, encoding="utf-8") as f:
                    user_data = json.load(f)
                self.device_id = user_data.get("deviceId", self.device_id)
                self.mi_account.token = {
                    "passToken": user_data["passToken"],
                    "userId": user_data["userId"],
                    "deviceId": self.device_id,
                }
                logger.info("从 auth.json 加载 token")
                return
            except Exception as e:
                logger.warning(f"读取 auth.json 失败: {e}")

        # 2. 从配置的 cookie 读取
        if self.cookie:
            try:
                cookie_dict = self._parse_cookie_string_to_dict(self.cookie)
                self.mi_account.token = {
                    "passToken": cookie_dict["passToken"],
                    "userId": cookie_dict["userId"],
                    "deviceId": self.device_id,
                }
                logger.info("从配置 Cookie 加载 token")
                return
            except Exception as e:
                logger.warning(f"解析 Cookie 失败: {e}")

        # 3. 从 .mi.token 文件读取
        if os.path.isfile(self.mi_token_home):
            try:
                with open(self.mi_token_home, encoding="utf-8") as f:
                    user_data = json.load(f)
                self.mi_account.token = {
                    "passToken": user_data["passToken"],
                    "userId": user_data["userId"],
                    "deviceId": self.device_id,
                }
                logger.info("从 .mi.token 文件加载 token")
                return
            except Exception as e:
                logger.warning(f"读取 .mi.token 失败: {e}")

    def _parse_cookie_string_to_dict(self, cookie: str) -> dict:
        """解析 Cookie 字符串为字典"""
        result = {}
        if not cookie:
            return result

        for item in cookie.split(";"):
            item = item.strip()
            if "=" in item:
                key, value = item.split("=", 1)
                key = key.strip()
                value = value.strip()
                result[key] = value

        return result

    def is_logged_in(self) -> bool:
        """是否已成功登录"""
        return self._logged_in and self.mina_service is not None

    async def get_device_list(self) -> list[dict]:
        """获取设备列表"""
        if not self.mina_service:
            return []
        try:
            hardware_data = await self.mina_service.device_list()
            devices = []
            for h in hardware_data:
                device_id = h.get("deviceID", "")
                hardware = h.get("hardware", "")
                did = h.get("miotDID", "")
                name = h.get("alias", "") or h.get("name", "未知设备")
                if device_id and hardware and did:
                    devices.append({
                        "device_id": device_id,
                        "hardware": hardware,
                        "did": did,
                        "name": name,
                        "is_online": True,  # 从设备列表获取的默认在线
                    })
            return devices
        except Exception as e:
            logger.error(f"Get device list failed: {e}")
            return []

    async def play_by_url(self, device_id: str, url: str) -> dict | None:
        """播放 URL"""
        if not self.mina_service:
            return None
        try:
            ret = await self.mina_service.play_by_url(device_id, url)
            logger.info(f"play_by_url: device={device_id} ret={ret}")
            return ret
        except Exception as e:
            logger.error(f"play_by_url failed: {e}")
            return None

    async def pause(self, device_id: str) -> dict | None:
        """暂停播放"""
        if not self.mina_service:
            return None
        try:
            return await self.mina_service.player_pause(device_id)
        except Exception as e:
            logger.error(f"pause failed: {e}")
            return None

    async def stop(self, device_id: str) -> dict | None:
        """停止播放"""
        if not self.mina_service:
            return None
        try:
            ret = await self.mina_service.player_stop(device_id)
            await self.pause(device_id)
            return ret
        except Exception as e:
            logger.error(f"stop failed: {e}")
            return None

    async def get_status(self, device_id: str) -> dict | None:
        """获取播放状态"""
        if not self.mina_service:
            return None
        try:
            status = await self.mina_service.player_get_status(device_id)
            if status and status.get("code") == 0:
                data = status.get("data", {})
                info_str = data.get("info", "{}")
                info = json.loads(info_str) if info_str else {}
                return {
                    "status": info.get("status", 0),
                    "volume": info.get("volume", 0),
                    "loop_type": info.get("loop_type", 0),
                }
            return None
        except Exception as e:
            logger.error(f"get_status failed: {e}")
            return None

    async def get_volume(self, device_id: str) -> int | None:
        """获取音量"""
        status = await self.get_status(device_id)
        if status:
            return status.get("volume")
        return None

    async def set_volume(self, device_id: str, volume: int) -> dict | None:
        """设置音量 (0-100)"""
        if not self.mina_service:
            return None
        try:
            volume = max(0, min(100, volume))
            return await self.mina_service.player_set_volume(device_id, volume)
        except Exception as e:
            logger.error(f"set_volume failed: {e}")
            return None

    async def close(self):
        """关闭 session"""
        if self.mi_session and not self.mi_session.closed:
            await self.mi_session.close()
        self.mi_session = None
        self.mina_service = None
        self.miio_service = None
        self._logged_in = False
