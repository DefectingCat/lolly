#!/usr/bin/env bash
# bench-compare.sh — 比较两次基准测试结果，检测性能回归。
#
# 用法:
#   ./scripts/bench-compare.sh <old-summary> <new-summary>
#
# 返回码:
#   0 — 无显著回归
#   1 — 检测到显著回归（默认阈值: latency +10%, allocs +10%）
#
# 示例:
#   ./scripts/bench-compare.sh benchmarks/v0.4.0/summary.txt benchmarks/v0.5.0/summary.txt

set -euo pipefail

OLD="${1:-}"
NEW="${2:-}"

if [[ -z "$OLD" || -z "$NEW" ]]; then
    echo "用法: $0 <old-summary> <new-summary>" >&2
    exit 2
fi

if [[ ! -f "$OLD" ]]; then
    echo "错误: 找不到旧摘要文件: $OLD" >&2
    exit 2
fi

if [[ ! -f "$NEW" ]]; then
    echo "错误: 找不到新摘要文件: $NEW" >&2
    exit 2
fi

LATENCY_THRESH="${LATENCY_THRESH:-10.0}"
ALLOCS_THRESH="${ALLOCS_THRESH:-10.0}"
RPS_THRESH="${RPS_THRESH:--10.0}"

echo "=== 基准比较 ==="
printf "%-40s %12s %12s %12s\n" "Benchmark" "Old" "New" "Change%"
echo "----------------------------------------------------------------------"

REGRESSION=0

# 提取并比较关键微基准（ns/op 和 B/op）
# 格式: BenchmarkName-NN 1234 ns/op 567 B/op 890 allocs/op
compare_metric() {
    local bench="$1"
    local metric="$2"
    local thresh="$3"
    local better_is_lower="${4:-1}"

    local old_val new_val
    old_val=$(grep -E "^${bench}" "$OLD" | grep -oE "[0-9]+(\.[0-9]+)?[[:space:]]*${metric}" | head -1 | awk '{print $1}')
    new_val=$(grep -E "^${bench}" "$NEW" | grep -oE "[0-9]+(\.[0-9]+)?[[:space:]]*${metric}" | head -1 | awk '{print $1}')

    if [[ -z "$old_val" || -z "$new_val" ]]; then
        return 0
    fi

    if awk -v o="$old_val" 'BEGIN { exit (o == 0) ? 0 : 1 }'; then
        printf "%-40s %12s %12s %11s%%\n" "$bench ($metric)" "$old_val" "$new_val" "N/A"
        return 0
    fi

    local change
    change=$(awk -v o="$old_val" -v n="$new_val" 'BEGIN { printf "%.2f", ((n - o) / o) * 100 }')
    local abs_change
    abs_change=$(awk -v c="$change" 'BEGIN { printf "%.2f", c < 0 ? -c : c }')

    printf "%-40s %12s %12s %11s%%\n" "$bench ($metric)" "$old_val" "$new_val" "$change"

    if awk -v c="$abs_change" -v t="$thresh" 'BEGIN { exit (c > t) ? 0 : 1 }'; then
        if [[ "$better_is_lower" == "1" && $(awk -v c="$change" 'BEGIN { print (c > 0) ? 1 : 0 }') -eq 1 ]]; then
            echo "  ⚠️  回归警告: $bench $metric 增加 ${change}% (阈值 ${thresh}%)" >&2
            REGRESSION=1
        elif [[ "$better_is_lower" == "0" && $(awk -v c="$change" 'BEGIN { print (c < 0) ? 1 : 0 }') -eq 1 ]]; then
            echo "  ⚠️  回归警告: $bench $metric 降低 ${change}% (阈值 ${thresh}%)" >&2
            REGRESSION=1
        fi
    fi
}

# 关键基准测试前缀列表（前缀匹配）
BENCHMARKS=(
    "BenchmarkAccessLogProcess"
    "BenchmarkFileCacheGet"
    "BenchmarkProxyCacheGet"
    "BenchmarkStaticFile"
    "BenchmarkStaticIndex"
    "BenchmarkStaticTryFiles"
    "BenchmarkProxyForward"
    "BenchmarkProxyHostClient"
    "BenchmarkProxyWithMockBackend"
    "BenchmarkMiddlewareProcessChain"
    "BenchmarkMiddlewareChainExecution"
    "BenchmarkCompressionMiddleware"
    "BenchmarkDNSResolverLookupWithCache"
)

for bench in "${BENCHMARKS[@]}"; do
    compare_metric "$bench" "ns/op" "$LATENCY_THRESH" 1
    compare_metric "$bench" "B/op" "$ALLOCS_THRESH" 1
done

echo ""
if [[ "$REGRESSION" -eq 0 ]]; then
    echo "✅ 未检测到显著性能回归"
    exit 0
else
    echo "❌ 检测到性能回归，请检查上述警告"
    exit 1
fi
