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
	"net"
	"time"
)

// Dialer 连接创建函数类型（客户端模式）
// 返回网络连接和错误
type Dialer func(ctx context.Context) (net.Conn, error)

// Acceptor 连接接受函数类型（服务器端模式）
// 从Listener接受新连接，返回网络连接和错误
type Acceptor func(ctx context.Context, listener net.Listener) (net.Conn, error)

// HealthChecker 健康检查函数类型
// 返回连接是否健康
type HealthChecker func(conn net.Conn) bool

// Config 连接池配置
type Config struct {
	// Mode 连接池模式：客户端或服务器端
	// 默认值为PoolModeClient（客户端模式）
	Mode PoolMode
	// MaxConnections 最大连接数，0表示无限制
	MaxConnections int

	// MinConnections 最小连接数（预热连接数）
	MinConnections int

	// MaxIdleConnections 最大空闲连接数
	MaxIdleConnections int

	// ConnectionTimeout 连接创建超时时间
	ConnectionTimeout time.Duration

	// IdleTimeout 空闲连接超时时间，超过此时间的空闲连接将被关闭
	IdleTimeout time.Duration

	// MaxLifetime 连接最大生命周期，超过此时间的连接将被关闭
	MaxLifetime time.Duration

	// GetConnectionTimeout 获取连接的超时时间
	GetConnectionTimeout time.Duration

	// HealthCheckInterval 健康检查间隔
	HealthCheckInterval time.Duration

	// HealthCheckTimeout 健康检查超时时间
	HealthCheckTimeout time.Duration

	// ConnectionLeakTimeout 连接泄漏检测超时时间
	// 如果连接在此时间内未归还，将触发泄漏警告
	ConnectionLeakTimeout time.Duration

	// Dialer 连接创建函数（客户端模式必需）
	// 在客户端模式下，用于主动创建连接到服务器
	Dialer Dialer

	// Listener 网络监听器（服务器端模式必需）
	// 在服务器端模式下，用于接受客户端连接
	Listener net.Listener

	// Acceptor 连接接受函数（服务器端模式可选）
	// 在服务器端模式下，用于从Listener接受连接
	// 如果为nil，将使用默认的Accept方法
	Acceptor Acceptor

	// HealthChecker 健康检查函数（可选）
	// 如果为nil，将使用默认的ping检查
	HealthChecker HealthChecker

	// CloseConn 连接关闭函数（可选）
	// 如果为nil，将尝试类型断言为io.Closer并调用Close方法
	CloseConn func(conn net.Conn) error

	// OnCreated 连接创建后调用
	OnCreated func(conn net.Conn) error

	// OnBorrow 连接从池中取出前调用
	OnBorrow func(conn net.Conn)

	// OnReturn 连接归还池中前调用
	OnReturn func(conn net.Conn)

	// EnableStats 是否启用统计信息
	EnableStats bool

	// EnableHealthCheck 是否启用健康检查
	EnableHealthCheck bool

	// ClearUDPBufferOnReturn 是否在归还UDP连接时清空读取缓冲区
	// 启用此选项可以防止UDP连接复用时的数据混淆
	// 默认值为true，建议保持启用
	ClearUDPBufferOnReturn bool

	// UDPBufferClearTimeout UDP缓冲区清理超时时间
	// 如果为0，将使用默认值100ms
	UDPBufferClearTimeout time.Duration

	// MaxBufferClearPackets UDP缓冲区清理最大包数
	// 默认值: 100
	MaxBufferClearPackets int
}

// DefaultConfig 返回默认配置（客户端模式）
func DefaultConfig() *Config {
	return &Config{
		Mode:                   PoolModeClient, // 默认客户端模式
		MaxConnections:         10,
		MinConnections:         2,
		MaxIdleConnections:     10,
		ConnectionTimeout:      10 * time.Second,
		IdleTimeout:            5 * time.Minute,
		MaxLifetime:            30 * time.Minute,
		GetConnectionTimeout:   5 * time.Second,
		HealthCheckInterval:    30 * time.Second,
		HealthCheckTimeout:     3 * time.Second,
		ConnectionLeakTimeout:  5 * time.Minute,
		EnableStats:            true,
		EnableHealthCheck:      true,
		ClearUDPBufferOnReturn: true,                   // 默认启用UDP缓冲区清理
		UDPBufferClearTimeout:  100 * time.Millisecond, // 默认100ms超时
		MaxBufferClearPackets:  100,                    // 默认清理100个包
	}
}

// DefaultServerConfig 返回默认服务器端配置
func DefaultServerConfig() *Config {
	return &Config{
		Mode:                   PoolModeServer, // 服务器端模式
		MaxConnections:         100,            // 服务器端通常需要更多连接
		MinConnections:         0,              // 服务器端通常不需要预热
		MaxIdleConnections:     50,
		ConnectionTimeout:      10 * time.Second,
		IdleTimeout:            5 * time.Minute,
		MaxLifetime:            30 * time.Minute,
		GetConnectionTimeout:   5 * time.Second,
		HealthCheckInterval:    30 * time.Second,
		HealthCheckTimeout:     3 * time.Second,
		ConnectionLeakTimeout:  5 * time.Minute,
		EnableStats:            true,
		EnableHealthCheck:      true,
		ClearUDPBufferOnReturn: true,
		UDPBufferClearTimeout:  100 * time.Millisecond,
		MaxBufferClearPackets:  100,
	}
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	// 根据模式验证必需的配置
	switch c.Mode {
	case PoolModeClient:
		// 客户端模式需要Dialer
		if c.Dialer == nil {
			return ErrInvalidConfig
		}
	case PoolModeServer:
		// 服务器端模式需要Listener
		if c.Listener == nil {
			return ErrInvalidConfig
		}
		// 如果未提供Acceptor，使用默认的Accept方法
		if c.Acceptor == nil {
			c.Acceptor = defaultAcceptor
		}
	default:
		return ErrInvalidConfig
	}

	if c.MinConnections < 0 {
		return ErrInvalidConfig
	}
	if c.MaxConnections > 0 && c.MinConnections > c.MaxConnections {
		return ErrInvalidConfig
	}
	if c.MaxIdleConnections <= 0 {
		return ErrInvalidConfig
	}
	if c.ConnectionTimeout <= 0 {
		return ErrInvalidConfig
	}
	if c.MaxIdleConnections > 0 && c.MaxConnections > 0 && c.MaxIdleConnections > c.MaxConnections {
		// 最大空闲连接数不应超过最大连接数
		c.MaxIdleConnections = c.MaxConnections
	}
	if c.HealthCheckInterval > 0 && c.HealthCheckTimeout > c.HealthCheckInterval {
		// 健康检查超时不应超过检查间隔
		c.HealthCheckTimeout = c.HealthCheckInterval / 2
	}
	if c.MaxBufferClearPackets <= 0 {
		c.MaxBufferClearPackets = 100
	}
	return nil
}

// defaultAcceptor 默认的连接接受函数
func defaultAcceptor(ctx context.Context, listener net.Listener) (net.Conn, error) {
	// 使用带超时的Accept
	type result struct {
		conn net.Conn
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		conn, err := listener.Accept()
		select {
		case resultChan <- result{conn: conn, err: err}:
			// 成功发送结果
		case <-ctx.Done():
			// Context已取消，关闭连接（如果有）并退出，防止goroutine泄漏
			if conn != nil {
				conn.Close()
			}
		}
	}()

	select {
	case res := <-resultChan:
		return res.conn, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
