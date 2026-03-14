// Copyright (c) 2025, vistone
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package netconnpool

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// HealthCheckManager health check manager
type HealthCheckManager struct {
	pool     *Pool
	config   *Config
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// NewHealthCheckManager creates health check manager
func NewHealthCheckManager(pool *Pool, config *Config) *HealthCheckManager {
	return &HealthCheckManager{
		pool:     pool,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start starts health check
func (h *HealthCheckManager) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running || !h.config.EnableHealthCheck {
		return
	}

	h.running = true
	h.ticker = time.NewTicker(h.config.HealthCheckInterval)

	h.wg.Add(1)
	go h.healthCheckLoop()
}

// Stop stops health check
func (h *HealthCheckManager) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return
	}

	h.running = false
	if h.ticker != nil {
		h.ticker.Stop()
	}
	close(h.stopChan)
	h.wg.Wait()
}

// healthCheckLoop health check loop
func (h *HealthCheckManager) healthCheckLoop() {
	defer h.wg.Done()

	for {
		select {
		case <-h.stopChan:
			return
		case <-h.ticker.C:
			h.performHealthCheck()
		}
	}
}

// performHealthCheck performs health check
func (h *HealthCheckManager) performHealthCheck() {
	connections := h.pool.getAllConnections()

	// Perform health checks concurrently, but limit concurrency
	const maxConcurrentChecks = 10
	semaphore := make(chan struct{}, maxConcurrentChecks)
	var wg sync.WaitGroup

	for _, conn := range connections {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			h.checkConnection(c)
		}(conn)
	}

	wg.Wait()
}

// checkConnection checks a single connection
func (h *HealthCheckManager) checkConnection(conn *Connection) {
	// Skip connections that are in use
	conn.mu.RLock()
	inUse := conn.InUse
	conn.mu.RUnlock()

	if inUse {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.config.HealthCheckTimeout)
	defer cancel()

	done := make(chan bool, 1)
	// Channel to notify goroutine to exit
	exitChan := make(chan struct{})

	// Use buffered channel to ensure goroutine won't block forever
	go func() {
		defer close(exitChan) // Ensure goroutine notifies main thread when exiting
		defer func() {
			// Prevent panic
			if r := recover(); r != nil {
				select {
				case done <- false:
				case <-ctx.Done():
					// Context cancelled, goroutine can exit safely
				}
			}
		}()
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			// Context cancelled, goroutine can exit safely
			return
		default:
		}
		healthy := h.performCheck(conn.GetConn())
		select {
		case done <- healthy:
		case <-ctx.Done():
			// Context cancelled, goroutine can exit safely
		}
	}()

	select {
	case healthy := <-done:
		conn.UpdateHealth(healthy)
		if h.pool.statsCollector != nil {
			h.pool.statsCollector.IncrementHealthCheckAttempts()
			if !healthy {
				h.pool.statsCollector.IncrementHealthCheckFailures()
				h.pool.statsCollector.IncrementUnhealthyConnections()
			}
		}
		// Wait for goroutine to exit
		select {
		case <-exitChan:
		case <-time.After(100 * time.Millisecond):
			// Timeout, continue
		}
	case <-ctx.Done():
		// Health check timeout, mark as unhealthy
		conn.UpdateHealth(false)
		if h.pool.statsCollector != nil {
			h.pool.statsCollector.IncrementHealthCheckAttempts()
			h.pool.statsCollector.IncrementHealthCheckFailures()
		}
		// Wait for goroutine to exit or timeout
		select {
		case <-exitChan:
		case <-time.After(100 * time.Millisecond):
			// Give goroutine some time to exit
		}
	}
}

// performCheck performs actual health check
func (h *HealthCheckManager) performCheck(conn net.Conn) bool {
	// If custom health check function is configured, use it
	if h.config.HealthChecker != nil {
		return h.config.HealthChecker(conn)
	}

	// Check if UDP connection
	if udpConn, ok := conn.(*net.UDPConn); ok {
		// Special health check for UDP connections
		// UDP is connectionless, cannot determine health status by reading data like TCP
		// We determine by checking if connection is closed

		// Set very short read timeout for probe
		udpConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
		defer udpConn.SetReadDeadline(time.Time{})

		// Try to read one byte, if can read, connection is normal (even timeout means connection is alive)
		buf := make([]byte, 1)
		_, err := udpConn.Read(buf)
		if err != nil {
			// Timeout error is normal, means connection is alive (just no data)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return true
			}
			// EOF error means connection is closed
			if err == io.EOF {
				return false
			}
			// For UDP, other errors (like connection closed, network error) should be considered unhealthy
			// Only timeout error means connection is normal (just no data)
			return false
		}
		// If can read data, connection is normal
		return true
	}

	// Default health check: probe connection status with timeout read
	// Timeout error means connection is alive (no data readable but connection is normal)
	// EOF means peer closed connection
	// Note: If connection has buffered data, Read will consume one byte
	return h.readProbeCheck(conn)
}

// readProbeCheck probes connection health status with timeout read
func (h *HealthCheckManager) readProbeCheck(conn net.Conn) bool {
	conn.SetReadDeadline(time.Now().Add(h.config.HealthCheckTimeout))
	one := make([]byte, 1)
	_, err := conn.Read(one)
	conn.SetReadDeadline(time.Time{})
	if err == io.EOF {
		return false
	}
	// Timeout means connection is alive but no data, which is normal
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	// Other errors (non-timeout, non-EOF), connection may be disconnected
	if err != nil {
		return false
	}
	return true
}
