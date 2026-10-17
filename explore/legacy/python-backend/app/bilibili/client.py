import httpx
import uuid
from app.config import BilibiliConfig


class BilibiliClient:
    def __init__(self, config: BilibiliConfig):
        self.base_url = config.base_url
        self.user_agent = config.user_agent
        self.referer = config.referer
        self.timeout = config.timeout
        self.buvid3 = str(uuid.uuid4())
        self._client: httpx.AsyncClient | None = None

    async def _get_client(self) -> httpx.AsyncClient:
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                headers=self._default_headers(),
            )
        return self._client

    def _default_headers(self) -> dict:
        return {
            "User-Agent": self.user_agent,
            "Referer": self.referer,
            "Origin": "https://search.bilibili.com",
            "Cookie": f"buvid3={self.buvid3}",
            "Accept": "application/json, text/plain, */*",
            "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        }

    async def get(self, path: str, params: dict | None = None) -> dict:
        client = await self._get_client()
        resp = await client.get(path, params=params)
        resp.raise_for_status()
        data = resp.json()
        if data.get("code") != 0:
            raise BilibiliAPIError(
                code=data.get("code", -1),
                message=data.get("message", "unknown error"),
            )
        return data.get("data", {})

    async def get_raw(self, url: str, headers: dict | None = None) -> httpx.Response:
        raw_headers = self._default_headers()
        if headers:
            raw_headers.update(headers)
        async with httpx.AsyncClient(timeout=self.timeout) as client:
            resp = await client.get(url, headers=raw_headers, follow_redirects=True)
            resp.raise_for_status()
            return resp

    async def close(self):
        if self._client and not self._client.is_closed:
            await self._client.aclose()


class BilibiliAPIError(Exception):
    def __init__(self, code: int, message: str):
        self.code = code
        self.message = message
        super().__init__(f"[{code}] {message}")
