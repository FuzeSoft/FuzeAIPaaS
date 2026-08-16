"""数据处理子域高层封装（标注 / 清洗 / 增强 / ETL）。

典型用法::

    from fuze_ml import FuzeClient
    client = FuzeClient(base_url="https://fuze.example.com", token=TOKEN)

    pipe = client.data.create_pipeline(
        name="clean-etl",
        dataset_name="ds-a",
        steps=[
            {"name": "dedup", "kind": "clean", "operator": "dedup",
             "input_path": "raw", "output_path": "clean", "params": {"method": "exact"}},
            {"name": "export", "kind": "etl", "operator": "format_convert",
             "depends_on": ["<step-id-of-dedup>"], "input_path": "clean",
             "output_path": "out", "params": {"from": "jsonl", "to": "csv"}},
        ],
    )
    client.data.submit_pipeline(pipe["id"])

    ann = client.data.create_annotation(name="label-cats", dataset_name="ds-a",
                                        task_type="image-detection",
                                        categories=["cat", "dog"], output_format="coco")
    client.data.export_annotation(ann["id"], src_format="jsonl",
                                  input_path="/mnt/data/labels.jsonl",
                                  output_path="/mnt/data/export/coco.json")
"""

from typing import Any, Dict, List, Optional

from .client import FuzeClient

class DataPipelineClient:
    """数据处理流水线（DAG 编排）客户端。"""

    def __init__(self, client: FuzeClient):
        self._client = client

    def create_pipeline(self, name: str, dataset_name: str,
                        steps: List[Dict[str, Any]],
                        description: str = "", mount_path: str = "/mnt/data",
                        priority: int = 0, queue_name: str = "",
                        cluster_id: str = "") -> Dict[str, Any]:
        """创建流水线（含步骤定义）。steps 中 kind ∈ clean/augment/etl/annotation。"""
        body: Dict[str, Any] = {
            "name": name,
            "description": description,
            "dataset_name": dataset_name,
            "mount_path": mount_path,
            "priority": priority,
            "queue_name": queue_name,
            "cluster_id": cluster_id,
            "steps": steps,
        }
        return self._client.request("POST", "data/pipelines", body)

    def list_pipelines(self) -> Dict[str, Any]:
        return self._client.request("GET", "data/pipelines")

    def get_pipeline(self, pipeline_id: str) -> Dict[str, Any]:
        return self._client.request("GET", f"data/pipelines/{pipeline_id}")

    def submit_pipeline(self, pipeline_id: str) -> Dict[str, Any]:
        return self._client.request("POST", f"data/pipelines/{pipeline_id}/submit")

    def cancel_pipeline(self, pipeline_id: str) -> Dict[str, Any]:
        return self._client.request("POST", f"data/pipelines/{pipeline_id}/cancel")

class AnnotationClient:
    """数据标注任务客户端。"""

    def __init__(self, client: FuzeClient):
        self._client = client

    def create_annotation(self, name: str, dataset_name: str,
                          task_type: str, categories: List[str],
                          data_glob: str = "", assignee: str = "",
                          output_format: str = "coco") -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "name": name,
            "dataset_name": dataset_name,
            "data_glob": data_glob,
            "task_type": task_type,
            "categories": categories,
            "assignee": assignee,
            "output_format": output_format,
        }
        return self._client.request("POST", "data/annotations", body)

    def list_annotations(self) -> Dict[str, Any]:
        return self._client.request("GET", "data/annotations")

    def get_annotation(self, annotation_id: str) -> Dict[str, Any]:
        return self._client.request("GET", f"data/annotations/{annotation_id}")

    def export_annotation(self, annotation_id: str, src_format: str,
                          input_path: str, output_path: str) -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "src_format": src_format,
            "input_path": input_path,
            "output_path": output_path,
        }
        return self._client.request("POST", f"data/annotations/{annotation_id}/export", body)