# Fuze AI PaaS Architecture Design Document (English)

> This document is derived from the actual repository implementation and serves as the
> overall architecture reference for the Fuze AI PaaS platform.
> Last updated: 2026-08-20. Where the document and the code diverge, the code is authoritative.

---

## 1. Platform Overview

Fuze AI PaaS is an enterprise **AI / LLM Platform-as-a-Service** that unifies the following for teams:

- **LLM capability & governance (LLMOps)**: multi-vendor gateway (OpenAI / Anthropic / self-hosted), a unified OpenAI-compatible interface (`/v1/chat/completions` and `/llm/chat`), chat / completion / tool-calling, fine-tune adapters (LoRA / QLoRA), routing table, token metering & cost, price management, call Trace, prompt engineering, RAG knowledge base, and guardrails.
- **AI Agent orchestration**: Agent DAG definition / compile / run / human-review resume, plus a tool registry (tool_call nodes).
- **Model training & optimization**: distributed training jobs (replacing the legacy `/jobs`), hyperparameter optimization & neural architecture search (AutoML / NAS), inference services (declarative spec + reconcile convergence), model compression (quantization / pruning / distillation / format conversion).
- **Experiment & reproduction**: experiment tracking, runs, experiment compare, reproducible experiments (Reproduce / Reproduction).
- **Evaluation**: model / Agent evaluation tasks, human & LLM-as-judge review, aggregated report and finalize.
- **Data & labeling**: dataset management (Fluid datasets), data-processing pipelines (labeling / cleaning / augmentation / ETL executed by `data-operator`).
- **Edge deployment & drift defense**: edge runtimes (mock / agent / KubeEdge), concept-drift detection, canary rollout and auto-rollback.
- **Unified identity & security (P4 enterprise base)**: multi-IdP (OIDC / LDAP / SAML) registry, SSO, MFA, Passkey / WebAuthn, RBAC roles, full audit, tenant / quota management, rate limiting, SSRF guard, strict CORS.

The platform follows **Domain-Driven Design (DDD) + Hexagonal architecture (Ports & Adapters)**,
implemented in Go (backend) / React (frontend), with a companion Python SDK.

---

## 2. Tech Stack

| Layer | Technology |
| --- | --- |
| Backend language | Go (Gin web framework; data pipelines may run in K8s/Volcano Jobs) |
| Frontend language | JavaScript (React 18 + Vite) |
| Frontend routing | react-router-dom 6 |
| UI / styling | Tailwind CSS, lucide-react icons, recharts charts, clsx utility |
| Storage | RDBMS (SQLite / PostgreSQL via GORM; `storage.Migrate` uses `AutoMigrate`) |
| Auth | Custom `auth.Manager`: IdP registry + MFA + Passkey + SSO + RBAC + audit |
| Deployment | Docker Compose / Kubernetes (incl. KubeEdge edge) |
| SDK | Python (`sdk/python/fuze_ml`) |
| Tests | `go test`, TDD style, `*_test.go` next to source |

---

## 3. Layered Architecture (Hexagonal / DDD)

```
┌──────────────────────────────────────────────────────────────┐
│                        API Layer (Gin)                         │
│  internal/api: handlers (handler.go/routes.go) + middleware     │
│  auth / RBAC / audit / rate-limit / SSRF / CORS middleware       │
└───────────────┬──────────────────────────────────────────────┘
                │ depends on (port interfaces)
┌───────────────▼──────────────────────────────────────────────┐
│                   Application Layer (internal/app)              │
│  anti-corruption layer; orchestrates domain.Service; maps ports │
│  app/{agent,cluster,data,dataset,edge,evaluation,experiment,     │
│      hpo,inference,job,llmgateway,metrics,optimize,token,        │
│      training,workspace}                                         │
└───────────────┬──────────────────────────────────────────────┘
                │ depends on (port interfaces)
┌───────────────▼──────────────────────────────────────────────┐
│                   Domain Layer (internal/domain)                │
│  entity + ports + domain service                                │
│  agent/ cluster/ data/ edge/ evaluation/ event/ experiment/     │
│  gpu/ hpo/ inference/ job/ lineage/ llm/ model/ optimize/        │
│  reproduction/ training/ workspace/  (18 packages)              │
└───────────────┬──────────────────────────────────────────────┘
                │ port implementations
┌───────────────▼──────────────────────────────────────────────┐
│           Infrastructure / Adapters (internal/storage, k8s)     │
│  `*Storage` implements many port interfaces; k8s runtimes        │
│  (agent / kubeedge)                                             │
└──────────────────────────────────────────────────────────────┘
```

**Core conventions**

- `domain/<subdomain>` defines only entities, port interfaces (`ports`), and domain services—no dependency on concrete storage/frameworks.
- `app/<subdomain>` is an anti-corruption layer that orchestrates domain services and adapts ports to concrete implementations.
- The `*Storage` on `internal/storage` implements multiple domain port interfaces on a single type (one store, many ports).
- Handlers are nil-safe: a non-injected repo degrades to `501` (e.g. Agent / Tool / edge / data-processing / compression).
- Standard flow to add a capability: create a `domain` package (Ports + Entity + Service) → `app` service → `storage` repo → `api` handler methods → register in `routes.go` (or subdomain-owned `*RegisterRoutes`) → wire in `bootstrap.go` (add to the `api.Repos` struct).

---

## 4. Backend Module Inventory (from real code)

### 4.1 Entrypoints & Assembly

| Module | Description |
| --- | --- |
| `cmd/main.go` | Backend main entrypoint: load config, assemble `api.Repos`, register routes, start Gin server. |
| `cmd/data-operator/` | **Data-processing operator container entrypoint**: launched by a K8s/Volcano Job, reads env `FUZE_DATA_SPEC` (JSON, `domain/data.DataJobSpec`: `{operator, params, input, output}`), dispatches to a built-in operator (labeling / cleaning / augmentation / ETL). Not invoked when a custom image overrides the command. |
| `internal/bootstrap` | Dependency assembly; builds `api.Repos` (aggregates all port impls) injected into handlers; centralizes env reading (`getEnv`). |
| `internal/config` | Config loading (file / env). Key: `AUTH_ENABLED`, `DB_DRIVER`/`DB_DSN`/`DB_PATH`, `EDGE_RUNTIME`, `LLM_BASE_URL`/`LLM_API_KEY`. |

### 4.2 Domain Layer (internal/domain, 18 packages)

| Package | Responsibility |
| --- | --- |
| `agent` | AI Agent: Agent DAG definition, compile, run orchestration. |
| `cluster` | Cluster topology: multi-cluster management, nodes, racks, fault domains, cluster registry (`k8s.ClusterRegistry`). |
| `data` | Datasets & data processing: datasets (Fluid), data operators (`operator`: `DataJobSpec`, labeling / cleaning / augmentation / ETL). |
| `edge` | Edge deployment: edge nodes / deployments / drift / baselines / label feedback; concept-drift detection & auto-rollback. |
| `evaluation` | Model & Agent evaluation: tasks, reviews, reports. |
| `event` | Event bus: publish/subscribe for audit, alerts, notifications. |
| `experiment` | Experiment tracking: experiments, runs, compare, reproducibility. |
| `gpu` | GPU resources: GPU nodes / cards / utilization / topology, GPU pricing. |
| `hpo` | Hyperparameter optimization & NAS (AutoML / NAS): search space, scheduled HPO tasks. |
| `inference` | Inference services: declarative spec, reconcile convergence, scaling. |
| `job` | Unified job abstraction & lifecycle (base for training/inference/HPO jobs). |
| `lineage` | Lineage: relationships among data / models / experiments (model-version lineage). |
| `llm` | LLM gateway domain: vendor abstraction (`llmgw`), chat / completion / tool calling, cost / price / quota / token metering, guardrail, RAG knowledge base, fine-tune adapters, Trace. |
| `model` | Model registry: versions, artifacts, registration metadata, lineage. |
| `optimize` | Model compression (inference acceleration): quantization / pruning / distillation / format conversion (TensorRT/ONNX/OpenVINO). |
| `reproduction` | Reproducible experiments: environment, seeds, dependency lock (`ReproduceRun` / `GetReproduction`). |
| `training` | Distributed training: job definition, state machine, checkpoints, GPU cost metering. |
| `workspace` | Notebook workspaces: team-isolated namespace, quota & multi-tenant boundary, with reverse proxy. |

### 4.3 Application Layer (internal/app, 16 packages)

Anti-corruption services adapting domain ports to handler-injectable implementations:

`agent`, `cluster`, `data`, `dataset`, `edge`, `evaluation`, `experiment`, `hpo`, `inference`, `job`, `llmgateway`, `metrics`, `optimize`, `token` (Personal Access Token self-service), `training`, `workspace`.

Each `app` service is typically constructed as `NewServiceWithMetrics(portImpl, ...)` and may inject a
monitoring sample source (e.g. `app/edge`'s `MetricsBackedSampleSource` mapping `ports.MetricsQuery` to `DriftSample`).

### 4.4 API Layer (internal/api)

`handler.go` defines the `api.Repos` struct (aggregates all port implementations); `routes.go`'s `RegisterRoutes` registers all route groups; subdomains (HPO, Edge) self-register via `h.HPORegisterRoutes` / `h.registerEdgeRoutes`. Middleware: auth, RBAC (`authMgr.RequireRole`), audit, rate-limit, SSRF guard, strict CORS.

All business routes are prefixed with `/api/v1`; the OpenAI-compatible endpoint is `/v1/chat/completions`; the Prometheus scrape endpoint is `/metrics`.

#### Registered route groups (from routes.go)

**Public group (no auth)**
- `GET /api/v1/health` — health check (returns `status` / `mode` / `auth`)
- `POST /api/v1/auth/login` — login
- `GET /api/v1/auth/sso` — list SSO providers
- `GET|POST /api/v1/auth/sso/:provider/start|callback|login` — OIDC / LDAP login (mounted when registry non-empty)
- `POST /api/v1/auth/mfa/verify` — MFA second-step verification
- `POST /api/v1/auth/passkey/login/begin|finish` — Passkey login assertion

**Protected group (full auth when `AUTH_ENABLED=true`, else dev-mode admin principal injected)**
- `GET /api/v1/auth/me`
- **Personal Access Token**: `POST|GET /api/v1/auth/tokens`, `POST /api/v1/auth/tokens/:id/rotate`, `DELETE /api/v1/auth/tokens/:id`
- **MFA / Passkey**: `POST /api/v1/auth/mfa/enroll|disable`, `POST /api/v1/auth/passkey/register/begin|finish`, `POST /api/v1/auth/passkey/disable`
- **Evaluation**: `/evaluations` (list/create/detail/result/fail/delete), `/evaluations/:id/reviews|llm-judge|report|finalize`, `/experiments/:id/evaluations`
- **Resources**: `GET|POST /api/v1/resources`, `GET /api/v1/resources/:id`
- **Training jobs**: `/training-jobs` (replaces legacy `/jobs`; list/create/detail/delete/logs/cancel/resume/complete/fail/checkpoints)
- **Training templates**: `GET /api/v1/training-templates`
- **Experiments**: `/experiments` (list/create/compare/detail/archive/delete), `/experiments/:id/runs` (list/create/complete/fail/cancel), `/experiments/runs/:runId/reproduce|reproduction`
- **AutoML / NAS**: `h.HPORegisterRoutes(protected)` self-registers
- **Metrics query**: `POST /api/v1/metrics/query|latest`
- **Alerts**: `/alerts/rules` (CRUD/toggle, TenantAdmin/PlatformAdmin), `/alerts/active`, `/alerts/silences`
- **Inference services**: `/inference-services` (list/detail/create/declarative Apply/Patch/delete)
- **Edge deployment**: `h.registerEdgeRoutes(protected)` (nodes/deployments/canary/rollback/drift; 501 if not assembled)
- **Model registry**: `/models` (list/detail/create/update/delete/versions/version lineage)
- **Datasets**: `/datasets` (list/detail/create/delete)
- **Monitoring/queues**: `GET /api/v1/metrics`, `GET /api/v1/queues`
- **Multi-cluster**: `/clusters` (list/register/detail/update/delete/discover/test/resources; writes limited to PlatformAdmin)
- **Tenant / quota / audit**: `/tenants`, `/quotas`, `/audit` (TenantAdmin/PlatformAdmin)
- **IdP registry mgmt**: `/sso/idps` (CRUD/test, PlatformAdmin)
- **LLMOps (batches 1-4)**:
  - Routing: `GET|PUT /api/v1/llm/routes`, `DELETE /api/v1/llm/routes/:model`
  - Metering & cost: `GET|PUT /api/v1/llm/quota`, `GET /api/v1/llm/usage|usage/sum`, `GET /api/v1/cost/summary`
  - Prices: `GET|PUT|DELETE /api/v1/llm/prices/llm|gpu` (admin)
  - Trace: `GET /api/v1/llm/traces|traces/:id`
  - Prompts: `/api/v1/llm/prompts` (list/create/version/activate/delete)
  - Guardrail: `GET|POST|DELETE /api/v1/llm/guardrail/rules` (admin)
  - RAG: `/api/v1/llm/knowledge` (list/create/detail/delete/documents)
  - Fine-tune adapters: `/api/v1/llm/finetune/adapters` (list/create/detail/delete/mount/unmount)
  - Gateway entry: `POST /api/v1/llm/chat`
- **Agent orchestration (batch 6)**: `/agents` (list/create/detail/compile/delete/run/run-list/run-detail/resume), `/tools` (list/create/detail/delete)
- **Notebook workspaces**: `/workspaces` (list/create/detail/start/stop/delete/activity/reverse-proxy `proxy/*`)
- **Data processing (labeling / cleaning / augmentation / ETL)**: `/data/pipelines` (create/list/detail/submit/cancel), `/data/annotations` (create/list/detail/export); 501 if not assembled
- **Model compression (inference acceleration)**: `/optimize/tasks` (list/create/detail/cancel/result/delete); 501 if not assembled

**OpenAI-compatible**
- `POST /v1/chat/completions` — reuses the protected-group auth policy

**Self-observability**
- `GET /metrics` — Prometheus standard exposition (business `fuze_*` + runtime `process_*`/`go_*` + HTTP `http_*`)

### 4.5 Auth & Security (internal/auth, internal/crypto)

- `auth.Manager`: unified auth facade holding an **IdP registry** (OIDC / LDAP / SAML).
- `sso.go` / `idp.go`: SSO & IdP integration; bootstrapped in `bootstrap`. Routes `/auth/sso`, `/sso/idps`.
- `mfa.go` / `passkey.go`: MFA and Passkey (WebAuthn).
- `rbac.go` / `guardrail.go`: role-based access control (roles `RolePlatformAdmin` / `RoleTenantAdmin` / `member`) and LLM guardrails (desensitization & interception rules).
- `audit.go`: audit middleware + event-bus `notify`, persisted via `auditRepo`, covered by `tests/api_audit_rbac_test.go`.
- `middleware`: rate limit, SSRF guard (cluster discover/test limited to admins), strict CORS (whitelist-only origins; credentials disabled for wildcard).
- `internal/crypto`: encryption / signing helpers (e.g. `KUBECONFIG_ENC_KEY` AES-256 master key).

### 4.6 LLM Gateway (internal/llmgw, internal/domain/llm)

Unified multi-vendor LLM access (`NewCompleterFromEnv` reads `LLM_BASE_URL` / `LLM_API_KEY`, graceful degradation when unset) providing:

- Chat / completion / streaming; tool calling (function calling).
- Fine-tune adapters (LoRA / QLoRA) registration, mount / unmount.
- **Metering & governance**: token counting, cost, price (`PriceBook` reads `LLM_DEFAULT_INPUT_PER_1K` etc.), quota.
- **Guardrail**: desensitization & interception rules.
- **RAG knowledge base** and **call Trace**.
- **OpenAI-compatible interface**: `/v1/chat/completions` and `/api/v1/llm/chat`.

### 4.7 Edge Deployment (internal/domain/edge, internal/app/edge, internal/k8s/edge)

- Runtimes: `EDGE_RUNTIME=mock|agent|kubeedge` (`EDGE_CLOUDHUB_URL` / `EDGE_CLOUDHUB_TOKEN` / `EDGE_CLOUDHUB_NAMESPACE` / `EDGE_CLOUDHUB_CA` for KubeEdge).
- `AgentRuntime`: lightweight mTLS edge agent; `KubeEdgeRuntime`: manages `apps/v1` Deployment + `nodeSelector` via CloudHub; `MockRuntime`: default, for tests.
- Drift detection: performance drift + **concept drift** (based on real label feedback via the `edge_label_feedback` table), aggregated into `OverallSeverity`.
- Auto-rollback: when `OverallSeverity` is high/critical AND `DriftGuard` + `AutoRollback` are on, triggers canary rollback.

### 4.8 Ports & Storage (internal/ports, internal/storage)

- `internal/ports`: domain port interfaces (e.g. `MetricsQuery` carries a `Labels` field appended to PromQL selectors; `ArtifactStore` supports local / S3).
- `internal/storage`: `*Storage` implements many domain port interfaces on a single type; `db.go`'s `Migrate(db)` uses GORM `AutoMigrate` to create tables; `sqlite.go` handles seeding and `ADMIN_PASSWORD` init. Grouped accessors such as `store.Edge()`.

---

## 5. Frontend Architecture (frontend/src)

React 18 + Vite + Tailwind. Routing centralized in `src/App.jsx` (react-router-dom 6). Auth state is decided by the `auth` field of `/api/v1/health` plus a local token; unauthenticated users are redirected to `/login`.

### 5.1 Delivered Pages (24, frontend/src/pages)

| Page | Route | Description |
| --- | --- | --- |
| `Login.jsx` | `/login` | Login |
| `Dashboard.jsx` | `/dashboard` | Overview dashboard |
| `Models.jsx` | `/models` (default home) | Model registry |
| `Datasets.jsx` | `/datasets` | Datasets |
| `Inference.jsx` | `/inference` | Inference services |
| `InferenceAccel.jsx` | `/inference-accel` | Inference acceleration (compression) |
| `Workspaces.jsx` | `/workspaces` | Notebook workspaces |
| `Training.jsx` | `/training` (legacy `/jobs` redirects here) | Training jobs |
| `Experiments.jsx` | `/experiments` | Experiment tracking |
| `ExperimentDetail.jsx` | `/experiments/:id` | Experiment detail |
| `ExperimentCompare.jsx` | `/experiments/compare` | Experiment compare |
| `AutoML.jsx` | `/automl` | HPO / NAS (AutoML) |
| `AutoMLStudy.jsx` | `/automl/:id` | AutoML study detail |
| `Evaluations.jsx` | `/evaluations` | Evaluations |
| `EvaluationReport.jsx` | `/evaluations/:id` | Evaluation report |
| `Resources.jsx` | `/resources` | Resources (GPU / cluster nodes) |
| `Clusters.jsx` | `/clusters` | Multi-cluster management |
| `LLMOps.jsx` | `/llmops` | LLM gateway (usage / cost / quota / routing / guardrail / knowledge) |
| `AgentStudio.jsx` | `/agents` | AI Agent orchestration |
| `Edge.jsx` | `/edge` | Edge deployment (drift / rollback board) |
| `Tools.jsx` | `/tools` | Tool registry |
| `Monitoring.jsx` | `/monitoring` | Monitoring |
| `Alerts.jsx` | `/alerts` | Alerts |
| `Settings.jsx` | `/settings` | Settings |
| `IdPAdmin.jsx` | `/admin/idps` | IdP / SSO administration |

> Note: The frontend is delivered incrementally. Backend capabilities that are API-exposed but have no
> dedicated frontend page yet include: model compression tasks (`/optimize/tasks`), data-processing pipelines
> & annotations (`/data/*`), some Agent-run history views, and a few advanced LLM admin sub-pages (price/Trace/
> prompts/fine-tune adapters). These are API-available on the backend and degrade to `501` when not assembled.

### 5.2 Cache Layer

The frontend organizes data via React state + query cache. The backend cache layer (e.g. a unified
`internal/cache` port) is a **deferred** item; direct DB queries are the current default.

---

## 6. Deployment & Operations

Deployment docs live under `docs/deployment/`:

- `k8s-deployment-guide_CN.md` / `k8s-deployment-guide_EN.md`: Kubernetes deployment guides (namespace `fuze-system`, incl. backend / frontend / postgres / EdgeRuntime CRD + KubeEdge CloudHub).
- `docs/references/k8s-deploy-troubleshooting.md`: troubleshooting guide.

> Docker Compose and edge (KubeEdge) quick start: see the repo-root `README.md` / `README_CN.md` and `docker-compose.yml`.

### 6.1 Key Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `AUTH_ENABLED` | `false` (dev injects admin principal) | `true` enforces full auth |
| `PORT` | `8080` | API server port |
| `DB_DRIVER` | `sqlite` | DB driver (`sqlite` / `postgres`) |
| `DB_DSN` | — | PostgreSQL connection string (when driver=postgres) |
| `DB_PATH` | `./fuze-ai-paas.db` | SQLite file path |
| `ADMIN_PASSWORD` | — | Initial admin password (sqlite seed) |
| `AUTH_SECRET` | — | Token signing secret |
| `KUBECONFIG_ENC_KEY` | — | kubeconfig encryption master key (AES-256, hex 64 chars) |
| `EDGE_RUNTIME` | `mock` | `mock` / `agent` / `kubeedge` |
| `EDGE_CLOUDHUB_URL` / `EDGE_CLOUDHUB_TOKEN` / `EDGE_CLOUDHUB_NAMESPACE` / `EDGE_CLOUDHUB_CA` | — | KubeEdge CloudHub connection |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | — | LLM vendor config (graceful degradation if unset) |
| `HPO_GATEWAY_BASE_URL` | — | AutoML/NAS gateway URL |
| `EVENT_WEBHOOK_URL` | — | Event-bus webhook notify URL |
| `WORKSPACE_PROXY_BASE_URL` | — | Notebook workspace reverse-proxy base URL |
| `ARTIFACT_BACKEND` / `ARTIFACT_LOCAL_ROOT` / `ARTIFACT_S3_*` | — | Artifact store (local / S3) |
| `FUZE_DATA_SPEC` | — | `data-operator` task spec (JSON) |

---

## 7. Testing Strategy

- **Backend**: TDD, `*_test.go` next to source; covers domain / app / api / storage / bootstrap (e.g. `tests/api_audit_rbac_test.go` for audit & RBAC; `storage/db_test.go` for migration & seed).
- **Frontend**: `frontend/src/*.test.jsx` next to source.
- **Run**: `go test ./...` (backend), `npm test` (frontend). CI in `.github/workflows` (if present).

---

## 8. Relationship to Other Docs

- Deployment details → `docs/deployment/*`
- API route details → `backend/internal/api/routes.go`
- SDK usage → `sdk/python/fuze_ml`
- Repo-level intro → `README.md` / `README_CN.md`

---

*Document derived from the repository `dev` branch code (2026-08-20 snapshot). Where architecture and code conflict, the code is authoritative.*
