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
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// connectionIDGenerator connection ID generator
	connectionIDGenerator uint64
)

// Connection connection wrapper
type Connection struct {
	// ID connection unique identifier
	ID uint64

	// Conn underlying connection object
	Conn net.Conn

	// Protocol protocol type (TCP or UDP)
	Protocol Protocol

	// IPVersion IP version (IPv4 or IPv6)
	IPVersion IPVersion

	// CreatedAt creation time
	CreatedAt time.Time

	// LastUsedAt last used time
	LastUsedAt time.Time

	// LastHealthCheckAt last health check time
	LastHealthCheckAt time.Time

	// IsHealthy whether healthy
	IsHealthy bool

	// InUse whether in use
	InUse bool

	// LeakDetected whether leak detected
	LeakDetected bool

	// ReuseCount connection reuse count (times obtained from connection pool)
	ReuseCount int64

	// mu mutex protecting connection
	mu sync.RWMutex

	// pool belonging connection pool
	pool *Pool

	// onClose close callback
	onClose func() error
}

// NewConnection creates a new connection
func NewConnection(conn net.Conn, pool *Pool, onClose func() error) *Connection {
	now := time.Now()
	// Auto-detect protocol type and IP version
	protocol := DetectProtocol(conn)
	ipVersion := DetectIPVersion(conn)

	return &Connection{
		ID:                atomic.AddUint64(&connectionIDGenerator, 1),
		Conn:              conn,
		Protocol:          protocol,
		IPVersion:         ipVersion,
		CreatedAt:         now,
		LastUsedAt:        now,
		LastHealthCheckAt: now,
		IsHealthy:         true,
		InUse:             false,
		LeakDetected:      false,
		ReuseCount:        0,
		pool:              pool,
		onClose:           onClose,
	}
}

// GetProtocol gets connection protocol type (lock-free, Protocol doesn't change after creation)
func (c *Connection) GetProtocol() Protocol {
	return c.Protocol
}

// GetIPVersion gets connection IP version (lock-free, IPVersion doesn't change after creation)
func (c *Connection) GetIPVersion() IPVersion {
	return c.IPVersion
}

// GetConn gets underlying connection object
func (c *Connection) GetConn() net.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Conn
}

// MarkInUse marks as in use
func (c *Connection) MarkInUse() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.InUse = true
	c.LastUsedAt = time.Now()
}

// MarkIdle marks as idle
func (c *Connection) MarkIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.InUse = false
	c.LastUsedAt = time.Now()
}

// UpdateHealth updates health status
func (c *Connection) UpdateHealth(healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IsHealthy = healthy
	c.LastHealthCheckAt = time.Now()
}

// IsExpired checks if connection is expired (exceeds MaxLifetime)
func (c *Connection) IsExpired(maxLifetime time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if maxLifetime <= 0 {
		return false
	}
	return time.Since(c.CreatedAt) > maxLifetime
}

// IsIdleTooLong checks if connection has been idle too long (exceeds IdleTimeout)
func (c *Connection) IsIdleTooLong(idleTimeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if idleTimeout <= 0 {
		return false
	}
	return !c.InUse && time.Since(c.LastUsedAt) > idleTimeout
}

// IsLeaked checks if connection is leaked (exceeds ConnectionLeakTimeout and still in use)
func (c *Connection) IsLeaked(leakTimeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if leakTimeout <= 0 || !c.InUse {
		return false
	}
	return time.Since(c.LastUsedAt) > leakTimeout
}

// Close closes connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Conn == nil {
		return nil
	}

	var err error
	if c.onClose != nil {
		err = c.onClose()
	} else {
		// Ensure underlying connection is closed even if onClose is nil
		err = c.Conn.Close()
	}
	c.Conn = nil
	return err
}

// GetAge gets connection age
func (c *Connection) GetAge() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.CreatedAt)
}

// GetIdleTime gets idle time
func (c *Connection) GetIdleTime() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.InUse {
		return 0
	}
	return time.Since(c.LastUsedAt)
}

// IncrementReuseCount increments reuse count
func (c *Connection) IncrementReuseCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ReuseCount++
}

// GetReuseCount gets reuse count
func (c *Connection) GetReuseCount() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ReuseCount
}

// IsInUse checks if connection is in use (thread-safe)
func (c *Connection) IsInUse() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.InUse
}

// GetHealthStatus gets connection health status (thread-safe)
func (c *Connection) GetHealthStatus() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.IsHealthy
}
