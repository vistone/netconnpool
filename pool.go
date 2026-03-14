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
	"sync"
	"time"
)

// Pool 连接池
type Pool struct {
	config      *Config
	connections map[uint64]*Connection // 所有连接（使用map实现O(1)查找）

	// 分离的空闲连接通道，避免协议混淆导致的性能抖动
	idleTCPConnections chan *Connection
	idleUDPConnections chan *Connection

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	closeChan chan struct{}

	// 管理器
	healthCheckManager *HealthCheckManager
	cleanupManager     *CleanupManager
	leakDetector       *LeakDetector

	// 统计
	statsCollector *StatsCollector

	// 等待队列（当连接池耗尽时）
	waitQueue chan chan *Connection

	// IP版本特定的等待队列
	waitQueueIPv4 chan chan *Connection
	waitQueueIPv6 chan chan *Connection

	// 协议特定的等待队列
	waitQueueTCP chan chan *Connection
	waitQueueUDP chan chan *Connection

	// 同步原语
	createSemaphore chan struct{} // 限制并发创建连接数
}

// NewPool 创建新的连接池
func NewPool(config *Config) (*Pool, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	pool := &Pool{
		config:             config,
		connections:        make(map[uint64]*Connection),
		idleTCPConnections: make(chan *Connection, config.MaxIdleConnections),
		idleUDPConnections: make(chan *Connection, config.MaxIdleConnections),
		closed:             false,
		closeChan:          make(chan struct{}),
		waitQueue:          make(chan chan *Connection, 100), // 等待队列缓冲区
		waitQueueIPv4:      make(chan chan *Connection, 100), // IPv4等待队列
		waitQueueIPv6:      make(chan chan *Connection, 100), // IPv6等待队列
		waitQueueTCP:       make(chan chan *Connection, 100), // TCP等待队列
		waitQueueUDP:       make(chan chan *Connection, 100), // UDP等待队列
		createSemaphore:    make(chan struct{}, 5),           // 最多5个并发创建连接
	}

	// 初始化统计收集器
	if config.EnableStats {
		pool.statsCollector = NewStatsCollector()
	}

	// 初始化健康检查管理器
	pool.healthCheckManager = NewHealthCheckManager(pool, config)

	// 初始化清理管理器
	pool.cleanupManager = NewCleanupManager(pool, config)

	// 初始化泄漏检测器
	pool.leakDetector = NewLeakDetector(pool, config)

	// 预热连接池
	if err := pool.warmUp(); err != nil {
		return nil, err
	}

	// 启动后台任务
	pool.startBackgroundTasks()

	return pool, nil
}

// Get 获取一个连接（自动选择IP版本）
func (p *Pool) Get(ctx context.Context) (*Connection, error) {
	return p.GetWithTimeout(ctx, p.config.GetConnectionTimeout)
}

// GetIPv4 获取一个IPv4连接
func (p *Pool) GetIPv4(ctx context.Context) (*Connection, error) {
	return p.GetWithIPVersion(ctx, IPVersionIPv4, p.config.GetConnectionTimeout)
}

// GetIPv6 获取一个IPv6连接
func (p *Pool) GetIPv6(ctx context.Context) (*Connection, error) {
	return p.GetWithIPVersion(ctx, IPVersionIPv6, p.config.GetConnectionTimeout)
}

// GetTCP 获取一个TCP连接
func (p *Pool) GetTCP(ctx context.Context) (*Connection, error) {
	return p.GetWithProtocol(ctx, ProtocolTCP, p.config.GetConnectionTimeout)
}

// GetUDP 获取一个UDP连接
func (p *Pool) GetUDP(ctx context.Context) (*Connection, error) {
	return p.GetWithProtocol(ctx, ProtocolUDP, p.config.GetConnectionTimeout)
}

// GetWithProtocol 获取指定协议的连接
func (p *Pool) GetWithProtocol(ctx context.Context, protocol Protocol, timeout time.Duration) (*Connection, error) {
	if protocol == ProtocolUnknown {
		// 如果指定了未知协议，回退到默认行为
		return p.GetWithTimeout(ctx, timeout)
	}

	if p.isClosed() {
		return nil, ErrPoolClosed
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementTotalGetRequests()
		startTime := time.Now()
		defer func() {
			p.statsCollector.RecordGetTime(time.Since(startTime))
		}()
	}

	// 创建带超时的上下文
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 确定要使用的空闲通道
	var idleChan chan *Connection
	if protocol == ProtocolTCP {
		idleChan = p.idleTCPConnections
	} else {
		idleChan = p.idleUDPConnections
	}

	// 尝试获取指定协议的连接
	maxAttempts := 10          // 最多尝试10次
	protocolMismatchCount := 0 // 跟踪协议不匹配的次数
	createFailedCount := 0     // 跟踪创建连接失败的次数

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 检查连接池是否已关闭
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从空闲连接池获取指定协议的连接
		select {
		case conn := <-idleChan:
			if conn == nil {
				continue
			}

			// 检查连接是否有效
			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			// 检查协议是否匹配
			if conn.GetProtocol() != protocol {
				// 协议不匹配，归还到正确的池并继续
				p.Put(conn)
				continue
			}

			// 标记为使用中，并记录连接复用
			conn.MarkInUse()
			conn.IncrementReuseCount() // 增加复用计数

			// 调用借出钩子
			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
				p.statsCollector.IncrementCurrentIdleConnections(-1)
				p.statsCollector.IncrementTotalConnectionsReused() // 记录连接复用
				// 更新协议空闲连接统计
				switch conn.GetProtocol() {
				case ProtocolTCP:
					p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
				case ProtocolUDP:
					p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
				}
			}

			return conn, nil
		default:
			// 空闲连接池为空，尝试创建新连接
		}

		// 检查是否已达到最大连接数
		if p.config.MaxConnections > 0 {
			current := p.getCurrentConnectionsCount()
			if current >= p.config.MaxConnections {
				// 等待可用连接或超时
				return p.waitForConnectionWithProtocol(ctx, protocol)
			}
		}

		// 创建新连接
		conn, err := p.createConnection(ctx)
		if err != nil {
			createFailedCount++
			// 如果创建连接多次失败，返回错误避免无限循环
			if createFailedCount >= 3 {
				if p.statsCollector != nil {
					p.statsCollector.IncrementFailedGets()
					p.statsCollector.IncrementConnectionErrors()
				}
				return nil, err
			}
			// 短暂延迟后重试
			select {
			case <-time.After(10 * time.Millisecond):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// 创建连接成功但返回nil（不应该发生），添加保护
		if conn == nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
			}
			return nil, ErrInvalidConnection
		}

		// 检查协议是否匹配（理论上createConnection应该返回正确的协议，但为了安全）
		if conn.GetProtocol() == protocol {
			conn.MarkInUse()

			// 调用借出钩子
			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
			}

			return conn, nil
		}

		// 协议不匹配，归还连接
		protocolMismatchCount++
		p.Put(conn)

		// 如果连续多次不匹配，返回错误，避免无限循环
		if protocolMismatchCount >= 3 {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
			}
			return nil, ErrNoConnectionForProtocol
		}
		// 继续循环尝试
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementFailedGets()
	}
	return nil, ErrNoConnectionForProtocol
}

// GetWithIPVersion 获取指定IP版本的连接
func (p *Pool) GetWithIPVersion(ctx context.Context, ipVersion IPVersion, timeout time.Duration) (*Connection, error) {
	if ipVersion == IPVersionUnknown {
		return p.GetWithTimeout(ctx, timeout)
	}

	if p.isClosed() {
		return nil, ErrPoolClosed
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementTotalGetRequests()
		startTime := time.Now()
		defer func() {
			p.statsCollector.RecordGetTime(time.Since(startTime))
		}()
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	maxAttempts := 10
	ipVersionMismatchCount := 0 // 跟踪IP版本不匹配的次数
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从两个空闲池中获取
		var conn *Connection
		select {
		case conn = <-p.idleTCPConnections:
		case conn = <-p.idleUDPConnections:
		default:
			// 都没有空闲
		}

		if conn != nil {
			// 检查IP版本是否匹配
			if conn.GetIPVersion() != ipVersion {
				p.Put(conn)
				ipVersionMismatchCount++
				// 如果连续多次不匹配，返回错误，避免无限循环
				if ipVersionMismatchCount >= 3 {
					if p.statsCollector != nil {
						p.statsCollector.IncrementFailedGets()
					}
					return nil, ErrNoConnectionForIPVersion
				}
				continue
			}

			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			conn.MarkInUse()
			conn.IncrementReuseCount()

			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
				p.statsCollector.IncrementCurrentIdleConnections(-1)
				p.statsCollector.IncrementTotalConnectionsReused()
				switch conn.GetIPVersion() {
				case IPVersionIPv4:
					p.statsCollector.IncrementCurrentIPv4IdleConnections(-1)
				case IPVersionIPv6:
					p.statsCollector.IncrementCurrentIPv6IdleConnections(-1)
				}
				switch conn.GetProtocol() {
				case ProtocolTCP:
					p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
				case ProtocolUDP:
					p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
				}
			}

			return conn, nil
		}

		// 尝试创建
		if p.config.MaxConnections > 0 {
			current := p.getCurrentConnectionsCount()
			if current >= p.config.MaxConnections {
				return p.waitForConnectionWithIPVersion(ctx, ipVersion)
			}
		}

		conn, err := p.createConnection(ctx)
		if err != nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			return nil, err
		}

		if conn == nil {
			// 不应该发生，但添加保护防止无限循环
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			return nil, ErrInvalidConnection
		}

		if conn.GetIPVersion() == ipVersion {
			conn.MarkInUse()
			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}
			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
			}
			return conn, nil
		}

		// IP版本不匹配，归还连接
		ipVersionMismatchCount++
		p.Put(conn)

		// 如果连续多次不匹配，返回错误，避免无限循环
		if ipVersionMismatchCount >= 3 {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
			}
			return nil, ErrNoConnectionForIPVersion
		}
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementFailedGets()
	}
	return nil, ErrNoConnectionForIPVersion
}

// GetWithTimeout 获取一个连接（带超时，自动选择IP版本）
func (p *Pool) GetWithTimeout(ctx context.Context, timeout time.Duration) (*Connection, error) {
	if p.isClosed() {
		return nil, ErrPoolClosed
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementTotalGetRequests()
		startTime := time.Now()
		defer func() {
			p.statsCollector.RecordGetTime(time.Since(startTime))
		}()
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	for {
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从任意空闲池获取
		var conn *Connection
		select {
		case conn = <-p.idleTCPConnections:
		case conn = <-p.idleUDPConnections:
		default:
		}

		if conn != nil {
			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			conn.MarkInUse()
			conn.IncrementReuseCount()

			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
				p.statsCollector.IncrementCurrentIdleConnections(-1)
				p.statsCollector.IncrementTotalConnectionsReused()
				switch conn.GetIPVersion() {
				case IPVersionIPv4:
					p.statsCollector.IncrementCurrentIPv4IdleConnections(-1)
				case IPVersionIPv6:
					p.statsCollector.IncrementCurrentIPv6IdleConnections(-1)
				}
				switch conn.GetProtocol() {
				case ProtocolTCP:
					p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
				case ProtocolUDP:
					p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
				}
			}

			return conn, nil
		}

		if p.config.MaxConnections > 0 {
			current := p.getCurrentConnectionsCount()
			if current >= p.config.MaxConnections {
				return p.waitForConnection(ctx)
			}
		}

		conn, err := p.createConnection(ctx)
		if err != nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			if err == ErrMaxConnectionsReached {
				return p.waitForConnection(ctx)
			}
			return nil, err
		}

		if conn == nil {
			// 不应该发生，但添加保护防止无限循环
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			return nil, ErrInvalidConnection
		}

		conn.MarkInUse()
		if p.config.OnBorrow != nil {
			p.config.OnBorrow(conn.GetConn())
		}
		if p.statsCollector != nil {
			p.statsCollector.IncrementSuccessfulGets()
			p.statsCollector.IncrementCurrentActiveConnections(1)
		}
		return conn, nil
	}
}

// Put 归还连接
func (p *Pool) Put(conn *Connection) error {
	if conn == nil {
		return ErrInvalidConnection
	}

	if p.isClosed() {
		return p.closeConnection(conn)
	}

	if !p.isConnectionValid(conn) {
		return p.closeConnection(conn)
	}

	// 调用归还钩子
	if p.config.OnReturn != nil {
		p.config.OnReturn(conn.GetConn())
	}

	if p.config.ClearUDPBufferOnReturn && conn.GetProtocol() == ProtocolUDP {
		timeout := p.config.UDPBufferClearTimeout
		if timeout <= 0 {
			timeout = 100 * time.Millisecond
		}
		ClearUDPReadBufferNonBlocking(conn.GetConn(), timeout, p.config.MaxBufferClearPackets)
	}

	conn.MarkIdle()

	if p.statsCollector != nil {
		p.statsCollector.IncrementCurrentActiveConnections(-1)
	}

	// 确定归还到哪个通道
	var idleChan chan *Connection
	if conn.GetProtocol() == ProtocolTCP {
		idleChan = p.idleTCPConnections
	} else {
		idleChan = p.idleUDPConnections
	}

	select {
	case idleChan <- conn:
		if p.statsCollector != nil {
			p.statsCollector.IncrementCurrentIdleConnections(1)
			switch conn.GetIPVersion() {
			case IPVersionIPv4:
				p.statsCollector.IncrementCurrentIPv4IdleConnections(1)
			case IPVersionIPv6:
				p.statsCollector.IncrementCurrentIPv6IdleConnections(1)
			}
			switch conn.GetProtocol() {
			case ProtocolTCP:
				p.statsCollector.IncrementCurrentTCPIdleConnections(1)
			case ProtocolUDP:
				p.statsCollector.IncrementCurrentUDPIdleConnections(1)
			}
		}

		p.notifyWaitQueue(conn)
		return nil
	default:
		return p.closeConnection(conn)
	}
}

// Close 关闭连接池
func (p *Pool) Close() error {
	var err error
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()

		close(p.closeChan)

		p.healthCheckManager.Stop()
		p.cleanupManager.Stop()
		p.leakDetector.Stop()

		p.mu.Lock()
		conns := make([]*Connection, 0, len(p.connections))
		for _, conn := range p.connections {
			conns = append(conns, conn)
		}
		p.connections = make(map[uint64]*Connection)
		p.mu.Unlock()

		for _, conn := range conns {
			if closeErr := conn.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if p.statsCollector != nil {
				p.statsCollector.IncrementTotalConnectionsClosed()
			}
		}

		// 清理等待队列
		closeWaitQueue := func(q chan chan *Connection) {
			defer func() {
				// 恢复可能的 panic（例如关闭已关闭的 channel）
				// 这是预期的行为，因为 channel 可能已经被关闭
				if r := recover(); r != nil {
					// 预期的 panic：关闭已关闭的 channel
					// 不重新抛出，确保其他资源能够被清理
					_ = r
				}
			}()
			for {
				select {
				case waitChan := <-q:
					// 安全关闭 waitChan
					func() {
						defer func() {
							// 预期的 panic：向已关闭的 channel 发送数据或关闭已关闭的 channel
							if r := recover(); r != nil {
								_ = r
							}
						}()
						close(waitChan)
					}()
				default:
					// 尝试关闭队列 channel
					func() {
						defer func() {
							if r := recover(); r != nil {
								// 预期的 panic：关闭已关闭的 channel
								_ = r
							}
						}()
						close(q)
					}()
					return
				}
			}
		}

		closeWaitQueue(p.waitQueue)
		closeWaitQueue(p.waitQueueIPv4)
		closeWaitQueue(p.waitQueueIPv6)
		closeWaitQueue(p.waitQueueTCP)
		closeWaitQueue(p.waitQueueUDP)

		// 清理空闲连接通道
		closeIdleChan := func(c chan *Connection) {
			for {
				select {
				case <-c:
				default:
					close(c)
					return
				}
			}
		}
		closeIdleChan(p.idleTCPConnections)
		closeIdleChan(p.idleUDPConnections)
	})
	return err
}

// Stats 获取统计信息
func (p *Pool) Stats() Stats {
	if p.statsCollector == nil {
		return Stats{}
	}
	return p.statsCollector.GetStats()
}

// createConnection 创建新连接
func (p *Pool) createConnection(ctx context.Context) (*Connection, error) {
	select {
	case p.createSemaphore <- struct{}{}:
		defer func() { <-p.createSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if p.isClosed() {
		return nil, ErrPoolClosed
	}

	if p.config.MaxConnections > 0 {
		p.mu.RLock()
		current := len(p.connections)
		p.mu.RUnlock()

		if current >= p.config.MaxConnections {
			return nil, ErrMaxConnectionsReached
		}
	}

	createCtx, cancel := context.WithTimeout(ctx, p.config.ConnectionTimeout)
	defer cancel()

	var conn net.Conn
	var err error

	switch p.config.Mode {
	case PoolModeServer:
		if p.config.Listener == nil {
			return nil, ErrInvalidConfig
		}
		if p.config.Acceptor == nil {
			conn, err = defaultAcceptor(createCtx, p.config.Listener)
		} else {
			conn, err = p.config.Acceptor(createCtx, p.config.Listener)
		}
	case PoolModeClient:
		if p.config.Dialer == nil {
			return nil, ErrInvalidConfig
		}
		conn, err = p.config.Dialer(createCtx)
	default:
		return nil, ErrInvalidConfig
	}

	if err != nil {
		return nil, err
	}

	// 调用创建钩子
	if p.config.OnCreated != nil {
		if err := p.config.OnCreated(conn); err != nil {
			conn.Close()
			return nil, err
		}
	}

	connection := NewConnection(conn, p, p.createCloseFunc(conn))

	if p.config.ClearUDPBufferOnReturn && connection.GetProtocol() == ProtocolUDP {
		timeout := p.config.UDPBufferClearTimeout
		if timeout <= 0 {
			timeout = 100 * time.Millisecond
		}
		ClearUDPReadBufferNonBlocking(conn, timeout, p.config.MaxBufferClearPackets)
	}

	p.mu.Lock()
	p.connections[connection.ID] = connection
	p.mu.Unlock()

	if p.statsCollector != nil {
		p.statsCollector.IncrementTotalConnectionsCreated()
		switch connection.GetIPVersion() {
		case IPVersionIPv4:
			p.statsCollector.IncrementCurrentIPv4Connections(1)
		case IPVersionIPv6:
			p.statsCollector.IncrementCurrentIPv6Connections(1)
		}
		switch connection.GetProtocol() {
		case ProtocolTCP:
			p.statsCollector.IncrementCurrentTCPConnections(1)
		case ProtocolUDP:
			p.statsCollector.IncrementCurrentUDPConnections(1)
		}
	}

	return connection, nil
}

// createCloseFunc 创建关闭函数
func (p *Pool) createCloseFunc(conn net.Conn) func() error {
	return func() error {
		if p.config.CloseConn != nil {
			return p.config.CloseConn(conn)
		}
		return conn.Close()
	}
}

// closeConnection 关闭连接
func (p *Pool) closeConnection(conn *Connection) error {
	if conn == nil {
		return nil
	}

	// 先标记连接为不健康，防止被复用
	conn.UpdateHealth(false)

	// 从map删除 - 在关闭连接之前删除，避免竞态条件
	p.mu.Lock()
	_, exists := p.connections[conn.ID]
	if exists {
		delete(p.connections, conn.ID)
	}
	p.mu.Unlock()

	// 然后关闭连接，确保连接不可用
	// 注意：在删除map后再关闭连接，避免其他goroutine在关闭期间获取到该连接
	err := conn.Close()

	// 更新统计信息
	if p.statsCollector != nil && exists {
		p.statsCollector.IncrementTotalConnectionsClosed()
		ipVersion := conn.GetIPVersion()
		switch ipVersion {
		case IPVersionIPv4:
			p.statsCollector.IncrementCurrentIPv4Connections(-1)
		case IPVersionIPv6:
			p.statsCollector.IncrementCurrentIPv6Connections(-1)
		}
		protocol := conn.GetProtocol()
		switch protocol {
		case ProtocolTCP:
			p.statsCollector.IncrementCurrentTCPConnections(-1)
		case ProtocolUDP:
			p.statsCollector.IncrementCurrentUDPConnections(-1)
		}
	}

	return err
}

// isConnectionValid 检查连接是否有效
func (p *Pool) isConnectionValid(conn *Connection) bool {
	if conn == nil || conn.GetConn() == nil {
		return false
	}

	if conn.IsExpired(p.config.MaxLifetime) {
		return false
	}

	if p.config.EnableHealthCheck && !conn.GetHealthStatus() {
		return false
	}

	return true
}

// waitForConnection 等待可用连接
func (p *Pool) waitForConnection(ctx context.Context) (*Connection, error) {
	waitChan := make(chan *Connection, 1)
	defer close(waitChan)

	select {
	case p.waitQueue <- waitChan:
		// 成功加入等待队列，等待连接
		for {
			select {
			case conn := <-waitChan:
				if conn != nil && p.isConnectionValid(conn) {
					conn.MarkInUse()
					conn.IncrementReuseCount()
					if p.config.OnBorrow != nil {
						p.config.OnBorrow(conn.GetConn())
					}
					if p.statsCollector != nil {
						p.statsCollector.IncrementSuccessfulGets()
						p.statsCollector.IncrementCurrentActiveConnections(1)
						p.statsCollector.IncrementTotalConnectionsReused()
						switch conn.GetIPVersion() {
						case IPVersionIPv4:
							p.statsCollector.IncrementCurrentIPv4IdleConnections(-1)
						case IPVersionIPv6:
							p.statsCollector.IncrementCurrentIPv6IdleConnections(-1)
						}
						switch conn.GetProtocol() {
						case ProtocolTCP:
							p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
						case ProtocolUDP:
							p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
						}
					}
					return conn, nil
				}
				// 连接无效，关闭它并继续等待下一个连接
				// 注意：这里不返回错误，而是继续等待，因为可能会有其他连接归还
				if conn != nil {
					p.closeConnection(conn)
				}
				continue
			case <-ctx.Done():
				// 检查waitChan中是否有数据，避免通知者阻塞
				select {
				case <-waitChan:
				default:
				}
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				// 检查waitChan中是否有数据，避免通知者阻塞
				select {
				case <-waitChan:
				default:
				}
				return nil, ErrPoolClosed
			}
		}
	case <-ctx.Done():
		if p.statsCollector != nil {
			p.statsCollector.IncrementTimeoutGets()
		}
		return nil, ErrGetConnectionTimeout
	case <-p.closeChan:
		return nil, ErrPoolClosed
	}
}

// waitForConnectionWithProtocol 等待指定协议的可用连接
func (p *Pool) waitForConnectionWithProtocol(ctx context.Context, protocol Protocol) (*Connection, error) {
	var waitQueue chan chan *Connection
	switch protocol {
	case ProtocolTCP:
		waitQueue = p.waitQueueTCP
	case ProtocolUDP:
		waitQueue = p.waitQueueUDP
	default:
		return p.waitForConnection(ctx)
	}

	waitChan := make(chan *Connection, 1)
	defer close(waitChan)

	select {
	case waitQueue <- waitChan:
		// 成功加入等待队列，等待连接
		for {
			select {
			case conn := <-waitChan:
				if conn != nil && conn.GetProtocol() == protocol && p.isConnectionValid(conn) {
					conn.MarkInUse()
					conn.IncrementReuseCount()
					if p.config.OnBorrow != nil {
						p.config.OnBorrow(conn.GetConn())
					}
					if p.statsCollector != nil {
						p.statsCollector.IncrementSuccessfulGets()
						p.statsCollector.IncrementCurrentActiveConnections(1)
						p.statsCollector.IncrementTotalConnectionsReused()
						switch conn.GetProtocol() {
						case ProtocolTCP:
							p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
						case ProtocolUDP:
							p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
						}
					}
					return conn, nil
				}
				// 连接无效或协议不匹配，关闭它并继续等待
				if conn != nil {
					p.closeConnection(conn)
				}
				continue
			case <-ctx.Done():
				// 检查waitChan中是否有数据，避免通知者阻塞
				select {
				case <-waitChan:
				default:
				}
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				// 检查waitChan中是否有数据，避免通知者阻塞
				select {
				case <-waitChan:
				default:
				}
				return nil, ErrPoolClosed
			}
		}
	case <-ctx.Done():
		if p.statsCollector != nil {
			p.statsCollector.IncrementTimeoutGets()
		}
		return nil, ErrGetConnectionTimeout
	case <-p.closeChan:
		return nil, ErrPoolClosed
	}
}

// notifyWaitQueue 通知等待队列
func (p *Pool) notifyWaitQueue(conn *Connection) {
	protocol := conn.GetProtocol()
	ipVersion := conn.GetIPVersion()

	if protocol == ProtocolTCP {
		if notified := p.notifySpecificWaitQueue(p.waitQueueTCP, conn); notified {
			return
		}
	} else if protocol == ProtocolUDP {
		if notified := p.notifySpecificWaitQueue(p.waitQueueUDP, conn); notified {
			return
		}
	}

	if ipVersion == IPVersionIPv4 {
		if notified := p.notifySpecificWaitQueue(p.waitQueueIPv4, conn); notified {
			return
		}
	} else if ipVersion == IPVersionIPv6 {
		if notified := p.notifySpecificWaitQueue(p.waitQueueIPv6, conn); notified {
			return
		}
	}

	p.notifySpecificWaitQueue(p.waitQueue, conn)
}

// notifySpecificWaitQueue 通知特定的等待队列
func (p *Pool) notifySpecificWaitQueue(waitQueue chan chan *Connection, conn *Connection) bool {
	for {
		select {
		case waitChan := <-waitQueue:
			// 使用 recover 防止向已关闭的 channel 发送数据导致 panic
			sent := func() (success bool) {
				defer func() {
					if r := recover(); r != nil {
						// Channel 已关闭，忽略这个等待者
						success = false
					}
				}()
				select {
				case waitChan <- conn:
					return true
				default:
					return false
				}
			}()

			if sent {
				return true
			}
			// 发送失败，尝试下一个等待者
		default:
			// 没有等待的请求
			return false
		}
	}
}

// waitForConnectionWithIPVersion 等待指定IP版本的可用连接
func (p *Pool) waitForConnectionWithIPVersion(ctx context.Context, ipVersion IPVersion) (*Connection, error) {
	var waitQueue chan chan *Connection
	switch ipVersion {
	case IPVersionIPv4:
		waitQueue = p.waitQueueIPv4
	case IPVersionIPv6:
		waitQueue = p.waitQueueIPv6
	default:
		return p.waitForConnection(ctx)
	}

	waitChan := make(chan *Connection, 1)
	closed := false
	defer func() {
		if !closed {
			close(waitChan)
		}
	}()

	select {
	case waitQueue <- waitChan:
		// 成功加入等待队列，等待连接
		for {
			select {
			case conn := <-waitChan:
				if conn != nil && conn.GetIPVersion() == ipVersion && p.isConnectionValid(conn) {
					closed = true
					close(waitChan)
					conn.MarkInUse()
					conn.IncrementReuseCount()
					if p.config.OnBorrow != nil {
						p.config.OnBorrow(conn.GetConn())
					}
					if p.statsCollector != nil {
						p.statsCollector.IncrementSuccessfulGets()
						p.statsCollector.IncrementCurrentActiveConnections(1)
						p.statsCollector.IncrementTotalConnectionsReused()
						switch conn.GetIPVersion() {
						case IPVersionIPv4:
							p.statsCollector.IncrementCurrentIPv4IdleConnections(-1)
						case IPVersionIPv6:
							p.statsCollector.IncrementCurrentIPv6IdleConnections(-1)
						}
						switch conn.GetProtocol() {
						case ProtocolTCP:
							p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
						case ProtocolUDP:
							p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
						}
					}
					return conn, nil
				}
				// 连接无效或IP版本不匹配，关闭它并继续等待
				if conn != nil {
					p.closeConnection(conn)
				}
				continue
			case <-ctx.Done():
				closed = true
				close(waitChan)
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				closed = true
				close(waitChan)
				return nil, ErrPoolClosed
			}
		}
	case <-ctx.Done():
		closed = true
		close(waitChan)
		if p.statsCollector != nil {
			p.statsCollector.IncrementTimeoutGets()
		}
		return nil, ErrGetConnectionTimeout
	case <-p.closeChan:
		closed = true
		close(waitChan)
		return nil, ErrPoolClosed
	}
}

// warmUp 预热连接池
func (p *Pool) warmUp() error {
	if p.config.Mode == PoolModeServer {
		return nil
	}

	if p.config.MinConnections <= 0 {
		return nil
	}

	// 防止超时时间溢出，设置最大超时时间为10分钟
	maxTimeout := 10 * time.Minute
	timeout := p.config.ConnectionTimeout * time.Duration(p.config.MinConnections)
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	successCount := 0
	for i := 0; i < p.config.MinConnections; i++ {
		conn, err := p.createConnection(ctx)
		if err != nil {
			// 预热失败，更新统计信息
			if p.statsCollector != nil {
				p.statsCollector.IncrementConnectionErrors()
			}
			continue
		}

		// 将连接放入空闲连接池
		if err := p.Put(conn); err != nil {
			// Put 失败（可能是池已满），确保连接被关闭
			// 注意：Put 方法在失败时会调用 closeConnection，
			// 但为了确保资源被释放，这里不需要额外操作
			// 因为 Put 内部已经处理了关闭逻辑
		} else {
			successCount++
		}
	}

	// 如果所有预热都失败，可以考虑返回错误（但为了兼容性，暂时不返回）
	// 统计信息已经记录了错误，调用者可以通过统计信息了解预热情况
	return nil
}

// startBackgroundTasks 启动后台任务
func (p *Pool) startBackgroundTasks() {
	if p.config.EnableHealthCheck {
		p.healthCheckManager.Start()
	}
	p.cleanupManager.Start()
	p.leakDetector.Start()
}

// isClosed 检查连接池是否已关闭
func (p *Pool) isClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// getCurrentConnectionsCount 获取当前连接数
func (p *Pool) getCurrentConnectionsCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections)
}

// getAllConnections 获取所有连接（用于健康检查和清理）
func (p *Pool) getAllConnections() []*Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	connections := make([]*Connection, 0, len(p.connections))
	for _, conn := range p.connections {
		connections = append(connections, conn)
	}
	return connections
}
