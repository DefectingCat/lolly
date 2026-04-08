#!/usr/bin/env python3
"""分析基准测试方差，推导回归阈值。

该脚本用于：
1. 解析 benchstat 输出
2. 计算每个测试的方差和阈值建议
3. 支持正态性检验
4. 生成分环境阈值配置

用法:
    python scripts/analyze_variance.py benchmark-results.txt
    python scripts/analyze_variance.py --format yaml benchmark-results.txt
    go test -bench=. -count=50 ./... | tee results.txt | python scripts/analyze_variance.py -
"""

import sys
import re
import statistics
import argparse
from pathlib import Path
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass, field


@dataclass
class BenchmarkResult:
    """单个基准测试的结果。"""
    name: str
    ns_op_values: List[float] = field(default_factory=list)
    b_op_values: List[float] = field(default_factory=list)
    allocs_op_values: List[float] = field(default_factory=list)

    # 统计量
    ns_op_mean: float = 0.0
    ns_op_stdev: float = 0.0
    b_op_mean: float = 0.0
    b_op_stdev: float = 0.0
    allocs_op_mean: float = 0.0
    allocs_op_stdev: float = 0.0

    # 变异系数
    ns_op_cv: float = 0.0
    b_op_cv: float = 0.0
    allocs_op_cv: float = 0.0

    # 建议阈值
    threshold_warning: float = 0.0
    threshold_block: float = 0.0


def parse_benchstat_line(line: str) -> Optional[Tuple[str, float, float, float]]:
    """解析单行 benchstat 输出。

    格式示例:
        BenchmarkVariableExpand-8    123.4 ± 5%   1024 B/op   32 allocs/op
        BenchmarkCacheGet-8          45.67 ± 2%   256 B/op    8 allocs/op

    返回: (name, ns_op, b_op, allocs_op) 或 None
    """
    # 跳过空行和分隔符
    if not line.strip() or line.startswith('name') or line.startswith('---'):
        return None

    # 匹配基准测试行
    # 格式: name  ns/op ±%  B/op  allocs/op
    pattern = r'^(\S+)\s+([\d.]+)\s*(?:±\s*([\d.]+)%)?\s+([\d.]+)\s+([\d.]+)'
    match = re.match(pattern, line.strip())

    if match:
        name = match.group(1)
        ns_op = float(match.group(2))
        b_op = float(match.group(4))
        allocs_op = float(match.group(5))
        return (name, ns_op, b_op, allocs_op)

    return None


def parse_benchstat_output(text: str) -> Dict[str, BenchmarkResult]:
    """解析 benchstat 输出，提取每个测试的统计数据。

    Args:
        text: benchstat 命令的输出文本

    Returns:
        字典，key 为测试名，value 为 BenchmarkResult
    """
    results: Dict[str, BenchmarkResult] = {}

    for line in text.split('\n'):
        parsed = parse_benchstat_line(line)
        if parsed:
            name, ns_op, b_op, allocs_op = parsed
            if name not in results:
                results[name] = BenchmarkResult(name=name)
            results[name].ns_op_values.append(ns_op)
            results[name].b_op_values.append(b_op)
            results[name].allocs_op_values.append(allocs_op)

    return results


def parse_raw_benchmark_output(text: str) -> Dict[str, BenchmarkResult]:
    """解析原始 go test -bench 输出（非 benchstat 格式）。

    格式示例:
        BenchmarkVariableExpand-8          1000000      1234 ns/op     1024 B/op      32 allocs/op

    Args:
        text: go test -bench 命令的原始输出

    Returns:
        字典，key 为测试名，value 为 BenchmarkResult
    """
    results: Dict[str, BenchmarkResult] = {}

    # 匹配基准测试输出行
    pattern = r'^(Benchmark\S+)\s+(\d+)\s+([\d.]+)\s+ns/op\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op'

    for line in text.split('\n'):
        match = re.match(pattern, line.strip())
        if match:
            name = match.group(1)
            ns_op = float(match.group(3))
            b_op = float(match.group(4))
            allocs_op = float(match.group(5))

            if name not in results:
                results[name] = BenchmarkResult(name=name)
            results[name].ns_op_values.append(ns_op)
            results[name].b_op_values.append(b_op)
            results[name].allocs_op_values.append(allocs_op)

    return results


def calculate_statistics(results: Dict[str, BenchmarkResult]) -> Dict[str, BenchmarkResult]:
    """计算每个测试的统计量和建议阈值。

    阈值推导方法:
        threshold_warning = 2 * std_dev / mean * 100 (百分比)
        threshold_block = 3 * std_dev / mean * 100

    Args:
        results: 解析后的基准测试结果

    Returns:
        更新了统计量的结果字典
    """
    for name, result in results.items():
        if len(result.ns_op_values) < 2:
            continue

        # 计算 ns/op 统计量
        result.ns_op_mean = statistics.mean(result.ns_op_values)
        if len(result.ns_op_values) >= 2:
            result.ns_op_stdev = statistics.stdev(result.ns_op_values)

        # 计算 B/op 统计量
        if result.b_op_values:
            result.b_op_mean = statistics.mean(result.b_op_values)
            if len(result.b_op_values) >= 2:
                result.b_op_stdev = statistics.stdev(result.b_op_values)

        # 计算 allocs/op 统计量
        if result.allocs_op_values:
            result.allocs_op_mean = statistics.mean(result.allocs_op_values)
            if len(result.allocs_op_values) >= 2:
                result.allocs_op_stdev = statistics.stdev(result.allocs_op_values)

        # 计算变异系数 (CV = stdev / mean)
        if result.ns_op_mean > 0:
            result.ns_op_cv = (result.ns_op_stdev / result.ns_op_mean) * 100
            # 建议阈值: warning = 2*CV, block = 3*CV
            result.threshold_warning = 2 * result.ns_op_cv
            result.threshold_block = 3 * result.ns_op_cv

    return results


def check_normality(values: List[float]) -> Tuple[bool, str]:
    """简化的正态性检验。

    使用变异系数作为简化的正态性指标：
    - CV < 10%: 近似正态分布
    - CV >= 10%: 可能非正态，建议增大样本量

    对于严格的正态性检验，应使用 Shapiro-Wilk 检验，
    但那需要 scipy.stats 库。

    Args:
        values: 样本值列表

    Returns:
        (is_likely_normal, reason)
    """
    if len(values) < 10:
        return False, f"样本量不足 ({len(values)} < 10)，建议至少 50 次采样"

    mean = statistics.mean(values)
    if mean == 0:
        return False, "均值为零，无法计算变异系数"

    stdev = statistics.stdev(values)
    cv = (stdev / mean) * 100

    if cv < 5:
        return True, f"CV={cv:.1f}% < 5%，非常稳定"
    elif cv < 10:
        return True, f"CV={cv:.1f}% < 10%，近似正态分布"
    elif cv < 20:
        return True, f"CV={cv:.1f}% < 20%，可接受范围（建议增大样本量）"
    else:
        return False, f"CV={cv:.1f}% >= 20%，方差过大，检查测试稳定性"


def generate_threshold_config(results: Dict[str, BenchmarkResult],
                               environment: str = "local") -> str:
    """生成阈值配置文件内容。

    Args:
        results: 计算过统计量的结果
        environment: 环境名称（local 或 ci）

    Returns:
        YAML 格式的配置文件内容
    """
    lines = [
        "# 阈值推导方法论:",
        "# 1. 运行基准测试 50 次获取样本",
        "# 2. 计算每个测试的变异系数 (CV = stdev / mean * 100)",
        "# 3. threshold_warning = 2 * CV",
        "# 4. threshold_block = 3 * CV",
        "#",
        f"# 环境类型: {environment}",
        "# 生成时间: 自动生成",
        "",
        f"environments:",
        f"  {environment}:",
        f"    description: \"{'本地稳定环境' if environment == 'local' else 'CI 共享 runner 环境'}\"",
        f"    thresholds:",
    ]

    # 计算全局默认阈值
    all_cvs = [r.ns_op_cv for r in results.values() if r.ns_op_cv > 0]
    if all_cvs:
        median_cv = statistics.median(all_cvs)
        default_warning = round(2 * median_cv, 1)
        default_block = round(3 * median_cv, 1)
    else:
        default_warning = 5.0
        default_block = 12.0

    lines.append(f"      default:")
    lines.append(f"        warning: -{default_warning}")
    lines.append(f"        block: -{default_block}")

    # 为每个模块生成阈值
    module_cvs: Dict[str, List[float]] = {}
    for name, result in results.items():
        # 提取模块名 (Benchmark<Module>... -> Module)
        module_match = re.match(r'Benchmark([A-Z][a-z]+)', name)
        if module_match:
            module = module_match.group(1).lower()
        else:
            module = "default"

        if module not in module_cvs:
            module_cvs[module] = []
        if result.ns_op_cv > 0:
            module_cvs[module].append(result.ns_op_cv)

    for module, cvs in sorted(module_cvs.items()):
        if len(cvs) >= 1 and module != "default":
            avg_cv = statistics.mean(cvs)
            warning = round(2 * avg_cv, 1)
            block = round(3 * avg_cv, 1)
            lines.append(f"      {module}:")
            lines.append(f"        warning: -{warning}")
            lines.append(f"        block: -{block}")

    return "\n".join(lines)


def print_summary(results: Dict[str, BenchmarkResult]) -> None:
    """打印分析摘要。"""
    print("\n" + "=" * 80)
    print("基准测试方差分析报告")
    print("=" * 80)
    print(f"{'测试名称':<45} {'均值(ns)':>12} {'标准差':>10} {'CV%':>8} {'建议阈值':>12}")
    print("-" * 80)

    # 按 CV 排序
    sorted_results = sorted(results.items(),
                           key=lambda x: x[1].ns_op_cv,
                           reverse=True)

    for name, result in sorted_results:
        if result.ns_op_mean > 0:
            short_name = name[:44] if len(name) > 44 else name
            print(f"{short_name:<45} {result.ns_op_mean:>12.2f} "
                  f"{result.ns_op_stdev:>10.2f} {result.ns_op_cv:>8.1f} "
                  f"±{result.threshold_warning:.1f}%/±{result.threshold_block:.1f}%")

    print("=" * 80)

    # 稳定性摘要
    stable = sum(1 for r in results.values() if r.ns_op_cv < 5)
    acceptable = sum(1 for r in results.values() if 5 <= r.ns_op_cv < 10)
    unstable = sum(1 for r in results.values() if r.ns_op_cv >= 10)

    print(f"\n稳定性摘要:")
    print(f"  非常稳定 (CV < 5%):  {stable} 个测试")
    print(f"  稳定 (CV 5-10%):    {acceptable} 个测试")
    print(f"  不稳定 (CV >= 10%): {unstable} 个测试")

    if unstable > 0:
        print(f"\n警告: {unstable} 个测试方差过大，建议检查:")
        for name, result in sorted_results:
            if result.ns_op_cv >= 10:
                print(f"  - {name} (CV={result.ns_op_cv:.1f}%)")


def main():
    parser = argparse.ArgumentParser(
        description='分析基准测试方差，推导回归阈值',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
    # 分析 benchstat 输出
    python scripts/analyze_variance.py benchmark.txt

    # 分析原始 go test 输出
    go test -bench=. -count=50 ./... | python scripts/analyze_variance.py -

    # 生成 YAML 配置
    python scripts/analyze_variance.py --format yaml benchmark.txt
        """
    )
    parser.add_argument('input', nargs='?', default='-',
                       help='输入文件路径，- 表示从 stdin 读取')
    parser.add_argument('--format', choices=['text', 'yaml', 'json'],
                       default='text',
                       help='输出格式 (默认: text)')
    parser.add_argument('--environment', choices=['local', 'ci'],
                       default='local',
                       help='环境类型 (默认: local)')
    parser.add_argument('--output', '-o',
                       help='输出文件路径 (默认: stdout)')

    args = parser.parse_args()

    # 读取输入
    if args.input == '-':
        text = sys.stdin.read()
    else:
        path = Path(args.input)
        if not path.exists():
            print(f"错误: 文件不存在: {args.input}", file=sys.stderr)
            sys.exit(1)
        text = path.read_text()

    # 解析输入
    # 尝试 benchstat 格式，如果失败则尝试原始格式
    results = parse_benchstat_output(text)
    if not results:
        results = parse_raw_benchmark_output(text)

    if not results:
        print("错误: 未能解析任何基准测试数据", file=sys.stderr)
        sys.exit(1)

    # 计算统计量
    results = calculate_statistics(results)

    # 输出结果
    output = ""
    if args.format == 'yaml':
        output = generate_threshold_config(results, args.environment)
    elif args.format == 'json':
        import json
        output = json.dumps({
            name: {
                'mean_ns_op': r.ns_op_mean,
                'stdev_ns_op': r.ns_op_stdev,
                'cv_percent': r.ns_op_cv,
                'threshold_warning': r.threshold_warning,
                'threshold_block': r.threshold_block,
                'mean_b_op': r.b_op_mean,
                'mean_allocs_op': r.allocs_op_mean,
            }
            for name, r in results.items()
        }, indent=2)
    else:
        print_summary(results)
        return

    # 写入输出
    if args.output:
        Path(args.output).write_text(output)
        print(f"结果已写入: {args.output}")
    else:
        print(output)


if __name__ == '__main__':
    main()