# lolly

高性能 HTTP 服务器，类似 nginx 的纯 Go 实现。YAML 配置，单二进制，支持反向代理、负载均衡、SSL/TLS、HTTP/2、HTTP/3、Lua 脚本。

Module: `rua.plus/lolly` | Go 1.26+ | CGO disabled (static builds)

## Commands

```
make build              # static build → bin/lolly
make test               # unit tests only (./internal/...)
make test-integration   # L2 integration tests (build tag: integration)
make test-e2e           # L3 E2E tests (build tag: e2e, requires Docker)
make test-all           # all three in parallel
make fmt                # gofumpt (not go fmt)
make lint               # golangci-lint (falls back to go vet)
make check              # fmt → lint → test-all
```

Run a single test or package:
```
go test -v -run TestName ./internal/config/...
go test -v ./internal/server/...
```

Integration/E2E require build tags:
```
go test -v -tags=integration ./internal/integration/...
go test -v -tags=e2e ./internal/e2e/...
```

## Architecture

- `main.go` → `internal/app` — CLI entry; `app.Run()` owns lifecycle
- `internal/config` — YAML config structs; `config.go` is the root, split into `server_config.go`, `ssl_config.go`, `proxy_config.go`, etc.
- `internal/server` — HTTP server core: routing, middleware chain, vhosts, upgrade/reload, connection pool
- `internal/proxy` — reverse proxy + load balancing
- `internal/middleware/` — individual middleware packages (compression, bodylimit, security, rewrite, accesslog, errorintercept)
- `internal/handler` — static file handler and other request handlers
- `internal/lua` — embedded Lua scripting (gopher-lua)
- `internal/converter/nginx` — nginx config importer
- `internal/{http2,http3,stream,ssl,sslutil,cache,loadbalance,resolver,logging,variable,matcher}` — supporting packages
- `internal/e2e` — L3 E2E tests (testcontainers, build tag `e2e`)
- `internal/integration` — L2 integration tests (build tag `integration`)

## Key Conventions

- HTTP stack: **fasthttp** + **fasthttp/router** (NOT net/http). All handlers use `fasthttp.RequestHandler` signature.
- Logging: **zerolog** (zero-alloc JSON). No fmt.Printf or log.Printf in production code.
- Config: YAML with `yaml` struct tags, parsed by `gopkg.in/yaml.v3`.
- Version: injected at build time via `-ldflags -X` into `internal/version`.
- Signals: SIGTERM/SIGINT = fast stop, SIGQUIT = graceful stop. Graceful upgrade via `internal/server/upgrade.go`.
- Formatter: **gofumpt** (stricter than gofmt). `make fmt` runs it.
- Linter: **golangci-lint** with v2 config (`.golangci.yml`). Uses `default: all` with many disabled — don't add new linter warnings.
- Build: always `CGO_ENABLED=0`, static linked, `-trimpath`.
- Tests use **testify** assertions.

## Gotchas

- fasthttp uses `[]byte` strings, NOT `string`. Use `[]byte("literal")` or `fasthttp.AcquireRequest()` patterns. Don't mix net/http types.
- E2E tests need Docker running (testcontainers-go). If Docker is unavailable, run `make test-e2e-short` for testutil-only tests.
- Config file path defaults to `lolly.yaml` in CWD. Use `-c path` or `--config path`.
- `--generate-config` outputs a full config template; `-o file` writes to file.
- `--import nginx.conf` converts nginx config to lolly YAML.
