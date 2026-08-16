#!/usr/bin/env bash
# tests/scripts/run_tests.sh
# 测试执行脚本：支持执行全部测试、指定测试、生成覆盖率报告
#
# 使用方式：
#   ./tests/scripts/run_tests.sh                  # 运行全部测试
#   ./tests/scripts/run_tests.sh --unit            # 仅运行单元测试
#   ./tests/scripts/run_tests.sh --api             # 仅运行 API 集成测试
#   ./tests/scripts/run_tests.sh --coverage        # 运行测试并生成覆盖率报告
#   ./tests/scripts/run_tests.sh --verbose         # 详细输出
#   ./tests/scripts/run_tests.sh --run TestName    # 运行指定测试
#   ./tests/scripts/run_tests.sh --help            # 查看帮助

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# 默认参数
RUN_UNIT=false
RUN_API=false
COVERAGE=false
VERBOSE=false
SPECIFIC_TEST=""

# 解析参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        --unit)     RUN_UNIT=true ;;
        --api)      RUN_API=true ;;
        --coverage) COVERAGE=true ;;
        --verbose)  VERBOSE=true ;;
        --run)      SPECIFIC_TEST="$2"; shift ;;
        --help|-h)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --unit        仅运行单元测试"
            echo "  --api         仅运行 API 集成测试"
            echo "  --coverage    运行测试并生成覆盖率报告"
            echo "  --verbose     详细输出测试结果"
            echo "  --run TEST    运行指定的测试 (e.g., TestGetJobs)"
            echo "  --help        查看帮助"
            exit 0
            ;;
        *) echo "未知参数: $1，使用 --help 查看帮助"; exit 1 ;;
    esac
    shift
done

cd "$PROJECT_ROOT"

# 构建 go test 参数
GO_TEST_ARGS=("./backend/tests/...")

if [[ -n "$SPECIFIC_TEST" ]]; then
    GO_TEST_ARGS+=("-run" "$SPECIFIC_TEST")
fi

if $VERBOSE; then
    GO_TEST_ARGS+=("-v")
fi

# 单元测试过滤（领域层 / 防腐层 / 应用服务 / 认证 / 调度 / 遥测）
if $RUN_UNIT; then
    echo "===== 运行单元测试 ====="
    go test "${GO_TEST_ARGS[@]}" -v -count=1 \
        -run "TestToken|TestPassword|TestRole|TestSynthetic|TestScheduler|TestQuota|TestEvent|TestTelemetry|TestInference|TestModel|TestJob|TestResource|TestCanary|TestScale|TestDesired|TestApply|TestMark|TestRuntime|TestPromote|TestLogin|TestAuth|TestHash|TestRequire|TestPassthrough|TestApp|TestKServe|TestSanitize|TestVolcano" 2>&1
    exit_code=$?
elif $RUN_API; then
    echo "===== 运行 API 集成测试 ====="
    go test "${GO_TEST_ARGS[@]}" -v -count=1 \
        -run "TestGet|TestCreate|TestUpdate|TestDelete|TestList|TestLogin|TestMe|TestSSO|TestRole|TestRegister|TestScale|TestPromote|TestSync|TestDiscover|TestTest|TestHealth|TestMetrics|TestAudit|TestCluster|TestTenant|TestDataset|TestResource|TestQuota|TestInference|TestJob|TestModel" 2>&1
    exit_code=$?
elif $COVERAGE; then
    echo "===== 运行全部测试（含覆盖率）====="
    COVERAGE_FILE="$PROJECT_ROOT/tests/coverage.out"
    mkdir -p "$PROJECT_ROOT/tests"

    # 运行所有 backend 测试包，覆盖率统计覆盖全部 internal 包
    go test -coverprofile="$COVERAGE_FILE" -covermode=atomic \
        -coverpkg="./backend/internal/..." \
        ./backend/... -count=1 2>&1
    exit_code=$?

    if [ $exit_code -eq 0 ]; then
        echo ""
        echo "===== 覆盖率报告 ====="
        go tool cover -func="$COVERAGE_FILE" | tail -1
        echo ""

        # 按包统计覆盖率
        echo "===== 按包统计覆盖率 ====="
        go tool cover -func="$COVERAGE_FILE" | grep -v "^total:" | python3 -c "
import sys
from collections import defaultdict
sums = defaultdict(float)
counts = defaultdict(int)
for line in sys.stdin:
    line = line.strip()
    if not line: continue
    parts = line.rsplit('\t', 1)
    if len(parts) != 2: continue
    path = parts[0]
    pct_str = parts[1].strip().rstrip('%')
    try:
        pct = float(pct_str)
    except: continue
    idx = path.rfind('/')
    if idx == -1: continue
    pkg = path[:idx]
    sums[pkg] += pct
    counts[pkg] += 1
for p in sorted(sums, key=lambda x: sums[x]/counts[x]):
    print(f'  {p}: {sums[p]/counts[p]:.1f}%')
" 2>/dev/null || true

        echo ""
        echo "HTML 覆盖率报告: go tool cover -html=$COVERAGE_FILE"
        echo "覆盖率文件: $COVERAGE_FILE"
    fi
else
    echo "===== 运行全部测试 ====="
    go test "${GO_TEST_ARGS[@]}" -count=1 2>&1
    exit_code=$?
fi

# 汇总统计
if [ $exit_code -eq 0 ]; then
    echo ""
    echo "✅ 全部测试通过！"
else
    echo ""
    echo "❌ 部分测试失败，请检查输出。"
fi

exit $exit_code