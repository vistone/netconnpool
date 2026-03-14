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

// Dialer connection creation function type (client mode)
// Returns network connection and error
type Dialer func(ctx context.Context) (net.Conn, error)

// Acceptor connection accept function type (server mode)
// Accepts new connections from Listener, returns network connection and error
type Acceptor func(ctx context.Context, listener net.Listener) (net.Conn, error)

// HealthChecker health check function type
// Returns whether connection is healthy
type HealthChecker func(conn net.Conn) bool

// Config connection pool configuration
type Config struct {
	// Mode connection pool mode: client or server
	// Default value is PoolModeClient (client mode)
	Mode PoolMode
	// MaxConnections max connection count, 0 means unlimited
	MaxConnections int

	// MinConnections min connection count (warm-up connection count)
	MinConnections int

	// MaxIdleConnections max idle connection count
	MaxIdleConnections int

	// ConnectionTimeout connection creation timeout
	ConnectionTimeout time.Duration

	// IdleTimeout idle connection timeout, idle connections exceeding this time will be closed
	IdleTimeout time.Duration

	// MaxLifetime connection max lifetime, connections exceeding this time will be closed
	MaxLifetime time.Duration

	// GetConnectionTimeout get connection timeout
	GetConnectionTimeout time.Duration

	// HealthCheckInterval health check interval
	HealthCheckInterval time.Duration

	// HealthCheckTimeout health check timeout
	HealthCheckTimeout time.Duration

	// ConnectionLeakTimeout connection leak detection timeout
	// If connection is not returned within this time, leak warning will be triggered
	ConnectionLeakTimeout time.Duration

	// LeakDetectionInterval leak detection loop interval
	// If 0, defaults to half of ConnectionLeakTimeout (min 1 second, max 1 minute)
	LeakDetectionInterval time.Duration

	// Dialer connection creation function (required for client mode)
	// In client mode, used to actively create connections to server
	Dialer Dialer

	// Listener network listener (required for server mode)
	// In server mode, used to accept client connections
	Listener net.Listener

	// Acceptor connection accept function (optional for server mode)
	// In server mode, used to accept connections from Listener
	// If nil, default Accept method will be used
	Acceptor Acceptor

	// HealthChecker health check function (optional)
	// If nil, default ping check will be used
	HealthChecker HealthChecker

	// CloseConn connection close function (optional)
	// If nil, will try type assertion to io.Closer and call Close method
	CloseConn func(conn net.Conn) error

	// OnCreated called after connection is created
	OnCreated func(conn net.Conn) error

	// OnBorrow called before connection is taken from pool
	OnBorrow func(conn net.Conn)

	// OnReturn called before connection is returned to pool
	OnReturn func(conn net.Conn)

	// EnableStats whether to enable statistics
	EnableStats bool

	// EnableHealthCheck whether to enable health check
	EnableHealthCheck bool

	// ClearUDPBufferOnReturn whether to clear read buffer when returning UDP connection
	// Enabling this option can prevent data confusion when UDP connections are reused
	// Default value is true, recommended to keep enabled
	ClearUDPBufferOnReturn bool

	// UDPBufferClearTimeout UDP buffer clear timeout
	// If 0, defaults to 100ms
	UDPBufferClearTimeout time.Duration

	// MaxBufferClearPackets max packets to clear from UDP buffer
	// Default: 100
	MaxBufferClearPackets int
}

// DefaultConfig returns default configuration (client mode)
func DefaultConfig() *Config {
	return &Config{
		Mode:                   PoolModeClient, // default client mode
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
		ClearUDPBufferOnReturn: true,                   // default enable UDP buffer clear
		UDPBufferClearTimeout:  100 * time.Millisecond, // default 100ms timeout
		MaxBufferClearPackets:  100,                    // default clear 100 packets
	}
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *Config {
	return &Config{
		Mode:                   PoolModeServer, // server mode
		MaxConnections:         100,            // server usually needs more connections
		MinConnections:         0,              // server usually doesn't need warm-up
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

// Validate validates configuration validity
func (c *Config) Validate() error {
	// Validate required configuration based on mode
	switch c.Mode {
	case PoolModeClient:
		// Client mode requires Dialer
		if c.Dialer == nil {
			return ErrInvalidConfig
		}
	case PoolModeServer:
		// Server mode requires Listener
		if c.Listener == nil {
			return ErrInvalidConfig
		}
		// If Acceptor is not provided, use default Accept method
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
		// Max idle connections should not exceed max connections
		c.MaxIdleConnections = c.MaxConnections
	}
	if c.HealthCheckInterval > 0 && c.HealthCheckTimeout > c.HealthCheckInterval {
		// Health check timeout should not exceed check interval
		c.HealthCheckTimeout = c.HealthCheckInterval / 2
	}
	if c.MaxBufferClearPackets <= 0 {
		c.MaxBufferClearPackets = 100
	}
	// If leak detection interval is not set, calculate automatically
	if c.LeakDetectionInterval <= 0 && c.ConnectionLeakTimeout > 0 {
		c.LeakDetectionInterval = c.ConnectionLeakTimeout / 2
		if c.LeakDetectionInterval < 1*time.Second {
			c.LeakDetectionInterval = 1 * time.Second
		}
		if c.LeakDetectionInterval > 1*time.Minute {
			c.LeakDetectionInterval = 1 * time.Minute
		}
	}
	return nil
}

// defaultAcceptor default connection accept function
func defaultAcceptor(ctx context.Context, listener net.Listener) (net.Conn, error) {
	// Use Accept with timeout
	type result struct {
		conn net.Conn
		err  error
	}
	resultChan := make(chan result, 1)
	// Channel to notify goroutine to exit
	doneChan := make(chan struct{})

	go func() {
		defer close(doneChan)
		conn, err := listener.Accept()
		// Use non-blocking send to ensure goroutine won't block forever
		select {
		case resultChan <- result{conn: conn, err: err}:
			// Successfully sent result
		case <-ctx.Done():
			// Context cancelled, close connection (if any) and exit to prevent goroutine leak
			if conn != nil {
				conn.Close()
			}
		}
	}()

	select {
	case res := <-resultChan:
		return res.conn, res.err
	case <-ctx.Done():
		// Wait for goroutine to exit or timeout to prevent goroutine leak
		select {
		case <-doneChan:
			// Goroutine has exited
		case <-time.After(100 * time.Millisecond):
			// Timeout, continue
		}
		return nil, ctx.Err()
	}
}
