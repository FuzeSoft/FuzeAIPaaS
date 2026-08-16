"""fuze_ml 客户端测试：桩住 urllib 网络调用。"""

import io
import json
import unittest
from unittest.mock import patch

from fuze_ml import FuzeExperimentClient, AuthError, APIError

def fake_response(status, payload):
    body = json.dumps(payload).encode("utf-8") if payload is not None else b""
    return io.BytesIO(body)

class TestClientAuth(unittest.TestCase):
    def test_requires_token(self):
        with self.assertRaises(AuthError):
            FuzeExperimentClient(base_url="http://x")

class TestExperimentFlow(unittest.TestCase):
    def setUp(self):
        self.calls = []

    def _urlopen(self, req, timeout=None):
        self.calls.append((req.get_method(), req.full_url, req.data))
        method = req.get_method()
        url = req.full_url
        if method == "POST" and url.endswith("/api/v1/experiments"):
            return fake_response(200, {"id": "exp-1", "name": "tune"})
        if method == "POST" and "/runs" in url and url.endswith("/runs"):
            return fake_response(200, {"id": "run-1"})
        if method == "POST" and "/complete" in url:
            return fake_response(200, {"id": "run-1", "status": "completed"})
        if method == "POST" and "/fail" in url:
            return fake_response(200, {"id": "run-1", "status": "failed"})
        if method == "GET":
            return fake_response(200, {"experiment": {"id": "exp-1"}, "runs": []})
        return fake_response(200, {})

    def _client(self):
        return FuzeExperimentClient(base_url="http://fuze.test", token="fuze_testtoken")

    def test_init_experiment(self):
        with patch("fuze_ml.client.urlopen", self._urlopen):
            c = self._client()
            exp = c.init_experiment(name="tune", objective="maximize", metric_name="acc")
        self.assertEqual(exp.id, "exp-1")
        self.assertEqual(self.calls[0][0], "POST")
        self.assertTrue(self.calls[0][1].endswith("/api/v1/experiments"))

    def test_create_run_and_log_and_complete(self):
        with patch("fuze_ml.client.urlopen", self._urlopen):
            c = self._client()
            exp = c.init_experiment(name="tune")
            run = exp.create_run(hyperparameters={"lr": 0.01})
            run.log_metric("acc", 0.9, step=1)
            run.log_metric("acc", 0.95, step=2)
            run.complete(metric_value=0.95, metrics={"loss": 0.1})
        
        self.assertTrue(self.calls[1][1].endswith("/api/v1/experiments/exp-1/runs"))
        
        complete_call = self.calls[2]
        self.assertTrue(complete_call[1].endswith("/runs/run-1/complete"))
        body = json.loads(complete_call[2])
        self.assertEqual(body["status"], "completed")
        self.assertEqual(body["metrics"]["acc"], 0.95)
        self.assertEqual(body["metrics"]["loss"], 0.1)

    def test_run_fail(self):
        with patch("fuze_ml.client.urlopen", self._urlopen):
            c = self._client()
            exp = c.init_experiment(name="tune")
            run = exp.create_run()
            run.fail(reason="diverged")
        fail_call = self.calls[2]
        body = json.loads(fail_call[2])
        self.assertEqual(body["status"], "failed")
        self.assertEqual(body["reason"], "diverged")

    def test_api_error_propagates(self):
        def boom(req, timeout=None):
            raise APIError(500, "boom")
        with patch("fuze_ml.client.urlopen", boom):
            c = self._client()
            with self.assertRaises(APIError):
                c.init_experiment(name="tune")

if __name__ == "__main__":
    unittest.main()