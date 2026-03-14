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
	"sync"
	"time"
)

// CleanupManager cleanup manager
type CleanupManager struct {
	pool     *Pool
	config   *Config
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// NewCleanupManager creates cleanup manager
func NewCleanupManager(pool *Pool, config *Config) *CleanupManager {
	return &CleanupManager{
		pool:     pool,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start starts cleanup manager
func (c *CleanupManager) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return
	}

	c.running = true
	// Clean up every 30 seconds
	c.ticker = time.NewTicker(30 * time.Second)

	c.wg.Add(1)
	go c.cleanupLoop()
}

// Stop stops cleanup manager
func (c *CleanupManager) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false
	if c.ticker != nil {
		c.ticker.Stop()
	}
	close(c.stopChan)
	c.wg.Wait()
}

// cleanupLoop cleanup loop
func (c *CleanupManager) cleanupLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopChan:
			return
		case <-c.ticker.C:
			c.performCleanup()
		}
	}
}

// performCleanup performs cleanup
func (c *CleanupManager) performCleanup() {
	connections := c.pool.getAllConnections()

	for _, conn := range connections {
		// Use thread-safe method to check if connection is in use
		if conn.IsInUse() {
			continue
		}

		shouldClose := false

		// Check if expired
		if conn.IsExpired(c.config.MaxLifetime) {
			shouldClose = true
		}

		// Check if idle too long
		if !shouldClose && conn.IsIdleTooLong(c.config.IdleTimeout) {
			shouldClose = true
		}

		// Check if unhealthy (use thread-safe method)
		if !shouldClose && c.config.EnableHealthCheck && !conn.GetHealthStatus() {
			shouldClose = true
		}

		if shouldClose {
			// Try to remove from idle pool
			removed := c.removeFromIdlePool(conn)
			// Check again if connection is in use (use thread-safe method)
			if removed || !conn.IsInUse() {
				c.pool.closeConnection(conn)
			}
		}
	}

	// Ensure idle connection count doesn't exceed MaxIdleConnections
	c.enforceMaxIdleConnections()
}

// removeFromIdlePool removes connection from idle pool
// Note: Due to channel characteristics, we cannot directly remove specific connections from channel
// This method marks connection as invalid, which will be validated and closed when taken from channel
// Returns false indicating connection may be in channel, needs validation when taken
func (c *CleanupManager) removeFromIdlePool(conn *Connection) bool {
	// Mark connection as unhealthy, so it will be validated and closed during Get
	conn.UpdateHealth(false)
	// Since we cannot directly remove from channel, return false
	// Connection will be closed when taken from channel due to validation failure
	return false
}

// enforceMaxIdleConnections enforces max idle connection limit
func (c *CleanupManager) enforceMaxIdleConnections() {
	if c.pool.statsCollector == nil {
		return
	}

	// Get connection snapshot
	connections := c.pool.getAllConnections()

	// Count actual idle connections (need to recheck as state may have changed)
	idleConns := make([]*Connection, 0)
	for _, conn := range connections {
		// Recheck connection state (use thread-safe method)
		if !conn.IsInUse() {
			idleConns = append(idleConns, conn)
		}
	}

	// Use actual idle connection count instead of statistics
	idleCount := len(idleConns)
	if idleCount <= c.config.MaxIdleConnections {
		return
	}

	// Calculate number of connections to close
	excess := idleCount - c.config.MaxIdleConnections

	// Limit number of connections to close to avoid closing too many at once
	if excess > len(idleConns) {
		excess = len(idleConns)
	}

	// Find connections with longest idle time, close them first
	toClose := make([]*Connection, 0, excess)
	for len(toClose) < excess && len(idleConns) > 0 {
		maxIdx := 0
		maxIdleTime := idleConns[0].GetIdleTime()
		for i := 1; i < len(idleConns); i++ {
			idleTime := idleConns[i].GetIdleTime()
			if idleTime > maxIdleTime {
				maxIdleTime = idleTime
				maxIdx = i
			}
		}
		toClose = append(toClose, idleConns[maxIdx])
		// Remove selected connection
		idleConns = append(idleConns[:maxIdx], idleConns[maxIdx+1:]...)
	}

	// Mark as unhealthy and close
	closedCount := 0
	for _, conn := range toClose {
		// Recheck if connection is in use (prevent being obtained during sorting)
		if !conn.IsInUse() {
			conn.UpdateHealth(false)
			// Try to close connection (may be in channel, will be closed when taken due to validation failure)
			if err := c.pool.closeConnection(conn); err == nil {
				closedCount++
			}
		}
	}

	// Update idle connection statistics - use actual closed connection count
	if closedCount > 0 && c.pool.statsCollector != nil {
		c.pool.statsCollector.IncrementCurrentIdleConnections(int64(-closedCount))
	}
}
