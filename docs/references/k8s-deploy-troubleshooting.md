# fuze-ai-paas Kubernetes 部署排错记录

> 记录从 `kubectl apply -k k8s/` 开始，部署 fuze-ai-paas（含 Volcano 批调度）过程中遇到的所有错误、根因与解决方案。
> 环境假设：本地 kind / k3d 集群（或节点无法直连公网），使用 Volcano v1.10.0。

---

## 一、问题总览（时间线）

| # | 报错关键词 | 根因 | 解决手段 |
| - | --------- | ---- | -------- |
| 1 | `no matches for kind "Queue" in version "scheduling.volcano.sh/v1beta1"` | 集群未安装 Volcano，CRD 不存在 | 安装 Volcano（含 CRDs） |
| 2 | `failed calling webhook "mutatequeue.volcano.sh" ... connection refused` | admission webhook Pod 未运行 / Service 无 endpoint | 排查 admission 组件状态 |
| 3 | `MountVolume.SetUp failed ... secret "volcano-admission-secret" not found` | Volcano 初始化 Job 未生成 TLS Secret | 手动生成证书并创建 Secret |
| 4 | webhook 配置 `not found` | 早期为绕过 webhook 执行过 `kubectl delete mutatingwebhookconfiguration` | 用自签证书重建 webhook 配置 |
| 5 | `ErrImagePull`（admission） | 节点拉不到 `volcanosh/...` 镜像 | 镜像加国内代理前缀 |
| 6 | `no route to host`（webhook）+ `vc-scheduler` panic | webhook 不可达，调度器创建默认队列失败 | webhook `failurePolicy: Ignore` |
| 7 | `deployments.apps "vc-scheduler" not found` | 调度器 Deployment 真实名为 `volcano-scheduler` | 用真实名称重启 |
| 8 | `ErrImagePull`（volcano-scheduler） | 调度器镜像同样拉不到 | 所有 Volcano Deployment 统一换国内代理 |
| 9 | `ErrImagePull`（fuze-backend / fuze-frontend） | 业务镜像为本地镜像，未灌入集群 | 本地构建并 `kind load` 进集群 |

---

## 二、各问题详细说明

### 问题 1：CRD 缺失

**报错**
```
no matches for kind "Queue" in version "scheduling.volcano.sh/v1beta1"
ensure CRDs are installed first
```

**根因**：`k8s/kustomization.yaml` 把 `volcano-queue.yaml` 列在必装 `resources` 中，而 `Queue` 是 Volcano 的 CRD，集群尚未安装 Volcano。

**解决**：先安装 Volcano（会自动创建 Queue/Job 等 CRD）。
```bash
kubectl apply -f https://raw.githubusercontent.com/volcano-sh/volcano/v1.10.0/installer/volcano-v1.10.0.yaml
kubectl get crds | grep volcano
```
> 说明：虽然 kustomization 注释把 `volcano-distributed-training.yaml` 标为可选，但 `volcano-queue.yaml` 在必装清单里，且后端调度器依赖它，因此 Volcano 必须安装。

---

### 问题 2 & 3：admission webhook 起不来（connection refused / Secret 缺失）

**报错**
```
failed calling webhook "mutatequeue.volcano.sh": ... connection refused
# 进一步定位：
MountVolume.SetUp failed for volume "admission-certs": secret "volcano-admission-secret" not found
```

**根因链**：
- admission Pod 卡在 `ContainerCreating`，因为挂载卷 `admission-certs` 需要的 Secret `volcano-admission-secret` 不存在；
- 该 Secret 本应由 Volcano 的初始化 Job（`volcano-admission-init`）生成，但初始化 Job 失败（镜像拉取失败等原因），所以从未创建 → Pod 永远起不来 → webhook 连不上。

**解决**：手动生成自签证书并创建 Secret（key 名必须为 `serverKey` / `serverCert`，供 admission 容器读取）。

bash 版（需 openssl / python3）：
```bash
NS=volcano-system
SVC=volcano-admission-service
KEY_FILE=server.key
CRT_FILE=server.crt

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$KEY_FILE" -out "$CRT_FILE" -days 3650 \
  -subj "/CN=${SVC}.${NS}.svc" \
  -addext "subjectAltName=DNS:${SVC}.${NS}.svc,DNS:${SVC}.${NS}.svc.cluster.local"

kubectl delete secret volcano-admission-secret -n $NS --ignore-not-found
kubectl create secret generic volcano-admission-secret \
  --from-file=serverKey=$KEY_FILE \
  --from-file=serverCert=$CRT_FILE \
  -n $NS
```

---

### 问题 4：webhook 配置被误删后 `not found`

**报错**
```
mutatingwebhookconfigurations.admissionregistration.k8s.io "volcano-admission-service" not found
```

**根因**：早期为临时绕过 webhook 执行过 `kubectl delete mutatingwebhookconfiguration volcano-admission-service`，配置已被删除。

**解决**：用同一张证书重建 mutating / validating webhook 配置（`caBundle` 必须与上面证书的 base64 一致）：
```bash
CA=$(base64 -w0 server.crt)
cat <<EOF | kubectl apply -f -
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: volcano-admission-service
webhooks:
- name: mutatequeue.volcano.sh
  clientConfig:
    caBundle: $CA
    service:
      name: volcano-admission-service
      namespace: volcano-system
      path: /queues/mutate
      port: 443
  rules:
  - apiGroups: ["scheduling.volcano.sh"]
    apiVersions: ["v1beta1"]
    operations: ["CREATE"]
    resources: ["queues"]
  failurePolicy: Fail
  sideEffects: None
  admissionReviewVersions: ["v1"]
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: volcano-admission-service
webhooks:
- name: validatequeue.volcano.sh
  clientConfig:
    caBundle: $CA
    service:
      name: volcano-admission-service
      namespace: volcano-system
      path: /queues/validate
      port: 443
  rules:
  - apiGroups: ["scheduling.volcano.sh"]
    apiVersions: ["v1beta1"]
    operations: ["CREATE", "UPDATE"]
    resources: ["queues"]
  failurePolicy: Fail
  sideEffects: None
  admissionReviewVersions: ["v1"]
EOF
```
> 注意 heredoc 用 `<<EOF`（无引号）才能让 `$CA` 展开。

---

### 问题 5 & 8：Volcano 镜像拉取失败（ErrImagePull）

**报错**
```
container "admission" in pod "volcano-admission-xxx" ... ErrImagePull
container "volcano-scheduler" in pod "volcano-scheduler-xxx" ... trying and failing to pull image
```

**根因**：Volcano 官方镜像在 `docker.io/volcanosh/...`，国内节点直连拉取失败。该问题会同时影响 `vc-admission-manager`、`volcano-scheduler`、`vc-controller-manager` 等所有组件。

**解决**：把镜像前缀换成国内 Docker Hub 代理。常用公共代理（任选其一）：

| 代理 | 前缀 |
| ---- | ---- |
| DaoCloud | `docker.m.daocloud.io` |
| 网易 | `hub-mirror.c.163.com` |
| 百度 | `mirror.baidubce.com` |
| 腾讯云 | `mirror.ccs.tencentyun.com` |

一次性改写 `volcano-system` 下所有 Deployment 的镜像：
```bash
for dep in $(kubectl get deployments -n volcano-system -o name); do
  kubectl get "$dep" -n volcano-system -o json | python3 -c "
import sys,json
d=json.load(sys.stdin)
for c in d['spec']['template']['spec']['containers']:
    img=c['image']
    if 'docker.m.daocloud.io' not in img:
        repo = img[len('docker.io/'):] if img.startswith('docker.io/') else img
        c['image']='docker.m.daocloud.io/'+repo
json.dump(d,sys.stdout)
" | kubectl replace -f -
done

kubectl rollout restart deployment -n volcano-system
```

> 治本方案：在节点 containerd 配置 `docker.io` 的 mirror，之后无需改镜像名：
> ```toml
> [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
>   endpoint = ["https://docker.m.daocloud.io"]
> ```
> 然后 `systemctl restart containerd`。

---

### 问题 6：`no route to host` + 调度器 panic

**报错**
```
failed calling webhook "mutatequeue.volcano.sh": ... no route to host
# vc-scheduler 日志：
panic: failed init default queue, with err: ... no route to host
```

**根因**：webhook Service 已有 endpoint（admission Pod 拿到 IP），但网络不可达。更严重的是——**`vc-scheduler` 启动时必须调用 webhook 创建默认队列**，webhook 不可达会导致调度器直接 panic（CrashLoopBackOff）。这会让 Volcano 调度整体不可用。

**解决**：把 webhook 的 `failurePolicy` 改为 `Ignore`，让 apiserver 在 webhook 不可达时放行（不影响业务运行，仅临时跳过队列校验）。

> ⚠️ 不能用 `kubectl patch --type=merge`：merge patch 对**数组**是整体替换，会把整个 `webhooks` 数组替换成只剩 `failurePolicy` 的元素，触发必填字段校验错误。必须用 `kubectl replace` + python 只改目标字段：

```bash
kubectl get mutatingwebhookconfiguration volcano-admission-service -o json | \
python3 -c "import sys,json; d=json.load(sys.stdin)
for wh in d['webhooks']: wh['failurePolicy']='Ignore'
json.dump(d,sys.stdout)" | kubectl replace -f -

kubectl get validatingwebhookconfiguration volcano-admission-service -o json | \
python3 -c "import sys,json; d=json.load(sys.stdin)
for wh in d['webhooks']: wh['failurePolicy']='Ignore'
json.dump(d,sys.stdout)" | kubectl replace -f -
```

重启调度器让配置生效（部署名是 `volcano-scheduler`，不是 `vc-scheduler`）：
```bash
kubectl rollout restart deployment -n volcano-system
kubectl delete pod -n volcano-system -l app=volcano-scheduler --ignore-not-found
```

---

### 问题 7：调度器 Deployment 名称错误

**报错**
```
deployments.apps "vc-scheduler" not found
```

**根因**：Volcano 官方 installer 中调度器 Deployment 名为 `volcano-scheduler`（容器名也是 `volcano-scheduler`），并非 `vc-scheduler`。

**解决**：用真实名称操作，或一条命令重启整个命名空间所有 Deployment：
```bash
kubectl rollout restart deployment -n volcano-system
```

---

### 问题 9：业务镜像（fuze-backend / fuze-frontend）拉取失败

**报错**
```
container "fuze-backend" in pod "fuze-backend-xxx" ... trying and failing to pull image
```

**根因**：`k8s/backend-deployment.yaml` 与 `k8s/frontend-deployment.yaml` 使用的是**本地镜像名** `fuze-ai-paas-backend:latest` / `fuze-ai-paas-frontend:latest`，未推送到任何远程仓库，集群节点拉不到。

> 注意：`make docker-build` 构建的是 `fuze-ai-paas:latest`（且引用了不存在的根 Dockerfile），与清单镜像名**不匹配**，请勿使用 make 来构建。

**解决**：用正确的 Dockerfile 与 tag 构建，并灌入本地集群。

```bash
# 后端（Dockerfile.backend 构建 backend/cmd/main.go）
docker build -f Dockerfile.backend -t fuze-ai-paas-backend:latest .

# 前端
docker build -f Dockerfile.frontend -t fuze-ai-paas-frontend:latest .

docker images | grep fuze-ai-paas
```

- **kind 集群**：`kind load docker-image fuze-ai-paas-backend:latest fuze-ai-paas-frontend:latest`
- **k3d 集群**：`k3d image import fuze-ai-paas-backend:latest fuze-ai-paas-frontend:latest`
- **远程集群**：推到可达仓库后，修改 `k8s/*-deployment.yaml` 的 `image` 字段为 `<仓库>/fuze-ai-paas-backend:latest`

灌入镜像后重启卡住的 Pod：
```bash
kubectl delete pod -n fuze-ai-paas -l app=fuze-backend --ignore-not-found
kubectl delete pod -n fuze-ai-paas -l app=fuze-frontend --ignore-not-found
kubectl get pods -n fuze-ai-paas -w
```

---

## 三、一键排错流程（推荐顺序）

```bash
# 1. 安装 Volcano（CRD）
kubectl apply -f https://raw.githubusercontent.com/volcano-sh/volcano/v1.10.0/installer/volcano-v1.10.0.yaml

# 2. 手动创建 admission TLS Secret（见问题 3）
#    （生成 server.key / server.crt 后）
kubectl create secret generic volcano-admission-secret \
  --from-file=serverKey=server.key --from-file=serverCert=server.crt -n volcano-system

# 3. 重建 webhook 配置并设置 caBundle（见问题 4）
# 4. 把所有 Volcano 镜像换国内代理（见问题 5/8）
# 5. webhook failurePolicy 改 Ignore（见问题 6）
# 6. 构建并灌入业务镜像（见问题 9）
# 7. 验证
kubectl get pods -n volcano-system -w
kubectl get pods -n fuze-ai-paas -w
kubectl apply -k k8s/
kubectl get queues -n fuze-ai-paas
```

---

## 四、验证清单

- [ ] `kubectl get crds | grep volcano` 有输出
- [ ] `kubectl get secret volcano-admission-secret -n volcano-system` 存在
- [ ] `kubectl get pods -n volcano-system` 全部 `Running`
- [ ] `volcano-scheduler` / `vc-controller-manager` / `volcano-admission` 镜像已成功拉取（非 ErrImagePull）
- [ ] `kubectl get pods -n fuze-ai-paas` 中 backend / frontend `Running`
- [ ] `kubectl apply -k k8s/` 不再报 webhook 错误
- [ ] `kubectl get queues -n fuze-ai-paas` 显示 `inference-queue` / `training-queue` / `batch-queue`

---

## 五、关键注意事项

1. **webhook 配置名 vs Secret 名不同**：配置名 `volcano-admission-service`，Secret 名 `volcano-admission-secret`，不要混用。
2. **改 webhook 字段用 `kubectl replace` + python，别用 `kubectl patch --type=merge`**（数组会被整体替换）。
3. **调度器 Deployment 名是 `volcano-scheduler`**，不是 `vc-scheduler`。
4. **`failurePolicy: Ignore` 是临时放行**：admission 网络修好后，应改回 `Fail` 恢复队列校验。
5. **业务镜像必须本地构建 + 灌入集群**（kind/k3d），或修改清单指向可达仓库；`make docker-build` 的 tag 与清单不一致。
6. **国内镜像优先用代理前缀或 containerd mirror**，否则 Volcano 各组件会反复 `ErrImagePull`。
