# Fuze AI PaaS

[English](README.md)

面向企业的 **AI / LLM 平台即服务**，基于 Go（Gin）+ React 构建。统一提供 LLM 网关与治理（LLMOps）、
AI Agent 编排、模型训练与优化（HPO/NAS、推理、压缩）、实验跟踪与复现、评估、数据标注/ETL，以及带漂移检测
与自动回滚的边缘部署，并以统一身份（SSO/MFA/Passkey/RBAC/审计/租户配额）收口安全。

> 本文档依据仓库实际实现整理。若与代码不一致，以代码为准。

---

## 功能特性

- **LLMOps 与治理** — 多供应商接入（OpenAI / Anthropic / 自建），OpenAI 兼容的 `/v1/chat/completions` 与 `/llm/chat`，工具调用、微调适配器（LoRA/QLoRA）、路由表、Token 计量与成本、价格管理、调用链路 Trace、提示词工程、RAG 知识库、安全护栏（Guardrail）。
- **AI Agent** — Agent DAG 定义 / 编译 / 运行，工具注册表（tool_call 节点）。
- **训练与优化** — 分布式训练作业、超参优化与神经架构搜索（AutoML/NAS）、推理服务（声明式 spec + reconcile）、模型压缩（量化/剪枝/蒸馏，TensorRT/ONNX/OpenVINO）。
- **实验与复现** — 实验跟踪、运行记录、对比、可复现实验、评估（人工 & LLM-as-judge）。
- **数据与标注** — Fluid 数据集、数据处理管线（标注 / 清洗 / 增强 / ETL，由 `data-operator` 执行）。
- **边缘部署** — 运行时 `mock` / `agent` / `kubeedge`；基于真实标注反馈的概念漂移检测；金丝雀发布与自动回滚。
- **统一安全** — IdP 注册表（OIDC / LDAP / SAML）、SSO、MFA、Passkey（WebAuthn）、RBAC、全量审计、租户与配额管理、限流、SSRF 防护、严格 CORS。

---

## 架构

六边形 / DDD。领域包位于 `backend/internal/domain`（18 个），应用服务位于 `backend/internal/app`（16 个），
适配器位于 `backend/internal/storage` 与 `backend/internal/k8s`。完整设计见
[docs/architecture/architecture_CN.md](docs/architecture/architecture_CN.md)。

```
backend/                Go + Gin 后端
  cmd/main.go           API 服务入口
  cmd/data-operator/    数据处理算子（K8s/Volcano Job；读取 FUZE_DATA_SPEC）
  internal/
    domain/<subdomain>  实体 + 端口 + 领域服务（agent, cluster, data, edge,
                        evaluation, event, experiment, gpu, hpo, inference, job, lineage,
                        llm, model, optimize, reproduction, training, workspace）
    app/<subdomain>     防腐层应用服务（agent, cluster, data, dataset,
                        edge, evaluation, experiment, hpo, inference, job, llmgateway,
                        metrics, optimize, token, training, workspace）
    api/                Gin handler（handler.go / routes.go）+ 中间件
    auth/ crypto/       身份与安全
    ports/ storage/     端口接口与 *Storage 适配器
    bootstrap/ config/  装配与配置
frontend/               React 18 + Vite + Tailwind
  src/pages/            24 个页面（见架构文档）
  src/App.jsx           路由表
sdk/python/fuze_ml/     Python SDK
docs/                   架构文档（中文/英文）+ 部署指南
k8s/                    Kubernetes 清单（fuze-system 命名空间）
scripts/                构建 / 运行 / 部署辅助脚本
```

### 后端 API 路由组（节选，除特别说明外均在 `/api/v1` 下）
- 鉴权：`/auth/login`、`/auth/me`、`/auth/mfa/*`、`/auth/passkey/*`、`/auth/tokens*`、`/auth/sso*`
- LLMOps：`/llm/routes`、`/llm/quota`、`/llm/usage*`、`/llm/prices/*`、`/cost/summary`、`/llm/traces*`、`/llm/prompts*`、`/llm/guardrail/rules`、`/llm/knowledge*`、`/llm/finetune/adapters*`、`/llm/chat`
- Agent 与工具：`/agents*`、`/tools*`
- 训练与实验：`/training-jobs*`、`/training-templates`、`/experiments*`、`/evaluations*`
- AutoML/NAS：`/automl*`
- 模型与数据：`/models*`、`/datasets*`、`/data/pipelines*`、`/data/annotations*`
- 推理与压缩：`/inference-services*`、`/optimize/tasks*`
- 边缘：`/edge*`
- 工作空间：`/workspaces*`
- 多集群 / 资源 / 告警 / 监控：`/clusters*`、`/resources*`、`/alerts*`、`/metrics*`、`/queues`
- 企业底座：`/tenants*`、`/quotas*`、`/audit`、`/sso/idps*`
- OpenAI 兼容：`POST /v1/chat/completions`
- Prometheus：`GET /metrics`

> 部分后端能力（模型压缩 `/optimize/tasks`、数据处理 `/data/*`、部分高级 LLM 管理子页）已通过 API 暴露，
> 但对应前端页面分阶段落地。未装配的 repo 优雅降级为 `501`。

---

## 快速开始

### 前置依赖
- Go 1.21+
- Node.js 18+
- Docker 与 Docker Compose（或自备 PostgreSQL 实例）

### 使用 Docker Compose 运行
```bash
docker compose up -d
# 后端 :8080，前端 :3000
```

### 本地运行
```bash
# 1. 后端（API 服务）
go run ./backend/cmd/main.go

# 2. 前端
cd frontend && npm install && npm run dev

# 3. （可选）数据处理算子 —— 由 K8s/Volcano Job 启动，
#    读取 FUZE_DATA_SPEC（JSON）描述 operator/params/input/output。
FUZE_DATA_SPEC='{"operator":"label","input":"...","output":"..."}' \
  go run ./backend/cmd/data-operator
```

数据库 Schema 在启动时通过 `storage.Migrate`（GORM `AutoMigrate`）自动建表；SQLite 下初始管理员
账号/密码由 `ADMIN_PASSWORD` 种子化。

### 关键环境变量
| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AUTH_ENABLED` | `false` | `true` 时全链路鉴权（开发态注入管理员主体） |
| `DB_DRIVER` | `sqlite` | `sqlite` / `postgres` |
| `DB_DSN` | — | PostgreSQL 连接串（driver=postgres） |
| `DB_PATH` | `./fuze-ai-paas.db` | SQLite 文件路径 |
| `ADMIN_PASSWORD` | — | 初始管理员密码（sqlite 种子） |
| `AUTH_SECRET` | — | 令牌签名密钥 |
| `EDGE_RUNTIME` | `mock` | `mock` / `agent` / `kubeedge` |
| `EDGE_CLOUDHUB_URL` / `EDGE_CLOUDHUB_TOKEN` / `EDGE_CLOUDHUB_NAMESPACE` / `EDGE_CLOUDHUB_CA` | — | KubeEdge CloudHub 连接参数 |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | — | LLM 供应商配置（未配置优雅降级） |
| `HPO_GATEWAY_BASE_URL` | — | AutoML/NAS 网关地址 |
| `FUZE_DATA_SPEC` | — | `data-operator` 任务描述（JSON） |

---

## 测试
```bash
# 后端（TDD，*_test.go 随源码）
go test ./...

# 前端
cd frontend && npm test
```

---

## 部署
- **Docker Compose**：`docker-compose.yml`。
- **Kubernetes**：`k8s/` 下清单（命名空间 `fuze-system`）；详见
  [docs/deployment/k8s-deployment-guide_CN.md](docs/deployment/k8s-deployment-guide_CN.md)。
- **边缘（KubeEdge）**：运行时 `EDGE_RUNTIME=kubeedge` 并配置 CloudHub 参数。
- **排障**：[docs/references/k8s-deploy-troubleshooting.md](docs/references/k8s-deploy-troubleshooting.md)。

---

## SDK
Python SDK 位于 `sdk/python/fuze_ml`（通过 `pip install -e sdk/python` 安装）。

---

## 目录结构
```
.
├── backend/        Go 后端（cmd, internal/{domain,app,api,auth,storage,...}）
├── frontend/       React 前端（src/pages, src/App.jsx）
├── sdk/            语言 SDK（python/fuze_ml）
├── k8s/            Kubernetes 清单
├── docs/           架构文档（中文/英文）+ 部署指南
├── scripts/        辅助 shell 脚本
├── docker-compose.yml
├── Makefile
├── README.md / README_CN.md
```

---

## 文档
- 架构设计（中文）：[docs/architecture/architecture_CN.md](docs/architecture/architecture_CN.md)
- 架构设计（英文）：[docs/architecture/architecture_EN.md](docs/architecture/architecture_EN.md)
- 部署概览：见 [docs/deployment/k8s-deployment-guide_EN.md](docs/deployment/k8s-deployment-guide_EN.md)（K8s）与 [docs/references/k8s-deploy-troubleshooting.md](docs/references/k8s-deploy-troubleshooting.md)（排障）
- K8s 部署手册（中文/英文）：[docs/deployment/k8s-deployment-guide_CN.md](docs/deployment/k8s-deployment-guide_CN.md) / [k8s-deployment-guide_EN.md](docs/deployment/k8s-deployment-guide_EN.md)

---

*最后更新：2026-08-20。依据撰写时的 `dev` 分支源码整理。*
