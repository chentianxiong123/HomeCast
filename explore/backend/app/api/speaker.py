from fastapi import APIRouter, Request
from pydantic import BaseModel
from app.service.speaker_service import SpeakerService, PlayRequest, ControlRequest
from app.proxy.audio_proxy import proxy_audio_handler
from app.speaker.auth import SpeakerAuth

router = APIRouter(prefix="/speaker", tags=["speaker"])

_speaker_service: SpeakerService | None = None


def set_speaker_service(service: SpeakerService):
    global _speaker_service
    _speaker_service = service


def get_speaker_service() -> SpeakerService:
    if _speaker_service is None:
        raise RuntimeError("SpeakerService not initialized")
    return _speaker_service


class LoginRequest(BaseModel):
    account: str = ""
    password: str = ""
    cookie: str = ""


class VolumeRequest(BaseModel):
    did: str
    volume: int


@router.get("/status")
async def get_login_status():
    """获取小爱音箱登录状态"""
    is_logged_in = _speaker_service is not None
    devices = []
    if _speaker_service:
        devices = [d.model_dump() for d in _speaker_service.manager.list_devices()]

    return {
        "code": 0,
        "message": "success",
        "data": {
            "is_logged_in": is_logged_in,
            "device_count": len(devices),
            "devices": devices,
        }
    }


@router.post("/login")
async def login(req: LoginRequest):
    """登录小米账号（支持账号密码或 Cookie）"""
    from app.speaker.auth import SpeakerAuth
    from app.config import get_config
    from app.bilibili.client import BilibiliClient
    from app.service.speaker_service import SpeakerService
    from loguru import logger

    # 检查登录方式
    if not req.account and not req.password and not req.cookie:
        return {
            "code": 400,
            "message": "请提供账号密码或 Cookie",
            "data": None
        }

    try:
        speaker_auth = SpeakerAuth(
            account=req.account,
            password=req.password,
            cookie=req.cookie,
        )
        login_success = await speaker_auth.login()

        if login_success:
            config = get_config()
            client = BilibiliClient(config.bilibili)
            speaker_service = SpeakerService(speaker_auth, client)

            global _speaker_service
            _speaker_service = speaker_service

            # 刷新设备列表
            devices = await speaker_service.manager.refresh_devices()

            # 保存登录信息到配置文件
            config.xiaomi.account = req.account
            config.xiaomi.password = req.password
            config.xiaomi.cookie = req.cookie
            config.xiaomi.enable = True

            return {
                "code": 0,
                "message": "登录成功",
                "data": {
                    "device_count": len(devices),
                    "account": req.account or "Cookie登录",
                    "devices": [d.model_dump() for d in devices],
                }
            }
        else:
            return {
                "code": 401,
                "message": "登录失败，请检查账号密码或 Cookie",
                "data": None
            }
    except Exception as e:
        logger.error(f"Speaker login error: {e}")
        return {
            "code": 500,
            "message": f"登录出错: {str(e)}",
            "data": None
        }


@router.post("/logout")
async def logout():
    """退出登录"""
    global _speaker_service
    if _speaker_service:
        await _speaker_service.auth.close()
        _speaker_service = None
    return {"code": 0, "message": "已退出登录"}


@router.get("/devices")
async def get_devices():
    """获取设备列表"""
    if _speaker_service is None:
        return {"code": 0, "message": "success", "data": []}
    service = get_speaker_service()
    devices = await service.get_devices()
    return {"code": 0, "message": "success", "data": [d.model_dump() for d in devices]}


@router.post("/play")
async def play(req: PlayRequest):
    """播放音乐到指定设备"""
    service = get_speaker_service()
    return await service.play(req)


@router.post("/control")
async def control(req: ControlRequest):
    """控制设备（暂停/停止/音量）"""
    service = get_speaker_service()
    return await service.control(req)


@router.get("/volume/{did}")
async def get_volume(did: str):
    """获取设备音量"""
    service = get_speaker_service()
    volume = await service.manager.get_volume(did)
    if volume is None:
        return {"code": 500, "message": "获取音量失败"}
    return {"code": 0, "message": "success", "data": {"volume": volume}}


@router.post("/volume")
async def set_volume(req: VolumeRequest):
    """设置设备音量"""
    service = get_speaker_service()
    result = await service.manager.set_volume(req.did, req.volume)
    if result is None:
        return {"code": 500, "message": "设置音量失败"}
    return {"code": 0, "message": "success", "data": {"volume": req.volume}}


@router.get("/player_status/{did}")
async def get_player_status(did: str):
    """获取设备播放状态"""
    service = get_speaker_service()
    status = await service.manager.get_status(did)
    if status is None:
        return {"code": 500, "message": "获取状态失败"}
    return {"code": 0, "message": "success", "data": status}


@router.get("/refresh")
async def refresh_devices():
    """刷新设备列表"""
    if _speaker_service is None:
        return {"code": 401, "message": "未登录", "data": []}
    service = get_speaker_service()
    devices = await service.manager.refresh_devices()
    return {
        "code": 0,
        "message": "success",
        "data": [d.model_dump() for d in devices],
    }


# ========== 二维码登录相关 ==========
_qr_login_manager = None


@router.post("/qr/generate")
async def generate_qr_code():
    """生成二维码"""
    global _qr_login_manager, _speaker_service
    from app.speaker.qrcode_login import MiQRCodeLogin
    from app.config import get_config
    from app.bilibili.client import BilibiliClient
    from app.service.speaker_service import SpeakerService
    from loguru import logger

    try:
        _qr_login_manager = MiQRCodeLogin(conf_path="data")

        # 如果已经登录，直接返回
        if _qr_login_manager.is_logged_in:
            # 使用已有的 cookie 初始化 speaker service
            cookie = _qr_login_manager.get_cookie_string()
            if cookie and _speaker_service is None:
                speaker_auth = SpeakerAuth(cookie=cookie)
                if await speaker_auth.login():
                    config = get_config()
                    client = BilibiliClient(config.bilibili)
                    _speaker_service = SpeakerService(speaker_auth, client)
                    await _speaker_service.manager.refresh_devices()

            return {
                "code": 0,
                "message": "已登录",
                "data": {
                    "is_logged_in": True,
                    "user_id": _qr_login_manager.get_user_id(),
                    "qr_image": "",
                    "status_url": "",
                }
            }

        # 生成二维码
        result = await _qr_login_manager.generate_qr_code()
        return {
            "code": 0,
            "message": result["message"],
            "data": {
                "is_logged_in": False,
                "qr_image": result.get("qr_image", ""),
                "qr_url": result.get("qr_url", ""),
                "status_url": result.get("status_url", ""),
            }
        }
    except Exception as e:
        logger.error(f"Generate QR code error: {e}")
        return {
            "code": 500,
            "message": f"生成二维码失败: {str(e)}",
            "data": None
        }


@router.get("/qr/status")
async def check_qr_status():
    """检查二维码登录状态"""
    global _qr_login_manager, _speaker_service
    from app.config import get_config
    from app.bilibili.client import BilibiliClient
    from app.service.speaker_service import SpeakerService
    from loguru import logger

    if _qr_login_manager is None:
        return {
            "code": 400,
            "message": "请先生成二维码",
            "data": {"status": "idle"}
        }

    try:
        result = await _qr_login_manager.check_login_status()

        # 如果登录成功，初始化 speaker service
        if result["status"] == "success" and _speaker_service is None:
            cookie = _qr_login_manager.get_cookie_string()
            if cookie:
                speaker_auth = SpeakerAuth(cookie=cookie)
                if await speaker_auth.login():
                    config = get_config()
                    client = BilibiliClient(config.bilibili)
                    _speaker_service = SpeakerService(speaker_auth, client)
                    devices = await _speaker_service.manager.refresh_devices()
                    result["device_count"] = len(devices)

        return {
            "code": 0,
            "message": result["message"],
            "data": result
        }
    except Exception as e:
        logger.error(f"Check QR status error: {e}")
        return {
            "code": 500,
            "message": f"检查状态失败: {str(e)}",
            "data": {"status": "failed", "message": str(e)}
        }


@router.post("/qr/reset")
async def reset_qr_login():
    """重置二维码登录状态"""
    global _qr_login_manager
    if _qr_login_manager:
        _qr_login_manager.reset()
        await _qr_login_manager.close()
        _qr_login_manager = None
    return {"code": 0, "message": "已重置"}
