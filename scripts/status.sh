#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$PROJECT_DIR/.pids"

echo "========================================="
echo "  Fuze AI PaaS - 服务状态"
echo "========================================="
echo ""

check_service() {
    local name=$1
    local port=$2
    local pid_file="$PID_DIR/${name}.pid"

    if [ -f "$pid_file" ]; then
        PID=$(cat "$pid_file")
        if kill -0 "$PID" 2>/dev/null; then
            echo "  [●] ${name} (PID: $PID, 端口: $port) - 运行中"
        else
            echo "  [○] ${name} (PID 文件存在但进程已退出) - 已停止"
        fi
    else
        # 也检查端口是否被占用
        if lsof -i :$port -sTCP:LISTEN -t > /dev/null 2>&1; then
            echo "  [●] ${name} (端口: $port) - 运行中 (非本脚本启动)"
        else
            echo "  [○] ${name} - 未运行"
        fi
    fi
}

check_service "backend"  "8080"
check_service "frontend" "3000"

echo ""
