# Makefile - Lolly Build Commands

APP_NAME := lolly
FALLBACK_VERSION := 0.3.0
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "$(FALLBACK_VERSION)")

GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC' 2>/dev/null || echo "unknown")
GO_VERSION := $(shell go env GOVERSION)
BUILD_PLATFORM := $(shell go env GOOS)/$(shell go env GOARCH)
BUILD_DIR := bin

CGO_DISABLE := CGO_ENABLED=0

LDFLAGS := -ldflags "-s -w \
	-X 'rua.plus/lolly/internal/version.Version=$(VERSION)' \
	-X 'rua.plus/lolly/internal/version.GitCommit=$(GIT_COMMIT)' \
	-X 'rua.plus/lolly/internal/version.GitBranch=$(GIT_BRANCH)' \
	-X 'rua.plus/lolly/internal/version.BuildTime=$(BUILD_TIME)' \
	-X 'rua.plus/lolly/internal/version.GoVersion=$(GO_VERSION)' \
	-X 'rua.plus/lolly/internal/version.BuildPlatform=$(BUILD_PLATFORM)'"

PERF_GCFLAGS := -gcflags="-l=4"
PERF_ASMFLAGS := -asmflags="-l=4"

MAIN_PATH := main.go

ifeq ($(OS),Windows_NT)
EXECUTABLE := $(BUILD_DIR)/$(APP_NAME).exe
else
EXECUTABLE := $(BUILD_DIR)/$(APP_NAME)
endif

.DEFAULT_GOAL := build

# ============================================
# 构建命令
# ============================================

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

build: | $(BUILD_DIR)
	@echo "Building $(APP_NAME) (static, optimized)..."
	$(CGO_DISABLE) go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(EXECUTABLE) $(MAIN_PATH)
	@echo "Build complete: $(EXECUTABLE)"

PGO_PROFILE ?= default.pgo
build-pgo: | $(BUILD_DIR)
	@echo "Building $(APP_NAME) with PGO optimization (static)..."
	if [ -f $(PGO_PROFILE) ]; then \
		$(CGO_DISABLE) go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
			-pgo=$(PGO_PROFILE) -o $(EXECUTABLE) $(MAIN_PATH); \
		echo "PGO build complete using: $(PGO_PROFILE)"; \
	else \
		echo "PGO profile not found: $(PGO_PROFILE)"; \
		echo "Run 'make pgo-collect' first to generate profile"; \
		exit 1; \
	fi

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

build-linux: | $(BUILD_DIR)
	@echo "Building for Linux amd64 (static)..."
	$(CGO_DISABLE) GOOS=linux GOARCH=amd64 go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-linux-amd64"

build-darwin: | $(BUILD_DIR)
	@echo "Building for macOS (static)..."
	$(CGO_DISABLE) GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	$(CGO_DISABLE) GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-darwin-{amd64,arm64}"

build-windows: | $(BUILD_DIR)
	@echo "Building for Windows amd64 (static)..."
	$(CGO_DISABLE) GOOS=windows GOARCH=amd64 go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe"

build-freebsd: | $(BUILD_DIR)
	@echo "Building for FreeBSD amd64 (static)..."
	$(CGO_DISABLE) GOOS=freebsd GOARCH=amd64 go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME)-freebsd-amd64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-freebsd-amd64"

build-openbsd: | $(BUILD_DIR)
	@echo "Building for OpenBSD amd64 (static)..."
	$(CGO_DISABLE) GOOS=openbsd GOARCH=amd64 go build $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath \
		-o $(BUILD_DIR)/$(APP_NAME)-openbsd-amd64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-openbsd-amd64"

build-all: build-linux build-darwin build-windows build-freebsd build-openbsd
	@echo "All platform builds complete."

# ============================================
# 开发命令
# ============================================

run:
	@echo "Running $(APP_NAME) in development mode..."
	go run $(MAIN_PATH) -c configs/lolly.yaml

test-config:
	@echo "Testing configuration..."
	go run $(MAIN_PATH) -t -c configs/lolly.yaml

version:
	@echo "$(APP_NAME) version $(VERSION)"
	@echo "Git: $(GIT_COMMIT) ($(GIT_BRANCH))"
	@echo "Built: $(BUILD_TIME)"
	@echo "Go: $(GO_VERSION)"

# ============================================
# 测试命令
# ============================================

test:
	@echo "Running tests..."
	go test -v ./internal/...

test-integration:
	@echo "Running L2 integration tests..."
	go test -v -tags=integration ./internal/integration/...

test-e2e:
	@echo "Running L3 E2E tests (parallel: $(or $(E2E_PARALLEL),4))..."
	go test -tags=e2e -parallel $(or $(E2E_PARALLEL),4) -count 1 ./internal/e2e/...

test-e2e-cover:
	@echo "Running L3 E2E tests with coverage..."
	go test -tags=e2e -coverprofile=e2e-coverage.out -coverpkg=./... ./internal/e2e/...
	go tool cover -html=e2e-coverage.out -o e2e-coverage.html
	@echo "E2E coverage report: e2e-coverage.html"

test-e2e-short:
	@echo "Running L3 E2E tests (short mode - testutil only)..."
	go test -tags=e2e -short -v ./internal/e2e/testutil/... -timeout 60s

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

test-cover:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

act:
	@echo "Running CI locally with act..."
	@if command -v act >/dev/null 2>&1; then \
		mkdir -p /tmp/artifacts && act --artifact-server-path /tmp/artifacts; \
	else \
		echo "act 未安装，运行: go install github.com/nektos/act@latest"; \
		exit 1; \
	fi

act-unit:
	@echo "Running unit tests job with act..."
	@if command -v act >/dev/null 2>&1; then \
		mkdir -p /tmp/artifacts && act -j unit --artifact-server-path /tmp/artifacts; \
	else \
		echo "act 未安装，运行: go install github.com/nektos/act@latest"; \
		exit 1; \
	fi

# ============================================
# 基准测试
# ============================================

bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -count=5 -run=^$$ ./internal/... | tee bench-results.txt

bench-stat:
	@echo "Running benchmarks with statistical sampling..."
	go test -bench=. -benchmem -count=10 -run=^$$ ./internal/... | tee benchmark-current.txt

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

bench-check:
	@echo "Checking for performance regressions..."
	@if [ ! -f .benchmark-thresholds.yaml ]; then \
		echo "阈值配置文件不存在: .benchmark-thresholds.yaml"; \
		echo "创建示例: echo 'threshold: 10%%' > .benchmark-thresholds.yaml"; \
		exit 1; \
	fi
	@if [ ! -f benchmark-baseline.txt ] || [ ! -f benchmark-current.txt ]; then \
		echo "缺少 baseline/current 数据，运行: make bench-save && make bench-stat"; \
		exit 1; \
	fi
	benchstat benchmark-baseline.txt benchmark-current.txt | python3 scripts/check_regression.py - --config .benchmark-thresholds.yaml

# ============================================
# 代码质量
# ============================================

fmt:
	@echo "Formatting code with gofumpt..."
	@if command -v gofumpt >/dev/null 2>&1; then \
		gofumpt -w .; \
	else \
		echo "gofumpt not installed. Run: go install mvdan.cc/gofumpt@latest"; \
		exit 1; \
	fi

lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		go vet ./...; \
	fi

modernize:
	@echo "Running modernize analyzer..."
	@if command -v modernize >/dev/null 2>&1; then \
		modernize ./internal/...; \
	else \
		echo "modernize not installed. Run: go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest"; \
		exit 1; \
	fi

modernize-fix:
	@echo "Running modernize analyzer with auto-fix..."
	@if command -v modernize >/dev/null 2>&1; then \
		modernize -fix ./internal/...; \
	else \
		echo "modernize not installed. Run: go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest"; \
		exit 1; \
	fi

check: fmt lint test-all
	@echo "All checks passed."

# ============================================
# 依赖管理
# ============================================

deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

update-deps:
	@echo "Updating dependencies..."
	go get -u
	go mod tidy

# ============================================
# 安装命令
# ============================================

install:
	@echo "Installing $(APP_NAME)..."
	$(CGO_DISABLE) go install $(LDFLAGS) $(PERF_GCFLAGS) $(PERF_ASMFLAGS) -trimpath $(MAIN_PATH)
	@echo "Installed to: $(shell go env GOPATH)/bin/$(APP_NAME)"

# ============================================
# 清理命令
# ============================================

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f e2e-coverage.out e2e-coverage.html
	rm -f bench-results.txt benchmark-current.txt benchmark-baseline.txt benchmark-comparison.txt
	go clean -cache -testcache
	@echo "Clean complete."

clean-mod:
	@echo "Cleaning module cache..."
	go clean -modcache
	@echo "Module cache cleaned."

# ============================================
# Docker 命令
# ============================================

DOCKER_ARCH := $(shell go env GOARCH 2>/dev/null || echo "amd64")

docker: | $(BUILD_DIR)
	@echo "Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown") \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		--build-arg BUILD_TIME='$(BUILD_TIME)' \
		--build-arg GO_VERSION='$(GO_VERSION)' \
		--build-arg BUILD_PLATFORM=linux/$(DOCKER_ARCH) \
		--build-arg GOPROXY='$(shell go env GOPROXY)' \
		--build-arg GOSUMDB='$(shell go env GOSUMDB)' \
		-t $(APP_NAME):$(VERSION) \
		-t $(APP_NAME):latest \
		.
	@echo "Docker image built: $(APP_NAME):$(VERSION), $(APP_NAME):latest"

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
	@echo "  make build-pgo      - PGO build (needs profile, use PGO_PROFILE=path)"
	@echo "  make pgo-collect    - Guide for collecting PGO profile"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make build-linux    - Build for Linux amd64"
	@echo "  make build-darwin   - Build for macOS (amd64 + arm64)"
	@echo "  make build-windows  - Build for Windows amd64"
	@echo "  make build-freebsd  - Build for FreeBSD amd64"
	@echo "  make build-openbsd  - Build for OpenBSD amd64"
	@echo ""
	@echo "Development:"
	@echo "  make run            - Run development server"
	@echo "  make test-config    - Test configuration file"
	@echo "  make version        - Show version info"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run unit tests"
	@echo "  make test-cover     - Run unit tests with coverage"
	@echo "  make test-integration - Run L2 integration tests"
	@echo "  make test-e2e       - Run L3 E2E tests (requires Docker)"
	@echo "  make test-e2e-cover - Run E2E tests with coverage"
	@echo "  make test-e2e-short - Run E2E tests (short mode)"
	@echo "  make test-all       - Run all tests (unit + integration + E2E)"
	@echo "  make act            - Run CI locally with act"
	@echo "  make act-unit       - Run unit tests job with act"
	@echo ""
	@echo "Benchmark:"
	@echo "  make bench          - Run benchmarks"
	@echo "  make bench-stat     - Run benchmarks with statistical sampling (10x)"
	@echo "  make bench-compare  - Compare against baseline (needs benchstat)"
	@echo "  make bench-save     - Save current results as baseline"
	@echo "  make bench-check    - Check for performance regressions (with thresholds)"
	@echo ""
	@echo "Quality:"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make modernize      - Check for modern Go patterns"
	@echo "  make modernize-fix  - Auto-fix modern Go patterns"
	@echo "  make check          - Format + lint + test-all"
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
	@echo "  make clean-mod      - Clean module cache"
	@echo ""

.PHONY: build build-pgo pgo-collect build-linux build-darwin build-windows build-freebsd build-openbsd build-all \
	run test-config version \
	test test-integration test-e2e test-e2e-cover test-e2e-short test-all test-cover \
	act act-unit \
	bench bench-stat bench-compare bench-save bench-check \
	fmt lint modernize modernize-fix check \
	deps update-deps install clean clean-mod \
	docker docker-push docker-clean help
