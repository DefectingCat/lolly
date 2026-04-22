// Package loadbalance 提供负载均衡算法实现。
//
// 该文件实现 SlowStartManager 慢启动管理器，支持：
//   - 目标恢复健康时渐进式增加权重
//   - 权重从 1 线性增加到配置权重
//   - EffectiveWeight 字段方案（零侵入）
//
// 主要用途：
//
//	防止刚恢复的后端服务器被大量请求压垮。
//
// 作者：xfy
package loadbalance

import (
	"sync"
	"sync/atomic"
	"time"
)

// SlowStartManager 慢启动管理器。
//
// 统一管理所有目标的慢启动状态和权重计算。
// 使用 EffectiveWeight 字段方案，无需修改 Balancer 实现。
type SlowStartManager struct {
	targets  map[string]*SlowStartState // key: target.URL
	mu       sync.RWMutex
	interval time.Duration // 权重更新间隔
	stopCh   chan struct{}
	running  atomic.Bool

	// 目标查找回调
	findTarget func(url string) *Target
}

// SlowStartState 慢启动状态。
type SlowStartState struct {
	BaseWeight  int           // 配置的基础权重
	RecoverTime time.Time     // 恢复健康的时间
	SlowStart   time.Duration // 慢启动持续时间
}

// NewSlowStartManager 创建慢启动管理器。
func NewSlowStartManager(interval time.Duration) *SlowStartManager {
	if interval <= 0 {
		interval = time.Second // 默认 1 秒更新一次
	}

	return &SlowStartManager{
		targets:  make(map[string]*SlowStartState),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// SetFindTarget 设置目标查找回调。
func (m *SlowStartManager) SetFindTarget(fn func(url string) *Target) {
	m.findTarget = fn
}

// OnTargetHealthy 目标恢复健康时调用。
//
// 初始化慢启动状态，设置 EffectiveWeight = 1。
func (m *SlowStartManager) OnTargetHealthy(target *Target) {
	if target.SlowStart <= 0 {
		return // 未配置慢启动
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.targets[target.URL] = &SlowStartState{
		BaseWeight:  target.Weight,
		RecoverTime: time.Now(),
		SlowStart:   target.SlowStart,
	}

	// 设置初始权重为 1
	target.EffectiveWeight.Store(1)
}

// OnTargetUnhealthy 目标变为不健康时调用。
//
// 清除慢启动状态，重置 EffectiveWeight = 0。
func (m *SlowStartManager) OnTargetUnhealthy(target *Target) {
	m.mu.Lock()
	delete(m.targets, target.URL)
	m.mu.Unlock()

	// 重置有效权重
	target.EffectiveWeight.Store(0)
}

// Start 启动后台权重更新。
//
// 定期遍历所有慢启动中的目标，计算并更新 EffectiveWeight。
func (m *SlowStartManager) Start() {
	if m.running.Swap(true) {
		return // 已经在运行
	}

	go m.updateLoop()
}

// Stop 停止后台更新。
func (m *SlowStartManager) Stop() {
	if !m.running.Load() {
		return
	}
	close(m.stopCh)
}

// updateLoop 后台更新循环。
func (m *SlowStartManager) updateLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.updateEffectiveWeights()
		case <-m.stopCh:
			return
		}
	}
}

// updateEffectiveWeights 更新所有慢启动目标的有效权重。
func (m *SlowStartManager) updateEffectiveWeights() {
	now := time.Now()
	var toDelete []string

	m.mu.Lock()
	defer m.mu.Unlock()

	for url, state := range m.targets {
		elapsed := now.Sub(state.RecoverTime)
		if elapsed >= state.SlowStart {
			// 慢启动完成，标记删除
			toDelete = append(toDelete, url)
			continue
		}

		// 线性增长：从 1 增加到 BaseWeight
		progress := float64(elapsed) / float64(state.SlowStart)
		effectiveWeight := int(1 + progress*float64(state.BaseWeight-1))
		if effectiveWeight < 1 {
			effectiveWeight = 1
		}
		if effectiveWeight > state.BaseWeight {
			effectiveWeight = state.BaseWeight
		}

		// 查找目标并更新 EffectiveWeight
		if m.findTarget != nil {
			if target := m.findTarget(url); target != nil {
				target.EffectiveWeight.Store(int64(effectiveWeight))
			}
		}
	}

	// 删除已完成的慢启动状态
	for _, url := range toDelete {
		delete(m.targets, url)
		// 重置 EffectiveWeight 为 0（表示使用配置权重）
		if m.findTarget != nil {
			if target := m.findTarget(url); target != nil {
				target.EffectiveWeight.Store(0)
			}
		}
	}
}

// GetState 获取目标的慢启动状态（用于调试/监控）。
func (m *SlowStartManager) GetState(url string) *SlowStartState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.targets[url]
}

// GetAllStates 获取所有慢启动状态（用于调试/监控）。
func (m *SlowStartManager) GetAllStates() map[string]*SlowStartState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*SlowStartState, len(m.targets))
	for k, v := range m.targets {
		result[k] = v
	}
	return result
}
