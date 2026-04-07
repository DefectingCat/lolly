<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# .github

## Purpose
GitHub 配置目录，包含 CI/CD 工作流定义，自动化构建、测试和基准测试回归检测。

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `workflows/` | GitHub Actions 工作流定义 |

## For AI Agents

### Working In This Directory
- CI 工作流在 PR 和推送时自动触发
- 基准测试回归检测防止性能退化合并
- 工作流使用 Makefile 命令执行任务

### Testing Requirements
- CI 自动运行测试，无需手动触发
- 本地可通过 `make check` 预验证

### Common Patterns
- 工作流触发条件：push 到 master、PR 创建/更新
- 测试步骤：fmt → lint → test → benchmark
- 回归检测：比较基准测试结果差异

## Dependencies

### Internal
- `../Makefile` - 构建命令定义
- `../scripts/` - 辅助脚本

<!-- MANUAL: -->