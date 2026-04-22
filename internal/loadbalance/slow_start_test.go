package loadbalance

import (
	"sync"
	"testing"
	"time"
)

func TestSlowStartManager_OnTargetHealthy(t *testing.T) {
	mgr := NewSlowStartManager(100 * time.Millisecond)

	target := &Target{
		URL:       "http://backend:8080",
		Weight:    10,
		SlowStart: 5 * time.Second,
	}

	mgr.OnTargetHealthy(target)

	// 验证初始权重为 1
	if target.EffectiveWeight.Load() != 1 {
		t.Errorf("EffectiveWeight = %d, want 1", target.EffectiveWeight.Load())
	}

	// 验证状态已记录
	state := mgr.GetState(target.URL)
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if state.BaseWeight != 10 {
		t.Errorf("BaseWeight = %d, want 10", state.BaseWeight)
	}
}

func TestSlowStartManager_OnTargetUnhealthy(t *testing.T) {
	mgr := NewSlowStartManager(100 * time.Millisecond)

	target := &Target{
		URL:       "http://backend:8080",
		Weight:    10,
		SlowStart: 5 * time.Second,
	}

	mgr.OnTargetHealthy(target)
	mgr.OnTargetUnhealthy(target)

	// 验证状态已清除
	state := mgr.GetState(target.URL)
	if state != nil {
		t.Error("state should be nil after unhealthy")
	}

	// 验证权重已重置
	if target.EffectiveWeight.Load() != 0 {
		t.Errorf("EffectiveWeight = %d, want 0", target.EffectiveWeight.Load())
	}
}

func TestSlowStartManager_WeightProgression(t *testing.T) {
	mgr := NewSlowStartManager(50 * time.Millisecond)

	target := &Target{
		URL:       "http://backend:8080",
		Weight:    10,
		SlowStart: 200 * time.Millisecond,
	}

	var mu sync.Mutex
	targets := map[string]*Target{target.URL: target}
	mgr.SetFindTarget(func(url string) *Target {
		mu.Lock()
		defer mu.Unlock()
		return targets[url]
	})

	mgr.OnTargetHealthy(target)
	mgr.Start()
	defer mgr.Stop()

	// 等待慢启动完成
	time.Sleep(300 * time.Millisecond)

	// 验证权重已达到配置值或重置为 0
	ew := target.EffectiveWeight.Load()
	if ew != 0 && ew != int64(target.Weight) {
		t.Errorf("EffectiveWeight = %d, want 0 or %d", ew, target.Weight)
	}
}

func TestSlowStartManager_NoSlowStart(t *testing.T) {
	mgr := NewSlowStartManager(100 * time.Millisecond)

	target := &Target{
		URL:       "http://backend:8080",
		Weight:    10,
		SlowStart: 0, // 未配置慢启动
	}

	mgr.OnTargetHealthy(target)

	// 验证没有设置有效权重
	if target.EffectiveWeight.Load() != 0 {
		t.Errorf("EffectiveWeight = %d, want 0 (no slow start)", target.EffectiveWeight.Load())
	}

	// 验证状态未记录
	state := mgr.GetState(target.URL)
	if state != nil {
		t.Error("state should be nil when slow_start is 0")
	}
}

func TestTarget_GetEffectiveWeight(t *testing.T) {
	tests := []struct {
		name            string
		weight          int
		effectiveWeight int64
		want            int
	}{
		{
			name:            "no slow start",
			weight:          10,
			effectiveWeight: 0,
			want:            10,
		},
		{
			name:            "slow start in progress",
			weight:          10,
			effectiveWeight: 5,
			want:            5,
		},
		{
			name:            "slow start at 1",
			weight:          10,
			effectiveWeight: 1,
			want:            1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &Target{
				Weight: tt.weight,
			}
			target.EffectiveWeight.Store(tt.effectiveWeight)

			got := target.GetEffectiveWeight()
			if got != tt.want {
				t.Errorf("GetEffectiveWeight() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSlowStartManager_GetAllStates(t *testing.T) {
	mgr := NewSlowStartManager(100 * time.Millisecond)

	target1 := &Target{
		URL:       "http://backend1:8080",
		Weight:    10,
		SlowStart: 5 * time.Second,
	}
	target2 := &Target{
		URL:       "http://backend2:8080",
		Weight:    5,
		SlowStart: 3 * time.Second,
	}

	mgr.OnTargetHealthy(target1)
	mgr.OnTargetHealthy(target2)

	states := mgr.GetAllStates()
	if len(states) != 2 {
		t.Errorf("len(states) = %d, want 2", len(states))
	}
}
