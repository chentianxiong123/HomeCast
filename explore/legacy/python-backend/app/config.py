from pydantic_settings import BaseSettings
from pydantic import Field
from pathlib import Path
import yaml


class ServerConfig(BaseSettings):
    host: str = "0.0.0.0"
    port: int = 28974
    # 外部访问地址（用于音箱推送等场景，如 http://192.168.1.100:28974）
    public_url: str = ""
    mode: str = "debug"
    read_timeout: int = 60
    write_timeout: int = 60


class BilibiliConfig(BaseSettings):
    base_url: str = "https://api.bilibili.com"
    user_agent: str = (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/120.0.0.0 Safari/537.36"
    )
    referer: str = "https://search.bilibili.com/"
    timeout: int = 30


class XiaomiConfig(BaseSettings):
    account: str = ""
    password: str = ""
    cookie: str = ""
    enable: bool = False


class FFmpegConfig(BaseSettings):
    path: str = "ffmpeg"
    audio_bitrate: str = "64k"


class CacheConfig(BaseSettings):
    dir: str = "data/audio_cache"
    max_size_mb: int = 500


class SnifferConfig(BaseSettings):
    adblock: bool = False
    headless: bool = True
    browser_timeout: int = 30


class AppConfig(BaseSettings):
    model_config = {"extra": "ignore"}

    server: ServerConfig = Field(default_factory=ServerConfig)
    bilibili: BilibiliConfig = Field(default_factory=BilibiliConfig)
    xiaomi: XiaomiConfig = Field(default_factory=XiaomiConfig)
    ffmpeg: FFmpegConfig = Field(default_factory=FFmpegConfig)
    sniffer: SnifferConfig = Field(default_factory=SnifferConfig)
    cache: CacheConfig = Field(default_factory=CacheConfig)


_config: AppConfig | None = None


def load_config(config_path: str | None = None) -> AppConfig:
    global _config
    data = {}
    if config_path:
        path = Path(config_path)
        if path.exists():
            with open(path, "r", encoding="utf-8") as f:
                data = yaml.safe_load(f) or {}
    _config = AppConfig(**data)
    return _config


def get_config() -> AppConfig:
    global _config
    if _config is None:
        _config = load_config("configs/config.yaml")
    return _config
