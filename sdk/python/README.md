# fuze_ml

Fuze AI PaaS 平台的 Python SDK，用于训练脚本中快速接入**实验跟踪**能力。
**零第三方依赖**（仅用标准库），可直接 `pip install` 或源码引入。

## 安装

```bash
pip install -e .
```

## 认证

SDK 使用 Personal Access Token（PAT）向后端认证：

1. 在平台控制台或通过 API 签发 PAT：
   ```bash
   curl -X POST https://<fuze>/api/v1/auth/tokens \
     -H "Authorization: Bearer <你的登录token>" \
     -d '{"name":"my-training"}'
   # 返回 {"id":..., "token":"fuze_xxxx....", "prefix":"fuze_xxxx"}
   # token 仅返回一次，请妥善保存。
   ```
2. 训练脚本通过环境变量读取：

```python
import os
from fuze_ml import FuzeExperimentClient

client = FuzeExperimentClient(
    base_url=os.environ["FUZE_BASE_URL"],
    token=os.environ["FUZE_TOKEN"],   # PAT
)
```

## 快速上手

```python
import os
from fuze_ml import FuzeExperimentClient

client = FuzeExperimentClient(
    base_url=os.environ["FUZE_BASE_URL"],
    token=os.environ["FUZE_TOKEN"],
)

# 1) 初始化实验
exp = client.init_experiment(
    name="resnet-tune",
    objective="maximize",   # 或 "minimize"
    metric_name="accuracy",
)

# 2) 创建一次运行
run = exp.create_run(hyperparameters={"lr": 0.01, "batch_size": 64})

# 3) 训练循环中记录指标（本地暂存）
for step, acc in train_loop():
    run.log_metric("accuracy", acc, step=step)

# 4) 完成运行（指标随 complete 上报；最优指标由平台按 objective 自动遴选）
run.complete(metric_value=best_acc, metrics={"loss": best_loss})

# 失败 / 取消
# run.fail(reason="diverged")
# run.cancel()
```

## API

| 方法 | 说明 |
| --- | --- |
| `FuzeExperimentClient(base_url, token)` | 构造客户端（缺 token 抛 `AuthError`） |
| `init_experiment(name, objective, metric_name, ...)` | 创建实验，返回 `Experiment` |
| `get_experiment(experiment_id)` | 获取实验详情 |
| `Experiment.create_run(name?, hyperparameters?)` | 创建运行，返回 `Run` |
| `Run.log_metric(name, value, step?)` | 累积指标（本地） |
| `Run.complete(metric_value?, metrics?)` | 完成运行并上报指标 |
| `Run.fail(reason?)` / `Run.cancel()` | 标记失败 / 取消 |

## 测试

```bash
python -m unittest discover -s tests -p "test_*.py"
```

> 注：`log_metric` 在 2-B 里程碑内为本地暂存，随 `complete`/`fail` 通过实验 API 的
> `metrics` 字段上报；Prometheus 远端推送（Pushgateway）为后续增强。
