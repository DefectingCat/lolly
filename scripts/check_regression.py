#!/usr/bin/env python3
"""
回归检测脚本 - 解析 benchstat 输出并检测性能回归

用法:
    python check_regression.py <benchstat_output_file>
    python check_regression.py --config .benchmark-thresholds.yaml benchmark.txt
    python check_regression.py --help

退出码:
    0 - 无回归或轻微变化
    1 - 检测到 WARNING 级别回归
    2 - 检测到 BLOCK 级别回归
"""

import argparse
import re
import sys
import os
from dataclasses import dataclass
from typing import List, Optional, Tuple, Dict

# 尝试导入 YAML 解析器
try:
    import yaml
    HAS_YAML = True
except ImportError:
    HAS_YAML = False


@dataclass
class BenchmarkResult:
    """单个基准测试结果"""
    name: str
    old_time: Optional[float]
    new_time: Optional[float]
    old_bytes: Optional[float]
    new_bytes: Optional[float]
    old_allocs: Optional[float]
    new_allocs: Optional[float]
    p_value: Optional[float]

    @property
    def time_change_pct(self) -> Optional[float]:
        """计算时间变化百分比 (负值表示性能下降)"""
        if self.old_time and self.new_time and self.old_time > 0:
            return (self.old_time - self.new_time) / self.old_time * 100
        return None

    @property
    def bytes_change_pct(self) -> Optional[float]:
        """计算内存分配变化百分比"""
        if self.old_bytes and self.new_bytes and self.old_bytes > 0:
            return (self.old_bytes - self.new_bytes) / self.old_bytes * 100
        return None


def parse_benchstat_line(line: str) -> Optional[BenchmarkResult]:
    """
    解析 benchstat 输出的一行

    格式示例:
        BenchmarkFoo-8    1000000    1000 ns/op    ~    950 ns/op     5.00%
    """
    # 匹配时间基准测试行
    # 格式: Name  old-ns/op  new-ns/op  delta
    time_pattern = r'^(\S+)\s+'  # 基准名称
    time_pattern += r'(?:(\d+(?:\.\d+)?)\s+ns/op\s+)?'  # 旧值
    time_pattern += r'(?:~\s+)?'  # 分隔符
    time_pattern += r'(?:(\d+(?:\.\d+)?)\s+ns/op\s+)?'  # 新值
    time_pattern += r'(?:([+-]?\d+\.\d+)%\s+)?'  # 变化百分比

    match = re.match(time_pattern, line.strip())
    if not match:
        return None

    name = match.group(1)
    old_time = float(match.group(2)) if match.group(2) else None
    new_time = float(match.group(3)) if match.group(3) else None

    # 尝试提取 p-value（如果有）
    p_value = None
    p_match = re.search(r'p=([\d.]+)', line)
    if p_match:
        p_value = float(p_match.group(1))

    return BenchmarkResult(
        name=name,
        old_time=old_time,
        new_time=new_time,
        old_bytes=None,
        new_bytes=None,
        old_allocs=None,
        new_allocs=None,
        p_value=p_value
    )


def parse_benchstat_output(content: str) -> List[BenchmarkResult]:
    """解析完整的 benchstat 输出"""
    results = []
    lines = content.split('\n')

    for line in lines:
        line = line.strip()
        if not line or line.startswith('name') or line.startswith('---'):
            continue

        result = parse_benchstat_line(line)
        if result:
            results.append(result)

    return results


def classify_regression(result: BenchmarkResult) -> Tuple[str, float, Optional[float]]:
    """
    分类回归级别

    返回值: (level, change_pct, p_value)
        level: "OK", "WARNING", "BLOCK"
    """
    change = result.time_change_pct
    if change is None:
        return "OK", 0.0, result.p_value

    # 正值表示性能提升，负值表示性能下降
    if change <= -15:
        return "BLOCK", change, result.p_value
    elif change <= -5:
        return "WARNING", change, result.p_value
    else:
        return "OK", change, result.p_value


def check_regressions(results: List[BenchmarkResult]) -> Tuple[int, int, int]:
    """
    检查所有基准测试的回归情况

    返回: (ok_count, warning_count, block_count)
    """
    ok_count = 0
    warning_count = 0
    block_count = 0

    print("=" * 80)
    print("性能回归检测结果")
    print("=" * 80)
    print(f"{'基准测试':<40} {'变化':<12} {'P值':<12} {'级别':<10}")
    print("-" * 80)

    for result in results:
        level, change, p_value = classify_regression(result)
        p_str = f"{p_value:.4f}" if p_value else "N/A"
        change_str = f"{change:+.2f}%" if change else "N/A"

        if level == "OK":
            ok_count += 1
            icon = "✓"
        elif level == "WARNING":
            warning_count += 1
            icon = "⚠"
        else:
            block_count += 1
            icon = "✗"

        print(f"{result.name:<40} {change_str:<12} {p_str:<12} {icon} {level}")

    print("-" * 80)
    print(f"总结: {ok_count} 正常, {warning_count} 警告, {block_count} 阻断")
    print("=" * 80)

    return ok_count, warning_count, block_count


def extract_module_name(benchmark_name: str) -> str:
    """从基准测试名称提取模块名。

    Args:
        benchmark_name: 完整的基准测试名称，如 "BenchmarkCacheGet-8"

    Returns:
        str: 模块名，如 "cache"
    """
    # 移除 Benchmark 前缀和 -N 后缀
    name = benchmark_name
    if name.startswith('Benchmark'):
        name = name[9:]  # 移除 "Benchmark"

    # 移除 -N 后缀
    if '-' in name:
        name = name.split('-')[0]

    # 提取模块名（第一个单词的小写形式）
    module = ''
    for c in name:
        if c.isupper() and module:
            break
        module += c.lower()

    # 常见模块名映射
    module_map = {
        'cache': 'cache',
        'proxy': 'proxy',
        'loadbalance': 'loadbalance',
        'round': 'loadbalance',
        'weighted': 'loadbalance',
        'consistent': 'loadbalance',
        'least': 'loadbalance',
        'ip': 'loadbalance',
        'variable': 'variable',
        'expand': 'variable',
        'gzip': 'compression',
        'brotli': 'compression',
        'compression': 'compression',
        'ratelimiter': 'ratelimit',
        'rate': 'ratelimit',
        'sliding': 'sliding_window',
        'accesslog': 'accesslog',
        'access': 'accesslog',
        'static': 'static',
        'resolver': 'resolver',
        'dns': 'resolver',
        'ssl': 'ssl',
        'vhost': 'vhost',
        'rewrite': 'rewrite',
        'bodylimit': 'bodylimit',
        'auth': 'auth',
        'headers': 'headers',
    }

    return module_map.get(module, module or 'default')


def load_threshold_config(config_path: str) -> dict:
    """加载阈值配置文件。

    Args:
        config_path: 配置文件路径

    Returns:
        dict: 配置字典
    """
    if not HAS_YAML:
        print("警告: PyYAML 未安装，无法加载配置文件", file=sys.stderr)
        return {}

    if not os.path.exists(config_path):
        print(f"警告: 配置文件不存在: {config_path}", file=sys.stderr)
        return {}

    try:
        with open(config_path, 'r') as f:
            return yaml.safe_load(f) or {}
    except Exception as e:
        print(f"警告: 加载配置文件失败: {e}", file=sys.stderr)
        return {}


def get_thresholds(config: dict, environment: str, module: str,
                   default_warning: float, default_block: float) -> Tuple[float, float]:
    """获取指定环境和模块的阈值。

    Args:
        config: 配置字典
        environment: 环境名称 ("local" 或 "ci")
        module: 模块名
        default_warning: 默认警告阈值
        default_block: 默认阻塞阈值

    Returns:
        (warning_threshold, block_threshold)
    """
    if not config:
        return default_warning, default_block

    # 获取环境配置
    env_config = config.get('environments', {}).get(environment, {})
    thresholds = env_config.get('thresholds', {})

    # 先查找模块特定阈值
    if module in thresholds:
        module_thresholds = thresholds[module]
        warning = module_thresholds.get('warning', -default_warning)
        block = module_thresholds.get('block', -default_block)
        return abs(warning), abs(block)

    # 使用默认阈值
    if 'default' in thresholds:
        default = thresholds['default']
        warning = default.get('warning', -default_warning)
        block = default.get('block', -default_block)
        return abs(warning), abs(block)

    return default_warning, default_block


def classify_regression_with_config(result: BenchmarkResult, config: dict,
                                     environment: str, default_warning: float,
                                     default_block: float) -> Tuple[str, float, Optional[float]]:
    """
    分类回归级别（支持配置文件）

    返回值: (level, change_pct, p_value)
        level: "OK", "WARNING", "BLOCK"
    """
    change = result.time_change_pct
    if change is None:
        return "OK", 0.0, result.p_value

    # 获取模块阈值
    module = extract_module_name(result.name)
    warning_threshold, block_threshold = get_thresholds(
        config, environment, module, default_warning, default_block
    )

    # 正值表示性能提升，负值表示性能下降
    if change <= -block_threshold:
        return "BLOCK", change, result.p_value
    elif change <= -warning_threshold:
        return "WARNING", change, result.p_value
    else:
        return "OK", change, result.p_value


def check_regressions_with_config(results: List[BenchmarkResult], config: dict,
                                   environment: str, default_warning: float,
                                   default_block: float) -> Tuple[int, int, int]:
    """
    检查所有基准测试的回归情况（支持配置文件）

    返回: (ok_count, warning_count, block_count)
    """
    ok_count = 0
    warning_count = 0
    block_count = 0

    print("=" * 80)
    print(f"性能回归检测结果 (环境: {environment})")
    print("=" * 80)
    print(f"{'基准测试':<40} {'变化':<12} {'P值':<12} {'级别':<10}")
    print("-" * 80)

    for result in results:
        level, change, p_value = classify_regression_with_config(
            result, config, environment, default_warning, default_block
        )
        p_str = f"{p_value:.4f}" if p_value else "N/A"
        change_str = f"{change:+.2f}%" if change else "N/A"

        if level == "OK":
            ok_count += 1
            icon = "✓"
        elif level == "WARNING":
            warning_count += 1
            icon = "⚠"
        else:
            block_count += 1
            icon = "✗"

        print(f"{result.name:<40} {change_str:<12} {p_str:<12} {icon} {level}")

    print("-" * 80)
    print(f"总结: {ok_count} 正常, {warning_count} 警告, {block_count} 阻断")
    print("=" * 80)

    return ok_count, warning_count, block_count


def main():
    parser = argparse.ArgumentParser(
        description='解析 benchstat 输出并检测性能回归',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
阈值说明:
  -5%%  ~  WARNING  - 性能下降超过5%，需要关注
  -15%% ~  BLOCK    - 性能下降超过15%，阻止合并

示例:
  python check_regression.py benchmark-comparison.txt
  python check_regression.py --config .benchmark-thresholds.yaml --environment ci benchmark.txt
  benchstat old.txt new.txt | python check_regression.py -
'''
    )
    parser.add_argument('file', help='benchstat 输出文件路径，或 "-" 从 stdin 读取')
    parser.add_argument('--warning-threshold', type=float, default=5.0,
                        help='警告阈值百分比（默认: 5）')
    parser.add_argument('--block-threshold', type=float, default=15.0,
                        help='阻断阈值百分比（默认: 15）')
    parser.add_argument('--p-value', type=float, default=0.05,
                        help='统计显著性 P 值阈值（默认: 0.05）')
    parser.add_argument('--config', '-c', type=str,
                        help='阈值配置文件路径 (.yaml)')
    parser.add_argument('--environment', '-e', type=str, default='local',
                        choices=['local', 'ci'],
                        help='环境类型（默认: local）')

    args = parser.parse_args()

    # 加载配置文件
    config = {}
    if args.config:
        config = load_threshold_config(args.config)

    # 读取输入
    if args.file == '-':
        content = sys.stdin.read()
    else:
        try:
            with open(args.file, 'r') as f:
                content = f.read()
        except FileNotFoundError:
            print(f"错误: 文件 '{args.file}' 不存在", file=sys.stderr)
            sys.exit(1)
        except IOError as e:
            print(f"错误: 无法读取文件: {e}", file=sys.stderr)
            sys.exit(1)

    if not content.strip():
        print("警告: 输入为空", file=sys.stderr)
        sys.exit(0)

    # 解析结果
    results = parse_benchstat_output(content)

    if not results:
        print("警告: 未找到有效的基准测试结果", file=sys.stderr)
        sys.exit(0)

    # 检查回归
    if config:
        ok_count, warning_count, block_count = check_regressions_with_config(
            results, config, args.environment,
            args.warning_threshold, args.block_threshold
        )
    else:
        ok_count, warning_count, block_count = check_regressions(results)

    # 设置退出码
    if block_count > 0:
        print(f"\n检测到 {block_count} 个 BLOCK 级别回归，建议阻止合并")
        sys.exit(2)
    elif warning_count > 0:
        print(f"\n检测到 {warning_count} 个 WARNING 级别回归，建议检查")
        sys.exit(1)
    else:
        print("\n未发现性能回归")
        sys.exit(0)


if __name__ == '__main__':
    main()
