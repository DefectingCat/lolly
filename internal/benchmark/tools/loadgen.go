// Package tools provides testing utilities and mock infrastructure for benchmark tests.
package tools

import (
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

// FasthttpLoadGenerator is a load generator using fasthttp client.
type FasthttpLoadGenerator struct {
	client *fasthttp.HostClient
	addr   string
	stats  LoadGenStats
	mu     sync.Mutex
}

// LoadGenStats contains load generator statistics.
type LoadGenStats struct {
	Latencies     []time.Duration // For percentile calculation
	TotalRequests int
	SuccessCount  int
	ErrorCount    int
	TotalDuration time.Duration
	MinLatency    time.Duration
	MaxLatency    time.Duration
	MeanLatency   time.Duration
	P50Latency    time.Duration
	P90Latency    time.Duration
	P99Latency    time.Duration
	QPS           float64
}

// NewFasthttpLoadGenerator creates a new load generator for the given address.
func NewFasthttpLoadGenerator(addr string) *FasthttpLoadGenerator {
	return &FasthttpLoadGenerator{
		client: &fasthttp.HostClient{
			Addr:     addr,
			MaxConns: 1000,
		},
		addr: addr,
		stats: LoadGenStats{
			MinLatency: time.Duration(math.MaxInt64),
		},
	}
}

// Run executes a load test with n requests using the specified concurrency.
// Returns collected statistics.
func (lg *FasthttpLoadGenerator) Run(n int, concurrency int) *LoadGenStats {
	var wg sync.WaitGroup
	requestsPerWorker := n / concurrency

	// Channels for collecting metrics
	latencyChan := make(chan time.Duration, n)
	errorChan := make(chan error, n)

	start := time.Now()

	for range concurrency {
		wg.Go(func() {
			req := fasthttp.AcquireRequest()
			resp := fasthttp.AcquireResponse()
			defer fasthttp.ReleaseRequest(req)
			defer fasthttp.ReleaseResponse(resp)

			for range requestsPerWorker {
				req.SetRequestURI("http://" + lg.addr + "/")
				req.Header.SetMethod("GET")

				reqStart := time.Now()
				err := lg.client.Do(req, resp)
				latency := time.Since(reqStart)

				latencyChan <- latency
				if err != nil {
					errorChan <- err
				}
			}
		})
	}

	wg.Wait()
	close(latencyChan)
	close(errorChan)

	totalDuration := time.Since(start)

	// Collect latencies
	latencies := make([]time.Duration, 0, n)
	for lat := range latencyChan {
		latencies = append(latencies, lat)
	}

	// Count errors
	errorCount := 0
	for err := range errorChan {
		_ = err // Error recorded, used for counting
		errorCount++
	}

	// Calculate statistics
	lg.mu.Lock()
	defer lg.mu.Unlock()

	lg.stats.TotalRequests = n
	lg.stats.ErrorCount = errorCount
	lg.stats.SuccessCount = n - errorCount
	lg.stats.TotalDuration = totalDuration
	lg.stats.QPS = float64(n) / totalDuration.Seconds()
	lg.stats.Latencies = latencies

	// Calculate latency distribution
	if len(latencies) > 0 {
		slices.Sort(latencies)

		lg.stats.MinLatency = latencies[0]
		lg.stats.MaxLatency = latencies[len(latencies)-1]

		// Calculate mean
		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		lg.stats.MeanLatency = sum / time.Duration(len(latencies))

		// Calculate percentiles
		lg.stats.P50Latency = latencies[len(latencies)*50/100]
		lg.stats.P90Latency = latencies[len(latencies)*90/100]
		lg.stats.P99Latency = latencies[len(latencies)*99/100]
	}

	return &lg.stats
}

// GetStats returns current statistics without running a test.
func (lg *FasthttpLoadGenerator) GetStats() *LoadGenStats {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	return &lg.stats
}

// RunParallel runs requests in parallel using testing.PB.
// This is designed for use with Go benchmark functions.
func (lg *FasthttpLoadGenerator) RunParallel(pb *testing.PB) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	for pb.Next() {
		req.SetRequestURI("http://" + lg.addr + "/")
		req.Header.SetMethod("GET")
		_ = lg.client.Do(req, resp)
	}
}
