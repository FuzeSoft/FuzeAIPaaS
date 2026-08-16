# Fuze AI PaaS — Kubernetes Deployment Guide

This document explains how to deploy Fuze AI PaaS (the AI computing power scheduling platform) to a Kubernetes cluster. All manifests live in the repository's `k8s/` directory and are managed together via Kustomize.

> Image and namespace conventions:
> - Backend image: `fuze-ai-paas-backend:latest`
> - Frontend image: `fuze-ai-paas-frontend:latest`
> - Namespace: `fuze-ai-paas`

---

## I. Prerequisites

- A working Kubernetes cluster (v1.24+) with `kubectl` access.
- Kustomize installed (or `kubectl` ≥ 1.14, which ships `kubectl apply -k`).
- Access to a container image registry (build locally for an on-prem cluster; push to a registry for remote clusters).
- Cluster nodes must satisfy the backend/frontend resource requests (CPU/memory, see manifests below).
- Optional components installed as needed: Volcano (batch scheduling), KServe+Knative (inference), HAMi (GPU isolation), Fluid (data acceleration).

## II. Architecture Overview

```text
Browser / Client
      │
      ▼
  Ingress (fuze-ai-paas.local)
   ├─ /        → fuze-frontend:3000 (NodePort 30300)
   ├─ /api     → fuze-backend:8080
   └─ /grafana → grafana:3000
      │
      ▼
  fuze-backend ──(CRD)──▶ Volcano / KServe / Fluid / HAMi
   (ServiceAccount + RBAC permissions)
      │
      ▼
  Prometheus + Grafana monitoring stack (scrapes /metrics)
```

Core component manifests:

| Component | Manifest | Description |
| :--- | :--- | :--- |
| Namespace | `namespace.yaml` | Namespace for all resources |
| Backend RBAC | `rbac.yaml` | ServiceAccount + Role to manage Volcano/KServe/Fluid CRDs |
| Backend | `backend-deployment.yaml` / `backend-service.yaml` | Go (Gin) service, :8080 |
| Frontend | `frontend-deployment.yaml` / `frontend-service.yaml` | React static service, :3000 (NodePort 30300) |
| Ingress | `ingress.yaml` | Routes `/`, `/api`, `/grafana` |
| Queues | `volcano-queue.yaml` | Inference/training/batch Queues |
| Monitoring | `monitoring.yaml` | Prometheus + Grafana (included in kustomization) |

## III. Build Images

Run from the repository root:

```bash
# Backend image
docker build -f Dockerfile.backend -t fuze-ai-paas-backend:latest .

# Frontend image
docker build -f Dockerfile.frontend -t fuze-ai-paas-frontend:latest .
```

> Note: `Dockerfile.backend` uses a multi-stage build; the final binary is `fuze-scheduler` listening on port `8080`. `Dockerfile.frontend` serves `dist` via `serve` listening on port `3000`.

If deploying to a remote cluster, push the images to a registry and update the `image` field in `backend-deployment.yaml` / `frontend-deployment.yaml` accordingly (configure `imagePullSecrets` if needed).

You can also use the Makefile:

```bash
make build            # local compile of backend + frontend
make docker-build     # docker build -t fuze-ai-paas:latest . (single image)
```

## IV. Deploy Core Platform (Required)

```bash
# Option 1: from the repo root
kubectl apply -k k8s/

# Option 2: from the k8s directory
cd k8s && kubectl apply -k .
```

This command, via `kustomization.yaml`, creates in order: namespace, RBAC, backend, frontend, Ingress, Volcano queues, and the monitoring stack.

Verify resources are ready:

```bash
kubectl -n fuze-ai-paas get all
kubectl -n fuze-ai-paas get pods
```

## V. Configure Ingress and Local Access

`ingress.yaml` defaults to `host: fuze-ai-paas.local` and requires an installed Ingress Controller (e.g., ingress-nginx).

For local access, add the following to `/etc/hosts`:

```text
<cluster node IP or Ingress IP>  fuze-ai-paas.local
```

Access URLs:

- Frontend UI: `http://fuze-ai-paas.local/`
- Backend API: `http://fuze-ai-paas.local/api/v1/...`
- Monitoring dashboard: `http://fuze-ai-paas.local/grafana` (Grafana admin admin/admin)

> The frontend Service also exposes NodePort `30300`; if Ingress is not enabled, access directly via `http://<nodeIP>:30300`.

## VI. Optional Capability Components (Deploy as Needed)

The backend automatically degrades to mock mode when the corresponding CRD is not detected, so the following components are all optional. After installing a component, `apply` its example manifest:

| Capability | Dependency | Example Manifest |
| :--- | :--- | :--- |
| Batch scheduling / distributed training | Volcano | `k8s/volcano-distributed-training.yaml` |
| Inference service | KServe + Knative | `k8s/kserve-inferenceservice-example.yaml` |
| GPU memory isolation | HAMi | `k8s/hami-device-plugin.yaml` |
| Data acceleration | Fluid (+ Alluxio/JuiceFS) | `k8s/fluid-dataset-example.yaml` |

Example (install Volcano first):

```bash
kubectl apply -f k8s/volcano-distributed-training.yaml
```

## VII. Configuration and Customization

### Resource Sizing
The backend defaults to `requests: cpu 100m / mem 128Mi`, `limits: cpu 500m / mem 512Mi`; the frontend defaults to `requests: cpu 50m / mem 64Mi`, `limits: cpu 200m / mem 256Mi`. Adjust `resources` in the `*-deployment.yaml` files based on node scale.

### Replicas
`backend-deployment.yaml` / `frontend-deployment.yaml` set `replicas: 1`; for production, increase replicas and pair with an HPA.

### Environment Variables
The backend injects `GIN_MODE=release` via `env`; extend as needed (e.g., log level, storage backend).

### Monitoring Integration
`monitoring.yaml` already includes Prometheus scrape config (probing Pods annotated with `prometheus.io/scrape`, directly scraping `fuze-backend:8080/metrics`) plus Grafana datasource and the "Fuze AI PaaS" dashboard (GPU utilization, memory utilization, running jobs, ready inference services, GPU trends, dataset cache hit rate).

## VIII. Operations

### Upgrade
After changing the image tag, roll out the update:

```bash
kubectl -n fuze-ai-paas set image deployment/fuze-backend fuze-backend=fuze-ai-paas-backend:<new-version>
kubectl -n fuze-ai-paas set image deployment/fuze-frontend fuze-frontend=fuze-ai-paas-frontend:<new-version>
```

### Scale
```bash
kubectl -n fuze-ai-paas scale deployment/fuze-backend --replicas=3
```

### View Logs
```bash
kubectl -n fuze-ai-paas logs -f deployment/fuze-backend
kubectl -n fuze-ai-paas logs -f deployment/fuze-frontend
```

### Health Check
```bash
kubectl -n fuze-ai-paas get pods
curl -s http://fuze-ai-paas.local/api/v1/health
```

### Teardown
```bash
kubectl delete -k k8s/
```

## IX. Troubleshooting

| Symptom | Suggestion |
| :--- | :--- |
| Backend Pod stuck in `ContainerCreating` | Check image existence/pullability, registry credentials, and node resource availability |
| Backend `CrashLoopBackOff` | Inspect `kubectl logs`; ensure `DB_DSN` points to a reachable Postgres, the Secret is configured, and the port is free |
| Ingress unreachable | Confirm ingress-nginx is installed, `/etc/hosts` is configured, and backend `Readiness` passes |
| `/api` returns 502 | Verify `fuze-backend` Service/Pod label match and that port `8080` is correct |
| Inference/training CRD errors | The backend has degraded to mock mode; install the corresponding component before submitting tasks |
| Grafana won't open | Ensure `GF_SERVER_ROOT_URL` sub-path `/grafana` matches the Ingress path; anonymous access is on by default |

## X. Directory and File Manifest

```text
k8s/
├── kustomization.yaml                 # unified entrypoint
├── namespace.yaml                     # namespace
├── rbac.yaml                          # backend ServiceAccount + Role
├── backend-deployment.yaml            # backend deployment
├── backend-service.yaml               # backend service (ClusterIP)
├── frontend-deployment.yaml           # frontend deployment
├── frontend-service.yaml              # frontend service (NodePort 30300)
├── ingress.yaml                       # ingress routing
├── volcano-queue.yaml                 # Volcano queues
├── monitoring.yaml                    # Prometheus + Grafana
├── hami-device-plugin.yaml            # (optional) GPU isolation
├── kserve-inferenceservice-example.yaml  # (optional) inference example
├── fluid-dataset-example.yaml         # (optional) data acceleration example
└── volcano-distributed-training.yaml  # (optional) distributed training example
```

---

> Related docs: Architecture design is in `design/architecture/architecture_EN.md`.
