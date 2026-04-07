<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# workflows

## Purpose
GitHub Actions 工作流定义，自动化构建、测试和基准测试回归检测流程。

## Key Files

| File | Description |
|------|-------------|
| `benchmark.yml` | 基准测试工作流，运行性能测试并检测回归 |

## For AI Agents

### Working In This Directory
- 工作流使用 Go 最新稳定版本
- 基准测试结果存储用于历史比较
- 回归检测失败会阻止 PR 合并

### Testing Requirements
- 工作流自动执行，本地可通过 `make benchmark` 预运行
- 回归阈值在 `scripts/check_regression.py` 中定义

### Common Patterns
- 触发条件：`on: push, pull_request`
- 步骤：checkout → setup-go → make benchmark → check_regression
- 失败处理：输出报告，标记 PR 检查失败

## Dependencies

### Internal
- `../../Makefile` - 构建命令
- `../../scripts/check_regression.py` - 回归检测脚本

### External
- GitHub Actions - 运行环境
- Go toolchain - 编译和测试

<!-- MANUAL: -->