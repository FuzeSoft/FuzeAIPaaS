"""实验跟踪高层封装。

典型用法::

    from fuze_ml import FuzeClient
    client = FuzeClient(base_url="https://fuze.example.com", token=TOKEN)
    exp = client.init_experiment(name="resnet-tune", objective="maximize",
                                 metric_name="accuracy")
    run = exp.create_run(hyperparameters={"lr": 0.01})
    for step, acc in train_loop():
        run.log_metric("accuracy", acc, step=step)
    run.complete(metric_value=best_acc, metrics={"loss": best_loss})
"""

from typing import Any, Dict, List, Optional

from .client import FuzeClient

class Run:
    """单次实验运行。指标先累积于本地，随 complete/fail 上报。"""

    def __init__(self, client: FuzeClient, experiment_id: str, run_id: str):
        self._client = client
        self.experiment_id = experiment_id
        self.run_id = run_id
        self._metrics: List[Dict[str, Any]] = []

    def log_metric(self, name: str, value: float, step: Optional[int] = None) -> None:
        """累积一条指标（本地暂存，complete/fail 时随 Run 上报）。"""
        self._metrics.append({"name": name, "value": value, "step": step})

    def _payload_metrics(self) -> Dict[str, Any]:
        
        out: Dict[str, Any] = {}
        for m in self._metrics:
            out[m["name"]] = m["value"]
        return out

    def complete(self, metric_value: Optional[float] = None,
                 metrics: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        final = dict(metrics or {})
        if metric_value is not None:
            final.setdefault("metric_value", metric_value)
        
        final.update(self._payload_metrics())
        body: Dict[str, Any] = {"status": "completed"}
        if final:
            body["metrics"] = final
        return self._client.request(
            "POST",
            f"experiments/{self.experiment_id}/runs/{self.run_id}/complete",
            body,
        )

    def fail(self, reason: Optional[str] = None) -> Dict[str, Any]:
        body: Dict[str, Any] = {"status": "failed"}
        if reason:
            body["reason"] = reason
        return self._client.request(
            "POST",
            f"experiments/{self.experiment_id}/runs/{self.run_id}/fail",
            body,
        )

    def cancel(self) -> Dict[str, Any]:
        return self._client.request(
            "POST",
            f"experiments/{self.experiment_id}/runs/{self.run_id}/cancel",
            {"status": "cancelled"},
        )

class Experiment:
    """实验聚合。"""

    def __init__(self, client: FuzeClient, experiment_id: str, raw: Dict[str, Any]):
        self._client = client
        self.id = experiment_id
        self.raw = raw

    def create_run(self, name: Optional[str] = None,
                   hyperparameters: Optional[Dict[str, Any]] = None) -> Run:
        body: Dict[str, Any] = {}
        if name:
            body["name"] = name
        if hyperparameters:
            body["hyperparameters"] = hyperparameters
        resp = self._client.request(
            "POST", f"experiments/{self.id}/runs", body
        )
        run_id = resp.get("id") or resp.get("run_id")
        if not run_id:
            raise ValueError(f"create_run: missing run id in response {resp!r}")
        return Run(self._client, self.id, run_id)

class FuzeExperimentClient(FuzeClient):
    """带实验跟踪能力的 Fuze 客户端。"""

    def init_experiment(self, name: str, objective: str = "maximize",
                        metric_name: str = "metric",
                        description: str = "",
                        tags: Optional[List[str]] = None) -> Experiment:
        resp = self.request("POST", "experiments", {
            "name": name,
            "objective": objective,
            "metric_name": metric_name,
            "description": description,
            "tags": tags or [],
        })
        exp_id = resp.get("id")
        if not exp_id:
            raise ValueError(f"init_experiment: missing id in response {resp!r}")
        return Experiment(self, exp_id, resp)

    def get_experiment(self, experiment_id: str) -> Experiment:
        resp = self.request("GET", f"experiments/{experiment_id}")
        return Experiment(self, experiment_id, resp)