package loadbalance

import (
	"sync"
	"testing"
	"time"
)

func TestEWMAStats_BasicRecord(t *testing.T) {
	stats := NewEWMAStats()

	stats.Record(50*time.Millisecond, 100*time.Millisecond)

	if stats.HeaderTime() != 50*time.Millisecond {
		t.Errorf("expected header time %v, got %v", 50*time.Millisecond, stats.HeaderTime())
	}
	if stats.LastByteTime() != 100*time.Millisecond {
		t.Errorf("expected last byte time %v, got %v", 100*time.Millisecond, stats.LastByteTime())
	}
	if stats.SampleCount() != 1 {
		t.Errorf("expected sample count 1, got %d", stats.SampleCount())
	}
}

func TestEWMAStats_Convergence(t *testing.T) {
	stats := NewEWMAStats()

	value := 100 * time.Millisecond
	for i := 0; i < 10; i++ {
		stats.Record(value, value)
	}

	// alpha=0.3, after 10 samples should be within 10ms of 100ms
	diff := stats.LastByteTime() - value
	if diff < 0 {
		diff = -diff
	}
	if diff > 10*time.Millisecond {
		t.Errorf("expected convergence within 10ms, got diff=%v, value=%v", diff, stats.LastByteTime())
	}
}

func TestEWMAStats_Concurrent(t *testing.T) {
	stats := NewEWMAStats()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				stats.Record(time.Millisecond, 2*time.Millisecond)
			}
		}()
	}
	wg.Wait()

	if stats.SampleCount() != 100*100 {
		t.Errorf("expected sample count %d, got %d", 100*100, stats.SampleCount())
	}
}
