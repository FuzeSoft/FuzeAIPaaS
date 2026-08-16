# Fuze AI PaaS 架构设计文档（中文版）

> 本文件基于当前仓库代码实际实现整理，作为 Fuze AI PaaS 平台的总体架构说明。
> 最后更新：2026-08-20。架构与代码存在偏差时，以代码为准。

---

## 1. 平台定位

Fuze AI PaaS 是一个面向企业的 **AI / LLM 平台即服务**，帮助团队统一完成：

- **大语言模型（LLM）能力与治理（LLMOps）**：多供应商网关（OpenAI / Anthropic / 自建模型）、统一 OpenAI 兼容接口（`/v1/chat/completions` 与 `/llm/chat`）、聊天 / 补全 / 工具调用、微调适配器（LoRA / QLoRA）、路由表、Token 计量与成本、价格管理、调用链路 Trace、提示词工程、RAG 知识库、安全护栏（Guardrail）。
- **AI Agent 编排**：Agent DAG 定义 / 编译 / 运行 / 人工审核恢复，工具注册表（tool_call 节点）。
- **模型训练与优化**：分布式训练作业（取代旧的 `/jobs`）、超参优化与神经架构搜索（AutoML / NAS）、推理服务（声明式 spec + reconcile 收敛）、模型压缩（量化 / 剪枝 / 蒸馏 / 格式转换）。
- **实验与复现**：实验跟踪、运行记录（runs）、实验对比、可复现实验（Reproduce / Reproduction）。
- **评估**：模型 / Agent 评估任务，人工评审（Human/LLM-as-judge）、聚合报告与 finalize。
- **数据与标注**：数据集管理（Fluid 数据集）、数据处理管线（标注 / 清洗 / 增强 / ETL，由 `data-operator` 执行）。
- **边缘部署与漂移防御**：边缘运行时（mock / agent / KubeEdge）、概念漂移检测、金丝雀发布与自动回滚。
- **统一身份与安全（P4 企业底座）**：多 IdP（OIDC / LDAP / SAML）注册表、SSO、MFA、Passkey / WebAuthn、RBAC 角色、全量审计、租户 / 配额管理、限流、SSRF 防护、严格 CORS。

平台采用 **领域驱动设计（DDD）+ 六边形架构（Hexagonal / Ports & Adapters）**，语言为 Go（后端）/ React（前端），并配套 Python SDK。

---

## 2. 技术栈

| 层 | 技术 |
| --- | --- |
| 后端语言 | Go（Gin Web 框架；数据管线可跑在 K8s/Volcano Job 中） |
| 前端语言 | JavaScript（React 18 + Vite） |
| 前端路由 | react-router-dom 6 |
| UI 样式 | Tailwind CSS，lucide-react 图标，recharts 图表，clsx 工具 |
| 存储 | 关系型数据库（SQLite / PostgreSQL，GORM；`storage.Migrate` 经 `AutoMigrate` 建表） |
| 鉴权 | 自研 `auth.Manager`：IdP 注册表 + MFA + Passkey + SSO + RBAC + 审计 |
| 部署 | Docker Compose / Kubernetes（含 KubeEdge 边缘） |
| SDK | Python（`sdk/python/fuze_ml`） |
| 测试 | `go test`，TDD 风格，测试文件随源码放置（`*_test.go`） |

---

## 3. 分层架构（六边形 / DDD）

```
┌──────────────────────────────────────────────────────────────┐
│                        API 层 (Gin)                            │
│  internal/api：handler（handler.go/routes.go）+ 中间件          │
│  auth 中间件 / RBAC 中间件 / 审计中间件 / 限流 / SSRF / CORS      │
└───────────────┬──────────────────────────────────────────────┘
                │ 依赖（端口接口）
┌───────────────▼──────────────────────────────────────────────┐
│                  应用层 (internal/app)                          │
│  防腐层（anti-corruption），编排 domain.Service，映射端口        │
│  app/{agent,cluster,data,dataset,edge,evaluation,experiment,    │
│      hpo,inference,job,llmgateway,metrics,optimize,token,       │
│      training,workspace}                                        │
└───────────────┬──────────────────────────────────────────────┘
                │ 依赖（端口接口）
┌───────────────▼──────────────────────────────────────────────┐
│                  领域层 (internal/domain)                       │
│  实体(entity) + 端口(ports) + 领域服务(service)                 │
│  agent/ cluster/ data/ edge/ evaluation/ event/ experiment/     │
│  gpu/ hpo/ inference/ job/ lineage/ llm/ model/ optimize/        │
│  reproduction/ training/ workspace/  （共 18 个）                │
└───────────────┬──────────────────────────────────────────────┘
                │ 端口实现
┌───────────────▼──────────────────────────────────────────────┐
│              基础设施 / 适配器 (internal/storage, k8s)          │
│  *Storage 实现大量端口接口；k8s 运行时（agent / kubeedge）        │
└──────────────────────────────────────────────────────────────┘
```

**核心约定**

- `domain/<subdomain>` 只定义实体、端口接口（`ports`）、领域服务，不依赖具体存储/框架。
- `app/<subdomain>` 作为防腐层，编排领域服务并将端口适配到具体实现。
- `storage` 上的 `*Storage` 在同一类型上实现多个领域端口接口（一个存储多端口）。
- Handler 对未注入的 repo 做 nil-safe 降级，返回 `501`（如 Agent / Tool / 边缘 / 数据处理 / 压缩）。
- 新增能力标准流程：新建 `domain` 包（Ports + Entity + Service）→ `app` 服务 → `storage` 仓储 → `api` handler 方法 → `routes.go`（或子域自持 `*RegisterRoutes`）注册 → `bootstrap.go` 装配（加入 `api.Repos` 结构）。

---

## 4. 后端模块清单（按真实代码）

### 4.1 入口与装配

| 模块 | 说明 |
| --- | --- |
| `cmd/main.go` | 后端主入口：加载配置、装配 `api.Repos`、注册路由、启动 Gin 服务。 |
| `cmd/data-operator/` | **数据处理算子容器入口**：由 K8s/Volcano Job 启动，读取环境变量 `FUZE_DATA_SPEC`（JSON，结构 `domain/data.DataJobSpec`：`{operator, params, input, output}`），分发到对应内置算子（标注 / 清洗 / 增强 / ETL）。自定义镜像覆盖场景下不被调用。 |
| `internal/bootstrap` | 依赖装配，构造 `api.Repos`（聚合所有 port 实现），注入到 handler；统一读取环境变量（`getEnv`）。 |
| `internal/config` | 配置加载（文件 / 环境变量）。关键：`AUTH_ENABLED`、`DB_DRIVER`/`DB_DSN`/`DB_PATH`、`EDGE_RUNTIME`、`LLM_BASE_URL`/`LLM_API_KEY` 等。 |

### 4.2 领域层（internal/domain，共 18 个）

| 包 | 职责 |
| --- | --- |
| `agent` | AI Agent：Agent DAG 定义、编译、运行编排。 |
| `cluster` | 集群拓扑：多集群纳管、节点、机架、故障域、集群注册表（`k8s.ClusterRegistry`）。 |
| `data` | 数据集与数据处理：数据集（Fluid）、数据处理算子（`operator`：`DataJobSpec`、标注 / 清洗 / 增强 / ETL）。 |
| `edge` | 边缘部署：边缘节点 / 部署 / 漂移（drift）/ 基线（baseline）/ 标注反馈（label_feedback）；概念漂移检测与自动回滚。 |
| `evaluation` | 模型与 Agent 评估：评估任务、评审、报告。 |
| `event` | 事件总线：审计、告警、通知的事件发布/订阅。 |
| `experiment` | 实验跟踪：实验、运行记录（run）、对比、可复现。 |
| `gpu` | GPU 资源：GPU 节点 / 卡 / 利用率 / 拓扑、GPU 价格。 |
| `hpo` | 超参优化与神经架构搜索（AutoML / NAS）：搜索空间、调度的 HPO 任务。 |
| `inference` | 推理服务：声明式 spec、reconcile 收敛、扩缩容。 |
| `job` | 作业抽象与生命周期（训练/推理/HPO 等作业的统一底座）。 |
| `lineage` | 血缘：数据 / 模型 / 实验之间的血缘关系（模型版本 lineage）。 |
| `llm` | 大模型网关领域：供应商抽象（`llmgw`）、聊天 / 补全 / 工具调用、成本 / 价格 / 配额 / Token 计量、Guardrail、RAG 知识库、微调适配器、Trace。 |
| `model` | 模型仓库与注册：模型、版本、工件、注册元数据、lineage。 |
| `optimize` | 模型压缩（推理加速）：量化 / 剪枝 / 蒸馏 / 格式转换（TensorRT/ONNX/OpenVINO）。 |
| `reproduction` | 可复现实验：环境、随机种子、依赖锁定，保证实验可复现（`ReproduceRun` / `GetReproduction`）。 |
| `training` | 分布式训练：训练作业定义、状态机、检查点、GPU 成本计量。 |
| `workspace` | Notebook 工作空间：团队的隔离命名空间，承载资源配额与多租户边界，含反向代理。 |

### 4.3 应用层（internal/app，共 16 个）

防腐层服务，将领域端口适配为可注入 handler 的实现：

`agent`、`cluster`、`data`、`dataset`、`edge`、`evaluation`、`experiment`、`hpo`、`inference`、`job`、`llmgateway`、`metrics`、`optimize`、`token`（Personal Access Token 自管理）、`training`、`workspace`。

每个 `app` 服务通常以 `NewServiceWithMetrics(端口实现, ...)` 形式构造，并可能注入监控采样源（如 `app/edge` 的 `MetricsBackedSampleSource` 将 `ports.MetricsQuery` 映射为 `DriftSample`）。

### 4.4 API 层（internal/api）

`handler.go` 定义 `api.Repos` 结构（聚合所有 port 实现）；`routes.go` 的 `RegisterRoutes` 注册全部路由组；子域（HPO、Edge）通过 `h.HPORegisterRoutes` / `h.registerEdgeRoutes` 自持路由挂载。中间件含鉴权、RBAC（`authMgr.RequireRole`）、审计、限流、SSRF 防护、严格 CORS。

所有业务路由前缀为 `/api/v1`；OpenAI 兼容端点为 `/v1/chat/completions`；Prometheus 抓取端点为 `/metrics`。

#### 已注册路由组（来自 routes.go）

**公共分组（免鉴权）**
- `GET /api/v1/health` — 健康检查（返回 `status` / `mode` / `auth`）
- `POST /api/v1/auth/login` — 登录
- `GET /api/v1/auth/sso` — 列出 SSO 供应商
- `GET|POST /api/v1/auth/sso/:provider/start|callback|login` — OIDC / LDAP 登录流程（注册表非空时挂载）
- `POST /api/v1/auth/mfa/verify` — MFA 第二步校验
- `POST /api/v1/auth/passkey/login/begin|finish` — Passkey 登录断言

**受保护分组（AUTH_ENABLED=true 时全链路鉴权，否则注入开发态管理员主体）**
- `GET /api/v1/auth/me`
- **Personal Access Token**：`POST|GET /api/v1/auth/tokens`、`POST /api/v1/auth/tokens/:id/rotate`、`DELETE /api/v1/auth/tokens/:id`
- **MFA / Passkey**：`POST /api/v1/auth/mfa/enroll|disable`、`POST /api/v1/auth/passkey/register/begin|finish`、`POST /api/v1/auth/passkey/disable`
- **评估**：`/evaluations`（列表/创建/详情/结果/失败/删除）、`/evaluations/:id/reviews|llm-judge|report|finalize`、`/experiments/:id/evaluations`
- **资源**：`GET|POST /api/v1/resources`、`GET /api/v1/resources/:id`
- **训练作业**：`/training-jobs`（取代旧 `/jobs`；列表/创建/详情/删除/日志/取消/恢复/完成/失败/检查点）
- **训练模板**：`GET /api/v1/training-templates`
- **实验**：`/experiments`（列表/创建/对比/详情/归档/删除）、`/experiments/:id/runs`（列表/创建/完成/失败/取消）、`/experiments/runs/:runId/reproduce|reproduction`
- **AutoML / NAS**：`h.HPORegisterRoutes(protected)` 自持挂载
- **指标查询**：`POST /api/v1/metrics/query|latest`
- **告警**：`/alerts/rules`（增删改查/切换，限 TenantAdmin/PlatformAdmin）、`/alerts/active`、`/alerts/silences`
- **推理服务**：`/inference-services`（列表/详情/创建/声明式 Apply/ Patch/删除）
- **边缘部署**：`h.registerEdgeRoutes(protected)`（节点/部署/灰度/回滚/漂移；未装配返回 501）
- **模型仓库**：`/models`（列表/详情/创建/更新/删除/版本/版本 lineage）
- **数据集**：`/datasets`（列表/详情/创建/删除）
- **队列 / 监控**：`GET /api/v1/metrics`、`GET /api/v1/queues`
- **多集群纳管**：`/clusters`（列表/注册/详情/更新/删除/发现/测试/资源，写操作限 PlatformAdmin）
- **租户 / 配额 / 审计**：`/tenants`、`/quotas`、`/audit`（限 TenantAdmin/PlatformAdmin）
- **IdP 注册表管理**：`/sso/idps`（增删改查/测试，限 PlatformAdmin）
- **LLMOps（批次 1-4）**：
  - 路由表：`GET|PUT /api/v1/llm/routes`、`DELETE /api/v1/llm/routes/:model`
  - 计量与成本：`GET|PUT /api/v1/llm/quota`、`GET /api/v1/llm/usage|usage/sum`、`GET /api/v1/cost/summary`
  - 价格：`GET|PUT|DELETE /api/v1/llm/prices/llm|gpu`（限管理员）
  - Trace：`GET /api/v1/llm/traces|traces/:id`
  - 提示词工程：`/api/v1/llm/prompts`（列表/创建/版本/激活/删除）
  - 护栏：`GET|POST|DELETE /api/v1/llm/guardrail/rules`（限管理员）
  - RAG 知识库：`/api/v1/llm/knowledge`（列表/创建/详情/删除/文档）
  - 微调适配器：`/api/v1/llm/finetune/adapters`（列表/创建/详情/删除/挂载/卸载）
  - 网关入口：`POST /api/v1/llm/chat`
- **Agent 编排（批次 6）**：`/agents`（列表/创建/详情/编译/删除/运行/运行列表/运行详情/恢复）、`/tools`（列表/创建/详情/删除）
- **Notebook 工作空间**：`/workspaces`（列表/创建/详情/启动/停止/删除/心跳/反向代理 `proxy/*`）
- **数据处理（标注 / 清洗 / 增强 / ETL）**：`/data/pipelines`（创建/列表/详情/提交/取消）、`/data/annotations`（创建/列表/详情/导出）；未装配返回 501
- **模型压缩（推理加速）**：`/optimize/tasks`（列表/创建/详情/取消/结果/删除）；未装配返回 501

**OpenAI 兼容**
- `POST /v1/chat/completions` — 复用受保护分组鉴权策略

**自观测**
- `GET /metrics` — Prometheus 标准暴露（业务 `fuze_*` + 运行时 `process_*`/`go_*` + HTTP `http_*`）

### 4.5 鉴权与安全（internal/auth, internal/crypto）

- `auth.Manager`：统一认证门面，持有 **IdP 注册表**，支持 OIDC / LDAP / SAML。
- `sso.go` / `idp.go`：SSO 与 IdP 接入；`bootstrap` 装配各 IdP。路由 `/auth/sso`、`/sso/idps`。
- `mfa.go` / `passkey.go`：多因子认证与 Passkey（WebAuthn）。
- `rbac.go` / `guardrail.go`：基于角色的访问控制（角色 `RolePlatformAdmin` / `RoleTenantAdmin` / `member`）与 LLM 安全护栏（脱敏与拦截规则）。
- `audit.go`：审计中间件 + 事件总线 `notify`，持久化到 `auditRepo`，并有 `tests/api_audit_rbac_test.go` 覆盖。
- `middleware`：限流、SSRF 防护（集群探测/测试入口限管理员）、严格 CORS（仅白名单来源、凭证模式不开通配符）。
- `internal/crypto`：加解密与签名辅助（如 `KUBECONFIG_ENC_KEY` AES-256 主密钥）。

### 4.6 LLM 网关（internal/llmgw, internal/domain/llm）

统一接入多供应商 LLM（`NewCompleterFromEnv` 读 `LLM_BASE_URL` / `LLM_API_KEY`，未配置优雅降级），提供：

- 聊天 / 补全 / 流式；工具调用（function calling）。
- 微调适配器（LoRA / QLoRA）登记、挂载 / 卸载。
- **计量治理**：Token 计数、成本、价格（`PriceBook` 读 `LLM_DEFAULT_INPUT_PER_1K` 等）、配额。
- **Guardrail**：脱敏与拦截规则。
- **RAG 知识库**与**调用链路 Trace**。
- **OpenAI 兼容接口**：`/v1/chat/completions` 与 `/api/v1/llm/chat`。

### 4.7 边缘部署（internal/domain/edge, internal/app/edge, internal/k8s/edge）

- 运行时：`EDGE_RUNTIME=mock|agent|kubeedge`（`EDGE_CLOUDHUB_URL` / `EDGE_CLOUDHUB_TOKEN` / `EDGE_CLOUDHUB_NAMESPACE` / `EDGE_CLOUDHUB_CA` 用于 KubeEdge）。
- `AgentRuntime`：基于 mTLS 的轻量边缘 Agent；`KubeEdgeRuntime`：经 CloudHub 管理 `apps/v1` Deployment + `nodeSelector`；`MockRuntime`：默认，用于测试。
- 漂移检测：性能漂移 + **概念漂移**（基于真实标注反馈，经 `edge_label_feedback` 表），聚合器输出 `OverallSeverity`。
- 自动回滚：当 `OverallSeverity` 为高/严重且 `DriftGuard` + `AutoRollback` 开启时，触发金丝雀回滚。

### 4.8 端口与存储（internal/ports, internal/storage）

- `internal/ports`：定义领域端口接口（如 `MetricsQuery` 含 `Labels` 字段以拼接到 PromQL 选择器；`ArtifactStore` 支持 local / S3）。
- `internal/storage`：`*Storage` 在同一类型上实现多个领域端口接口；`db.go` 的 `Migrate(db)` 经 GORM `AutoMigrate` 建表；`sqlite.go` 负责种子数据与 `ADMIN_PASSWORD` 初始化。`store.Edge()` 等分组访问器。

---

## 5. 前端架构（frontend/src）

React 18 + Vite + Tailwind。路由集中在 `src/App.jsx`（react-router-dom 6）。登录态由 `/api/v1/health` 的 `auth` 字段与本地 token 决定；未登录跳转 `/login`。

### 5.1 已落地页面（24 个，frontend/src/pages）

| 页面 | 路由 | 说明 |
| --- | --- | --- |
| `Login.jsx` | `/login` | 登录 |
| `Dashboard.jsx` | `/dashboard` | 总览仪表盘 |
| `Models.jsx` | `/models`（默认首页） | 模型仓库 |
| `Datasets.jsx` | `/datasets` | 数据集 |
| `Inference.jsx` | `/inference` | 推理服务 |
| `InferenceAccel.jsx` | `/inference-accel` | 推理加速（模型压缩） |
| `Workspaces.jsx` | `/workspaces` | Notebook 工作空间 |
| `Training.jsx` | `/training`（旧 `/jobs` 重定向至此） | 训练作业 |
| `Experiments.jsx` | `/experiments` | 实验跟踪 |
| `ExperimentDetail.jsx` | `/experiments/:id` | 实验详情 |
| `ExperimentCompare.jsx` | `/experiments/compare` | 实验对比 |
| `AutoML.jsx` | `/automl` | 超参优化 / NAS（AutoML） |
| `AutoMLStudy.jsx` | `/automl/:id` | AutoML 研究详情 |
| `Evaluations.jsx` | `/evaluations` | 评估 |
| `EvaluationReport.jsx` | `/evaluations/:id` | 评估报告 |
| `Resources.jsx` | `/resources` | 资源（GPU / 集群节点） |
| `Clusters.jsx` | `/clusters` | 多集群纳管 |
| `LLMOps.jsx` | `/llmops` | LLM 网关（用量 / 成本 / 配额 / 路由 / 护栏 / 知识库） |
| `AgentStudio.jsx` | `/agents` | AI Agent 编排 |
| `Edge.jsx` | `/edge` | 边缘部署（漂移 / 回滚看板） |
| `Tools.jsx` | `/tools` | 工具注册表 |
| `Monitoring.jsx` | `/monitoring` | 监控 |
| `Alerts.jsx` | `/alerts` | 告警 |
| `Settings.jsx` | `/settings` | 设置 |
| `IdPAdmin.jsx` | `/admin/idps` | IdP / SSO 管理 |

> 说明：前端页面分阶段落地。后端已通过 API 暴露但前端暂未提供独立页面的能力包括：模型压缩任务（`/optimize/tasks`）、数据处理管线与标注（`/data/*`）、Agent 运行历史的部分视图、部分 LLM 高级管理子页（价格/Trace/提示词/微调适配器的独立页面）。这些能力在后端为 API 可用、未装配时 nil-safe 返回 `501`。

### 5.2 缓存层

前端以 React 状态 + 查询缓存组织数据；后端缓存层（如统一的 `internal/cache` 端口）属于**延迟实现**项，当前以数据库直查为主。

---

## 6. 部署与运维

部署相关文档位于 `docs/deployment/`：

- `k8s-deployment-guide_CN.md` / `k8s-deployment-guide_EN.md`：Kubernetes 部署手册（命名空间 `fuze-system`，含 backend / frontend / postgres / EdgeRuntime CRD + KubeEdge CloudHub）。
- `docs/references/k8s-deploy-troubleshooting.md`：排障手册。

> Docker Compose 与边缘（KubeEdge）的快速启动见仓库顶层 `README.md` / `README_CN.md` 与 `docker-compose.yml`。

### 6.1 关键环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AUTH_ENABLED` | `false`（开发态注入管理员主体） | `true` 时全链路鉴权 |
| `PORT` | `8080` | API 服务端口 |
| `DB_DRIVER` | `sqlite` | 数据库驱动（`sqlite` / `postgres`） |
| `DB_DSN` | — | PostgreSQL 连接串（driver=postgres 时） |
| `DB_PATH` | `./fuze-ai-paas.db` | SQLite 文件路径 |
| `ADMIN_PASSWORD` | — | 初始管理员密码（sqlite 种子） |
| `AUTH_SECRET` | — | 令牌签名密钥 |
| `KUBECONFIG_ENC_KEY` | — | kubeconfig 加密主密钥（AES-256，hex 64 字符） |
| `EDGE_RUNTIME` | `mock` | `mock` / `agent` / `kubeedge` |
| `EDGE_CLOUDHUB_URL` / `EDGE_CLOUDHUB_TOKEN` / `EDGE_CLOUDHUB_NAMESPACE` / `EDGE_CLOUDHUB_CA` | — | KubeEdge CloudHub 连接参数 |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | — | LLM 供应商接入（未配置优雅降级） |
| `HPO_GATEWAY_BASE_URL` | — | AutoML/NAS 网关地址 |
| `EVENT_WEBHOOK_URL` | — | 事件总线 Webhook 通知地址 |
| `WORKSPACE_PROXY_BASE_URL` | — | Notebook 工作空间反向代理基址 |
| `ARTIFACT_BACKEND` / `ARTIFACT_LOCAL_ROOT` / `ARTIFACT_S3_*` | — | 工件存储（local / S3） |
| `FUZE_DATA_SPEC` | — | `data-operator` 算子任务描述（JSON） |

---

## 7. 测试策略

- **后端**：TDD，`*_test.go` 与源码同目录；覆盖 domain / app / api / storage / bootstrap 各层（如 `tests/api_audit_rbac_test.go` 覆盖审计与 RBAC；`storage/db_test.go` 覆盖迁移与种子）。
- **前端**：`frontend/src/*.test.jsx` 与源文件同目录。
- **运行**：`go test ./...`（后端）、`npm test`（前端）。CI 见 `.github/workflows`（若存在）。

---

## 8. 与其他文档的关系

- 部署细节 → `docs/deployment/*`
- API 路由细节 → `backend/internal/api/routes.go`
- SDK 用法 → `sdk/python/fuze_ml`
- 本仓库顶层说明 → `README.md` / `README_CN.md`

---

*文档基于仓库 `dev` 分支代码（2026-08-20 快照）整理。架构与代码冲突时，以代码为准。*
