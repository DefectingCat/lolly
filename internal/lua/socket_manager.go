// Package lua 提供 Cosocket 管理功能。
//
// 该文件实现 TCP Cosocket 管理器，包括：
//   - SocketState：Socket 生命周期状态机
//   - SocketOperation：单个 Socket 操作的封装，支持异步等待
//   - CosocketManager：操作生命周期管理、超时检测、统计追踪
//
// 特性：
//   - 原子操作标记完成状态，避免竞态条件
//   - 后台清理循环定期检测并取消超时操作
//   - 统计信息使用 atomic 操作保证并发安全
//
// 注意事项：
//   - 操作一旦标记完成（CompareAndSwap），不可重复完成
//   - 管理器关闭时会取消所有未完成的操作
//
// 作者：xfy
package lua

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// SocketState 表示 Socket 操作状态。
//
// 状态机流转：Idle -> Connecting -> Connected -> Sending/Receiving -> Closing -> Closed
type SocketState int

// Socket 生命周期状态常量
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

// String 返回状态的字符串表示
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

// 操作类型常量
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

// SocketOperation 表示一个 Socket 操作。
//
// 封装单个异步操作的生命周期，包括创建、执行、完成和等待。
// 使用 atomic 操作标记完成状态，通过 Done channel 通知等待方。
type SocketOperation struct {
	// ID 操作唯一标识
	ID uint64

	// Type 操作类型
	Type OperationType

	// State 当前 Socket 状态
	State SocketState

	// Socket 关联的 TCP Socket
	Socket *TCPSocket

	// Timeout 操作超时时间
	Timeout time.Duration

	// CreatedAt 操作创建时间
	CreatedAt time.Time

	// LastActivity 最后活动时间（用于超时检测）
	LastActivity time.Time

	// Result 操作结果
	Result interface{}

	// Error 操作错误
	Error error

	// Done 完成信号 channel，操作完成时关闭
	Done chan struct{}

	// completed 原子标记，1=已完成，0=未完成
	completed int32
}

// IsCompleted 检查操作是否已完成。
//
// 返回值：
//   - bool: true 表示已完成
func (op *SocketOperation) IsCompleted() bool {
	return atomic.LoadInt32(&op.completed) == 1
}

// Complete 标记操作完成。
//
// 使用 CompareAndSwap 确保只完成一次，完成后关闭 Done channel 通知等待方。
//
// 参数：
//   - result: 操作结果
//   - err: 操作错误（nil 表示成功）
func (op *SocketOperation) Complete(result interface{}, err error) {
	if atomic.CompareAndSwapInt32(&op.completed, 0, 1) {
		op.Result = result
		op.Error = err
		close(op.Done)
	}
}

// Wait 等待操作完成。
//
// 阻塞直到操作完成或上下文取消。
//
// 参数：
//   - ctx: 取消上下文
//
// 返回值：
//   - interface{}: 操作结果
//   - error: 操作错误或上下文取消错误
func (op *SocketOperation) Wait(ctx context.Context) (interface{}, error) {
	select {
	case <-op.Done:
		return op.Result, op.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Touch 更新活动时间（用于超时检测）
func (op *SocketOperation) Touch() {
	op.LastActivity = time.Now()
}

// CosocketStats Cosocket 统计信息。
//
// 包含操作和 Socket 的创建、活跃、超时、错误等统计。
type CosocketStats struct {
	// TotalOperations 总操作数
	TotalOperations uint64

	// ActiveOperations 当前活跃操作数
	ActiveOperations uint64

	// TimeoutOperations 超时操作数
	TimeoutOperations uint64

	// ErrorOperations 错误操作数
	ErrorOperations uint64

	// ActiveSockets 当前活跃 Socket 数
	ActiveSockets uint64

	// TotalSocketsCreated 累计创建的 Socket 总数
	TotalSocketsCreated uint64

	// TotalSocketsClosed 累计关闭的 Socket 总数
	TotalSocketsClosed uint64
}

// CosocketManager Cosocket 管理器。
//
// 负责管理 Socket 操作的生命周期，包括：
//   - 创建和跟踪异步操作
//   - 检测并清理超时操作
//   - 统计操作和 Socket 的使用情况
type CosocketManager struct {
	// ctx 上下文（用于控制清理循环）
	ctx context.Context

	// cancel 取消函数
	cancel context.CancelFunc

	// operations 进行中的操作映射（ID -> 操作）
	operations map[uint64]*SocketOperation

	// mu 读写锁
	mu sync.RWMutex

	// nextID 下一个操作 ID
	nextID uint64

	// defaultTimeout 默认超时时间
	defaultTimeout time.Duration

	// timeoutChecker 超时检查定时器
	timeoutChecker *time.Ticker

	// cleanupInterval 清理间隔
	cleanupInterval time.Duration

	// stats 统计信息
	stats CosocketStats
}

// DefaultCosocketManager 全局默认 Cosocket 管理器
var DefaultCosocketManager = NewCosocketManager()

// NewCosocketManager 创建新的 Cosocket 管理器。
//
// 启动后台清理循环，每 30 秒检查一次超时操作。
//
// 返回值：
//   - *CosocketManager: 初始化的管理器实例
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

// StartOperation 开始一个新的 Socket 操作。
//
// 创建操作实例，分配唯一 ID，注册到管理器中。
//
// 参数：
//   - socket: 关联的 TCP Socket
//   - opType: 操作类型
//   - timeout: 超时时间，零值时使用默认超时
//
// 返回值：
//   - *SocketOperation: 新创建的操作实例
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

// CompleteOperation 完成指定 ID 的操作。
//
// 从管理器中移除操作，标记完成，更新统计。
//
// 参数：
//   - id: 操作 ID
//   - result: 操作结果
//   - err: 操作错误
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

// GetOperation 获取指定 ID 的操作。
//
// 参数：
//   - id: 操作 ID
//
// 返回值：
//   - *SocketOperation: 操作实例，不存在时返回 nil
func (cm *CosocketManager) GetOperation(id uint64) *SocketOperation {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.operations[id]
}

// cleanupLoop 清理循环，定期检测超时操作。
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

// cleanup 清理超时操作。
//
// 扫描所有未完成的操作，标记超过 LastActivity + Timeout 的操作为超时。
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

// Stats 获取 Cosocket 统计信息。
//
// 返回值：
//   - CosocketStats: 当前统计快照
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

// SetDefaultTimeout 设置默认超时时间。
//
// 参数：
//   - timeout: 新的默认超时
func (cm *CosocketManager) SetDefaultTimeout(timeout time.Duration) {
	cm.defaultTimeout = timeout
}

// Close 关闭 Cosocket 管理器。
//
// 停止清理循环，取消所有未完成的操作。
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

// TrackSocketCreated 跟踪 Socket 创建（更新统计）。
func (cm *CosocketManager) TrackSocketCreated() {
	atomic.AddUint64(&cm.stats.TotalSocketsCreated, 1)
	atomic.AddUint64(&cm.stats.ActiveSockets, 1)
}

// TrackSocketClosed 跟踪 Socket 关闭（更新统计）。
func (cm *CosocketManager) TrackSocketClosed() {
	atomic.AddUint64(&cm.stats.TotalSocketsClosed, 1)
	atomic.AddUint64(&cm.stats.ActiveSockets, ^uint64(0))
}

// TCPAddr 解析 TCP 地址。
//
// 参数：
//   - host: 主机地址
//   - port: 端口号
//
// 返回值：
//   - *net.TCPAddr: 解析后的 TCP 地址
//   - error: 解析失败时返回错误
func (cm *CosocketManager) TCPAddr(host string, port int) (*net.TCPAddr, error) {
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}, nil
}
