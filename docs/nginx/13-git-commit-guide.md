# Git Commit Message 规范

本项目采用 [Conventional Commits](https://www.conventionalcommits.org/) 规范。

## 格式

```
type(scope): subject

[optional body]

[optional footer(s)]
```

## 类型（Type）

| 类型 | 说明 | 示例 |
|------|------|------|
| `feat` | 新功能 | feat(proxy): add upstream health check |
| `fix` | 修复 bug | fix(server): correct graceful shutdown timing |
| `docs` | 文档变更 | docs(config): update directive reference |
| `refactor` | 重构（不改变功能行为） | refactor(http): simplify header parsing |
| `test` | 测试相关 | test(proxy): add load balancer unit tests |
| `perf` | 性能优化 | perf(event): reduce connection memory overhead |
| `chore` | 构建、工具、依赖 | chore(deps): bump go to 1.26 |
| `style` | 代码格式（不影响逻辑） | style(log): fix indentation |

## 范围（Scope）

范围标识变更涉及的模块，基于项目结构划分：

| 范围 | 说明 |
|------|------|
| `server` | 服务器启动、生命周期管理 |
| `proxy` | 反向代理、上游连接 |
| `config` | 配置解析、指令处理 |
| `http` | HTTP 协议处理、请求响应 |
| `stream` | TCP/UDP 流代理 |
| `event` | 事件循环、连接管理 |
| `log` | 日志系统 |
| `tls` | SSL/TLS 支持 |
| `core` | 核心模块、公共工具 |

## 规则

### Subject（必填）

- 使用祈使句（add、fix、update，而非 added、fixes）
- 首字母小写
- 不以句号结尾
- 限制 50 字符以内

### Body（可选）

- 说明变更的**原因**和**影响**
- 与 subject 空一行
- 每行 72 字符以内

### Footer（可选）

- 关联 Issue：`Closes #123`、`Fixes #456`
- Breaking change：`BREAKING CHANGE: xxx`

## 示例

### 简单提交

```
feat(http): add HTTP/2 protocol support
```

### 带 Body

```
fix(proxy): resolve upstream connection leak

Connection pool was not properly releasing idle connections
when upstream server closed the socket, causing resource
exhaustion under high load.

Fixes #42
```

### Breaking Change

```
refactor(config)!: change directive syntax for upstream block

BREAKING CHANGE: `upstream` block now requires `server` directive
instead of direct address specification. Update config files:

  upstream backend {
    server 127.0.0.1:8080;  # new format
  }
```

### 多范围

```
feat(server,proxy): support dynamic upstream reload

Allow adding/removing upstream servers without full restart.
```

## 工具配置（可选）

### commitlint

```json
// .commitlintrc.json
{
  "extends": ["@commitlint/config-conventional"],
  "rules": {
    "scope-enum": [2, "always", [
      "server", "proxy", "config", "http", "stream",
      "event", "log", "tls", "core"
    ]]
  }
}
```

### Git Hook (husky)

```bash
# .husky/commit-msg
npx --no -- commitlint --edit $1
```

---

## 参考

- [Conventional Commits](https://www.conventionalcommits.org/)
- [Angular Commit Guidelines](https://github.com/angular/angular/blob/master/CONTRIBUTING.md#commit)
- [Go Project Commit Style](https://github.com/golang/go/wiki/CommitMessage)