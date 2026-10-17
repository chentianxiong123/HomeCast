"""小米账号二维码登录模块

参考 XiaoMusic 项目的实现
"""

import base64
import hashlib
import json
import os
import random
import time
from datetime import datetime, timedelta
from io import BytesIO
from urllib import parse

import qrcode
from aiohttp import ClientSession


class MiQRCodeLogin:
    """小米账号二维码登录器"""

    def __init__(self, conf_path: str = "data"):
        self.conf_path = conf_path
        self.auth_data_path = os.path.join(conf_path, "auth.json")
        self.auth_data = {}
        self._session = None
        self._device_id = self._generate_device_id()
        self._locale = "zh_CN"
        self._login_status = "idle"  # idle, pending, scanning, success, failed
        self._login_message = ""

        # 加载已有认证数据
        self._load_auth_data()

    def _generate_device_id(self) -> str:
        """生成随机设备ID"""
        chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
        return "".join(random.choices(chars, k=16))

    def _generate_user_agent(self) -> str:
        """生成 User-Agent"""
        ua_id1 = "".join(random.choices("0123456789ABCDEF", k=40))
        ua_id2 = "".join(random.choices("0123456789ABCDEF", k=32))
        ua_id3 = "".join(random.choices("0123456789ABCDEF", k=32))
        ua_id4 = "".join(random.choices("0123456789ABCDEF", k=40))
        return (
            f"Android-15-11.0.701-Xiaomi-23046RP50C-OS2.0.212.0.VMYCNXM-"
            f"{ua_id1}-CN-"
            f"{ua_id3}-{ua_id2}-SmartHome-MI_APP_STORE-{ua_id1}|{ua_id4}|test-64"
        )

    def _load_auth_data(self):
        """加载已有认证数据"""
        if os.path.exists(self.auth_data_path):
            try:
                with open(self.auth_data_path, "r", encoding="utf-8") as f:
                    self.auth_data = json.load(f)
            except Exception:
                self.auth_data = {}

    def _save_auth_data(self):
        """保存认证数据"""
        self.auth_data["saveTime"] = int(time.time() * 1000)
        os.makedirs(self.conf_path, exist_ok=True)
        with open(self.auth_data_path, "w", encoding="utf-8") as f:
            json.dump(self.auth_data, f, indent=2, ensure_ascii=False)
        os.chmod(self.auth_data_path, 0o600)

    @property
    def is_logged_in(self) -> bool:
        """检查是否已登录"""
        if not self.auth_data:
            return False
        required_keys = ["userId", "passToken", "ssecurity"]
        return all(key in self.auth_data for key in required_keys)

    def get_user_id(self) -> str:
        """获取用户ID"""
        return self.auth_data.get("userId", "")

    def get_pass_token(self) -> str:
        """获取 passToken"""
        return self.auth_data.get("passToken", "")

    def get_cookie_string(self) -> str:
        """获取 Cookie 字符串"""
        if not self.is_logged_in:
            return ""
        return f"userId={self.get_user_id()}; passToken={self.get_pass_token()}"

    async def _get_session(self) -> ClientSession:
        """获取或创建 session"""
        if self._session is None or self._session.closed:
            self._session = ClientSession()
        return self._session

    async def generate_qr_code(self) -> dict:
        """生成二维码

        Returns:
            dict: {
                "qr_url": str,  # 二维码内容 URL
                "qr_image": str,  # base64 编码的二维码图片
                "status_url": str,  # 轮询状态的 URL
            }
        """
        session = await self._get_session()

        # Step 1: 获取登录链接参数
        service_login_url = (
            f"https://account.xiaomi.com/pass/serviceLogin?_json=true"
            f"&sid=mijia&_locale={self._locale}"
        )

        headers = {
            "User-Agent": self._generate_user_agent(),
            "Connection": "keep-alive",
            "Accept-Encoding": "gzip",
            "Content-Type": "application/x-www-form-urlencoded",
        }

        # 如果有已有的 token，尝试刷新
        if self.auth_data.get("passToken"):
            cookie = f"deviceId={self._device_id}; passToken={self.auth_data['passToken']}; userId={self.auth_data.get('userId', '')}"
            headers["Cookie"] = cookie

        async with session.get(service_login_url, headers=headers) as resp:
            text = await resp.text()
            text = text.replace("&&&START&&&", "")
            service_data = json.loads(text)

        # 如果 Token 有效，直接返回成功
        if service_data.get("code") == 0:
            location = service_data.get("location", "")
            if location:
                async with session.get(location, headers=headers) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        if text == "ok":
                            # Token 有效，刷新成功
                            cookies = {k: v.value for k, v in resp.cookies.items()}
                            self.auth_data.update(cookies)
                            self.auth_data["ssecurity"] = service_data.get("ssecurity", "")
                            self._save_auth_data()
                            self._login_status = "success"
                            self._login_message = "Token 有效，无需重新登录"
                            return {
                                "success": True,
                                "message": "Token 有效，无需重新登录",
                                "qr_url": "",
                                "qr_image": "",
                                "status_url": "",
                            }

        # 需要生成二维码
        location_data = {
            k: v[0] if isinstance(v, list) else v
            for k, v in parse.parse_qs(parse.urlparse(service_data.get("location", "")).query).items()
        }

        # Step 2: 获取二维码
        location_data.update({
            "theme": "",
            "bizDeviceType": "",
            "_hasLogo": "false",
            "_qrsize": "240",
            "_dc": str(int(time.time() * 1000)),
        })

        login_url = "https://account.xiaomi.com/longPolling/loginUrl?" + parse.urlencode(location_data)

        async with session.get(login_url, headers=headers) as resp:
            text = await resp.text()
            text = text.replace("&&&START&&&", "")
            login_data = json.loads(text)

        # 生成二维码图片
        qr_url = login_data.get("loginUrl", "")
        lp_url = login_data.get("lp", "")

        # 创建二维码
        qr = qrcode.QRCode(
            version=1,
            error_correction=qrcode.constants.ERROR_CORRECT_L,
            box_size=10,
            border=4,
        )
        qr.add_data(qr_url)
        qr.make(fit=True)

        # 生成 base64 图片
        img = qr.make_image(fill_color="black", back_color="white")
        buffered = BytesIO()
        img.save(buffered, format="PNG")
        img_base64 = base64.b64encode(buffered.getvalue()).decode()

        self._login_status = "pending"
        self._login_message = "等待扫码"
        self._lp_url = lp_url
        self._headers = headers

        return {
            "success": True,
            "message": "请使用米家 APP 扫描二维码",
            "qr_url": qr_url,
            "qr_image": f"data:image/png;base64,{img_base64}",
            "status_url": lp_url,
        }

    async def check_login_status(self) -> dict:
        """检查登录状态

        Returns:
            dict: {
                "status": str,  # idle, pending, scanning, success, failed
                "message": str,
                "user_id": str,
            }
        """
        if self._login_status in ["idle", "success", "failed"]:
            return {
                "status": self._login_status,
                "message": self._login_message,
                "user_id": self.get_user_id(),
            }

        if not hasattr(self, "_lp_url"):
            return {
                "status": "failed",
                "message": "未生成二维码",
                "user_id": "",
            }

        session = await self._get_session()

        try:
            async with session.get(
                self._lp_url, headers=self._headers, timeout=120
            ) as resp:
                text = await resp.text()
                text = text.replace("&&&START&&&", "")
                lp_data = json.loads(text)

            # 检查是否登录成功
            if lp_data.get("code") == 0:
                # 登录成功，保存认证数据
                auth_keys = [
                    "psecurity", "nonce", "ssecurity", "passToken",
                    "userId", "cUserId", "serviceToken"
                ]
                for key in auth_keys:
                    if key in lp_data:
                        self.auth_data[key] = lp_data[key]

                # 设置过期时间（30天）
                self.auth_data["expireTime"] = int(
                    (datetime.now() + timedelta(days=30)).timestamp() * 1000
                )
                self._save_auth_data()

                self._login_status = "success"
                self._login_message = "登录成功"

                return {
                    "status": "success",
                    "message": "登录成功",
                    "user_id": self.get_user_id(),
                }
            else:
                # 仍在等待
                self._login_status = "pending"
                self._login_message = "等待扫码..."
                return {
                    "status": "pending",
                    "message": "等待扫码...",
                    "user_id": "",
                }

        except Exception as e:
            self._login_status = "failed"
            self._login_message = f"登录失败: {str(e)}"
            return {
                "status": "failed",
                "message": str(e),
                "user_id": "",
            }

    async def close(self):
        """关闭 session"""
        if self._session and not self._session.closed:
            await self._session.close()
            self._session = None

    def reset(self):
        """重置登录状态"""
        self._login_status = "idle"
        self._login_message = ""
        if hasattr(self, "_lp_url"):
            delattr(self, "_lp_url")
        if hasattr(self, "_headers"):
            delattr(self, "_headers")
