# Fuze AI PaaS

[中文](README_CN.md)

An enterprise **AI / LLM Platform-as-a-Service** built with Go (Gin) + React. It unifies LLM
gateway & governance (LLMOps), AI Agent orchestration, model training & optimization (HPO/NAS,
inference, compression), experiment tracking & reproduction, evaluation, data labeling/ETL, and
edge deployment with drift detection & auto-rollback, all behind unified identity
(SSO/MFA/Passkey/RBAC/audit/tenant-quota).

> This README reflects the actual repository implementation. Where it diverges from the code, the code wins.

---

## Features

- **LLMOps & Governance** — multi-vendor LLM access (OpenAI / Anthropic / self-hosted), OpenAI-compatible `/v1/chat/completions` and `/llm/chat`, tool calling, fine-tune adapters (LoRA/QLoRA), routing table, token metering & cost, price management, call Trace, prompt engineering, RAG knowledge base, guardrails.
- **AI Agents** — Agent DAG definition / compile / run, tool registry (tool_call nodes).
- **Training & Optimization** — distributed training jobs, hyperparameter optimization & NAS (AutoML), inference services (declarative spec + reconcile), model compression (quantization/pruning/distillation, TensorRT/ONNX/OpenVINO).
- **Experiments & Reproduction** — experiment tracking, runs, compare, reproducible experiments, evaluation (human & LLM-as-judge).
- **Data & Labeling** — Fluid datasets, data-processing pipelines (labeling / cleaning / augmentation / ETL run by `data-operator`).
- **Edge Deployment** — runtimes `mock` / `agent` / `kubeedge`; concept-drift detection via real label feedback; canary rollout & auto-rollback.
- **Unified Security** — IdP registry (OIDC / LDAP / SAML), SSO, MFA, Passkey (WebAuthn), RBAC, full audit, tenant & quota management, rate limiting, SSRF guard, strict CORS.

---

## Architecture

Hexagonal / DDD. Domain packages under `backend/internal/domain` (18), application services under
`backend/internal/app` (16), adapters under `backend/internal/storage` and `backend/internal/k8s`.
Full design: [docs/architecture/architecture_EN.md](docs/architecture/architecture_EN.md).

```
backend/                Go + Gin backend
  cmd/main.go           API server entrypoint
  cmd/data-operator/    Data-processing operator (K8s/Volcano Job; reads FUZE_DATA_SPEC)
  internal/
    domain/<subdomain>  entities + ports + domain service (agent, cluster, data, edge,
                        evaluation, event, experiment, gpu, hpo, inference, job, lineage,
                        llm, model, optimize, reproduction, training, workspace)
    app/<subdomain>     anti-corruption application services (agent, cluster, data, dataset,
                        edge, evaluation, experiment, hpo, inference, job, llmgateway,
                        metrics, optimize, token, training, workspace)
    api/                Gin handlers (handler.go / routes.go) + middleware
    auth/ crypto/       identity & security
    ports/ storage/     port interfaces & *Storage adapters
    bootstrap/ config/  assembly & configuration
frontend/               React 18 + Vite + Tailwind
  src/pages/            24 pages (see architecture doc)
  src/App.jsx           route table
sdk/python/fuze_ml/     Python SDK
docs/                   architecture (CN/EN) + deployment guides
k8s/                    Kubernetes manifests (fuze-system namespace)
scripts/                build / run / deploy helpers
```

### Backend API route groups (excerpt, all under `/api/v1` unless noted)
- Auth: `/auth/login`, `/auth/me`, `/auth/mfa/*`, `/auth/passkey/*`, `/auth/tokens*`, `/auth/sso*`
- LLMOps: `/llm/routes`, `/llm/quota`, `/llm/usage*`, `/llm/prices/*`, `/cost/summary`, `/llm/traces*`, `/llm/prompts*`, `/llm/guardrail/rules`, `/llm/knowledge*`, `/llm/finetune/adapters*`, `/llm/chat`
- Agents & tools: `/agents*`, `/tools*`
- Training & experiments: `/training-jobs*`, `/training-templates`, `/experiments*`, `/evaluations*`
- AutoML/NAS: `/automl*`
- Models & data: `/models*`, `/datasets*`, `/data/pipelines*`, `/data/annotations*`
- Inference & optimize: `/inference-services*`, `/optimize/tasks*`
- Edge: `/edge*`
- Workspaces: `/workspaces*`
- Multi-cluster / resources / alerts / monitoring: `/clusters*`, `/resources*`, `/alerts*`, `/metrics*`, `/queues`
- Enterprise base: `/tenants*`, `/quotas*`, `/audit`, `/sso/idps*`
- OpenAI-compatible: `POST /v1/chat/completions`
- Prometheus: `GET /metrics`

> Some backend capabilities (model compression `/optimize/tasks`, data processing `/data/*`, a few
> advanced LLM admin sub-pages) are API-exposed but their dedicated frontend pages are delivered
> incrementally. Unassembled repos degrade to `501`.

---

## Quick Start

### Prerequisites
- Go 1.21+
- Node.js 18+
- Docker & Docker Compose (or a PostgreSQL instance)

### Run with Docker Compose
```bash
docker compose up -d
# backend on :8080, frontend on :3000
```

### Run locally
```bash
# 1. Backend (API server)
go run ./backend/cmd/main.go

# 2. Frontend
cd frontend && npm install && npm run dev

# 3. (Optional) Data-processing operator — launched by a K8s/Volcano Job,
#    reads FUZE_DATA_SPEC (JSON) describing the operator/params/input/output.
FUZE_DATA_SPEC='{"operator":"label","input":"...","output":"..."}' \
  go run ./backend/cmd/data-operator
```

Database schema is created automatically on startup via `storage.Migrate` (GORM `AutoMigrate`).
For SQLite the initial admin user/password is seeded from `ADMIN_PASSWORD`.

### Key environment variables
| Variable | Default | Description |
| --- | --- | --- |
| `AUTH_ENABLED` | `false` | `true` enforces full auth (dev injects admin principal) |
| `DB_DRIVER` | `sqlite` | `sqlite` / `postgres` |
| `DB_DSN` | — | PostgreSQL connection string (driver=postgres) |
| `DB_PATH` | `./fuze-ai-paas.db` | SQLite file path |
| `ADMIN_PASSWORD` | — | Initial admin password (sqlite seed) |
| `AUTH_SECRET` | — | Token signing secret |
| `EDGE_RUNTIME` | `mock` | `mock` / `agent` / `kubeedge` |
| `EDGE_CLOUDHUB_URL` / `EDGE_CLOUDHUB_TOKEN` / `EDGE_CLOUDHUB_NAMESPACE` / `EDGE_CLOUDHUB_CA` | — | KubeEdge CloudHub connection |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | — | LLM vendor config (graceful degradation) |
| `HPO_GATEWAY_BASE_URL` | — | AutoML/NAS gateway URL |
| `FUZE_DATA_SPEC` | — | `data-operator` task spec (JSON) |

---

## Testing
```bash
# Backend (TDD, *_test.go beside source)
go test ./...

# Frontend
cd frontend && npm test
```

---

## Deployment
- **Docker Compose**: `docker-compose.yml`.
- **Kubernetes**: manifests under `k8s/` (namespace `fuze-system`); see
  [docs/deployment/k8s-deployment-guide_EN.md](docs/deployment/k8s-deployment-guide_EN.md).
- **Edge (KubeEdge)**: runtime `EDGE_RUNTIME=kubeedge` with CloudHub params.
- **Troubleshooting**: [docs/references/k8s-deploy-troubleshooting.md](docs/references/k8s-deploy-troubleshooting.md).

---

## SDK
Python SDK under `sdk/python/fuze_ml` (install via `pip install -e sdk/python`).

---

## Project Layout
```
.
├── backend/        Go backend (cmd, internal/{domain,app,api,auth,storage,...})
├── frontend/       React frontend (src/pages, src/App.jsx)
├── sdk/            Language SDKs (python/fuze_ml)
├── k8s/            Kubernetes manifests
├── docs/           architecture (CN/EN) + deployment guides
├── scripts/        helper shell scripts
├── docker-compose.yml
├── Makefile
├── README.md / README_CN.md
```

---

## Documentation
- Architecture (CN): [docs/architecture/architecture_CN.md](docs/architecture/architecture_CN.md)
- Architecture (EN): [docs/architecture/architecture_EN.md](docs/architecture/architecture_EN.md)
- Deployment overview: see [docs/deployment/k8s-deployment-guide_EN.md](docs/deployment/k8s-deployment-guide_EN.md) (K8s) and [docs/references/k8s-deploy-troubleshooting.md](docs/references/k8s-deploy-troubleshooting.md) (troubleshooting)
- K8s guide (CN/EN): [docs/deployment/k8s-deployment-guide_CN.md](docs/deployment/k8s-deployment-guide_CN.md) / [k8s-deployment-guide_EN.md](docs/deployment/k8s-deployment-guide_EN.md)

---

*Last updated: 2026-08-20. Generated from the `dev` branch source at the time of writing.*
