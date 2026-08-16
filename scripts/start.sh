#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$PROJECT_DIR/.pids"
LOG_DIR="$PROJECT_DIR/.logs"

mkdir -p "$PID_DIR" "$LOG_DIR"

# ==================== 启动后端 ====================
start_backend() {
    echo "[INFO] 启动后端服务 (Go) ..."
    cd "$PROJECT_DIR/backend"
    go run cmd/main.go > "$LOG_DIR/backend.log" 2>&1 &
    BACKEND_PID=$!
    echo "$BACKEND_PID" > "$PID_DIR/backend.pid"
    echo "[OK] 后端已启动 (PID: $BACKEND_PID, 端口: 8080)"
}

# ==================== 启动前端 ====================
start_frontend() {
    echo "[INFO] 启动前端服务 (Vite) ..."
    cd "$PROJECT_DIR/frontend"
    npm run dev > "$LOG_DIR/frontend.log" 2>&1 &
    FRONTEND_PID=$!
    echo "$FRONTEND_PID" > "$PID_DIR/frontend.pid"
    echo "[OK] 前端已启动 (PID: $FRONTEND_PID, 端口: 3000)"
}

# ==================== 服务就绪检查 ====================
wait_for_service() {
    local url=$1
    local name=$2
    local max_retries=30
    local count=0
    echo "[INFO] 等待 ${name} 就绪 ($url) ..."
    while [ $count -lt $max_retries ]; do
        if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q "200\|204"; then
            echo "[OK] ${name} 已就绪"
            return 0
        fi
        sleep 1
        count=$((count + 1))
    done
    echo "[WARN] ${name} 可能未完全就绪，请检查日志"
}

# ==================== 主流程 ====================
echo "========================================="
echo "  Fuze AI PaaS - 系统启动"
echo "========================================="

case "${1:-all}" in
    backend)
        if [ -f "$PID_DIR/backend.pid" ] && kill -0 $(cat "$PID_DIR/backend.pid") 2>/dev/null; then
            echo "[WARN] 后端已在运行中 (PID: $(cat $PID_DIR/backend.pid))"
            exit 0
        fi
        start_backend
        wait_for_service "http://localhost:8080/api/v1/health" "后端"
        ;;
    frontend)
        if [ -f "$PID_DIR/frontend.pid" ] && kill -0 $(cat "$PID_DIR/frontend.pid") 2>/dev/null; then
            echo "[WARN] 前端已在运行中 (PID: $(cat $PID_DIR/frontend.pid))"
            exit 0
        fi
        start_frontend
        wait_for_service "http://localhost:3000" "前端"
        ;;
    all)
        # 后端
        if [ -f "$PID_DIR/backend.pid" ] && kill -0 $(cat "$PID_DIR/backend.pid") 2>/dev/null; then
            echo "[WARN] 后端已在运行中 (PID: $(cat $PID_DIR/backend.pid))"
        else
            start_backend
            wait_for_service "http://localhost:8080/api/v1/health" "后端"
        fi

        # 前端
        if [ -f "$PID_DIR/frontend.pid" ] && kill -0 $(cat "$PID_DIR/frontend.pid") 2>/dev/null; then
            echo "[WARN] 前端已在运行中 (PID: $(cat $PID_DIR/frontend.pid))"
        else
            start_frontend
            wait_for_service "http://localhost:3000" "前端"
        fi
        ;;
    *)
        echo "用法: $0 {backend|frontend|all}"
        exit 1
        ;;
esac

echo ""
echo "========================================="
echo "  系统启动完成"
echo "  前端: http://localhost:3000"
echo "  后端: http://localhost:8080"
echo "  PID 目录: $PID_DIR"
echo "  日志目录: $LOG_DIR"
echo "========================================="
