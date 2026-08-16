#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$PROJECT_DIR/.pids"

# ==================== 停止服务 ====================
stop_service() {
    local name=$1
    local pid_file="$PID_DIR/${name}.pid"

    if [ ! -f "$pid_file" ]; then
        echo "[INFO] ${name} 未运行 (无 PID 文件)"
        return 0
    fi

    PID=$(cat "$pid_file")
    if kill -0 "$PID" 2>/dev/null; then
        echo "[INFO] 停止 ${name} (PID: $PID) ..."
        kill "$PID"

        # 等待进程退出（最多 10 秒）
        for i in $(seq 1 10); do
            if ! kill -0 "$PID" 2>/dev/null; then
                break
            fi
            sleep 1
        done

        # 仍未退出则强制终止
        if kill -0 "$PID" 2>/dev/null; then
            echo "[WARN] 强制终止 ${name} (PID: $PID)"
            kill -9 "$PID" 2>/dev/null || true
        fi
        echo "[OK] ${name} 已停止"
    else
        echo "[INFO] ${name} 进程不存在 (PID: $PID)，清理 PID 文件"
    fi

    rm -f "$pid_file"
}

# ==================== 主流程 ====================
echo "========================================="
echo "  Fuze AI PaaS - 系统停止"
echo "========================================="

case "${1:-all}" in
    backend)
        stop_service "backend"
        ;;
    frontend)
        stop_service "frontend"
        ;;
    all)
        stop_service "backend"
        stop_service "frontend"
        ;;
    *)
        echo "用法: $0 {backend|frontend|all}"
        exit 1
        ;;
esac

echo ""
echo "========================================="
echo "  系统停止完成"
echo "========================================="
