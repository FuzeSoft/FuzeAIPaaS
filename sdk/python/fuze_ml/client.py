"""fuze_ml 底层 HTTP 客户端。

仅依赖标准库（urllib），无第三方包。测试中可通过替换 ``urlopen``
（``fuze_ml.client.urlopen``）来桩住网络。
"""

import json
from typing import Any, Dict, Optional
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

from .errors import APIError, AuthError

urlopen = urlopen

MAX_RESPONSE_BYTES = 64 * 1024 * 1024

MIN_TIMEOUT = 5

class FuzeClient:
    """Fuze AI PaaS 平台客户端。

    Args:
        base_url: 平台 API 根地址，例如 ``https://fuze.example.com``。
        token: Personal Access Token（PAT）。缺失时抛出 AuthError。
        timeout: 单次请求超时（秒），低于 MIN_TIMEOUT 时取 MIN_TIMEOUT。
    """

    def __init__(self, base_url: str, token: Optional[str] = None, timeout: int = 30):
        if not token:
            raise AuthError("FuzeClient requires a Personal Access Token (token=...)")
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = max(int(timeout), MIN_TIMEOUT) if timeout else MIN_TIMEOUT

    def _headers(self) -> Dict[str, str]:
        return {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }

    def _url(self, path: str) -> str:
        path = path.lstrip("/")
        return f"{self.base_url}/api/v1/{path}"

    def request(self, method: str, path: str, body: Optional[Any] = None) -> Any:
        """发起请求并返回解析后的 JSON（或无内容时返回 None）。"""
        data = None
        if body is not None:
            data = json.dumps(body).encode("utf-8")
        req = Request(self._url(path), data=data, headers=self._headers(), method=method)
        try:
            with urlopen(req, timeout=self.timeout) as resp:
                
                raw = resp.read(MAX_RESPONSE_BYTES + 1)
                if len(raw) > MAX_RESPONSE_BYTES:
                    raise APIError(413, "response body exceeds SDK read limit; not parsed")
        except HTTPError as e:
            detail = e.read().decode("utf-8", "replace") if e.fp else ""
            raise APIError(e.code, detail)
        except URLError as e:
            
            reason = e.reason if isinstance(e.reason, str) else str(getattr(e, "reason", e))
            raise APIError(0, reason)
        if not raw:
            return None
        return json.loads(raw)