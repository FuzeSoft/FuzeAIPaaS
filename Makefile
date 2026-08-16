.PHONY: build clean run docker-build docker-run frontend-dev frontend-build \
        scripts-perm e2e-distributed e2e-kserve e2e-hami e2e-fluid test-e2e \
        db-up db-down db-reset

# 清除所有生成物（与 .gitignore 对齐）：
#   - build/                : make build 输出目录（后端二进制 + 前端产物）
#   - bin/                  : go install / 手动放置的二进制
#   - frontend/dist         : 前端构建产物（npm run build 默认输出）
#   - frontend/node_modules : 前端依赖（条目众多，删除可能被环境安全策略拦截，
#                             用 - 前缀忽略错误，确保后续清理不被阻断）
#   - backend/data-operator : 手工 go build ./backend/cmd/data-operator 落到
#                             backend/ 下的游离二进制（源码在 backend/cmd/）
#   - *.db *.sqlite*        : 本地 SQLite 数据库文件（开发态回退数据库）
#   - *.test                : go test -c 编译产物
#   - *.out                 : go test -coverprofile 输出
#   - .pids/                : 运行时 PID 文件
#   - .logs/                : 运行时日志
#   - fuze-scheduler        : 历史顶层二进制（兼容旧路径）
clean:
	rm -rf build bin
	rm -rf frontend/dist
	-rm -rf frontend/node_modules
	rm -f backend/data-operator
	rm -f *.db *.sqlite *.sqlite3
	rm -f *.test *.out
	rm -rf .pids .logs
	rm -f fuze-scheduler

# 重新构建，全部生成物统一输出到顶层 build/ 目录。
# 不依赖 clean：仅清旧 build/ 产物，保留 frontend/node_modules 避免每次重装依赖。
# 如需彻底清理（含 node_modules / 数据库 / 覆盖率等），显式执行 make clean。
build:
	@rm -rf build
	@mkdir -p build
	go build -o build/fuze-scheduler ./backend/cmd/
	cd frontend && npm run build -- --outDir ../build/frontend

# 本地后端默认回退 SQLite（零依赖）；切 Postgres 需先 make db-up 再设环境变量：
#   export DB_DRIVER=postgres
#   export DB_DSN="postgres://fuze:fuze@localhost:5432/fuze?sslmode=disable"
# 多副本仅 Postgres 支持；SQLite 文件无法跨实例共享。
run:
	go run ./backend/cmd/

# ---- 本地数据库（docker-compose 自托管 Postgres）----
db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-reset:
	docker compose down -v

# 跑后端单测。默认 SQLite 内存库；可在 Postgres 下验证可移植性：
#   TEST_DB_DRIVER=postgres TEST_DB_DSN="postgres://fuze:fuze@localhost:5432/fuze?sslmode=disable" make test
test:
	TEST_DB_DRIVER=$(TEST_DB_DRIVER) TEST_DB_DSN=$(TEST_DB_DSN) go test ./backend/...

# 镜像 tag 可经 IMAGE_TAG 覆盖（默认取当前 commit 短 SHA，保证可复现）；
# 生产必须固定版本，勿使用 latest（不可复现且升级不可控）。
IMAGE_TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

docker-build:
	docker build -f Dockerfile.backend -t fuze-ai-paas-backend:$(IMAGE_TAG) .
	docker build -f Dockerfile.frontend -t fuze-ai-paas-frontend:$(IMAGE_TAG) .

docker-run:
	docker run -d -p 8080:8080 fuze-ai-paas-backend:latest
	docker run -d -p 3000:3000 fuze-ai-paas-frontend:latest

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

# 确保 scripts/ 下的脚本具备可执行权限
scripts-perm:
	chmod +x scripts/*.sh

# 分布式训练 (Volcano) 端到端联调
# 用法: make e2e-distributed [ARGS="--no-cleanup"] [WORKERS=4 FRAMEWORK=tensorflow]
# 也可透传环境变量，例如: make e2e-distributed BACKEND_URL=http://10.0.0.1:8080
e2e-distributed: scripts-perm
	./scripts/e2e-distributed-training.sh $(ARGS)

# KServe 推理服务端到端联调
e2e-kserve: scripts-perm
	./scripts/e2e-kserve.sh $(ARGS)

# HAMi GPU 显存/算力隔离端到端联调
e2e-hami: scripts-perm
	./scripts/e2e-hami.sh $(ARGS)

# Fluid 数据加速端到端联调
e2e-fluid: scripts-perm
	./scripts/e2e-fluid.sh $(ARGS)

# 端到端联调聚合目标：依次运行所有能力脚本
# 包含: distributed(Vocano) / kserve / hami / fluid
# 后续新增能力时，在此追加对应的 e2e-<capability> 依赖即可
test-e2e: scripts-perm e2e-distributed e2e-kserve e2e-hami e2e-fluid
	@echo "==> 所有端到端联调已执行完毕"
