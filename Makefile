# mgate-cloud Makefile
#
# 常用目标：make help 查看全部。version 由 git describe 派生（可用 VERSION=... 覆盖），经 ldflags 注入二进制。

SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
VERSION := dev
endif
LDFLAGS := -s -w -X mgate-cloud/internal/version.Version=$(VERSION)
BIN := dist/mgate-cloud

.PHONY: help web build test vet fmt-check security release clean run

help: ## 显示可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

web: ## 安装并构建前端（输出 web/dist）
	npm --prefix web install
	npm --prefix web run build

build: web ## 构建内嵌前端的单二进制到 dist/
	@mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/mgate-cloud
	@echo "built $(BIN) (version=$(VERSION))"

test: ## 运行后端测试
	go test ./...

vet: ## go vet 静态检查
	go vet ./...

security: ## 仅运行禁止远程 shell 能力的静态安全测试
	go test ./internal/securitycheck/...

release: ## 完整发布构建（测试 + 前端 + 二进制）
	bash ./scripts/release.sh

clean: ## 清理构建产物
	rm -rf dist mgate-cloud mgate-cloud.exe mgate-agent-sim mgate-agent-sim.exe

run: build ## 本地构建并运行（dev 模式）
	MGATE_ADMIN_USERNAME=admin MGATE_ADMIN_PASSWORD=change-me $(BIN)
