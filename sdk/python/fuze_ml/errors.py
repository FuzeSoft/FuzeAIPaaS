"""fuze_ml 异常定义。"""

class FuzeError(Exception):
    """SDK 基础异常。"""

class AuthError(FuzeError):
    """认证失败（缺少 token 或令牌被拒绝）。"""

class APIError(FuzeError):
    """服务端返回了非 2xx 状态码。

    Attributes:
        status_code: HTTP 状态码
        body: 响应体文本
    """

    def __init__(self, status_code: int, body: str):
        self.status_code = status_code
        self.body = body
        super().__init__(f"API error {status_code}: {body}")