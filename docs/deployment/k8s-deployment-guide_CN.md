# Fuze AI PaaS —— Kubernetes 部署指南

本文档说明如何将 Fuze AI PaaS（AI 算力调度平台）部署到 Kubernetes 集群。部署清单均位于仓库的 `k8s/` 目录，通过 Kustomize 统一管理。

> 镜像与命名空间约定：
> - 后端镜像：`fuze-ai-paas-backend:latest`
> - 前端镜像：`fuze-ai-paas-frontend:latest`
> - 命名空间：`fuze-ai-paas`

---

## 一、前置要求

- 一个可用的 Kubernetes 集群（v1.24+），且具备 `kubectl` 访问权限。
- 已安装 Kustomize（或 `kubectl` 版本 ≥ 1.14，自带 `kubectl apply -k`）。
- 容器镜像仓库访问能力（本地集群可直接 `docker build`，远程集群需推送到镜像仓库）。
- 集群节点需满足后端/前端资源请求（CPU/内存见下方清单）。
- 可选组件按需安装：Volcano（批调度）、KServe+Knative（推理）、HAMi（GPU 隔离）、Fluid（数据加速）。

## 二、架构概览

```text
浏览器 / 客户端
      │
      ▼
  Ingress (fuze-ai-paas.local)
   ├─ /        → fuze-frontend:3000 (NodePort 30300)
   ├─ /api     → fuze-backend:8080
   └─ /grafana → grafana:3000
      │
      ▼
  fuze-backend ──(CRD)──▶ Volcano / KServe / Fluid / HAMi
   (ServiceAccount + RBAC 权限)
      │
      ▼
  Prometheus + Grafana 监控栈 (抓取 /metrics)
```

核心组件部署清单：

| 组件 | 清单文件 | 说明 |
| :--- | :--- | :--- |
| 命名空间 | `namespace.yaml` | 所有资源所属命名空间 |
| 后端权限 | `rbac.yaml` | ServiceAccount + Role 管理 Volcano/KServe/Fluid CRD |
| 后端 | `backend-deployment.yaml` / `backend-service.yaml` | Go(Gin) 服务，:8080 |
| 前端 | `frontend-deployment.yaml` / `frontend-service.yaml` | React 静态服务，:3000（NodePort 30300） |
| 入口 | `ingress.yaml` | 路由 `/`、`/api`、`/grafana` |
| 队列 | `volcano-queue.yaml` | 推理/训练/批处理三类 Queue |
| 监控 | `monitoring.yaml` | Prometheus + Grafana（已纳入 kustomization） |

## 三、构建镜像

在项目根目录执行：

```bash
# 后端镜像
docker build -f Dockerfile.backend -t fuze-ai-paas-backend:latest .

# 前端镜像
docker build -f Dockerfile.frontend -t fuze-ai-paas-frontend:latest .
```

> 说明：`Dockerfile.backend` 使用多阶段构建，最终二进制为 `fuze-scheduler`，监听 `8080` 端口；`Dockerfile.frontend` 使用 `serve` 托管 `dist`，监听 `3000` 端口。

若部署到远程集群，请将镜像推送到镜像仓库，并相应修改 `backend-deployment.yaml` / `frontend-deployment.yaml` 中的 `image` 字段（必要时设置 `imagePullSecrets`）。

也可使用 Makefile：

```bash
make build            # 本地编译 backend + frontend
make docker-build     # docker build -t fuze-ai-paas:latest .（单镜像）
```

## 四、部署平台核心（必选）

```bash
# 方式一：从仓库根目录
kubectl apply -k k8s/

# 方式二：进入 k8s 目录
cd k8s && kubectl apply -k .
```

该命令通过 `kustomization.yaml` 依次创建：命名空间、RBAC、后端、前端、Ingress、Volcano 队列、监控栈。

验证资源已就绪：

```bash
kubectl -n fuze-ai-paas get all
kubectl -n fuze-ai-paas get pods
```

## 五、配置 Ingress 与本地访问

`ingress.yaml` 默认 `host: fuze-ai-paas.local`，需配合已安装的 Ingress Controller（如 ingress-nginx）。

本地访问时，将以下条目加入 `/etc/hosts`：

```text
<集群节点IP或Ingress IP>  fuze-ai-paas.local
```

访问地址：

- 前端界面：`http://fuze-ai-paas.local/`
- 后端 API：`http://fuze-ai-paas.local/api/v1/...`
- 监控大盘：`http://fuze-ai-paas.local/grafana`（Grafana 管理员 admin/admin）

> 前端 Service 同时暴露了 NodePort `30300`，未启用 Ingress 时可直接 `http://<节点IP>:30300` 访问。

## 六、可选能力组件（按需部署）

后端在未检测到对应 CRD 时会自动降级为 mock 模式，因此下列组件均为可选。安装对应组件后，再 `apply` 其示例清单：

| 能力 | 依赖组件 | 示例清单 |
| :--- | :--- | :--- |
| 批调度 / 分布式训练 | Volcano | `k8s/volcano-distributed-training.yaml` |
| 推理服务 | KServe + Knative | `k8s/kserve-inferenceservice-example.yaml` |
| GPU 显存隔离 | HAMi | `k8s/hami-device-plugin.yaml` |
| 数据加速 | Fluid (+ Alluxio/JuiceFS) | `k8s/fluid-dataset-example.yaml` |

示例（需先安装 Volcano）：

```bash
kubectl apply -f k8s/volcano-distributed-training.yaml
```

## 七、配置与定制

### 资源规格
后端默认 `requests: cpu 100m / mem 128Mi`、`limits: cpu 500m / mem 512Mi`；前端默认 `requests: cpu 50m / mem 64Mi`、`limits: cpu 200m / mem 256Mi`。可按节点规模调整 `*-deployment.yaml` 中的 `resources`。

### 副本数
`backend-deployment.yaml` / `frontend-deployment.yaml` 中 `replicas: 1`，生产环境建议提高副本数并配合 HPA。

### 环境变量
后端通过 `env` 注入 `GIN_MODE=release`，可按需扩展（如日志级别、存储后端等）。

### 监控接入
`monitoring.yaml` 已包含 Prometheus 抓取配置（探测带 `prometheus.io/scrape` 注解的 Pod，直接抓取 `fuze-backend:8080/metrics`）以及 Grafana 数据源与「Fuze AI PaaS」大盘（GPU 利用率、显存利用率、运行中任务、就绪推理服务、GPU 趋势、数据集缓存命中率）。

## 八、运维操作

### 升级
修改镜像版本后滚动更新：

```bash
kubectl -n fuze-ai-paas set image deployment/fuze-backend fuze-backend=fuze-ai-paas-backend:<新版本>
kubectl -n fuze-ai-paas set image deployment/fuze-frontend fuze-frontend=fuze-ai-paas-frontend:<新版本>
```

### 扩缩容
```bash
kubectl -n fuze-ai-paas scale deployment/fuze-backend --replicas=3
```

### 查看日志
```bash
kubectl -n fuze-ai-paas logs -f deployment/fuze-backend
kubectl -n fuze-ai-paas logs -f deployment/fuze-frontend
```

### 健康检查
```bash
kubectl -n fuze-ai-paas get pods
curl -s http://fuze-ai-paas.local/api/v1/health
```

### 清理
```bash
kubectl delete -k k8s/
```

## 九、故障排查

| 现象 | 排查建议 |
| :--- | :--- |
| 后端 Pod 一直 `ContainerCreating` | 检查镜像是否存在/可拉取、镜像仓库凭据、节点资源是否充足 |
| 后端 `CrashLoopBackOff` | 查看 `kubectl logs`，确认 `DB_DSN` 指向的 Postgres 可达、Secret 已配置、端口未被占用 |
| Ingress 无法访问 | 确认 ingress-nginx 已安装、`/etc/hosts` 已配置、后端 `Readiness` 通过 |
| `/api` 返回 502 | 检查 `fuze-backend` Service 与 Pod 标签匹配、端口 `8080` 是否正确 |
| 推理/训练 CRD 报错 | 后端已降级 mock 模式，需安装对应组件后再提交任务 |
| Grafana 打不开 | 确认 `GF_SERVER_ROOT_URL` 子路径 `/grafana` 与 Ingress path 一致；匿名访问默认开启 |

## 十、目录与文件清单

```text
k8s/
├── kustomization.yaml                 # 统一入口
├── namespace.yaml                     # 命名空间
├── rbac.yaml                          # 后端 ServiceAccount + Role
├── backend-deployment.yaml            # 后端部署
├── backend-service.yaml               # 后端服务(ClusterIP)
├── frontend-deployment.yaml           # 前端部署
├── frontend-service.yaml              # 前端服务(NodePort 30300)
├── ingress.yaml                       # Ingress 路由
├── volcano-queue.yaml                 # Volcano 队列
├── monitoring.yaml                    # Prometheus + Grafana
├── hami-device-plugin.yaml            # (可选) GPU 隔离
├── kserve-inferenceservice-example.yaml  # (可选) 推理示例
├── fluid-dataset-example.yaml         # (可选) 数据加速示例
└── volcano-distributed-training.yaml  # (可选) 分布式训练示例
```

---

> 相关文档：架构设计见 `design/architecture/architecture_CN.md`。
