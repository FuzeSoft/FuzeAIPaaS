"""AutoML / NAS 高层封装（设计 4.4）。

训练脚本侧的典型用法（由平台侧 Study 调度，脚本只负责上报指标）::

    from fuze_ml import Client
    client = Client(base_url=..., token=...)

    # trial_id / study_id 由训练作业环境变量或平台注入获取。
    trial = client.automl.trial(study_id, trial_id)
    for step, acc in train_loop():
        trial.report_intermediate(step=step, value=acc)
        if should_stop:  # 平台早停信号（见 report_intermediate 返回值）
            break
    trial.report_final(value=best_acc)

Study 的创建与调度在平台侧通过 ``/api/v1/hpo`` 完成；SDK 侧的 ``Study`` 仅作
只读视图与便捷入口。
"""

from typing import Any, Dict, List, Optional

from .client import FuzeClient

class Trial:
    """AutoML/NAS 的一次试验。脚本侧只负责上报指标。"""

    def __init__(self, client: FuzeClient, study_id: str, trial_id: str):
        self._client = client
        self.study_id = study_id
        self.trial_id = trial_id

    def report_intermediate(self, step: int, value: float) -> bool:
        """上报一次中间指标。返回 bool：平台是否建议停止（ASHA 早停）。"""
        resp = self._client.request(
            "POST",
            f"hpo/{self.study_id}/trials/{self.trial_id}/report",
            {"step": step, "value": value},
        )
        if isinstance(resp, dict):
            return bool(resp.get("stop", False))
        return False

    def report_final(self, value: float) -> Dict[str, Any]:
        """上报最终目标值（训练脚本收尾调用）。"""
        return self._client.request(
            "POST",
            f"hpo/{self.study_id}/trials/{self.trial_id}/final",
            {"step": 0, "value": value},
        )

class Study:
    """AutoML/NAS 研究（只读视图 + 便捷入口）。"""

    def __init__(self, client: FuzeClient, study_id: str, raw: Dict[str, Any]):
        self._client = client
        self.id = study_id
        self.raw = raw

    def trial(self, trial_id: str) -> Trial:
        return Trial(self._client, self.id, trial_id)

    def get_trials(self) -> List[Dict[str, Any]]:
        return self._client.request("GET", f"hpo/{self.id}/trials") or []

    def run(self) -> Dict[str, Any]:
        """手动触发一轮调度（拉起/停止/完成 trial）。"""
        return self._client.request("POST", f"hpo/{self.id}/run", {})

class FuzeAutoMLClient(FuzeClient):
    """附带 AutoML/NAS 能力的 Fuze 客户端。"""

    @property
    def automl(self) -> "AutoMLView":
        return AutoMLView(self)

    def create_study(
        self,
        name: str,
        objective_metric: str,
        search_space: List[Dict[str, Any]],
        objective_direction: str = "maximize",
        algorithm: str = "tpe",
        max_trials: int = 20,
        max_parallel: int = 2,
        max_duration_sec: int = 0,
        early_stop: Optional[Dict[str, Any]] = None,
        training_template: Optional[Dict[str, Any]] = None,
        experiment_id: Optional[str] = None,
    ) -> Study:
        body: Dict[str, Any] = {
            "name": name,
            "objective_metric": objective_metric,
            "objective_direction": objective_direction,
            "search_space": search_space,
            "algorithm": algorithm,
            "max_trials": max_trials,
            "max_parallel": max_parallel,
        }
        if max_duration_sec:
            body["max_duration_sec"] = max_duration_sec
        if early_stop:
            body["early_stop"] = early_stop
        if training_template:
            body["training_template"] = training_template
        if experiment_id:
            body["experiment_id"] = experiment_id
        resp = self.request("POST", "hpo", body)
        study_id = resp.get("id")
        if not study_id:
            raise ValueError(f"create_study: missing id in response {resp!r}")
        return Study(self, study_id, resp)

    def get_study(self, study_id: str) -> Study:
        resp = self.request("GET", f"hpo/{study_id}")
        return Study(self, study_id, resp)

    def list_studies(self) -> List[Dict[str, Any]]:
        return self.request("GET", "hpo") or []

class AutoMLView:
    """通过 ``client.automl`` 访问 AutoML 子域的便捷视图。"""

    def __init__(self, client: FuzeClient):
        self._client = client

    def create_study(self, *args, **kwargs) -> Study:
        return FuzeAutoMLClient.create_study(self._client, *args, **kwargs)

    def get_study(self, study_id: str) -> Study:
        return FuzeAutoMLClient.get_study(self._client, study_id)

    def list_studies(self) -> List[Dict[str, Any]]:
        return FuzeAutoMLClient.list_studies(self._client)

    def trial(self, study_id: str, trial_id: str) -> Trial:
        return Trial(self._client, study_id, trial_id)