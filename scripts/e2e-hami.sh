#!/usr/bin/env bash
#
# e2e-hami.sh
# ============================================================
# HAMi GPU 显存/算力隔离 端到端联调脚本
#
# HAMi 不是独立 API，而是在提交训练/推理任务时通过
# gpus + gpu_memory(MiB) + gpu_cores(%) 三个字段启用。
# 本脚本提交一个带 HAMi 隔离的普通训练任务，校验：
#   1. 健康检查 & 调度模式 (mock / volcano)
#   2. 提交任务            POST /api/v1/training-jobs (gpus>0, gpu_memory>0, gpu_cores>0)
#   3. 校验元数据持久化    GET  /api/v1/training-jobs/:id
#   4. Volcano 模式深度校验 (kubectl 探测 volcanojob 容器 limits:
#        - nvidia.com/gpu      (共享的物理卡数)
#        - nvidia.com/gpumem   (每张卡显存上限 MiB)
#        - nvidia.com/gpucores (每张卡算力占比 %)
#   5. 状态轮询            (Running 表示调度成功并应用了 HAMi 限制)
#   6. 指标校验            GET /metrics (GPU/显存相关指标)
#   7. 清理                DELETE /api/v1/training-jobs/:id
#
# 使用方式：
#   ./scripts/e2e-hami.sh
#   GPUS=1 GPU_MEMORY=4096 GPU_CORES=50 ./scripts/e2e-hami.sh
#   ./scripts/e2e-hami.sh --no-cleanup
#
set -uo pipefail

BACKEND_URL="${BACKEND_URL:-http://localhost:8080}"
NAMESPACE="${K8S_NAMESPACE:-fuze-ai-paas}"
API="${BACKEND_URL}/api/v1"

GPUS="${GPUS:-1}"              # 共享的物理 GPU 卡数
GPU_MEMORY="${GPU_MEMORY:-4096}"   # 每张卡显存上限 (MiB)
GPU_CORES="${GPU_CORES:-50}"       # 每张卡算力占比 (%)
MEMORY_GI="${MEMORY_PER_TASK:-4}"  # 任务内存 (GiB)
IMAGE="${TRAIN_IMAGE:-pytorch/pytorch:2.2.0-cuda12.1-cudnn8-devel}"
SLEEP_SECONDS="${SLEEP_SECONDS:-20}"

POLL_INTERVAL="${POLL_INTERVAL:-5}"
TIMEOUT="${TIMEOUT:-600}"

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

for arg in "$@"; do
  case "$arg" in
    --no-cleanup) CLEANUP="false" ;;
    --help|-h) sed -n '3,40p' "$0"; exit 0 ;;
    *) log_warn "未知参数: $arg" ;;
  esac
done

log_step "0. 前置检查 & 调度模式探测"
api_call GET "/health"
if [ "$CODE" != "200" ]; then
  log_err "后端不可达 ($BACKEND_URL)，请先执行 scripts/start.sh 启动服务。"; exit 3
fi
MODE=$(printf '%s' "$BODY" | jq -r '.mode // "unknown"')
log_info "后端健康，调度模式 = ${C_BOLD}$MODE${C_RESET}"

log_step "1. 提交带 HAMi 隔离的训练任务 (gpu=$GPUS, gpumem=${GPU_MEMORY}MiB, gpucores=${GPU_CORES}%)"
HAMI_CMD="echo \"[HAMi] restricted gpu=\$(nvidia-smi -L 2>/dev/null | head -1); gpumem=${GPU_MEMORY}MiB cores=${GPU_CORES}%\"; sleep ${SLEEP_SECONDS}; echo done"

PAYLOAD=$(jq -n \
  --arg name "e2e-hami" \
  --arg image "$IMAGE" \
  --arg command "$HAMI_CMD" \
  --argjson gpus "$GPUS" \
  --argjson memory "$MEMORY_GI" \
  --argjson gpu_memory "$GPU_MEMORY" \
  --argjson gpu_cores "$GPU_CORES" \
  '{name:$name, type:"training", image:$image, command:$command,
    gpus:$gpus, memory:$memory, gpu_memory:$gpu_memory, gpu_cores:$gpu_cores}')

[ "$VERBOSE" = "true" ] && log_info "Payload: $PAYLOAD"

api_call POST "/training-jobs" "$PAYLOAD"
if [ "$CODE" != "201" ]; then
  record_fail "提交任务失败 (HTTP $CODE): $BODY"; exit 4
fi
record_pass "任务创建成功 (HTTP 201)"

JOB_ID=$(printf '%s' "$BODY" | jq -r '.id')
VJ_NAME=$(printf '%s' "$BODY" | jq -r '.volcano_job_name // ""')
log_info "任务 ID       = $JOB_ID"
log_info "VolcanoJob 名称 = ${VJ_NAME:-<空>}"

log_step "2. 校验 HAMi 元数据持久化"
api_call GET "/training-jobs/$JOB_ID"
assert_eq "HTTP 状态" "200" "$CODE"
assert_eq "gpus" "$GPUS" "$(printf '%s' "$BODY" | jq -r '.gpus')"
assert_eq "gpu_memory" "$GPU_MEMORY" "$(printf '%s' "$BODY" | jq -r '.gpu_memory')"
assert_eq "gpu_cores" "$GPU_CORES" "$(printf '%s' "$BODY" | jq -r '.gpu_cores')"

if [ "$MODE" = "volcano" ]; then
  log_step "3. HAMi 资源深度校验 (kubectl volcanojob container limits)"
  if ! command -v kubectl >/dev/null 2>&1; then
    log_warn "未安装 kubectl，跳过 HAMi 资源校验。"
  elif [ -z "$VJ_NAME" ]; then
    record_fail "Volcano 模式但 volcano_job_name 为空 —— 提交被集群拒绝（检查 Volcano CRD/权限或 Queue）。"
  else
    VJ_JSON=$(kubectl -n "$NAMESPACE" get volcanojob "$VJ_NAME" -o json 2>/dev/null) || VJ_JSON=""
    if [ -n "$VJ_JSON" ]; then
      # 取第一个容器的 limits
      lim=$(printf '%s' "$VJ_JSON" | jq -c '[.spec.tasks[].template.spec.containers[]?.resources.limits] | add // {}')
      ngpu=$(printf '%s' "$lim" | jq -r '.["nvidia.com/gpu"] // ""')
      gmem=$(printf '%s' "$lim" | jq -r '.["nvidia.com/gpumem"] // ""')
      gcore=$(printf '%s' "$lim" | jq -r '.["nvidia.com/gpucores"] // ""')
      assert_eq "nvidia.com/gpu" "$GPUS" "$ngpu"
      assert_eq "nvidia.com/gpumem (MiB)" "$GPU_MEMORY" "$gmem"
      assert_eq "nvidia.com/gpucores (%)" "$GPU_CORES" "$gcore"
    else
      record_fail "无法获取 volcanojob $VJ_NAME。"
    fi
  fi
else
  log_step "3. Mock 模式说明"
  log_info "后端处于 mock 模式，不会向集群提交 volcanojob（无容器 limits 可校验）。"
fi

log_step "4. 状态轮询 ($TIMEOUTs 超时)"
elapsed=0; final_status=""
while [ "$elapsed" -lt "$TIMEOUT" ]; do
  if [ "$MODE" = "volcano" ] && [ -n "$VJ_NAME" ] && command -v kubectl >/dev/null 2>&1; then
    phase=$(kubectl -n "$NAMESPACE" get volcanojob "$VJ_NAME" -o jsonpath='{.status.state.phase}' 2>/dev/null)
    [ -z "$phase" ] && phase="Pending"
  else
    api_call GET "/training-jobs/$JOB_ID"
    phase=$(printf '%s' "$BODY" | jq -r '.status // "unknown"')
  fi
  if [ "$VERBOSE" = "true" ]; then log_info "t=${elapsed}s -> $phase"; fi
  case "$phase" in
    Running|Completed) final_status="$phase"; record_pass "任务进入 $phase 状态（HAMi 限制已生效）"; break ;;
    Failed|Aborted|Terminated) final_status="$phase"; record_fail "任务异常终止: $phase"; break ;;
  esac
  sleep "$POLL_INTERVAL"; elapsed=$((elapsed + POLL_INTERVAL))
done
if [ -z "$final_status" ]; then
  record_fail "在 ${TIMEOUT}s 内任务未进入 Running/Completed。"
fi

log_step "5. 指标校验 (GET /metrics)"
api_call GET "/metrics"
assert_eq "metrics HTTP 状态" "200" "$CODE"
assert_contains "包含 fuze_gpu_total" "fuze_gpu_total" "$BODY"
assert_contains "包含 fuze_memory_used_gb" "fuze_memory_used_gb" "$BODY"
assert_contains "包含 fuze_jobs" "fuze_jobs" "$BODY"

if [ "$CLEANUP" = "false" ]; then
  log_step "6. 跳过清理 (--no-cleanup)"
  log_info "保留任务: $JOB_ID (VolcanoJob: ${VJ_NAME:-<空>})"
else
  log_step "6. 清理任务 (DELETE /api/v1/training-jobs/$JOB_ID)"
  api_call DELETE "/training-jobs/$JOB_ID"
  assert_eq "删除 HTTP 状态" "204" "$CODE"
  if [ "$MODE" = "volcano" ] && [ -n "$VJ_NAME" ] && command -v kubectl >/dev/null 2>&1; then
    deleted=0; wait=0
    while [ "$wait" -lt 60 ]; do
      if ! kubectl -n "$NAMESPACE" get volcanojob "$VJ_NAME" >/dev/null 2>&1; then deleted=1; break; fi
      sleep 3; wait=$((wait+3))
    done
    if [ "$deleted" -eq 1 ]; then record_pass "volcanojob $VJ_NAME 已回收";
    else record_fail "volcanojob $VJ_NAME 在 60s 内未被删除"; fi
  fi
fi

log_step "联调结果汇总"
echo "  ${C_GREEN}PASS${C_RESET}: $PASS   ${C_RED}FAIL${C_RESET}: $FAIL"
if [ "$FAIL" -gt 0 ]; then log_err "HAMi 端到端联调存在失败项。"; exit 1; fi
log_ok "HAMi 端到端联调全部通过。"
