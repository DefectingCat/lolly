//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
package testutil

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// ConcurrentRequestConfig 并发请求配置。
type ConcurrentRequestConfig struct {
	URL        string
	Count      int
	Timeout    time.Duration
	ExpectCode int
}

// ConcurrentRequestResult 并发请求结果。
type ConcurrentRequestResult struct {
	Index      int
	StatusCode int
	Error      error
}

// RunConcurrentRequests 运行并发请求并返回结果。
//
// 使用 sync.WaitGroup 实现真正的并发。
// 返回所有请求的结果，包括状态码和错误。
func RunConcurrentRequests(cfg ConcurrentRequestConfig) []ConcurrentRequestResult {
	results := make([]ConcurrentRequestResult, cfg.Count)
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	for i := 0; i < cfg.Count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			result := ConcurrentRequestResult{Index: index}

			resp, err := client.Get(cfg.URL)
			if err != nil {
				result.Error = fmt.Errorf("request %d failed: %w", index, err)
				results[index] = result
				return
			}

			// 读取并丢弃响应体，确保连接可复用
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			result.StatusCode = resp.StatusCode
			results[index] = result
		}(i)
	}

	wg.Wait()
	return results
}

// VerifyConcurrentResults 验证并发请求结果。
//
// 检查所有请求是否返回预期状态码。
// 返回失败的请求列表。
func VerifyConcurrentResults(t *testing.T, results []ConcurrentRequestResult, expectCode int) []ConcurrentRequestResult {
	var failures []ConcurrentRequestResult

	for _, r := range results {
		if r.Error != nil {
			t.Logf("Request %d error: %v", r.Index, r.Error)
			failures = append(failures, r)
			continue
		}

		if r.StatusCode != expectCode {
			t.Logf("Request %d: expected %d, got %d", r.Index, expectCode, r.StatusCode)
			failures = append(failures, r)
		}
	}

	return failures
}

// RunAndVerifyConcurrentRequests 运行并发请求并验证结果。
//
// 组合 RunConcurrentRequests 和 VerifyConcurrentResults。
// 如果有任何失败，返回错误列表。
func RunAndVerifyConcurrentRequests(t *testing.T, cfg ConcurrentRequestConfig) []ConcurrentRequestResult {
	results := RunConcurrentRequests(cfg)
	failures := VerifyConcurrentResults(t, results, cfg.ExpectCode)
	return failures
}
