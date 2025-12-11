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

// Pool 连接池
type Pool struct {
	config          *Config
	connections     map[uint64]*Connection // 所有连接（使用map实现O(1)查找）
	idleConnections chan *Connection       // 空闲连接通道
	mu              sync.RWMutex
	closed          bool
	closeOnce       sync.Once
	closeChan       chan struct{}

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
		config:          config,
		connections:     make(map[uint64]*Connection),
		idleConnections: make(chan *Connection, config.MaxIdleConnections),
		closed:          false,
		closeChan:       make(chan struct{}),
		waitQueue:       make(chan chan *Connection, 100), // 等待队列缓冲区
		waitQueueIPv4:   make(chan chan *Connection, 100), // IPv4等待队列
		waitQueueIPv6:   make(chan chan *Connection, 100), // IPv6等待队列
		waitQueueTCP:    make(chan chan *Connection, 100), // TCP等待队列
		waitQueueUDP:    make(chan chan *Connection, 100), // UDP等待队列
		createSemaphore: make(chan struct{}, 5),           // 最多5个并发创建连接
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

	// 尝试获取指定协议的连接
	maxAttempts := 10 // 最多尝试10次
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 检查连接池是否已关闭
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从空闲连接池获取指定协议的连接
		select {
		case conn := <-p.idleConnections:
			if conn == nil {
				continue
			}

			// 检查协议是否匹配
			if conn.GetProtocol() != protocol {
				// 协议不匹配，归还连接并继续
				p.Put(conn)
				continue
			}

			// 检查连接是否有效
			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			// 标记为使用中，并记录连接复用
			conn.MarkInUse()
			conn.IncrementReuseCount() // 增加复用计数

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

		// 创建新连接（注意：这里创建的新连接可能不是指定协议）
		// 如果创建的连接协议不匹配，需要重试
		conn, err := p.createConnection(ctx)
		if err != nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			return nil, err
		}

		if conn != nil {
			// 检查协议是否匹配
			if conn.GetProtocol() == protocol {
				conn.MarkInUse()

				if p.statsCollector != nil {
					p.statsCollector.IncrementSuccessfulGets()
					p.statsCollector.IncrementCurrentActiveConnections(1)
				}

				return conn, nil
			}

			// 协议不匹配，归还连接并继续尝试
			p.Put(conn)
			// 继续循环尝试
		}
	}

	// 如果尝试多次仍未找到匹配的连接，返回错误
	if p.statsCollector != nil {
		p.statsCollector.IncrementFailedGets()
	}
	return nil, ErrNoConnectionForProtocol
}

// GetWithIPVersion 获取指定IP版本的连接
func (p *Pool) GetWithIPVersion(ctx context.Context, ipVersion IPVersion, timeout time.Duration) (*Connection, error) {
	if ipVersion == IPVersionUnknown {
		// 如果指定了未知IP版本，回退到默认行为
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

	// 尝试获取指定IP版本的连接
	maxAttempts := 10 // 最多尝试10次
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 检查连接池是否已关闭
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从空闲连接池获取指定IP版本的连接
		select {
		case conn := <-p.idleConnections:
			if conn == nil {
				continue
			}

			// 检查IP版本是否匹配
			if conn.GetIPVersion() != ipVersion {
				// IP版本不匹配，归还连接并继续
				p.Put(conn)
				continue
			}

			// 检查连接是否有效
			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			// 标记为使用中，并记录连接复用
			conn.MarkInUse()
			conn.IncrementReuseCount() // 增加复用计数

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
				p.statsCollector.IncrementCurrentIdleConnections(-1)
				p.statsCollector.IncrementTotalConnectionsReused() // 记录连接复用
				// 更新IP版本空闲连接统计
				switch conn.GetIPVersion() {
				case IPVersionIPv4:
					p.statsCollector.IncrementCurrentIPv4IdleConnections(-1)
				case IPVersionIPv6:
					p.statsCollector.IncrementCurrentIPv6IdleConnections(-1)
				}
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
				return p.waitForConnectionWithIPVersion(ctx, ipVersion)
			}
		}

		// 创建新连接（注意：这里创建的新连接可能不是指定IP版本）
		// 如果创建的连接IP版本不匹配，需要重试
		conn, err := p.createConnection(ctx)
		if err != nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			return nil, err
		}

		if conn != nil {
			// 检查IP版本是否匹配
			if conn.GetIPVersion() == ipVersion {
				conn.MarkInUse()

				if p.statsCollector != nil {
					p.statsCollector.IncrementSuccessfulGets()
					p.statsCollector.IncrementCurrentActiveConnections(1)
				}

				return conn, nil
			}

			// IP版本不匹配，归还连接并继续尝试
			p.Put(conn)
			// 继续循环尝试
		}
	}

	// 如果尝试多次仍未找到匹配的连接，返回错误
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

	// 创建带超时的上下文
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	for {
		// 检查连接池是否已关闭
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从空闲连接池获取
		select {
		case conn := <-p.idleConnections:
			if conn == nil {
				continue
			}

			// 检查连接是否有效
			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			// 标记为使用中，并记录连接复用
			conn.MarkInUse()
			conn.IncrementReuseCount() // 增加复用计数

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
				p.statsCollector.IncrementCurrentIdleConnections(-1)
				p.statsCollector.IncrementTotalConnectionsReused() // 记录连接复用
				// 更新IP版本空闲连接统计
				switch conn.GetIPVersion() {
				case IPVersionIPv4:
					p.statsCollector.IncrementCurrentIPv4IdleConnections(-1)
				case IPVersionIPv6:
					p.statsCollector.IncrementCurrentIPv6IdleConnections(-1)
				}
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
				return p.waitForConnection(ctx)
			}
		}

		// 创建新连接
		conn, err := p.createConnection(ctx)
		if err != nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			// 如果是因为达到最大连接数限制而失败，应该等待可用连接
			if err == ErrMaxConnectionsReached {
				return p.waitForConnection(ctx)
			}
			return nil, err
		}

		if conn != nil {
			conn.MarkInUse()

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
			}

			return conn, nil
		}
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

	// 检查连接是否有效
	if !p.isConnectionValid(conn) {
		return p.closeConnection(conn)
	}

	// 对于UDP连接，如果配置了缓冲区清理，在归还前清空读取缓冲区
	// 这可以防止UDP连接复用时的数据混淆问题
	if p.config.ClearUDPBufferOnReturn && conn.GetProtocol() == ProtocolUDP {
		timeout := p.config.UDPBufferClearTimeout
		if timeout <= 0 {
			timeout = 100 * time.Millisecond // 默认100ms
		}
		// 清空UDP读取缓冲区（使用非阻塞方式，避免在高并发下阻塞太多goroutine）
		// 使用goroutine异步清理，不阻塞当前goroutine
		ClearUDPReadBufferNonBlocking(conn.GetConn(), timeout)
	}

	// 标记为空闲
	conn.MarkIdle()

	if p.statsCollector != nil {
		p.statsCollector.IncrementCurrentActiveConnections(-1)
	}

	// 尝试归还到空闲连接池
	select {
	case p.idleConnections <- conn:
		if p.statsCollector != nil {
			p.statsCollector.IncrementCurrentIdleConnections(1)
			// 更新IP版本空闲连接统计
			switch conn.GetIPVersion() {
			case IPVersionIPv4:
				p.statsCollector.IncrementCurrentIPv4IdleConnections(1)
			case IPVersionIPv6:
				p.statsCollector.IncrementCurrentIPv6IdleConnections(1)
			}
			// 更新协议空闲连接统计
			switch conn.GetProtocol() {
			case ProtocolTCP:
				p.statsCollector.IncrementCurrentTCPIdleConnections(1)
			case ProtocolUDP:
				p.statsCollector.IncrementCurrentUDPIdleConnections(1)
			}
		}

		// 通知等待队列
		p.notifyWaitQueue(conn)
		return nil
	default:
		// 空闲连接池已满，关闭连接
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

		// 先关闭closeChan通知所有等待的goroutine
		close(p.closeChan)

		// 停止后台任务（必须在closeChan关闭后，让它们能响应关闭信号）
		p.healthCheckManager.Stop()
		p.cleanupManager.Stop()
		p.leakDetector.Stop()

		// 关闭所有连接
		p.mu.Lock()
		conns := make([]*Connection, 0, len(p.connections))
		for _, conn := range p.connections {
			conns = append(conns, conn)
		}
		p.connections = make(map[uint64]*Connection) // 清空map
		p.mu.Unlock()

		// 在锁外关闭连接，避免死锁
		for _, conn := range conns {
			// 直接关闭，不再调用closeConnection（避免重复从map中删除）
			if closeErr := conn.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if p.statsCollector != nil {
				p.statsCollector.IncrementTotalConnectionsClosed()
			}
		}

		// 清理等待队列（通知等待的goroutine）
		for {
			select {
			case waitChan := <-p.waitQueue:
				close(waitChan)
			default:
				goto doneWaitQueue
			}
		}
	doneWaitQueue:
		close(p.waitQueue)

		// 清理IPv4等待队列
		for {
			select {
			case waitChan := <-p.waitQueueIPv4:
				close(waitChan)
			default:
				goto doneWaitQueueIPv4
			}
		}
	doneWaitQueueIPv4:
		close(p.waitQueueIPv4)

		// 清理IPv6等待队列
		for {
			select {
			case waitChan := <-p.waitQueueIPv6:
				close(waitChan)
			default:
				goto doneWaitQueueIPv6
			}
		}
	doneWaitQueueIPv6:
		close(p.waitQueueIPv6)

		// 清理TCP等待队列
		for {
			select {
			case waitChan := <-p.waitQueueTCP:
				close(waitChan)
			default:
				goto doneWaitQueueTCP
			}
		}
	doneWaitQueueTCP:
		close(p.waitQueueTCP)

		// 清理UDP等待队列
		for {
			select {
			case waitChan := <-p.waitQueueUDP:
				close(waitChan)
			default:
				goto doneWaitQueueUDP
			}
		}
	doneWaitQueueUDP:
		close(p.waitQueueUDP)

		// 清理空闲连接通道
		for {
			select {
			case <-p.idleConnections:
			default:
				goto doneIdleConnections
			}
		}
	doneIdleConnections:
		close(p.idleConnections)
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
	// 获取信号量，限制并发创建
	select {
	case p.createSemaphore <- struct{}{}:
		defer func() { <-p.createSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 再次检查是否已关闭
	if p.isClosed() {
		return nil, ErrPoolClosed
	}

	// 再次检查是否达到最大连接数
	if p.config.MaxConnections > 0 {
		p.mu.RLock()
		current := len(p.connections)
		p.mu.RUnlock()

		if current >= p.config.MaxConnections {
			return nil, ErrMaxConnectionsReached
		}
	}

	// 根据模式创建连接
	createCtx, cancel := context.WithTimeout(ctx, p.config.ConnectionTimeout)
	defer cancel()

	var conn any
	var err error

	switch p.config.Mode {
	case PoolModeServer:
		// 服务器端模式：从Listener接受连接
		if p.config.Listener == nil {
			return nil, ErrInvalidConfig
		}
		if p.config.Acceptor == nil {
			// 使用默认的Accept方法
			conn, err = defaultAcceptor(createCtx, p.config.Listener)
		} else {
			conn, err = p.config.Acceptor(createCtx, p.config.Listener)
		}
	case PoolModeClient:
		// 客户端模式：主动创建连接
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

	// 创建连接包装
	connection := NewConnection(conn, p, p.createCloseFunc(conn))

	// 对于UDP连接，如果配置了缓冲区清理，在创建时清空可能的初始数据
	// 这可以防止新创建的UDP连接包含之前的残留数据
	if p.config.ClearUDPBufferOnReturn && connection.GetProtocol() == ProtocolUDP {
		timeout := p.config.UDPBufferClearTimeout
		if timeout <= 0 {
			timeout = 100 * time.Millisecond
		}
		// 在后台清理，不阻塞连接创建
		ClearUDPReadBufferNonBlocking(conn, timeout)
	}

	// 添加到连接列表
	p.mu.Lock()
	p.connections[connection.ID] = connection
	p.mu.Unlock()

	if p.statsCollector != nil {
		p.statsCollector.IncrementTotalConnectionsCreated()
		// 更新IP版本统计
		switch connection.GetIPVersion() {
		case IPVersionIPv4:
			p.statsCollector.IncrementCurrentIPv4Connections(1)
		case IPVersionIPv6:
			p.statsCollector.IncrementCurrentIPv6Connections(1)
		}
		// 更新协议统计
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
func (p *Pool) createCloseFunc(conn any) func() error {
	return func() error {
		if p.config.CloseConn != nil {
			return p.config.CloseConn(conn)
		}

		// 尝试类型断言为io.Closer
		if closer, ok := conn.(io.Closer); ok {
			return closer.Close()
		}

		// 尝试类型断言为net.Conn
		if netConn, ok := conn.(net.Conn); ok {
			return netConn.Close()
		}

		return nil
	}
}

// closeConnection 关闭连接
func (p *Pool) closeConnection(conn *Connection) error {
	if conn == nil {
		return nil
	}

	// 先从列表中移除（如果还存在），再关闭（避免并发问题）
	p.mu.Lock()
	_, exists := p.connections[conn.ID]
	if exists {
		delete(p.connections, conn.ID)
	}
	p.mu.Unlock()

	// 即使不在列表中也要尝试关闭（可能已经被移除但连接还未关闭）
	err := conn.Close()

	if p.statsCollector != nil && exists {
		p.statsCollector.IncrementTotalConnectionsClosed()
		// 更新IP版本统计
		ipVersion := conn.GetIPVersion()
		switch ipVersion {
		case IPVersionIPv4:
			p.statsCollector.IncrementCurrentIPv4Connections(-1)
		case IPVersionIPv6:
			p.statsCollector.IncrementCurrentIPv6Connections(-1)
		}
		// 更新协议统计
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

	// 检查连接是否过期
	if conn.IsExpired(p.config.MaxLifetime) {
		return false
	}

	// 检查连接是否健康（如果启用了健康检查）
	if p.config.EnableHealthCheck && !conn.IsHealthy {
		return false
	}

	return true
}

// waitForConnection 等待可用连接
func (p *Pool) waitForConnection(ctx context.Context) (*Connection, error) {
	waitChan := make(chan *Connection, 1)
	defer close(waitChan) // 确保通道被关闭，防止泄漏

	select {
	case p.waitQueue <- waitChan:
		// 已加入等待队列
		select {
		case conn := <-waitChan:
			if conn != nil && p.isConnectionValid(conn) {
				conn.MarkInUse()
				conn.IncrementReuseCount() // 从等待队列获取的连接也是复用的
				if p.statsCollector != nil {
					p.statsCollector.IncrementSuccessfulGets()
					p.statsCollector.IncrementCurrentActiveConnections(1)
					p.statsCollector.IncrementTotalConnectionsReused()
				}
				return conn, nil
			}
			// 连接无效，继续等待
			select {
			case conn := <-waitChan:
				if conn != nil && p.isConnectionValid(conn) {
					conn.MarkInUse()
					if p.statsCollector != nil {
						p.statsCollector.IncrementSuccessfulGets()
						p.statsCollector.IncrementCurrentActiveConnections(1)
					}
					return conn, nil
				}
			case <-ctx.Done():
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				return nil, ErrPoolClosed
			}
		case <-ctx.Done():
			// 从等待队列中移除
			select {
			case <-p.waitQueue:
				// 成功移除
			default:
				// 队列为空或已关闭
			}
			if p.statsCollector != nil {
				p.statsCollector.IncrementTimeoutGets()
			}
			return nil, ErrGetConnectionTimeout
		case <-p.closeChan:
			// 从等待队列中移除
			select {
			case <-p.waitQueue:
				// 成功移除
			default:
				// 队列为空或已关闭
			}
			return nil, ErrPoolClosed
		}
	case <-ctx.Done():
		if p.statsCollector != nil {
			p.statsCollector.IncrementTimeoutGets()
		}
		return nil, ErrGetConnectionTimeout
	case <-p.closeChan:
		return nil, ErrPoolClosed
	}

	return nil, ErrGetConnectionTimeout
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
	defer close(waitChan) // 确保通道被关闭，防止泄漏

	select {
	case waitQueue <- waitChan:
		// 已加入等待队列
		select {
		case conn := <-waitChan:
			if conn != nil && conn.GetProtocol() == protocol && p.isConnectionValid(conn) {
				conn.MarkInUse()
				if p.statsCollector != nil {
					p.statsCollector.IncrementSuccessfulGets()
					p.statsCollector.IncrementCurrentActiveConnections(1)
				}
				return conn, nil
			}
			// 连接无效或协议不匹配，继续等待
			select {
			case conn := <-waitChan:
				if conn != nil && conn.GetProtocol() == protocol && p.isConnectionValid(conn) {
					conn.MarkInUse()
					if p.statsCollector != nil {
						p.statsCollector.IncrementSuccessfulGets()
						p.statsCollector.IncrementCurrentActiveConnections(1)
					}
					return conn, nil
				}
			case <-ctx.Done():
				// 从等待队列中移除
				select {
				case <-waitQueue:
					// 成功移除
				default:
					// 队列为空或已关闭
				}
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				// 从等待队列中移除
				select {
				case <-waitQueue:
					// 成功移除
				default:
					// 队列为空或已关闭
				}
				return nil, ErrPoolClosed
			}
		case <-ctx.Done():
			// 从等待队列中移除
			select {
			case <-waitQueue:
				// 成功移除
			default:
				// 队列为空或已关闭
			}
			if p.statsCollector != nil {
				p.statsCollector.IncrementTimeoutGets()
			}
			return nil, ErrGetConnectionTimeout
		case <-p.closeChan:
			// 从等待队列中移除
			select {
			case <-waitQueue:
				// 成功移除
			default:
				// 队列为空或已关闭
			}
			return nil, ErrPoolClosed
		}
	case <-ctx.Done():
		if p.statsCollector != nil {
			p.statsCollector.IncrementTimeoutGets()
		}
		return nil, ErrGetConnectionTimeout
	case <-p.closeChan:
		return nil, ErrPoolClosed
	}

	return nil, ErrGetConnectionTimeout
}

// notifyWaitQueue 通知等待队列
func (p *Pool) notifyWaitQueue(conn *Connection) {
	protocol := conn.GetProtocol()
	ipVersion := conn.GetIPVersion()

	// 首先尝试通知对应协议的等待队列
	if protocol == ProtocolTCP {
		if notified := p.notifySpecificWaitQueue(p.waitQueueTCP, conn); notified {
			return
		}
	} else if protocol == ProtocolUDP {
		if notified := p.notifySpecificWaitQueue(p.waitQueueUDP, conn); notified {
			return
		}
	}

	// 然后尝试通知对应IP版本的等待队列
	if ipVersion == IPVersionIPv4 {
		if notified := p.notifySpecificWaitQueue(p.waitQueueIPv4, conn); notified {
			return
		}
	} else if ipVersion == IPVersionIPv6 {
		if notified := p.notifySpecificWaitQueue(p.waitQueueIPv6, conn); notified {
			return
		}
	}

	// 如果对应协议和IP版本的等待队列都为空，尝试通用等待队列
	p.notifySpecificWaitQueue(p.waitQueue, conn)
}

// notifySpecificWaitQueue 通知特定的等待队列
func (p *Pool) notifySpecificWaitQueue(waitQueue chan chan *Connection, conn *Connection) bool {
	for {
		select {
		case waitChan := <-waitQueue:
			// 尝试发送连接给等待者
			select {
			case waitChan <- conn:
				// 成功发送，返回
				return true
			default:
				// 等待通道已满或已关闭，尝试下一个等待者
				select {
				case <-waitChan:
					// 通道已关闭，跳过
				default:
					// 继续尝试下一个
				}
			}
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
	defer close(waitChan) // 确保通道被关闭，防止泄漏

	select {
	case waitQueue <- waitChan:
		// 已加入等待队列
		select {
		case conn := <-waitChan:
			if conn != nil && conn.GetIPVersion() == ipVersion && p.isConnectionValid(conn) {
				conn.MarkInUse()
				if p.statsCollector != nil {
					p.statsCollector.IncrementSuccessfulGets()
					p.statsCollector.IncrementCurrentActiveConnections(1)
				}
				return conn, nil
			}
			// 连接无效或IP版本不匹配，继续等待
			select {
			case conn := <-waitChan:
				if conn != nil && conn.GetIPVersion() == ipVersion && p.isConnectionValid(conn) {
					conn.MarkInUse()
					if p.statsCollector != nil {
						p.statsCollector.IncrementSuccessfulGets()
						p.statsCollector.IncrementCurrentActiveConnections(1)
					}
					return conn, nil
				}
			case <-ctx.Done():
				// 从等待队列中移除
				select {
				case <-waitQueue:
					// 成功移除
				default:
					// 队列为空或已关闭
				}
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				// 从等待队列中移除
				select {
				case <-waitQueue:
					// 成功移除
				default:
					// 队列为空或已关闭
				}
				return nil, ErrPoolClosed
			}
		case <-ctx.Done():
			// 从等待队列中移除
			select {
			case <-waitQueue:
				// 成功移除
			default:
				// 队列为空或已关闭
			}
			if p.statsCollector != nil {
				p.statsCollector.IncrementTimeoutGets()
			}
			return nil, ErrGetConnectionTimeout
		case <-p.closeChan:
			// 从等待队列中移除
			select {
			case <-waitQueue:
				// 成功移除
			default:
				// 队列为空或已关闭
			}
			return nil, ErrPoolClosed
		}
	case <-ctx.Done():
		if p.statsCollector != nil {
			p.statsCollector.IncrementTimeoutGets()
		}
		return nil, ErrGetConnectionTimeout
	case <-p.closeChan:
		return nil, ErrPoolClosed
	}

	return nil, ErrGetConnectionTimeout
}

// warmUp 预热连接池
func (p *Pool) warmUp() error {
	// 服务器端模式不支持预热（只能被动接受连接）
	if p.config.Mode == PoolModeServer {
		return nil
	}

	if p.config.MinConnections <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectionTimeout*time.Duration(p.config.MinConnections))
	defer cancel()

	for i := 0; i < p.config.MinConnections; i++ {
		conn, err := p.createConnection(ctx)
		if err != nil {
			// 预热失败不影响连接池创建，记录错误即可
			continue
		}

		// 将连接放入空闲连接池
		select {
		case p.idleConnections <- conn:
			if p.statsCollector != nil {
				p.statsCollector.IncrementCurrentIdleConnections(1)
			}
		default:
			// 空闲连接池已满，关闭连接
			p.closeConnection(conn)
		}
	}

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
