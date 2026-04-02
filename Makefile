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

# 生产构建标志
LDFLAGS := -ldflags "-s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.gitCommit=$(GIT_COMMIT)' \
	-X 'main.gitBranch=$(GIT_BRANCH)' \
	-X 'main.buildTime=$(BUILD_TIME)' \
	-X 'main.goVersion=$(GO_VERSION)' \
	-X 'main.buildPlatform=$(BUILD_PLATFORM)'"

# Go 文件
MAIN_PATH := main.go

# 默认目标
.DEFAULT_GOAL := build

# ============================================
# 构建命令
# ============================================

# 本地构建
build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)"
	@echo "Version: $(VERSION) | Commit: $(GIT_COMMIT) | Platform: $(BUILD_PLATFORM)"

# 生产构建（优化）
build-prod:
	@echo "Building $(APP_NAME) for production..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "Production build complete: $(BUILD_DIR)/$(APP_NAME)"

# 跨平台构建
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-linux-amd64"

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "Built: $(BUILD_DIR)/$(APP_NAME)-darwin-{amd64,arm64}"

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
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

# ============================================
# 代码质量
# ============================================

# 格式化代码
fmt:
	@echo "Formatting code..."
	go fmt ./...

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
	@echo "Build:"
	@echo "  make build          - Build for current platform"
	@echo "  make build-prod     - Production build (optimized)"
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