import asyncio
import hashlib
import time
from loguru import logger


class TokenStore:
    _instance = None
    _tokens: dict[str, dict] = {}

    def __new__(cls):
        if cls._instance is None:
            cls._instance = super().__new__(cls)
        return cls._instance

    def store(self, url: str, metadata: dict | None = None) -> str:
        token = hashlib.md5(f"{url}{time.time()}".encode()).hexdigest()[:12]
        self._tokens[token] = {
            "url": url,
            "metadata": metadata or {},
            "created_at": time.time(),
        }
        logger.debug(f"Token stored: {token} -> {url[:80]}...")
        return token

    def get(self, token: str) -> dict | None:
        data = self._tokens.get(token)
        if data is None:
            return None
        if time.time() - data["created_at"] > 3600:
            del self._tokens[token]
            return None
        return data

    def delete(self, token: str):
        self._tokens.pop(token, None)

    def cleanup(self, max_age: int = 3600):
        now = time.time()
        expired = [t for t, d in self._tokens.items() if now - d["created_at"] > max_age]
        for t in expired:
            del self._tokens[t]


token_store = TokenStore()
