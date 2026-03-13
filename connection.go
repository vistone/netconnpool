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
	// connectionIDGenerator 连接ID生成器
	connectionIDGenerator uint64
)

// Connection 连接封装
type Connection struct {
	// ID 连接唯一标识符
	ID uint64

	// Conn 底层连接对象
	Conn net.Conn

	// Protocol 协议类型（TCP或UDP）
	Protocol Protocol

	// IPVersion IP版本（IPv4或IPv6）
	IPVersion IPVersion

	// CreatedAt 创建时间
	CreatedAt time.Time

	// LastUsedAt 最后使用时间
	LastUsedAt time.Time

	// LastHealthCheckAt 最后健康检查时间
	LastHealthCheckAt time.Time

	// IsHealthy 是否健康
	IsHealthy bool

	// InUse 是否正在使用中
	InUse bool

	// LeakDetected 是否检测到泄漏
	LeakDetected bool

	// ReuseCount 连接复用次数（从连接池中获取的次数）
	ReuseCount int64

	// mu 保护连接的互斥锁
	mu sync.RWMutex

	// pool 所属的连接池
	pool *Pool

	// onClose 关闭回调
	onClose func() error
}

// NewConnection 创建新连接
func NewConnection(conn net.Conn, pool *Pool, onClose func() error) *Connection {
	now := time.Now()
	// 自动检测协议类型和IP版本
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

// GetProtocol 获取连接的协议类型（无锁，因为Protocol在创建后不会改变）
func (c *Connection) GetProtocol() Protocol {
	return c.Protocol
}

// GetIPVersion 获取连接的IP版本（无锁，因为IPVersion在创建后不会改变）
func (c *Connection) GetIPVersion() IPVersion {
	return c.IPVersion
}

// GetConn 获取底层连接对象
func (c *Connection) GetConn() net.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Conn
}

// MarkInUse 标记为使用中
func (c *Connection) MarkInUse() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.InUse = true
	c.LastUsedAt = time.Now()
}

// MarkIdle 标记为空闲
func (c *Connection) MarkIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.InUse = false
	c.LastUsedAt = time.Now()
}

// UpdateHealth 更新健康状态
func (c *Connection) UpdateHealth(healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IsHealthy = healthy
	c.LastHealthCheckAt = time.Now()
}

// IsExpired 检查连接是否过期（超过MaxLifetime）
func (c *Connection) IsExpired(maxLifetime time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if maxLifetime <= 0 {
		return false
	}
	return time.Since(c.CreatedAt) > maxLifetime
}

// IsIdleTooLong 检查连接是否空闲太久（超过IdleTimeout）
func (c *Connection) IsIdleTooLong(idleTimeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if idleTimeout <= 0 {
		return false
	}
	return !c.InUse && time.Since(c.LastUsedAt) > idleTimeout
}

// IsLeaked 检查连接是否泄漏（超过ConnectionLeakTimeout且仍在使用时）
func (c *Connection) IsLeaked(leakTimeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if leakTimeout <= 0 || !c.InUse {
		return false
	}
	return time.Since(c.LastUsedAt) > leakTimeout
}

// Close 关闭连接
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
		// 确保即使 onClose 为 nil，底层连接也被关闭
		err = c.Conn.Close()
	}
	c.Conn = nil
	return err
}

// GetAge 获取连接年龄
func (c *Connection) GetAge() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.CreatedAt)
}

// GetIdleTime 获取空闲时间
func (c *Connection) GetIdleTime() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.InUse {
		return 0
	}
	return time.Since(c.LastUsedAt)
}

// IncrementReuseCount 增加复用次数
func (c *Connection) IncrementReuseCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ReuseCount++
}

// GetReuseCount 获取复用次数
func (c *Connection) GetReuseCount() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ReuseCount
}

// IsInUse 检查连接是否正在使用中（线程安全）
func (c *Connection) IsInUse() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.InUse
}

// GetHealthStatus 获取连接健康状态（线程安全）
func (c *Connection) GetHealthStatus() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.IsHealthy
}
