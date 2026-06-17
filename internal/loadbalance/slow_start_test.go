package loadbalance

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSlowStartManager_StartStopStart(t *testing.T) {
	m := NewSlowStartManager(10 * time.Millisecond)

	// 第一次启动
	m.Start()
	if !m.running.Load() {
		t.Fatal("manager should be running after first Start")
	}

	// 停止
	m.Stop()
	if m.running.Load() {
		t.Fatal("manager should be stopped after Stop")
	}

	// 再次启动：验证 stopCh 被正确重建，updateLoop 不会立即退出
	m.Start()
	if !m.running.Load() {
		t.Fatal("manager should be running after second Start")
	}

	// 等待一段时间，确认 updateLoop 仍在运行
	time.Sleep(50 * time.Millisecond)
	if !m.running.Load() {
		t.Fatal("manager should still be running after sleep")
	}

	m.Stop()
}

// TestSlowStartManager_SetFindTarget 验证 findTarget 回调被正确使用。
func TestSlowStartManager_SetFindTarget(t *testing.T) {
	m := NewSlowStartManager(10 * time.Millisecond)

	target := &Target{URL: "http://backend:8080", Weight: 10, SlowStart: 100 * time.Millisecond}
	m.OnTargetHealthy(target)

	called := atomic.Bool{}
	m.SetFindTarget(func(url string) *Target {
		called.Store(true)
		if url == "http://backend:8080" {
			return target
		}
		return nil
	})

	m.Start()
	defer m.Stop()

	// 等待 updateLoop 执行
	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("findTarget callback should be called by updateLoop")
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
