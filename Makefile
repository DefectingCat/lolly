# Makefile - Lolly Build Commands

# 版本信息
APP_NAME := lolly
VERSION := 0.2.2

# 临时目录（避免 tmpfs 空间不足）
TMPDIR := $(shell mkdir -p tmp && realpath tmp)
export TMPDIR
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC' 2>/dev/null || echo "unknown")
GO_VERSION := $(shell go env GOVERSION)
BUILD_PLATFORM := $(shell go env GOOS)/$(shell go env GOARCH)
BUILD_DIR := bin

# 静态构建（禁用 CGO）
CGO_DISABLE := CGO_ENABLED=0

# 生产构建标志
LDFLAGS := -ldflags "-s -w \
	-X 'rua.plus/lolly/internal/version.Version=$(VERSION)' \
	-X 'rua.plus/lolly/internal/version.GitCommit=$(GIT_COMMIT)' \
	-X 'rua.plus/lolly/internal/version.GitBranch=$(GIT_BRANCH)' \
	-X 'rua.plus/lolly/internal/version.BuildTime=$(BUILD_TIME)' \
	-X 'rua.plus/lolly/internal/version.GoVersion=$(GO_VERSION)' \
	-X 'rua.plus/lolly/internal/version.BuildPlatform=$(BUILD_PLATFORM)'"

# 运行时性能优化标志
PERF_GCFLAGS := -gcflags="-l=4"
PERF_ASMFLAGS := -asmflags="-l=4"

# Go 文件
MAIN_PATH := main.go

# Windows 可执行文件扩展名
ifeq ($(OS),Windows_NT)
EXECUTABLE := $(BUILD_DIR)/$(APP_NAME).exe
else
EXECUTABLE := $(BUILD_DIR)/$(APP_NAME)
endif

# 默认目标
.DEFAULT_GOAL := build

# ============================================
# 构建命令
# ============================================

# 生产构建（最大运行时性能，静态链接）
build:
	@echo "Building $(APP_NAME) with max runtime performance (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(EXECUTABLE) $(MAIN_PATH)
	@echo "Performance build complete: $(EXECUTABLE)"

# PGO 构建（需先收集 profile，静态链接）
PGO_PROFILE ?= default.pgo
build-pgo:
	@echo "Building $(APP_NAME) with PGO optimization (static)..."
	@mkdir -p $(BUILD_DIR)
	if [ -f $(PGO_PROFILE) ]; then \
		$(CGO_DISABLE) go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
			-pgo=$(PGO_PROFILE) -o $(EXECUTABLE) $(MAIN_PATH); \
		echo "PGO build complete using: $(PGO_PROFILE)"; \
	else \
		echo "PGO profile not found: $(PGO_PROFILE)"; \
		echo "Run 'make pgo-collect' first to generate profile"; \
		exit 1; \
	fi

# 收集 PGO profile（运行代表性 workload）
pgo-collect:
	@echo "=== PGO Profile Collection Guide ==="
	@echo ""
	@echo "Step 1: Enable pprof in your config file:"
	@echo "  monitoring:"
	@echo "    pprof:"
	@echo "      enabled: true"
	@echo "      path: /debug/pprof"
	@echo "      allow: [\"127.0.0.1\"]"
	@echo ""
	@echo "Step 2: Build and run lolly with representative workload:"
	@echo "  make build && ./bin/lolly -c configs/lolly.yaml"
	@echo ""
	@echo "Step 3: Collect CPU profile (run during peak load):"
	@echo "  curl http://localhost:<port>/debug/pprof/profile?seconds=30 > $(PGO_PROFILE)"
	@echo ""
	@echo "Step 4: Build with PGO optimization:"
	@echo "  make build-pgo"
	@echo ""
	@echo "Available pprof endpoints:"
	@echo "  /debug/pprof          - Index page"
	@echo "  /debug/pprof/profile  - CPU profile (add ?seconds=N)"
	@echo "  /debug/pprof/heap     - Memory profile"
	@echo "  /debug/pprof/goroutine - Goroutine count"
	@echo ""
	@echo "Tip: Profile during real workload for best PGO results"

# 跨平台构建（静态链接）
build-linux:
	@echo "Building for Linux (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-linux-amd64"

build-darwin:
	@echo "Building for macOS (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	$(CGO_DISABLE) GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-darwin-{amd64,arm64}"

build-windows:
	@echo "Building for Windows (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe"

# 构建所有平台
build-all: build-linux build-darwin build-windows
	@echo "All platform builds complete."

# ============================================
# 开发命令
# ============================================

# 运行开发服务器
run:
	@echo "Running $(APP_NAME) in development mode..."
	go run $(MAIN_PATH) -c configs/lolly.yaml

# 测试配置
test-config:
	@echo "Testing configuration..."
	go run $(MAIN_PATH) -t -c configs/lolly.yaml

# 显示版本
version:
	@echo "$(APP_NAME) version $(VERSION)"
	@echo "Git: $(GIT_COMMIT) ($(GIT_BRANCH))"
	@echo "Built: $(BUILD_TIME)"
	@echo "Go: $(GO_VERSION)"

# ============================================
# 测试命令
# ============================================

# 运行所有测试
test:
	@echo "Running tests..."
	go test -v ./internal/...

# 运行 L2 集成测试（无需 Docker）
test-integration:
	@echo "Running L2 integration tests..."
	go test -v -tags=integration ./internal/integration/...

# 运行 L3 E2E 测试（需要 Docker）
test-e2e:
	@echo "Running L3 E2E tests (parallel: $(or $(E2E_PARALLEL),4))..."
	go test -tags=e2e -parallel $(or $(E2E_PARALLEL),4) -count 1 ./internal/e2e/...

# 运行 L3 E2E 测试（带覆盖率）
test-e2e-cover:
	@echo "Running L3 E2E tests with coverage..."
	go test -tags=e2e -coverprofile=e2e-coverage.out -coverpkg=./... ./internal/e2e/...
	go tool cover -html=e2e-coverage.out -o e2e-coverage.html
	@echo "E2E coverage report: e2e-coverage.html"

# 运行 L3 E2E 测试（短模式，仅运行工具测试）
test-e2e-short:
	@echo "Running L3 E2E tests (short mode - testutil only)..."
	go test -tags=e2e -short -v ./internal/e2e/testutil/... -timeout 60s

# 运行所有测试（单元 + 集成 + E2E）— 并行执行
test-all:
	@echo "Running all tests in parallel..."
	@FAIL=0; \
	$(MAKE) test & PID1=$$!; \
	$(MAKE) test-integration & PID2=$$!; \
	$(MAKE) test-e2e & PID3=$$!; \
	wait $$PID1 || FAIL=1; \
	wait $$PID2 || FAIL=1; \
	wait $$PID3 || FAIL=1; \
	if [ $$FAIL -eq 0 ]; then echo "All tests passed."; fi; \
	exit $$FAIL

# 运行测试（带覆盖率）
test-cover:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# 运行 act 本地 CI 测试
act:
	@echo "Running CI locally with act..."
	@if command -v act >/dev/null 2>&1; then \
		mkdir -p /tmp/artifacts && act --artifact-server-path /tmp/artifacts; \
	else \
		echo "act 未安装，运行: go install github.com/nektos/act@latest"; \
		exit 1; \
	fi

# 运行 act 单个 job
act-unit:
	@echo "Running unit tests job with act..."
	@if command -v act >/dev/null 2>&1; then \
		mkdir -p /tmp/artifacts && act -j unit --artifact-server-path /tmp/artifacts; \
	else \
		echo "act 未安装，运行: go install github.com/nektos/act@latest"; \
		exit 1; \
	fi

# 运行基准测试（输出到文件，仅扫描 internal 目录）
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -count=5 -run=^$$ ./internal/... | tee bench-results.txt

# 运行基准测试（统计模式，10次采样）
bench-stat:
	@echo "Running benchmarks with statistical sampling..."
	go test -bench=. -benchmem -count=10 ./... | tee benchmark-current.txt

# 对比基准测试结果（需要 benchstat）
bench-compare:
	@echo "Comparing benchmarks..."
	@if command -v benchstat >/dev/null 2>&1; then \
		if [ -f benchmark-baseline.txt ]; then \
			benchstat benchmark-baseline.txt benchmark-current.txt; \
		else \
			echo "基准线文件 benchmark-baseline.txt 不存在，运行当前基准测试..."; \
			$(MAKE) bench-stat; \
		fi \
	else \
		echo "benchstat 未安装，运行: go install golang.org/x/perf/cmd/benchstat@latest"; \
		exit 1; \
	fi

# 保存当前基准结果为基准线
bench-save:
	@echo "Saving benchmark baseline..."
	@if [ -f benchmark-current.txt ]; then \
		cp benchmark-current.txt benchmark-baseline.txt; \
		echo "基准线已保存到 benchmark-baseline.txt"; \
	else \
		echo "运行基准测试并保存..."; \
		$(MAKE) bench-stat; \
		cp benchmark-current.txt benchmark-baseline.txt; \
	fi

# 检查性能回归（需要 Python）
bench-check:
	@echo "Checking for performance regressions..."
	@if [ -f benchmark-comparison.txt ]; then \
		python scripts/check_regression.py benchmark-comparison.txt; \
	elif command -v benchstat >/dev/null 2>&1 && [ -f benchmark-baseline.txt ] && [ -f benchmark-current.txt ]; then \
		benchstat benchmark-baseline.txt benchmark-current.txt > benchmark-comparison.txt; \
		python scripts/check_regression.py benchmark-comparison.txt; \
	else \
		echo "需要 benchstat 和基准线/当前结果文件"; \
		echo "运行: make bench-save && make bench-stat && make bench-check"; \
		exit 1; \
	fi

# ============================================
# 代码质量
# ============================================

# 格式化代码（使用 goimports 替代 go fmt）
fmt:
	@echo "Formatting code with gofumpt..."
	@if command -v gofumpt >/dev/null 2>&1; then \
		gofumpt -w .; \
	else \
		echo "gofumpt not installed. Run: go install mvdan.cc/gofumpt@latest"; \
		exit 1; \
	fi

# 静态检查
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		go vet ./...; \
	fi

# 现代化检查（检测可使用新 Go 特性的代码）
modernize:
	@echo "Running modernize analyzer..."
	@if command -v modernize >/dev/null 2>&1; then \
		modernize ./internal/...; \
	else \
		echo "modernize not installed. Run: go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest"; \
		exit 1; \
	fi

# 现代化检查并自动修复
modernize-fix:
	@echo "Running modernize analyzer with auto-fix..."
	@if command -v modernize >/dev/null 2>&1; then \
		modernize -fix ./internal/...; \
	else \
		echo "modernize not installed. Run: go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest"; \
		exit 1; \
	fi

# 代码检查
check: fmt lint test-all
	@echo "All checks passed."

# ============================================
# 依赖管理
# ============================================

# 下载依赖
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# 更新依赖
update-deps:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

# ============================================
# 安装命令
# ============================================

# 安装到系统
install:
	@echo "Installing $(APP_NAME)..."
	go install $(LDFLAGS) $(MAIN_PATH)
	@echo "Installed to: $(shell go env GOPATH)/bin/$(APP_NAME)"

# ============================================
# 清理命令
# ============================================

# 清理构建产物
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean -cache -testcache
	# go clean -modcache
	@echo "Clean complete."

# ============================================
# Docker 命令
# ============================================

# 构建 Docker 镜像
docker:
	@echo "Building Docker image..."
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown") --build-arg GIT_BRANCH=$(GIT_BRANCH) --build-arg BUILD_TIME='$(BUILD_TIME)' --build-arg GO_VERSION='$(GO_VERSION)' --build-arg BUILD_PLATFORM=linux/$(shell go env GOARCH 2>/dev/null || echo "amd64") --build-arg GOPROXY='$(shell go env GOPROXY)' --build-arg GOSUMDB='$(shell go env GOSUMDB)' -t $(APP_NAME):$(VERSION) -t $(APP_NAME):latest .
	@echo "Docker image built: $(APP_NAME):$(VERSION), $(APP_NAME):latest"

# 推送 Docker 镜像（需要先 docker login）
docker-push:
	@echo "Pushing Docker image..."
	@echo "Usage: make docker-push REGISTRY=<registry>"
	@if [ -z "$(REGISTRY)" ]; then \
		echo "Error: REGISTRY not specified"; \
		echo "Example: make docker-push REGISTRY=docker.io/myuser"; \
		exit 1; \
	fi
	docker tag $(APP_NAME):$(VERSION) $(REGISTRY)/$(APP_NAME):$(VERSION)
	docker tag $(APP_NAME):latest $(REGISTRY)/$(APP_NAME):latest
	docker push $(REGISTRY)/$(APP_NAME):$(VERSION)
	docker push $(REGISTRY)/$(APP_NAME):latest
	@echo "Pushed to: $(REGISTRY)/$(APP_NAME):$(VERSION)"

# 清理 Docker 镜像
docker-clean:
	@echo "Cleaning Docker images..."
	docker rmi $(APP_NAME):$(VERSION) $(APP_NAME):latest 2>/dev/null || true
	@echo "Docker images cleaned."

# ============================================
# 帮助
# ============================================

help:
	@echo "$(APP_NAME) Makefile Commands"
	@echo ""
	@echo "Build (static linked):"
	@echo "  make build          - Build for current platform"
	@echo "  make build-prod     - Production build (size optimized)"
	@echo "  make build-perf     - Production build (max runtime performance)"
	@echo "  make build-pgo      - PGO build (needs profile, use PGO_PROFILE=path)"
	@echo "  make pgo-collect    - Guide for collecting PGO profile"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make build-linux    - Build for Linux amd64"
	@echo "  make build-darwin   - Build for macOS (amd64 + arm64)"
	@echo "  make build-windows  - Build for Windows amd64"
	@echo ""
	@echo "Development:"
	@echo "  make run            - Run development server"
	@echo "  make test-config    - Test configuration file"
	@echo "  make version        - Show version info"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run all tests"
	@echo "  make test-cover     - Run tests with coverage"
	@echo "  make test-integration - Run L2 integration tests"
	@echo "  make test-e2e       - Run L3 E2E tests (requires Docker)"
	@echo "  make test-e2e-cover - Run E2E tests with coverage"
	@echo "  make test-e2e-short - Run E2E tests (short mode)"
	@echo "  make test-all       - Run all tests (unit + integration + E2E)"
	@echo "  make act            - Run CI locally with act"
	@echo "  make act-unit       - Run unit tests job with act"
	@echo "  make bench          - Run benchmarks"
	@echo "  make bench-stat     - Run benchmarks with statistical sampling (10x)"
	@echo "  make bench-compare  - Compare against baseline (needs benchstat)"
	@echo "  make bench-save     - Save current results as baseline"
	@echo "  make bench-check    - Check for performance regressions"
	@echo ""
	@echo "Quality:"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make modernize      - Check for modern Go patterns"
	@echo "  make modernize-fix  - Auto-fix modern Go patterns"
	@echo "  make check          - Format + lint + test"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps           - Download dependencies"
	@echo "  make update-deps    - Update dependencies"
	@echo ""
	@echo "Docker:"
	@echo "  make docker         - Build Docker image"
	@echo "  make docker-push    - Push to registry (REGISTRY=<url>)"
	@echo "  make docker-clean   - Remove local images"
	@echo ""
	@echo "Other:"
	@echo "  make install        - Install to GOPATH/bin"
	@echo "  make clean          - Clean build artifacts"
	@echo ""

# 回归检测（使用阈值配置文件）
bench-regression:
	@echo "Detecting performance regressions with thresholds..."
	@if [ ! -f .benchmark-thresholds.yaml ]; then \
		echo "阈值配置文件不存在: .benchmark-thresholds.yaml"; \
		exit 1; \
	fi
	@if [ -f bench-old.txt ] && [ -f bench-new.txt ]; then \
		benchstat bench-old.txt bench-new.txt | python3 scripts/check_regression.py - --config .benchmark-thresholds.yaml; \
	elif [ -f benchmark-baseline.txt ] && [ -f benchmark-current.txt ]; then \
		benchstat benchmark-baseline.txt benchmark-current.txt | python3 scripts/check_regression.py - --config .benchmark-thresholds.yaml; \
	else \
		echo "需要 bench-old.txt 和 bench-new.txt 或 baseline/current 文件"; \
		echo "运行: make bench-stat && mv benchmark-current.txt bench-old.txt && make bench-stat && mv benchmark-current.txt bench-new.txt"; \
		exit 1; \
	fi
