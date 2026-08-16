"""fuze_ml — Fuze AI PaaS 训练/实验跟踪 Python SDK。"""

from .client import FuzeClient
from .experiment import Experiment, Run, FuzeExperimentClient
from .automl import (
    FuzeAutoMLClient,
    Study,
    Trial,
    AutoMLView,
)
from .data import DataPipelineClient, AnnotationClient
from .errors import FuzeError, AuthError, APIError
from .version import __version__

class FuzeDataClient(FuzeClient):
    """附带数据处理子域能力的客户端。"""

    @property
    def data(self) -> DataPipelineClient:
        return DataPipelineClient(self)

    @property
    def annotation(self) -> AnnotationClient:
        return AnnotationClient(self)

class Client(FuzeExperimentClient, FuzeAutoMLClient):
    """默认客户端：实验跟踪 + 数据处理 + AutoML/NAS。"""

__all__ = [
    "FuzeClient",
    "FuzeExperimentClient",
    "FuzeAutoMLClient",
    "FuzeDataClient",
    "Client",
    "Experiment",
    "Run",
    "Study",
    "Trial",
    "AutoMLView",
    "DataPipelineClient",
    "AnnotationClient",
    "FuzeError",
    "AuthError",
    "APIError",
    "__version__",
]