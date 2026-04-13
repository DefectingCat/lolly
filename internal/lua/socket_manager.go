// Package lua 提供 Cosocket 管理功能
package lua

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// SocketState 表示 socket 操作状态
type SocketState int

const (
	// SocketStateIdle 空闲状态
	SocketStateIdle SocketState = iota
	// SocketStateConnecting 连接中
	SocketStateConnecting
	// SocketStateConnected 已连接
	SocketStateConnected
	// SocketStateSending 发送中
	SocketStateSending
	// SocketStateReceiving 接收中
	SocketStateReceiving
	// SocketStateClosing 关闭中
	SocketStateClosing
	// SocketStateClosed 已关闭
	SocketStateClosed
	// SocketStateError 错误状态
	SocketStateError
)

func (s SocketState) String() string {
	switch s {
	case SocketStateIdle:
		return "idle"
	case SocketStateConnecting:
		return "connecting"
	case SocketStateConnected:
		return "connected"
	case SocketStateSending:
		return "sending"
	case SocketStateReceiving:
		return "receiving"
	case SocketStateClosing:
		return "closing"
	case SocketStateClosed:
		return "closed"
	case SocketStateError:
		return "error"
	default:
		return "unknown"
	}
}

// OperationType 操作类型
type OperationType string

const (
	// OpConnect 连接操作
	OpConnect OperationType = "connect"
	// OpSend 发送操作
	OpSend OperationType = "send"
	// OpReceive 接收操作
	OpReceive OperationType = "receive"
	// OpClose 关闭操作
	OpClose OperationType = "close"
)

// SocketOperation 表示一个 socket 操作
type SocketOperation struct {
	CreatedAt    time.Time
	LastActivity time.Time
	Error        error
	Result       interface{}
	Socket       *TCPSocket
	Done         chan struct{}
	Type         OperationType
	ID           uint64
	State        SocketState
	Timeout      time.Duration
	completed    int32
}

// IsCompleted 检查操作是否已完成
func (op *SocketOperation) IsCompleted() bool {
	return atomic.LoadInt32(&op.completed) == 1
}

// Complete 标记操作完成
func (op *SocketOperation) Complete(result interface{}, err error) {
	if atomic.CompareAndSwapInt32(&op.completed, 0, 1) {
		op.Result = result
		op.Error = err
		close(op.Done)
	}
}

// Wait 等待操作完成
func (op *SocketOperation) Wait(ctx context.Context) (interface{}, error) {
	select {
	case <-op.Done:
		return op.Result, op.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Touch 更新活动时间
func (op *SocketOperation) Touch() {
	op.LastActivity = time.Now()
}

// CosocketStats Cosocket 统计信息
type CosocketStats struct {
	// 总操作数
	TotalOperations uint64

	// 活跃操作数
	ActiveOperations uint64

	// 超时操作数
	TimeoutOperations uint64

	// 错误操作数
	ErrorOperations uint64

	// 当前 socket 数
	ActiveSockets uint64

	// 总创建 socket 数
	TotalSocketsCreated uint64

	// 总关闭 socket 数
	TotalSocketsClosed uint64
}

// CosocketManager Cosocket 管理器
type CosocketManager struct {
	ctx             context.Context
	operations      map[uint64]*SocketOperation
	timeoutChecker  *time.Ticker
	cancel          context.CancelFunc
	stats           CosocketStats
	nextID          uint64
	defaultTimeout  time.Duration
	cleanupInterval time.Duration
	mu              sync.RWMutex
}

// DefaultCosocketManager 全局默认管理器
var DefaultCosocketManager = NewCosocketManager()

// NewCosocketManager 创建新的 Cosocket 管理器
func NewCosocketManager() *CosocketManager {
	ctx, cancel := context.WithCancel(context.Background())
	cm := &CosocketManager{
		operations:      make(map[uint64]*SocketOperation),
		nextID:          0,
		timeoutChecker:  time.NewTicker(30 * time.Second),
		ctx:             ctx,
		cancel:          cancel,
		defaultTimeout:  60 * time.Second,
		cleanupInterval: 30 * time.Second,
	}

	// 启动清理循环
	go cm.cleanupLoop()

	return cm
}

// StartOperation 开始一个新的 socket 操作
func (cm *CosocketManager) StartOperation(socket *TCPSocket, opType OperationType, timeout time.Duration) *SocketOperation {
	if timeout <= 0 {
		timeout = cm.defaultTimeout
	}

	id := atomic.AddUint64(&cm.nextID, 1)
	now := time.Now()

	op := &SocketOperation{
		ID:           id,
		Socket:       socket,
		Type:         opType,
		State:        SocketStateIdle,
		CreatedAt:    now,
		LastActivity: now,
		Timeout:      timeout,
		Done:         make(chan struct{}),
	}

	cm.mu.Lock()
	cm.operations[id] = op
	cm.mu.Unlock()

	atomic.AddUint64(&cm.stats.TotalOperations, 1)
	atomic.AddUint64(&cm.stats.ActiveOperations, 1)

	return op
}

// CompleteOperation 完成操作
func (cm *CosocketManager) CompleteOperation(id uint64, result interface{}, err error) {
	cm.mu.Lock()
	op, exists := cm.operations[id]
	if exists {
		delete(cm.operations, id)
	}
	cm.mu.Unlock()

	if exists && op != nil {
		op.Complete(result, err)
		atomic.AddUint64(&cm.stats.ActiveOperations, ^uint64(0))
		if err != nil {
			atomic.AddUint64(&cm.stats.ErrorOperations, 1)
		}
	}
}

// GetOperation 获取操作
func (cm *CosocketManager) GetOperation(id uint64) *SocketOperation {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.operations[id]
}

// cleanupLoop 清理循环
func (cm *CosocketManager) cleanupLoop() {
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-cm.timeoutChecker.C:
			cm.cleanup()
		}
	}
}

// cleanup 清理超时操作
func (cm *CosocketManager) cleanup() {
	now := time.Now()
	timeoutOps := make([]*SocketOperation, 0)

	cm.mu.RLock()
	for _, op := range cm.operations {
		if !op.IsCompleted() && now.Sub(op.LastActivity) > op.Timeout {
			timeoutOps = append(timeoutOps, op)
		}
	}
	cm.mu.RUnlock()

	for _, op := range timeoutOps {
		cm.CompleteOperation(op.ID, nil, context.DeadlineExceeded)
		atomic.AddUint64(&cm.stats.TimeoutOperations, 1)
	}
}

// Stats 获取统计信息
func (cm *CosocketManager) Stats() CosocketStats {
	return CosocketStats{
		TotalOperations:     atomic.LoadUint64(&cm.stats.TotalOperations),
		ActiveOperations:    atomic.LoadUint64(&cm.stats.ActiveOperations),
		TimeoutOperations:   atomic.LoadUint64(&cm.stats.TimeoutOperations),
		ErrorOperations:     atomic.LoadUint64(&cm.stats.ErrorOperations),
		ActiveSockets:       atomic.LoadUint64(&cm.stats.ActiveSockets),
		TotalSocketsCreated: atomic.LoadUint64(&cm.stats.TotalSocketsCreated),
		TotalSocketsClosed:  atomic.LoadUint64(&cm.stats.TotalSocketsClosed),
	}
}

// SetDefaultTimeout 设置默认超时
func (cm *CosocketManager) SetDefaultTimeout(timeout time.Duration) {
	cm.defaultTimeout = timeout
}

// Close 关闭管理器
func (cm *CosocketManager) Close() {
	cm.cancel()
	cm.timeoutChecker.Stop()

	// 取消所有未完成操作
	cm.mu.Lock()
	ops := make([]*SocketOperation, 0, len(cm.operations))
	for _, op := range cm.operations {
		ops = append(ops, op)
	}
	cm.operations = make(map[uint64]*SocketOperation)
	cm.mu.Unlock()

	for _, op := range ops {
		op.Complete(nil, context.Canceled)
	}
}

// TrackSocketCreated 跟踪 socket 创建
func (cm *CosocketManager) TrackSocketCreated() {
	atomic.AddUint64(&cm.stats.TotalSocketsCreated, 1)
	atomic.AddUint64(&cm.stats.ActiveSockets, 1)
}

// TrackSocketClosed 跟踪 socket 关闭
func (cm *CosocketManager) TrackSocketClosed() {
	atomic.AddUint64(&cm.stats.TotalSocketsClosed, 1)
	atomic.AddUint64(&cm.stats.ActiveSockets, ^uint64(0))
}

// TCPAddr 解析 TCP 地址
func (cm *CosocketManager) TCPAddr(host string, port int) (*net.TCPAddr, error) {
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}, nil
}
