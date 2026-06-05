package server

import (
	"time"
)

func waitForServerRunning(s *Server, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.running.Load() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
