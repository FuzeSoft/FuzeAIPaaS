#!/usr/bin/env bash
#
# e2e-distributed-training.sh
# ============================================================
# 分布式训练 (Volcano) 端到端联调脚本
#
# 覆盖链路：
#   1. 健康检查 & 探测调度模式 (mock / volcano)
#   2. 提交分布式训练任务  POST /api/v1/training-jobs  (distributed=true)
#   3. 校验任务元数据持久化  GET  /api/v1/training-jobs/:id
#   4. Volcano 模式深度校验  (kubectl 探测 volcanojob:
#        - minAvailable  == 1 + worker 副本数 (Gang Scheduling 全有或全无)
#        - tasks 包含 master(1) + worker(N)
#        - plugins 包含 env / svc / pytorch
#        - queue == training-queue
#   5. 状态轮询  (Gang 调度：所有副本就绪后才 Running)
#   6. 指标校验  GET /metrics (fuze_jobs 计数随任务变化)
#   7. 清理       DELETE /api/v1/training-jobs/:id，并确认 volcanojob 已回收
#
# 使用方式：
#   ./scripts/e2e-distributed-training.sh                 # 默认 2 worker 的 pytorch 任务
#   WORKERS=4 FRAMEWORK=tensorflow ./scripts/e2e-distributed-training.sh
#   BACKEND_URL=http://10.0.0.1:8080 ./scripts/e2e-distributed-training.sh
#   ./scripts/e2e-distributed-training.sh --no-cleanup    # 调试时保留资源
#   ./scripts/e2e-distributed-training.sh --help
#
set -uo pipefail

# ============================================================
# 配置（可通过环境变量覆盖）
# ============================================================
BACKEND_URL="${BACKEND_URL:-http://localhost:8080}"
NAMESPACE="${K8S_NAMESPACE:-fuze-ai-paas}"
API="${BACKEND_URL}/api/v1"

WORKERS="${WORKERS:-2}"                       # Worker 副本数（不含 master）
FRAMEWORK="${FRAMEWORK:-pytorch}"             # pytorch | tensorflow | mpi
GPUS_PER_TASK="${GPUS_PER_TASK:-1}"           # 每个 task 申请的 GPU 卡数
MEMORY_GI="${MEMORY_PER_TASK:-2}"             # 每个 task 申请的内存 (GiB)
MIN_AVAILABLE="${MIN_AVAILABLE:-$((WORKERS + 1))}"  # Gang 最小可用副本数
IMAGE="${TRAIN_IMAGE:-pytorch/pytorch:2.2.0-cuda12.1-cudnn8-devel}"
SLEEP_SECONDS="${SLEEP_SECONDS:-30}"          # 容器内训练命令的占位时长

POLL_INTERVAL="${POLL_INTERVAL:-5}"           # 状态轮询间隔 (秒)
TIMEOUT="${TIMEOUT:-600}"                     # 单次状态等待超时 (秒)

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

# ============================================================
# 测试计数
# ============================================================
PASS=0
FAIL=0
record_pass() { PASS=$((PASS+1)); log_ok "$1"; }
record_fail() { FAIL=$((FAIL+1)); log_err "$1"; }

# ============================================================
# 工具检查
# ============================================================
need_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log_err "缺少必要命令: $1，请先安装后重试。"
    exit 2
  fi
}
check_tools() {
  need_tool curl
  need_tool jq
}

# ============================================================
# HTTP 辅助：api_call METHOD PATH [DATA]
#   返回响应体到 $BODY，状态码到 $CODE
# ============================================================
BODY=""
CODE=""
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

# ============================================================
# 断言辅助
# ============================================================
assert_eq() {     # desc expected actual
  if [ "$2" = "$3" ]; then record_pass "$1 (期望=$2, 实际=$3)";
  else record_fail "$1 (期望=$2, 实际=$3)"; fi
}
assert_ge() {     # desc expected actual
  if [ "$3" -ge "$2" ] 2>/dev/null; then record_pass "$1 (>= $2, 实际=$3)";
  else record_fail "$1 (>= $2, 实际=$3)"; fi
}
assert_contains() { # desc needle haystack
  if printf '%s' "$3" | grep -q -- "$2"; then record_pass "$1";
  else record_fail "$1 (未找到: $2)"; fi
}

# ============================================================
# 入口参数
# ============================================================
for arg in "$@"; do
  case "$arg" in
    --no-cleanup) CLEANUP="false" ;;
    --help|-h) sed -n '3,40p' "$0"; exit 0 ;;
    *) log_warn "未知参数: $arg" ;;
  esac
done

# ============================================================
# 前置校验
# ============================================================
check_tools

log_step "0. 前置检查 & 调度模式探测"
api_call GET "/health"
if [ "$CODE" != "200" ]; then
  log_err "后端不可达 ($BACKEND_URL)，请先执行 scripts/start.sh 启动服务。"
  exit 3
fi
MODE=$(printf '%s' "$BODY" | jq -r '.mode // "unknown"')
log_info "后端健康，调度模式 = ${C_BOLD}$MODE${C_RESET}"

# 校验 min_available 取值合法（代码要求 <= 1 + worker）
if [ "$MIN_AVAILABLE" -gt "$((WORKERS + 1))" ]; then
  log_warn "min_available($MIN_AVAILABLE) > 1+worker($((WORKERS+1)))，Volcano 会忽略并回退为全量副本。"
fi

# ============================================================
# 1. 提交分布式训练任务
# ============================================================
log_step "1. 提交分布式训练任务 (distributed=true, framework=$FRAMEWORK, workers=$WORKERS)"

# 容器内命令：打印 task role 并占位 sleep；真实集群可替换为 torchrun 启动命令
TRAIN_CMD="echo \"[role=\$FUZE_TASK_ROLE] distributed training started on \$(hostname)\"; sleep ${SLEEP_SECONDS}; echo \"[role=\$FUZE_TASK_ROLE] done\""

PAYLOAD=$(jq -n \
  --arg name "e2e-dt-${FRAMEWORK}-${WORKERS}w" \
  --arg image "$IMAGE" \
  --arg framework "$FRAMEWORK" \
  --arg command "$TRAIN_CMD" \
  --argjson gpus "$GPUS_PER_TASK" \
  --argjson memory "$MEMORY_GI" \
  --argjson replicas "$WORKERS" \
  --argjson min_available "$MIN_AVAILABLE" \
  '{name:$name, type:"training", image:$image, command:$command,
    gpus:$gpus, memory:$memory, distributed:true, framework:$framework,
    replicas:$replicas, min_available:$min_available}')

[ "$VERBOSE" = "true" ] && log_info "Payload: $PAYLOAD"

api_call POST "/training-jobs" "$PAYLOAD"
if [ "$CODE" != "201" ]; then
  record_fail "提交任务失败 (HTTP $CODE): $BODY"
  exit 4
fi
record_pass "任务创建成功 (HTTP 201)"

JOB_ID=$(printf '%s' "$BODY" | jq -r '.id')
VJ_NAME=$(printf '%s' "$BODY" | jq -r '.volcano_job_name // ""')
QUEUE_NAME=$(printf '%s' "$BODY" | jq -r '.queue_name // ""')
log_info "任务 ID         = $JOB_ID"
log_info "VolcanoJob 名称 = ${VJ_NAME:-<空>}"
log_info "Queue 名称      = ${QUEUE_NAME:-<空>}"

# ============================================================
# 2. 元数据持久化校验
# ============================================================
log_step "2. 校验任务元数据持久化"
api_call GET "/training-jobs/$JOB_ID"
assert_eq "HTTP 状态" "200" "$CODE"
assert_eq "distributed 字段" "true" "$(printf '%s' "$BODY" | jq -r '.distributed')"
assert_eq "framework 字段" "$FRAMEWORK" "$(printf '%s' "$BODY" | jq -r '.framework')"
assert_eq "replicas 字段" "$WORKERS" "$(printf '%s' "$BODY" | jq -r '.replicas')"
assert_eq "type 字段" "training" "$(printf '%s' "$BODY" | jq -r '.type')"

# ============================================================
# 3. Volcano 模式深度校验（需要真实集群）
# ============================================================
if [ "$MODE" = "volcano" ]; then
  log_step "3. Volcano 模式深度校验 (kubectl)"

  if ! command -v kubectl >/dev/null 2>&1; then
    log_warn "未安装 kubectl，跳过 volcanojob 结构校验（仅做 API 层校验）。"
  elif [ -z "$VJ_NAME" ]; then
    record_fail "Volcano 模式但 volcano_job_name 为空 —— 提交被集群拒绝（常见原因：缺少 Volcano Queue '$QUEUE_NAME' 或无 CRD 权限）。"
  else
    # 命名空间 & Queue 预检
    if ! kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
      log_warn "命名空间 $NAMESPACE 不存在，volcanojob 可能无法调度。"
    fi
    if [ -n "$QUEUE_NAME" ] && ! kubectl get queue "$QUEUE_NAME" >/dev/null 2>&1; then
      log_warn "Volcano Queue '$QUEUE_NAME' 不存在 —— 集群可能拒绝该 Job（请先 kubectl apply 对应 Queue）。"
    fi

    # 抓取 volcanojob
    VJ_JSON=$(kubectl -n "$NAMESPACE" get volcanojob "$VJ_NAME" -o json 2>/dev/null) || {
      record_fail "无法获取 volcanojob $VJ_NAME（可能尚未创建）。"
      VJ_JSON=""
    }

    if [ -n "$VJ_JSON" ]; then
      v_minavail=$(printf '%s' "$VJ_JSON" | jq -r '.spec.minAvailable // -1')
      v_tasks=$(printf '%s' "$VJ_JSON" | jq -r '.spec.tasks | length')
      v_master=$(printf '%s' "$VJ_JSON" | jq -r '.spec.tasks[] | select(.name=="master") | .replicas')
      v_worker=$(printf '%s' "$VJ_JSON" | jq -r '.spec.tasks[] | select(.name=="worker") | .replicas')
      v_queue=$(printf '%s' "$VJ_JSON" | jq -r '.spec.queue // ""')
      v_sched=$(printf '%s' "$VJ_JSON" | jq -r '.spec.schedulerName // ""')
      # 插件集合
      v_plugins=$(printf '%s' "$VJ_JSON" | jq -r '.spec.plugins | keys | join(",")')

      assert_eq "schedulerName=volcano" "volcano" "$v_sched"
      assert_eq "minAvailable(Gang)" "$MIN_AVAILABLE" "$v_minavail"
      assert_eq "tasks 数量=2 (master+worker)" "2" "$v_tasks"
      assert_eq "master replicas=1" "1" "$v_master"
      assert_eq "worker replicas=$WORKERS" "$WORKERS" "$v_worker"
      assert_eq "queue=$QUEUE_NAME" "$QUEUE_NAME" "$v_queue"
      assert_contains "plugins 含 svc" "svc" "$v_plugins"
      assert_contains "plugins 含 env" "env" "$v_plugins"
      case "$FRAMEWORK" in
        pytorch)     assert_contains "plugins 含 pytorch" "pytorch" "$v_plugins" ;;
        tensorflow)  assert_contains "plugins 含 tensorflow" "tensorflow" "$v_plugins" ;;
        *)           assert_contains "plugins 含 ssh" "ssh" "$v_plugins" ;;
      esac
    fi
  fi
else
  log_step "3. Mock 模式说明"
  log_info "后端处于 mock 模式，不会向集群提交 volcanojob（无 CRD 结构可校验）。"
  log_info "如需 Volcano 结构校验，请在具备 Volcano 的集群内以 in-cluster/kubeconfig 方式运行后端。"
fi

# ============================================================
# 4. 状态轮询（Gang 调度：全部副本就绪后才 Running）
# ============================================================
log_step "4. 状态轮询 ($TIMEOUTs 超时)"

elapsed=0
final_status=""
while [ "$elapsed" -lt "$TIMEOUT" ]; do
  if [ "$MODE" = "volcano" ] && [ -n "$VJ_NAME" ] && command -v kubectl >/dev/null 2>&1; then
    phase=$(kubectl -n "$NAMESPACE" get volcanojob "$VJ_NAME" \
      -o jsonpath='{.status.state.phase}' 2>/dev/null)
    [ -z "$phase" ] && phase="Pending"
  else
    api_call GET "/training-jobs/$JOB_ID"
    phase=$(printf '%s' "$BODY" | jq -r '.status // "unknown"')
  fi

  if [ "$VERBOSE" = "true" ]; then
    log_info "t=${elapsed}s -> $phase"
  fi

  case "$phase" in
    Running|Completed)
      final_status="$phase"
      record_pass "任务进入 $phase 状态（Gang 调度全部副本就绪）"
      break
      ;;
    Failed|Aborted|Terminated)
      final_status="$phase"
      record_fail "任务异常终止: $phase"
      break
      ;;
  esac

  sleep "$POLL_INTERVAL"
  elapsed=$((elapsed + POLL_INTERVAL))
done

if [ -z "$final_status" ]; then
  record_fail "在 ${TIMEOUT}s 内任务未进入 Running/Completed（当前仍 Pending，可能资源不足或 Gang 无法满足）。"
fi

# ============================================================
# 5. 指标校验
# ============================================================
log_step "5. 指标校验 (GET /metrics)"
api_call GET "/metrics"
assert_eq "metrics HTTP 状态" "200" "$CODE"
assert_contains "包含 fuze_jobs 指标" "fuze_jobs" "$BODY"
assert_contains "包含任务总数标签" 'fuze_jobs{status="total"}' "$BODY"

# 若任务已运行/完成，running 计数应 >= 1
if [ "$final_status" = "Running" ] || [ "$final_status" = "Completed" ]; then
  running=$(printf '%s' "$BODY" | grep -E '^fuze_jobs\{status="running"\}' | awk '{print $2}')
  assert_ge "running 任务计数 >= 1" 1 "${running:-0}"
fi

# ============================================================
# 6. 清理
# ============================================================
if [ "$CLEANUP" = "false" ]; then
  log_step "6. 跳过清理 (--no-cleanup)"
  log_info "保留任务: $JOB_ID  (VolcanoJob: ${VJ_NAME:-<空>})"
else
  log_step "6. 清理任务 (DELETE /api/v1/training-jobs/$JOB_ID)"
  api_call DELETE "/training-jobs/$JOB_ID"
  assert_eq "删除 HTTP 状态" "204" "$CODE"

  if [ "$MODE" = "volcano" ] && [ -n "$VJ_NAME" ] && command -v kubectl >/dev/null 2>&1; then
    # 等待 volcanojob 被回收
    deleted=0
    wait=0
    while [ "$wait" -lt 60 ]; do
      if ! kubectl -n "$NAMESPACE" get volcanojob "$VJ_NAME" >/dev/null 2>&1; then
        deleted=1
        break
      fi
      sleep 3
      wait=$((wait + 3))
    done
    if [ "$deleted" -eq 1 ]; then
      record_pass "volcanojob $VJ_NAME 已回收"
    else
      record_fail "volcanojob $VJ_NAME 在 60s 内未被删除"
    fi
  fi
fi

# ============================================================
# 汇总
# ============================================================
log_step "联调结果汇总"
echo "  ${C_GREEN}PASS${C_RESET}: $PASS   ${C_RED}FAIL${C_RESET}: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  log_err "分布式训练端到端联调存在失败项，请检查上方 [FAIL] 明细。"
  exit 1
fi
log_ok "分布式训练端到端联调全部通过。"
