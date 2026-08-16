#!/usr/bin/env bash
#
# e2e-kserve.sh
# ============================================================
# KServe 推理服务 端到端联调脚本
#
# 覆盖链路：
#   1. 健康检查 & 探测调度模式 (mock / volcano)
#   2. 提交推理服务        POST /api/v1/inference-services
#   3. 校验元数据持久化     GET  /api/v1/inference-services/:id
#   4. Volcano 模式深度校验 (kubectl 探测 inferenceservice:
#        - spec.predictor.minReplicas / maxReplicas
#        - spec.predictor.model.modelFormat.name (非 custom 时)
#        - spec.predictor.model.storageUri
#        - HAMi 资源: nvidia.com/gpu / gpumem / gpucores (启用时)
#   5. 状态轮询             (等待 Ready / status.url 就绪)
#   6. 指标校验             GET /metrics (fuze_inference_services_*)
#   7. 清理                 DELETE /api/v1/inference-services/:id
#
# 使用方式：
#   ./scripts/e2e-kserve.sh
#   FRAMEWORK=pytorch STORAGE_URI=s3://bucket/model ./scripts/e2e-kserve.sh
#   GPUS=1 GPU_MEMORY=4096 GPU_CORES=50 ./scripts/e2e-kserve.sh   # 带 HAMi 显存隔离
#   ./scripts/e2e-kserve.sh --no-cleanup
#
set -uo pipefail

# ============================================================
# 配置（可通过环境变量覆盖）
# ============================================================
BACKEND_URL="${BACKEND_URL:-http://localhost:8080}"
NAMESPACE="${K8S_NAMESPACE:-fuze-ai-paas}"
API="${BACKEND_URL}/api/v1"

FRAMEWORK="${FRAMEWORK:-sklearn}"        # sklearn|pytorch|tensorflow|triton|xgboost|onnx|custom
STORAGE_URI="${STORAGE_URI:-s3://e2e-bucket/models/${FRAMEWORK}}"
IMAGE="${INFERENCE_IMAGE:-}"             # 仅 framework=custom 时使用
MIN_REPLICAS="${MIN_REPLICAS:-0}"        # 0 = Scale-to-Zero
MAX_REPLICAS="${MAX_REPLICAS:-2}"
CPU="${CPU:-1}"
MEMORY="${MEMORY:-2Gi}"

# HAMi GPU 显存/算力隔离（可选）
GPUS="${GPUS:-0}"
GPU_MEMORY="${GPU_MEMORY:-0}"            # MiB
GPU_CORES="${GPU_CORES:-0}"              # %

POLL_INTERVAL="${POLL_INTERVAL:-5}"
TIMEOUT="${TIMEOUT:-600}"

CLEANUP="${CLEANUP:-true}"
VERBOSE="${VERBOSE:-false}"

# ============================================================
# 颜色 & 日志
# ============================================================
if [ -t 1 ]; then
  C_RESET=$'\033[0m'; C_GREEN=$'\033[32m'; C_RED=$'\033[31m'
  C_YELLOW=$'\033[33m'; C_CYAN=$'\033[36m'; C_BOLD=$'\033[1m'
else
  C_RESET=""; C_GREEN=""; C_RED=""; C_YELLOW=""; C_CYAN=""; C_BOLD=""
fi
log_info() { echo "${C_CYAN}[INFO]${C_RESET} $*"; }
log_ok()   { echo "${C_GREEN}[PASS]${C_RESET} $*"; }
log_warn() { echo "${C_YELLOW}[WARN]${C_RESET} $*"; }
log_err()  { echo "${C_RED}[FAIL]${C_RESET} $*"; }
log_step() { echo; echo "${C_BOLD}==== $* ====${C_RESET}"; }

PASS=0
FAIL=0
record_pass() { PASS=$((PASS+1)); log_ok "$1"; }
record_fail() { FAIL=$((FAIL+1)); log_err "$1"; }

need_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log_err "缺少必要命令: $1，请先安装后重试。"; exit 2
  fi
}
need_tool curl
need_tool jq

BODY=""; CODE=""
api_call() {
  local method="$1" path="$2" data="${3:-}"
  local url="${API}${path}"
  if [ -n "$data" ]; then
    resp=$(curl -s -w $'\n__HTTP_CODE__%{http_code}' -X "$method" "$url" \
      -H 'Content-Type: application/json' --data "$data") || true
  else
    resp=$(curl -s -w $'\n__HTTP_CODE__%{http_code}' -X "$method" "$url") || true
  fi
  CODE=$(printf '%s' "$resp" | sed -n 's/^__HTTP_CODE__//p')
  BODY=$(printf '%s' "$resp" | sed '/^__HTTP_CODE__/d')
  [ "$VERBOSE" = "true" ] && log_info "HTTP $method $path -> $CODE"
}
assert_eq() { if [ "$2" = "$3" ]; then record_pass "$1 (期望=$2, 实际=$3)";
  else record_fail "$1 (期望=$2, 实际=$3)"; fi; }
assert_ge() { if [ "$3" -ge "$2" ] 2>/dev/null; then record_pass "$1 (>= $2, 实际=$3)";
  else record_fail "$1 (>= $2, 实际=$3)"; fi; }
assert_contains() { if printf '%s' "$3" | grep -q -- "$2"; then record_pass "$1";
  else record_fail "$1 (未找到: $2)"; fi; }

for arg in "$@"; do
  case "$arg" in
    --no-cleanup) CLEANUP="false" ;;
    --help|-h) sed -n '3,40p' "$0"; exit 0 ;;
    *) log_warn "未知参数: $arg" ;;
  esac
done

# ============================================================
# 0. 健康检查 & 模式
# ============================================================
log_step "0. 前置检查 & 调度模式探测"
api_call GET "/health"
if [ "$CODE" != "200" ]; then
  log_err "后端不可达 ($BACKEND_URL)，请先执行 scripts/start.sh 启动服务。"; exit 3
fi
MODE=$(printf '%s' "$BODY" | jq -r '.mode // "unknown"')
log_info "后端健康，调度模式 = ${C_BOLD}$MODE${C_RESET}"

# ============================================================
# 1. 提交推理服务
# ============================================================
log_step "1. 提交 KServe 推理服务 (framework=$FRAMEWORK, min=$MIN_REPLICAS, max=$MAX_REPLICAS)"

PAYLOAD=$(jq -n \
  --arg name "e2e-isvc-$FRAMEWORK" \
  --arg framework "$FRAMEWORK" \
  --arg storage_uri "$STORAGE_URI" \
  --arg image "$IMAGE" \
  --arg cpu "$CPU" \
  --arg memory "$MEMORY" \
  --argjson min_replicas "$MIN_REPLICAS" \
  --argjson max_replicas "$MAX_REPLICAS" \
  --argjson gpus "$GPUS" \
  --argjson gpu_memory "$GPU_MEMORY" \
  --argjson gpu_cores "$GPU_CORES" \
  '{spec: {name:$name, framework:$framework, storage_uri:$storage_uri, image:$image,
    cpu:$cpu, memory:$memory, min_replicas:$min_replicas, max_replicas:$max_replicas,
    gpus:$gpus, gpu_memory:$gpu_memory, gpu_cores:$gpu_cores}}')

[ "$VERBOSE" = "true" ] && log_info "Payload: $PAYLOAD"

api_call POST "/inference-services" "$PAYLOAD"
if [ "$CODE" != "201" ]; then
  record_fail "提交推理服务失败 (HTTP $CODE): $BODY"; exit 4
fi
record_pass "推理服务创建成功 (HTTP 201)"

SVC_ID=$(printf '%s' "$BODY" | jq -r '.id')
log_info "推理服务 ID    = $SVC_ID"
log_info "创建后状态     = $(printf '%s' "$BODY" | jq -r '.status.phase')（期望态已声明，部署由控制循环完成）"

# 声明式：创建只写期望态，运行时名称由 reconcile 回填。
# 仅 volcano 模式有真实调度器回填 runtime_name；mock 模式无集群，跳过等待。
KNAME=""
if [ "$MODE" = "volcano" ]; then
  wait_elapsed=0
  while [ "$wait_elapsed" -lt 30 ]; do
    api_call GET "/inference-services/$SVC_ID"
    KNAME=$(printf '%s' "$BODY" | jq -r '.status.runtime_name // ""')
    [ -n "$KNAME" ] && break
    sleep 2; wait_elapsed=$((wait_elapsed + 2))
  done
  log_info "运行时名称     = ${KNAME:-<空>}（等待 ${wait_elapsed}s）"
else
  log_info "运行时名称     = <空>（mock 模式无调度器回填，跳过等待）"
fi

# ============================================================
# 2. 元数据持久化校验
# ============================================================
log_step "2. 校验期望态持久化 (spec)"
api_call GET "/inference-services/$SVC_ID"
assert_eq "HTTP 状态" "200" "$CODE"
assert_eq "framework 字段" "$FRAMEWORK" "$(printf '%s' "$BODY" | jq -r '.spec.framework')"
assert_eq "min_replicas" "$MIN_REPLICAS" "$(printf '%s' "$BODY" | jq -r '.spec.min_replicas')"
assert_eq "max_replicas" "$MAX_REPLICAS" "$(printf '%s' "$BODY" | jq -r '.spec.max_replicas')"
assert_eq "storage_uri" "$STORAGE_URI" "$(printf '%s' "$BODY" | jq -r '.spec.storage_uri')"
assert_eq "gpu_memory" "$GPU_MEMORY" "$(printf '%s' "$BODY" | jq -r '.spec.gpu_memory')"
assert_eq "gpu_cores" "$GPU_CORES" "$(printf '%s' "$BODY" | jq -r '.spec.gpu_cores')"

# ============================================================
# 3. Volcano 模式深度校验
# ============================================================
if [ "$MODE" = "volcano" ]; then
  log_step "3. KServe 结构深度校验 (kubectl)"
  if ! command -v kubectl >/dev/null 2>&1; then
    log_warn "未安装 kubectl，跳过 inferenceservice 结构校验。"
  elif [ -z "$KNAME" ]; then
    record_fail "Volcano 模式但 status.runtime_name 为空 —— reconcile 未能完成部署（检查 KServe CRD 或权限）。"
  else
      ISVC_JSON=$(kubectl -n "$NAMESPACE" get inferenceservice "$KNAME" -o json 2>/dev/null) || ISVC_JSON=""
      if [ -n "$ISVC_JSON" ]; then
        minr=$(printf '%s' "$ISVC_JSON" | jq -r '.spec.predictor.minReplicas')
        maxr=$(printf '%s' "$ISVC_JSON" | jq -r '.spec.predictor.maxReplicas')
        assert_eq "minReplicas" "$MIN_REPLICAS" "$minr"
        assert_eq "maxReplicas" "$MAX_REPLICAS" "$maxr"

        if [ "$FRAMEWORK" != "custom" ]; then
          mf=$(printf '%s' "$ISVC_JSON" | jq -r '.spec.predictor.model.modelFormat.name // ""')
          su=$(printf '%s' "$ISVC_JSON" | jq -r '.spec.predictor.model.storageUri // ""')
          assert_eq "modelFormat.name=$FRAMEWORK" "$FRAMEWORK" "$mf"
          assert_eq "storageUri" "$STORAGE_URI" "$su"
        fi

        # HAMi 资源校验
        if [ "$GPUS" -gt 0 ]; then
          lim=$(printf '%s' "$ISVC_JSON" | jq -c '.spec.predictor | (.model.resources.limits // .containers[0].resources.limits)')
        ngpu=$(printf '%s' "$lim" | jq -r '.["nvidia.com/gpu"] // ""')
        gmem=$(printf '%s' "$lim" | jq -r '.["nvidia.com/gpumem"] // ""')
        gcore=$(printf '%s' "$lim" | jq -r '.["nvidia.com/gpucores"] // ""')
        assert_eq "nvidia.com/gpu" "$GPUS" "$ngpu"
        if [ "$GPU_MEMORY" -gt 0 ]; then assert_eq "nvidia.com/gpumem" "$GPU_MEMORY" "$gmem"; fi
        if [ "$GPU_CORES" -gt 0 ]; then assert_eq "nvidia.com/gpucores" "$GPU_CORES" "$gcore"; fi
      fi
    else
      record_fail "无法获取 inferenceservice $KNAME。"
    fi
  fi
else
  log_step "3. Mock 模式说明"
  log_info "后端处于 mock 模式，不会向集群提交 KServe InferenceService（无 CRD 结构可校验）。"
fi

# ============================================================
# 4. 状态轮询
# ============================================================
log_step "4. 状态轮询 ($TIMEOUTs 超时)"
elapsed=0
final_status=""
while [ "$elapsed" -lt "$TIMEOUT" ]; do
  if [ "$MODE" = "volcano" ] && [ -n "$KNAME" ] && command -v kubectl >/dev/null 2>&1; then
    url=$(kubectl -n "$NAMESPACE" get inferenceservice "$KNAME" \
      -o jsonpath='{.status.url}' 2>/dev/null)
    phase="$url"
  else
    api_call GET "/inference-services/$SVC_ID"
    phase=$(printf '%s' "$BODY" | jq -r '.status.phase // "unknown"')
  fi
  if [ "$VERBOSE" = "true" ]; then log_info "t=${elapsed}s -> ${phase:-<未就绪>}"; fi
  if [ -n "$phase" ] && [ "$phase" != "unknown" ] && [ "$phase" != "pending" ]; then
    final_status="$phase"
    if [ "$MODE" = "volcano" ]; then
      record_pass "推理服务已就绪 (url=$phase)"
    else
      record_pass "任务状态 = $phase (mock 模式无集群 Reconcile)"
    fi
    break
  fi
  sleep "$POLL_INTERVAL"
  elapsed=$((elapsed + POLL_INTERVAL))
done
if [ -z "$final_status" ] && [ "$MODE" = "volcano" ]; then
  record_fail "在 ${TIMEOUT}s 内推理服务未就绪（可能镜像拉取/模型下载较慢）。"
elif [ -z "$final_status" ]; then
  log_info "mock 模式不触发集群就绪，跳过状态判定。"
fi

# ============================================================
# 5. 指标校验
# ============================================================
log_step "5. 指标校验 (GET /metrics)"
api_call GET "/metrics"
assert_eq "metrics HTTP 状态" "200" "$CODE"
assert_contains "包含 fuze_inference_services_total" "fuze_inference_services_total" "$BODY"
assert_contains "包含 fuze_inference_services_ready" "fuze_inference_services_ready" "$BODY"

# ============================================================
# 6. 清理
# ============================================================
if [ "$CLEANUP" = "false" ]; then
  log_step "6. 跳过清理 (--no-cleanup)"
  log_info "保留推理服务: $SVC_ID (KServe: ${KNAME:-<空>})"
else
  log_step "6. 清理推理服务 (DELETE /api/v1/inference-services/$SVC_ID)"
  api_call DELETE "/inference-services/$SVC_ID"
  assert_eq "删除 HTTP 状态" "204" "$CODE"
  if [ "$MODE" = "volcano" ] && [ -n "$KNAME" ] && command -v kubectl >/dev/null 2>&1; then
    deleted=0; wait=0
    while [ "$wait" -lt 60 ]; do
      if ! kubectl -n "$NAMESPACE" get inferenceservice "$KNAME" >/dev/null 2>&1; then deleted=1; break; fi
      sleep 3; wait=$((wait+3))
    done
    if [ "$deleted" -eq 1 ]; then record_pass "inferenceservice $KNAME 已回收";
    else record_fail "inferenceservice $KNAME 在 60s 内未被删除"; fi
  fi
fi

log_step "联调结果汇总"
echo "  ${C_GREEN}PASS${C_RESET}: $PASS   ${C_RED}FAIL${C_RESET}: $FAIL"
if [ "$FAIL" -gt 0 ]; then log_err "KServe 端到端联调存在失败项。"; exit 1; fi
log_ok "KServe 端到端联调全部通过。"
