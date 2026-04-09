# Makefile - Lolly Build Commands

# 版本信息
APP_NAME := lolly
VERSION := 0.1.0
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
GO_VERSION := $(shell go version | awk '{print $$3}')
BUILD_PLATFORM := $(shell go env GOOS)/$(shell go env GOARCH)
BUILD_DIR := bin

# 静态构建（禁用 CGO）
CGO_DISABLE := CGO_ENABLED=0

# 生产构建标志（体积优化）
LDFLAGS := -ldflags "-s -w \
	-X 'rua.plus/lolly/internal/app.Version=$(VERSION)' \
	-X 'rua.plus/lolly/internal/app.GitCommit=$(GIT_COMMIT)' \
	-X 'rua.plus/lolly/internal/app.GitBranch=$(GIT_BRANCH)' \
	-X 'rua.plus/lolly/internal/app.BuildTime=$(BUILD_TIME)' \
	-X 'rua.plus/lolly/internal/app.GoVersion=$(GO_VERSION)' \
	-X 'rua.plus/lolly/internal/app.BuildPlatform=$(BUILD_PLATFORM)'"

# 运行时性能优化标志
PERF_GCFLAGS := -gcflags="-l=4"
PERF_ASMFLAGS := -asmflags="-l=4"

# Go 文件
MAIN_PATH := main.go

# 默认目标
.DEFAULT_GOAL := build

# ============================================
# 构建命令
# ============================================

# 本地构建（静态链接）
build:
	@echo "Building $(APP_NAME) (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)"
	@echo "Version: $(VERSION) | Commit: $(GIT_COMMIT) | Platform: $(BUILD_PLATFORM)"

# 生产构建（体积优化，静态链接）
build-prod:
	@echo "Building $(APP_NAME) for production (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "Production build complete: $(BUILD_DIR)/$(APP_NAME)"

# 生产构建（最大运行时性能，静态链接）
build-perf:
	@echo "Building $(APP_NAME) with max runtime performance (static)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLE) go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "Performance build complete: $(BUILD_DIR)/$(APP_NAME)"

# PGO 构建（需先收集 profile，静态链接）
PGO_PROFILE ?= default.pgo
build-pgo:
	@echo "Building $(APP_NAME) with PGO optimization (static)..."
	@mkdir -p $(BUILD_DIR)
	if [ -f $(PGO_PROFILE) ]; then \
		$(CGO_DISABLE) go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
			-pgo=$(PGO_PROFILE) -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH); \
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
	go test -v ./...

# 运行测试（带覆盖率）
test-cover:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# 运行基准测试
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

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
	@echo "Formatting code with goimports..."
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	else \
		echo "goimports not installed. Run: go install golang.org/x/tools/cmd/goimports@latest"; \
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

# 代码检查
check: fmt lint test
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
	@echo "Clean complete."

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
	@echo "  make bench          - Run benchmarks"
	@echo "  make bench-stat     - Run benchmarks with statistical sampling (10x)"
	@echo "  make bench-compare  - Compare against baseline (needs benchstat)"
	@echo "  make bench-save     - Save current results as baseline"
	@echo "  make bench-check    - Check for performance regressions"
	@echo ""
	@echo "Quality:"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make check          - Format + lint + test"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps           - Download dependencies"
	@echo "  make update-deps    - Update dependencies"
	@echo ""
	@echo "Other:"
	@echo "  make install        - Install to GOPATH/bin"
	@echo "  make clean          - Clean build artifacts"
	@echo ""