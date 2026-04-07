<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# scripts

## Purpose
构建和测试辅助脚本目录，包含回归检测、CI/CD 相关的自动化脚本。

## Key Files

| File | Description |
|------|-------------|
| `check_regression.py` | 基准测试回归检测脚本，比较新旧结果检测性能退化 |

## For AI Agents

### Working In This Directory
- 回归检测脚本用于 CI/CD 流程中自动检测性能退化
- Python 脚本需要依赖环境配置
- 脚本通过 Makefile 命令调用

### Testing Requirements
- 无独立测试，作为 CI/CD 工具使用
- 通过 `make benchmark-regression` 执行回归检测

### Common Patterns
- 回归检测比较基准测试结果的 QPS 和延迟变化
- 配置阈值定义可接受的性能波动范围
- 输出报告用于 PR 审核和决策

## Dependencies

### Internal
- `../internal/benchmark/` - 基准测试结果数据源

### External
- Python 3.x - 脚本运行环境

<!-- MANUAL: -->