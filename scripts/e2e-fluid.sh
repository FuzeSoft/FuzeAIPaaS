#!/usr/bin/env bash
#
# e2e-fluid.sh
# ============================================================
# Fluid 数据加速 端到端联调脚本
#
# 覆盖链路：
#   1. 健康检查 & 调度模式 (mock / volcano)
#   2. 创建数据集          POST /api/v1/datasets (Dataset + Runtime)
#   3. 校验元数据持久化     GET  /api/v1/datasets/:id
#   4. Volcano 模式深度校验 (kubectl 探测 dataset + <runtime>runtime:
#        - dataset.spec.mounts[0].mountPoint
#        - dataset.spec.accessModes (ReadOnlyMany / ReadWriteMany)
#        - <runtime>runtime.spec.replicas
#        - <runtime>runtime.spec.tieredstore.levels[0].mediumtype / quota
#   5. 状态轮询             (Fluid Dataset phase = Bound)
#   6. 指标校验             GET /metrics (fuze_dataset_cached_percent)
#   7. 清理                 DELETE /api/v1/datasets/:id
#
# 使用方式：
#   ./scripts/e2e-fluid.sh
#   RUNTIME=juicefs MOUNT_POINT=oss://my-bucket/data ./scripts/e2e-fluid.sh
#   ./scripts/e2e-fluid.sh --no-cleanup
#
set -uo pipefail

BACKEND_URL="${BACKEND_URL:-http://localhost:8080}"
NAMESPACE="${K8S_NAMESPACE:-fuze-ai-paas}"
API="${BACKEND_URL}/api/v1"

MOUNT_POINT="${MOUNT_POINT:-oss://e2e-bucket/models}"
SUB_PATH="${SUB_PATH:-/}"
RUNTIME="${RUNTIME:-alluxio}"          # alluxio|juicefs|goosefs|vineyard
REPLICAS="${REPLICAS:-1}"
CACHE_CAPACITY="${CACHE_CAPACITY:-20Gi}"
CACHE_MEDIUM="${CACHE_MEDIUM:-MEM}"    # MEM|SSD|HDD
CACHE_PATH="${CACHE_PATH:-/dev/shm}"
ACCESS_MODE="${ACCESS_MODE:-ReadOnly}" # ReadOnly | ReadWrite

POLL_INTERVAL="${POLL_INTERVAL:-5}"
TIMEOUT="${TIMEOUT:-420}"

CLEANUP="${CLEANUP:-true}"
VERBOSE="${VERBOSE:-false}"

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

PASS=0; FAIL=0
record_pass() { PASS=$((PASS+1)); log_ok "$1"; }
record_fail() { FAIL=$((FAIL+1)); log_err "$1"; }

need_tool curl; need_tool jq

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
assert_contains() { if printf '%s' "$3" | grep -q -- "$2"; then record_pass "$1";
  else record_fail "$1 (未找到: $2)"; fi; }

# 运行时 -> kubectl 资源复数
runtime_resource() {
  case "$1" in
    juicefs)  echo juicefsruntimes ;;
    goosefs)  echo goosefsruntimes ;;
    vineyard) echo vineyardruntimes ;;
    *)        echo alluxioruntimes ;;
  esac
}
# 访问模式 -> K8s AccessMode
accessmode_k8s() {
  if [ "$ACCESS_MODE" = "ReadWrite" ]; then echo ReadWriteMany; else echo ReadOnlyMany; fi
}

for arg in "$@"; do
  case "$arg" in
    --no-cleanup) CLEANUP="false" ;;
    --help|-h) sed -n '3,40p' "$0"; exit 0 ;;
    *) log_warn "未知参数: $arg" ;;
  esac
done

K8S_ACCESS=$(accessmode_k8s)
RT_RES=$(runtime_resource "$RUNTIME")
DS_NAME="e2e-ds-$RUNTIME"

log_step "0. 前置检查 & 调度模式探测"
api_call GET "/health"
if [ "$CODE" != "200" ]; then
  log_err "后端不可达 ($BACKEND_URL)，请先执行 scripts/start.sh 启动服务。"; exit 3
fi
MODE=$(printf '%s' "$BODY" | jq -r '.mode // "unknown"')
log_info "后端健康，调度模式 = ${C_BOLD}$MODE${C_RESET}"

log_step "1. 创建 Fluid 数据集 (runtime=$RUNTIME, mount=$MOUNT_POINT)"
PAYLOAD=$(jq -n \
  --arg name "$DS_NAME" \
  --arg mount_point "$MOUNT_POINT" \
  --arg sub_path "$SUB_PATH" \
  --arg runtime "$RUNTIME" \
  --arg cache_capacity "$CACHE_CAPACITY" \
  --arg cache_medium "$CACHE_MEDIUM" \
  --arg cache_path "$CACHE_PATH" \
  --arg access_mode "$ACCESS_MODE" \
  --argjson replicas "$REPLICAS" \
  '{name:$name, mount_point:$mount_point, sub_path:$sub_path, runtime:$runtime,
    replicas:$replicas, cache_capacity:$cache_capacity, cache_medium:$cache_medium,
    cache_path:$cache_path, access_mode:$access_mode}')

[ "$VERBOSE" = "true" ] && log_info "Payload: $PAYLOAD"

api_call POST "/datasets" "$PAYLOAD"
if [ "$CODE" != "201" ]; then
  record_fail "创建数据集失败 (HTTP $CODE): $BODY"; exit 4
fi
record_pass "数据集创建成功 (HTTP 201)"

DS_ID=$(printf '%s' "$BODY" | jq -r '.id')
log_info "数据集 ID = $DS_ID"

log_step "2. 校验元数据持久化"
api_call GET "/datasets/$DS_ID"
assert_eq "HTTP 状态" "200" "$CODE"
assert_eq "name" "$DS_NAME" "$(printf '%s' "$BODY" | jq -r '.name')"
assert_eq "mount_point" "$MOUNT_POINT" "$(printf '%s' "$BODY" | jq -r '.mount_point')"
assert_eq "runtime" "$RUNTIME" "$(printf '%s' "$BODY" | jq -r '.runtime')"
assert_eq "replicas" "$REPLICAS" "$(printf '%s' "$BODY" | jq -r '.replicas')"
assert_eq "cache_capacity" "$CACHE_CAPACITY" "$(printf '%s' "$BODY" | jq -r '.cache_capacity')"
assert_eq "cache_medium" "$CACHE_MEDIUM" "$(printf '%s' "$BODY" | jq -r '.cache_medium')"
assert_eq "access_mode" "$ACCESS_MODE" "$(printf '%s' "$BODY" | jq -r '.access_mode')"

if [ "$MODE" = "volcano" ]; then
  log_step "3. Fluid 结构深度校验 (kubectl dataset + $RT_RES)"
  if ! command -v kubectl >/dev/null 2>&1; then
    log_warn "未安装 kubectl，跳过 Fluid 结构校验。"
  else
    DS_JSON=$(kubectl -n "$NAMESPACE" get dataset "$DS_NAME" -o json 2>/dev/null) || DS_JSON=""
    if [ -n "$DS_JSON" ]; then
      mp=$(printf '%s' "$DS_JSON" | jq -r '.spec.mounts[0].mountPoint // ""')
      am=$(printf '%s' "$DS_JSON" | jq -r '.spec.accessModes[0] // ""')
      assert_eq "mountPoint" "$MOUNT_POINT" "$mp"
      assert_eq "accessModes[0]" "$K8S_ACCESS" "$am"
    else
      record_fail "无法获取 dataset $DS_NAME（Fluid 可能未安装）。"
    fi

    RT_JSON=$(kubectl -n "$NAMESPACE" get "$RT_RES" "$DS_NAME" -o json 2>/dev/null) || RT_JSON=""
    if [ -n "$RT_JSON" ]; then
      rep=$(printf '%s' "$RT_JSON" | jq -r '.spec.replicas // -1')
      med=$(printf '%s' "$RT_JSON" | jq -r '.spec.tieredstore.levels[0].mediumtype // ""')
      quota=$(printf '%s' "$RT_JSON" | jq -r '.spec.tieredstore.levels[0].quota // ""')
      assert_eq "runtime replicas" "$REPLICAS" "$rep"
      assert_eq "mediumtype" "$CACHE_MEDIUM" "$med"
      assert_eq "cache quota" "$CACHE_CAPACITY" "$quota"
    else
      record_fail "无法获取 $RT_RES $DS_NAME。"
    fi
  fi
else
  log_step "3. Mock 模式说明"
  log_info "后端处于 mock 模式，不会向集群提交 Fluid Dataset/Runtime（无 CRD 结构可校验）。"
fi

log_step "4. 状态轮询 (等待 Dataset Bound, $TIMEOUTs 超时)"
elapsed=0; final_status=""
while [ "$elapsed" -lt "$TIMEOUT" ]; do
  if [ "$MODE" = "volcano" ] && command -v kubectl >/dev/null 2>&1; then
    phase=$(kubectl -n "$NAMESPACE" get dataset "$DS_NAME" -o jsonpath='{.status.phase}' 2>/dev/null)
    [ -z "$phase" ] && phase="Pending"
  else
    api_call GET "/datasets/$DS_ID"
    phase=$(printf '%s' "$BODY" | jq -r '.status // "unknown"')
  fi
  if [ "$VERBOSE" = "true" ]; then log_info "t=${elapsed}s -> $phase"; fi
  if [ "$phase" = "Bound" ] || [ "$phase" = "bound" ]; then
    final_status="Bound"; record_pass "数据集已绑定 (phase=Bound)"; break
  fi
  if [ "$phase" = "Failed" ] || [ "$phase" = "failed" ]; then
    final_status="Failed"; record_fail "数据集绑定失败"; break
  fi
  sleep "$POLL_INTERVAL"; elapsed=$((elapsed + POLL_INTERVAL))
done
if [ -z "$final_status" ] && [ "$MODE" = "volcano" ]; then
  record_fail "在 ${TIMEOUT}s 内数据集未进入 Bound（缓存 Worker 启动/镜像拉取较慢）。"
elif [ -z "$final_status" ]; then
  log_info "mock 模式不触发集群绑定，跳过状态判定。"
fi

log_step "5. 指标校验 (GET /metrics)"
api_call GET "/metrics"
assert_eq "metrics HTTP 状态" "200" "$CODE"
assert_contains "包含 fuze_dataset_cached_percent" "fuze_dataset_cached_percent" "$BODY"
assert_contains "包含本数据集缓存指标" "fuze_dataset_cached_percent{dataset=\"$DS_NAME\"}" "$BODY"

if [ "$CLEANUP" = "false" ]; then
  log_step "6. 跳过清理 (--no-cleanup)"
  log_info "保留数据集: $DS_ID (Dataset: $DS_NAME)"
else
  log_step "6. 清理数据集 (DELETE /api/v1/datasets/$DS_ID)"
  api_call DELETE "/datasets/$DS_ID"
  assert_eq "删除 HTTP 状态" "204" "$CODE"
  if [ "$MODE" = "volcano" ] && command -v kubectl >/dev/null 2>&1; then
    deleted=0; wait=0
    while [ "$wait" -lt 60 ]; do
      if ! kubectl -n "$NAMESPACE" get dataset "$DS_NAME" >/dev/null 2>&1; then deleted=1; break; fi
      sleep 3; wait=$((wait+3))
    done
    if [ "$deleted" -eq 1 ]; then record_pass "dataset $DS_NAME 已回收";
    else record_fail "dataset $DS_NAME 在 60s 内未被删除"; fi
  fi
fi

log_step "联调结果汇总"
echo "  ${C_GREEN}PASS${C_RESET}: $PASS   ${C_RED}FAIL${C_RESET}: $FAIL"
if [ "$FAIL" -gt 0 ]; then log_err "Fluid 端到端联调存在失败项。"; exit 1; fi
log_ok "Fluid 端到端联调全部通过。"
