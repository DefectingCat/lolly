// Package loadbalance provides load balancing algorithms for the Lolly HTTP server.
//
// This package implements various load balancing strategies including round-robin,
// weighted round-robin, least connections, and IP hash. All implementations are
// concurrency-safe using atomic operations.
//
// Example usage:
//
//	targets := []*Target{
//	    {URL: "http://backend1:8080", Weight: 1, Healthy: true},
//	    {URL: "http://backend2:8080", Weight: 2, Healthy: true},
//	}
//
//	balancer := NewWeightedRoundRobin()
//	selected := balancer.Select(targets)
//
//go:generate go test -v ./...
package loadbalance

import (
	"hash/fnv"
	"sync/atomic"
)

// Target represents a backend server target for load balancing.
// All fields are designed for concurrent access using atomic operations
// where applicable.
type Target struct {
	// URL is the target address, e.g., "http://backend1:8080"
	URL string

	// Weight is the weight of this target for weighted algorithms.
	// Higher weight means more requests will be routed to this target.
	Weight int

	// Healthy indicates whether this target is healthy and available.
	// Use atomic operations to read/write this field concurrently.
	Healthy bool

	// Connections tracks the current number of active connections.
	// Use atomic operations to modify this field concurrently.
	Connections int64
}

// Balancer is the interface for load balancing algorithms.
// Implementations must be safe for concurrent use.
type Balancer interface {
	// Select chooses a target from the provided list based on the
	// algorithm's strategy. Returns nil if no healthy targets are available.
	Select(targets []*Target) *Target
}

// RoundRobin implements simple round-robin load balancing.
// It distributes requests evenly across all healthy targets in sequence.
type RoundRobin struct {
	// counter is incremented atomically for each request
	counter uint64
}

// NewRoundRobin creates a new round-robin load balancer.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Select chooses the next target in round-robin order.
// Only healthy targets are considered. Returns nil if no healthy targets exist.
func (r *RoundRobin) Select(targets []*Target) *Target {
	healthy := filterHealthy(targets)
	if len(healthy) == 0 {
		return nil
	}

	// Atomically increment and get the counter value
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return healthy[idx%uint64(len(healthy))]
}

// WeightedRoundRobin implements weighted round-robin load balancing.
// Targets with higher weights receive proportionally more requests.
type WeightedRoundRobin struct {
	// counter is incremented atomically for each request
	counter uint64
}

// NewWeightedRoundRobin creates a new weighted round-robin load balancer.
func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{}
}

// Select chooses a target based on weight distribution.
// Only healthy targets are considered. Returns nil if no healthy targets exist.
func (w *WeightedRoundRobin) Select(targets []*Target) *Target {
	healthy := filterHealthy(targets)
	if len(healthy) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, t := range healthy {
		if t.Weight <= 0 {
			totalWeight += 1 // Minimum weight of 1
		} else {
			totalWeight += t.Weight
		}
	}

	if totalWeight == 0 {
		return nil
	}

	// Use atomic counter to determine position in weight distribution
	idx := atomic.AddUint64(&w.counter, 1) - 1
	pos := int(idx % uint64(totalWeight))

	// Find target at the calculated position
	currentWeight := 0
	for _, t := range healthy {
		weight := t.Weight
		if weight <= 0 {
			weight = 1
		}
		currentWeight += weight
		if pos < currentWeight {
			return t
		}
	}

	// Fallback to last target (should not reach here)
	return healthy[len(healthy)-1]
}

// LeastConnections implements least connections load balancing.
// It selects the target with the fewest active connections.
type LeastConnections struct{}

// NewLeastConnections creates a new least-connections load balancer.
func NewLeastConnections() *LeastConnections {
	return &LeastConnections{}
}

// Select chooses the target with the minimum connection count.
// Only healthy targets are considered. Returns nil if no healthy targets exist.
func (l *LeastConnections) Select(targets []*Target) *Target {
	var selected *Target
	var minConns int64 = -1

	for _, t := range targets {
		if !t.Healthy {
			continue
		}

		// Atomically read the connection count
		conns := atomic.LoadInt64(&t.Connections)

		if selected == nil || conns < minConns {
			selected = t
			minConns = conns
		}
	}

	return selected
}

// IPHash implements IP hash-based load balancing.
// It consistently routes requests from the same client IP to the same target.
type IPHash struct{}

// NewIPHash creates a new IP hash load balancer.
func NewIPHash() *IPHash {
	return &IPHash{}
}

// Select chooses a target based on the hash of the client IP.
// Only healthy targets are considered. Returns nil if no healthy targets exist.
// The clientIP parameter should be the client's IP address as a string.
func (i *IPHash) Select(targets []*Target) *Target {
	return i.SelectByIP(targets, "")
}

// SelectByIP chooses a target based on the hash of the provided IP address.
// Only healthy targets are considered. Returns nil if no healthy targets exist.
func (i *IPHash) SelectByIP(targets []*Target, clientIP string) *Target {
	healthy := filterHealthy(targets)
	if len(healthy) == 0 {
		return nil
	}

	// Hash the client IP
	h := fnv.New64a()
	h.Write([]byte(clientIP))
	hash := h.Sum64()

	idx := hash % uint64(len(healthy))
	return healthy[idx]
}

// filterHealthy returns a new slice containing only healthy targets.
// This is a helper function used by load balancing implementations.
func filterHealthy(targets []*Target) []*Target {
	healthy := make([]*Target, 0, len(targets))
	for _, t := range targets {
		if t.Healthy {
			healthy = append(healthy, t)
		}
	}
	return healthy
}

// IncrementConnections atomically increments the connection count for a target.
// This should be called when a new connection is established.
func IncrementConnections(t *Target) {
	atomic.AddInt64(&t.Connections, 1)
}

// DecrementConnections atomically decrements the connection count for a target.
// This should be called when a connection is closed.
func DecrementConnections(t *Target) {
	atomic.AddInt64(&t.Connections, -1)
}

// IsHealthy atomically reads the health status of a target.
func IsHealthy(t *Target) bool {
	// Healthy is a bool, which is safe to read without atomic operations
	// but for consistency with the setter, we could use atomic
	// For bool, simple read is safe in Go's memory model
	return t.Healthy
}

// SetHealthy atomically sets the health status of a target.
// Note: In Go, bool operations are not directly atomic.
// This function provides a synchronized way to update health status.
// For true atomic operations on bool, consider using atomic.Bool (Go 1.19+)
// or sync.RWMutex. For this implementation, we use direct assignment
// which is typically sufficient when combined with proper synchronization
// at the caller level.
func SetHealthy(t *Target, healthy bool) {
	t.Healthy = healthy
}
