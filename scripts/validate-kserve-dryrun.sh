#!/usr/bin/env bash
#
# validate-kserve-dryrun.sh
# ============================================================
# KServe InferenceService schema 灰盒校验脚本
#
# 目的：
#   用 kubectl apply --dry-run=server 把后端生成的 InferenceService
#   manifest 直接打到真实集群的 CRD 上做结构校验，捕获代码与集群
#   CRD 不一致的问题（例如 spec.components[] 被 CRD 结构性 schema
#   prune、canaryTrafficPercent 不被当前部署形态支持等）。
#
#   脚本在本地复刻了 backend/internal/k8s/kserve.go 中
#   BuildInferenceServiceObject 的构造逻辑，覆盖多框架 / 多芯片
#   组合，逐一做 --dry-run=server，并可选回读 CRD 校验。
#
# 前置条件：
#   - kubectl 已配置且能连到目标集群
#   - 集群已安装 KServe，且 CRD serving.kserve.io/inferenceservices 存在
#
# 使用方式：
#   ./scripts/validate-kserve-dryrun.sh
#   KUBECTL_CONTEXT=myctx NAMESPACE=inf ./scripts/validate-kserve-dryrun.sh
#   ./scripts/validate-kserve-dryrun.sh --read-back     # 额外回读 CRD 校验 spec 字段
#   ./scripts/validate-kserve-dryrun.sh --help
#
# 退出码：
#   0 = 全部校验通过
#   1 = 存在 dry-run 失败项
#   2 = 缺少必要命令 / 集群不可达
#
set -uo pipefail

# ============================================================
# 配置
# ============================================================
NAMESPACE="${K8S_NAMESPACE:-fuze-ai-paas}"
KUBECTL_BIN="kubectl"
if [ -n "${KUBECTL_CONTEXT:-}" ]; then
  KUBECTL_BIN="kubectl --context=$KUBECTL_CONTEXT"
fi

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

PASS=0; FAIL=0
record_pass() { PASS=$((PASS+1)); log_ok "$1"; }
record_fail() { FAIL=$((FAIL+1)); log_err "$1"; }

READ_BACK="false"
for arg in "$@"; do
  case "$arg" in
    --read-back) READ_BACK="true" ;;
    --help|-h) sed -n '3,38p' "$0"; exit 0 ;;
    *) log_warn "未知参数: $arg" ;;
  esac
done

need_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log_err "缺少必要命令: $1，请先安装后重试。"; exit 2
  fi
}
need_tool kubectl

# ------------------------------------------------------------
# 0. 集群 & CRD 探测
# ------------------------------------------------------------
log_step "0. 集群 & KServe CRD 探测"
if ! $KUBECTL_BIN version --request-timeout=10s >/dev/null 2>&1; then
  log_err "无法连接集群，请检查 kubeconfig / context。"; exit 2
fi
log_info "集群连接正常。"

DEPLOY_MODE="unknown"
if $KUBECTL_BIN get deploy -n istio-system istio-ingressgateway >/dev/null 2>&1 \
   && $KUBECTL_BIN get crd serving.kserve.io >/dev/null 2>&1; then
  # Serverless/Knative 形态：CRD 与 istio 同时存在，canaryTrafficPercent 可用
  DEPLOY_MODE="serverless"
elif $KUBECTL_BIN get crd serving.kserve.io >/dev/null 2>&1; then
  DEPLOY_MODE="raw"
fi
log_info "探测到的 KServe 部署形态 = ${C_BOLD}$DEPLOY_MODE${C_RESET}" \
  "(serverless 支持 canaryTrafficPercent，raw 不支持)"

if ! $KUBECTL_BIN get crd inferenceservices.serving.kserve.io >/dev/null 2>&1; then
  log_err "未找到 CRD inferenceservices.serving.kserve.io，请确认 KServe 已安装。"; exit 2
fi
log_ok "KServe CRD 存在。"

# ------------------------------------------------------------
# 1. manifest 构造（复刻 BuildInferenceServiceObject）
# ------------------------------------------------------------
# 用法: build_manifest NAME FRAMEWORK IMAGE STORAGE_URI CHIP GPUS GPUMEM GPUCORES MINREP MAXREP
# 注意：此处严格对齐 kserve.go 的 spec.predictor 具名对象结构（非 components[]）。
build_manifest() {
  local name="$1" fw="$2" image="$3" uri="$4" chip="$5"
  local gpus="$6" gmem="$7" gcores="$8" minrep="$9" maxrep="${10}"
  local vendor="${chip:-nvidia}"

  # 芯片差异化 device 资源键（对齐 chip.Spec.ResourceLimits）
  local devkey="nvidia.com/gpu"
  case "$vendor" in
    ascend)   devkey="ascend.com/vnpu" ;;
    cambricon)devkey="cambricon.com/mlu" ;;
    *)        devkey="nvidia.com/gpu" ;;
  esac

  # 资源 limits（CPU/内存 + 可选 device 资源）
  local limits='"cpu":"1","memory":"2Gi"'
  local requests='"cpu":"1","memory":"2Gi"'
  if [ "$gpus" -gt 0 ]; then
    limits="$limits,\"$devkey\":\"$gpus\""
    requests="$requests,\"$devkey\":\"$gpus\""
  fi

  # 芯片差异化注解（对齐 chip.Annotations）
  local ann=""
  case "$vendor" in
    nvidia)
      [ "$gpus" -gt 0 ]   && ann="${ann:+$ann,}\"nvidia.com/gpu\":\"$gpus\""
      [ "$gmem" -gt 0 ]   && ann="${ann:+$ann,}\"nvidia.com/gpumem\":\"$gmem\""
      [ "$gcores" -gt 0 ] && ann="${ann:+$ann,}\"nvidia.com/gpucores\":\"$gcores\""
      ;;
    ascend)
      [ "$gpus" -gt 0 ]   && ann="${ann:+$ann,}\"ascend.com/vnpu\":\"$gpus\""
      [ "$gmem" -gt 0 ]   && ann="${ann:+$ann,}\"ascend.com/virMemory\":\"$gmem\""
      [ "$gcores" -gt 0 ] && ann="${ann:+$ann,}\"ascend.com/virAICore\":\"$gcores\""
      ;;
    cambricon)
      [ "$gpus" -gt 0 ]   && ann="${ann:+$ann,}\"cambricon.com/mlu\":\"$gpus\""
      [ "$gmem" -gt 0 ]   && ann="${ann:+$ann,}\"cambricon.com/virMemory\":\"$gmem\""
      [ "$gcores" -gt 0 ] && ann="${ann:+$ann,}\"cambricon.com/virAICore\":\"$gcores\""
      ;;
  esac

  local predictor="\"minReplicas\":$minrep,\"maxReplicas\":$maxrep,"
  predictor="$predictor\"resources\":{\"limits\":{$limits},\"requests\":{$requests}}"
  if [ -n "$ann" ]; then
    predictor="$predictor,\"annotations\":{$ann}"
  fi
  if [ "$fw" = "custom" ]; then
    predictor="$predictor,\"containers\":[{\"image\":\"$image\",\"resources\":{\"limits\":{$limits},\"requests\":{$requests}}}]"
  else
    predictor="$predictor,\"model\":{\"modelFormat\":{\"name\":\"$fw\"},\"storageUri\":\"$uri\",\"resources\":{\"limits\":{$limits},\"requests\":{$requests}}}"
  fi

  cat <<EOF
{
  "apiVersion": "serving.kserve.io/v1beta1",
  "kind": "InferenceService",
  "metadata": {
    "name": "$name",
    "namespace": "$NAMESPACE",
    "labels": {"app":"fuze-ai-paas","managed-by":"fuze-scheduler"}
  },
  "spec": { "predictor": { $predictor } }
}
EOF
}

# 构造 canary patch（对齐 canaryPatch：JSON Patch 精确替换 predictor.canaryTrafficPercent）
# 用法: build_canary_patch WEIGHT
build_canary_patch() {
  local weight="$1"
  cat <<EOF
[{"op":"replace","path":"/spec/predictor/canaryTrafficPercent","value":$weight}]
EOF
}

# ------------------------------------------------------------
# 2. dry-run 用例
# ------------------------------------------------------------
# 用例矩阵: 名称|框架|镜像|storageUri|芯片|gpus|gpumem|gcores|min|max
CASES=(
  "sklearn|sklearn||s3://b/m-sklearn|nvidia|0|0|0|0|2"
  "pytorch|pytorch||s3://b/m-pytorch|nvidia|1|4096|50|1|2"
  "custom|custom|registry/inf:latest||nvidia|0|0|0|1|1"
  "ascend|pytorch||s3://b/m-ascend|ascend|1|8192|80|1|2"
  "cambricon|onnx||s3://b/m-cambricon|cambricon|1|0|0|1|2"
  "s2z|sklearn||s3://b/m-s2z|nvidia|0|0|0|0|3"
)

log_step "1. --dry-run=server 校验 (spec.predictor 结构)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# 先确认 namespace 存在（dry-run 不创建，但 server 端会校验 namespace scope）
$KUBECTL_BIN get ns "$NAMESPACE" >/dev/null 2>&1 || {
  log_warn "namespace $NAMESPACE 不存在，dry-run=server 可能因 namespace 不存在而失败（不影响 schema 校验）。"
}

for c in "${CASES[@]}"; do
  IFS='|' read -r name fw image uri chip gpus gmem gcores minrep maxrep <<<"$c"
  manifest="$TMP/$name.json"
  build_manifest "$name" "$fw" "$image" "$uri" "$chip" \
    "$gpus" "$gmem" "$gcores" "$minrep" "$maxrep" >"$manifest"

  if $KUBECTL_BIN apply --dry-run=server -f "$manifest" >/dev/null 2>&1; then
    record_pass "dry-run OK: $name (fw=$fw, chip=$chip)"
  else
    out=$($KUBECTL_BIN apply --dry-run=server -f "$manifest" 2>&1)
    record_fail "dry-run 失败: $name -> $out"
  fi
done

# ------------------------------------------------------------
# 3. canaryTrafficPercent 校验（仅 serverless 形态预期通过）
# ------------------------------------------------------------
log_step "2. canaryTrafficPercent patch 校验"
CANARY_NAME="canary-dryrun"
build_manifest "$CANARY_NAME" "sklearn" "" "s3://b/m-canary" "nvidia" \
  0 0 0 1 1 >"$TMP/$CANARY_NAME.json"

if $KUBECTL_BIN apply --dry-run=server -f "$TMP/$CANARY_NAME.json" >/dev/null 2>&1; then
  PATCH="$TMP/canary.patch.json"
  build_canary_patch 30 >"$PATCH"
  if $KUBECTL_BIN apply --dry-run=server \
       --patch "$(cat "$PATCH")" --type=json \
       -f "$TMP/$CANARY_NAME.json" >/dev/null 2>&1; then
    record_pass "canary patch OK (canaryTrafficPercent=30)"
  else
    out=$($KUBECTL_BIN apply --dry-run=server --patch "$(cat "$PATCH")" \
      --type=json -f "$TMP/$CANARY_NAME.json" 2>&1)
    if [ "$DEPLOY_MODE" = "serverless" ]; then
      record_fail "canary patch 失败 (serverless 预期应支持): $out"
    else
      record_fail "canary patch 失败 (raw 形态不支持，需后端规避): $out"
    fi
  fi
else
  out=$($KUBECTL_BIN apply --dry-run=server -f "$TMP/$CANARY_NAME.json" 2>&1)
  record_fail "canary 基线 manifest dry-run 失败: $out"
fi

# ------------------------------------------------------------
# 4. 可选：回读 CRD schema 校验（结构不被 prune）
# ------------------------------------------------------------
if [ "$READ_BACK" = "true" ]; then
  log_step "3. CRD schema 回读校验 (spec.predictor 结构存在性)"
  schema=$($KUBECTL_BIN get crd inferenceservices.serving.kserve.io \
    -o jsonpath='{.spec.versions[?(@.name=="v1beta1")].schema.openAPIV3Schema}' 2>/dev/null)
  if [ -z "$schema" ]; then
    log_warn "无法读取 CRD v1beta1 schema，跳过回读校验。"
  else
    if printf '%s' "$schema" | grep -q '"predictor"'; then
      record_pass "CRD schema 包含 spec.predictor 字段"
    else
      record_fail "CRD schema 不含 spec.predictor —— 后端结构与集群 CRD 不符"
    fi
    if printf '%s' "$schema" | grep -q 'components'; then
      record_fail "CRD schema 仍声明 components —— 与当前后端 spec.predictor 结构冲突"
    else
      record_pass "CRD schema 不含 components（与后端一致）"
    fi
  fi
else
  log_step "3. 回读校验已跳过（加 --read-back 启用）"
fi

# ------------------------------------------------------------
# 5. 汇总
# ------------------------------------------------------------
log_step "校验结果汇总"
echo "  部署形态: $DEPLOY_MODE"
echo "  ${C_GREEN}PASS${C_RESET}: $PASS   ${C_RED}FAIL${C_RESET}: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  log_err "KServe dry-run 校验存在失败项，请检查 manifest 与集群 CRD 的一致性。"
  exit 1
fi
log_ok "KServe dry-run 校验全部通过。"
