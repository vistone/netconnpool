// 修复建议代码示例
// 本文件包含了对发现问题的修复建议代码
// 注意：这些代码仅供参考，需要根据实际情况调整

package netconnpool

import (
	"context"
	"net"
	"sync"
	"time"
)

// ============================================================================
// 修复1: defaultAcceptor - 防止goroutine泄漏
// ============================================================================

// FixedDefaultAcceptor 修复后的连接接受函数
func fixedDefaultAcceptor(ctx context.Context, listener net.Listener) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		conn, err := listener.Accept()
		select {
		case resultChan <- result{conn: conn, err: err}:
			// 成功发送
		case <-ctx.Done():
			// Context已取消，关闭连接（如果有）并退出
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

// ============================================================================
// 修复2: ClearUDPReadBufferNonBlocking - 限制并发数
// ============================================================================

var (
	// udpCleanupSemaphore 限制UDP清理的并发数
	udpCleanupSemaphore = make(chan struct{}, 10)
)

// FixedClearUDPReadBufferNonBlocking 修复后的非阻塞UDP缓冲区清理
func fixedClearUDPReadBufferNonBlocking(conn net.Conn, timeout time.Duration, maxPackets int) {
	select {
	case udpCleanupSemaphore <- struct{}{}:
		go func() {
			defer func() { <-udpCleanupSemaphore }()
			ClearUDPReadBuffer(conn, timeout, maxPackets)
		}()
	default:
		// 如果清理goroutine已满，使用更短的超时时间同步清理
		// 或者直接跳过（如果性能允许）
		ClearUDPReadBuffer(conn, timeout/2, maxPackets)
	}
}

// ============================================================================
// 修复3: waitForConnectionWithProtocol - 正确处理channel关闭
// ============================================================================

// FixedWaitForConnectionWithProtocol 修复后的等待指定协议连接
func (p *Pool) fixedWaitForConnectionWithProtocol(ctx context.Context, protocol Protocol) (*Connection, error) {
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
				if conn != nil && conn.GetProtocol() == protocol && p.isConnectionValid(conn) {
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
					}
					return conn, nil
				}
				// 连接无效或协议不匹配，关闭它并继续等待
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

// ============================================================================
// 修复4: GetWithProtocol - 防止无限循环
// ============================================================================

// FixedGetWithProtocol 修复后的获取指定协议连接
func (p *Pool) fixedGetWithProtocol(ctx context.Context, protocol Protocol, timeout time.Duration) (*Connection, error) {
	if protocol == ProtocolUnknown {
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

	var idleChan chan *Connection
	if protocol == ProtocolTCP {
		idleChan = p.idleTCPConnections
	} else {
		idleChan = p.idleUDPConnections
	}

	maxAttempts := 10
	protocolMismatchCount := 0

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// 尝试从空闲连接池获取指定协议的连接
		select {
		case conn := <-idleChan:
			if conn == nil {
				continue
			}

			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			// 检查协议是否匹配
			if conn.GetProtocol() != protocol {
				// 协议不匹配，归还到正确的池
				p.Put(conn)
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
				switch conn.GetProtocol() {
				case ProtocolTCP:
					p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
				case ProtocolUDP:
					p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
				}
			}

			return conn, nil
		default:
			// 空闲连接池为空
		}

		// 检查是否已达到最大连接数
		if p.config.MaxConnections > 0 {
			current := p.getCurrentConnectionsCount()
			if current >= p.config.MaxConnections {
				return p.fixedWaitForConnectionWithProtocol(ctx, protocol)
			}
		}

		// 创建新连接
		conn, err := p.createConnection(ctx)
		if err != nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
				p.statsCollector.IncrementConnectionErrors()
			}
			return nil, err
		}

		if conn != nil {
			if conn.GetProtocol() == protocol {
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

			// 协议不匹配
			protocolMismatchCount++
			p.Put(conn)

			// 如果连续多次不匹配，返回错误
			if protocolMismatchCount >= 3 {
				if p.statsCollector != nil {
					p.statsCollector.IncrementFailedGets()
				}
				return nil, ErrNoConnectionForProtocol
			}
		}
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementFailedGets()
	}
	return nil, ErrNoConnectionForProtocol
}

// ============================================================================
// 修复5: waitForConnection - 正确处理无效连接
// ============================================================================

// FixedWaitForConnection 修复后的等待连接
func (p *Pool) fixedWaitForConnection(ctx context.Context) (*Connection, error) {
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
					}
					return conn, nil
				}
				// 连接无效，关闭它并继续等待
				if conn != nil {
					p.closeConnection(conn)
				}
				continue
			case <-ctx.Done():
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
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

// ============================================================================
// 修复6: getAllConnections - 优化内存分配
// ============================================================================

var (
	// connectionSlicePool 用于复用Connection slice的对象池
	connectionSlicePool = sync.Pool{
		New: func() interface{} {
			return make([]*Connection, 0, 100)
		},
	}
)

// FixedGetAllConnections 修复后的获取所有连接（优化内存分配）
func (p *Pool) fixedGetAllConnections() []*Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 从pool获取slice
	connections := connectionSlicePool.Get().([]*Connection)
	connections = connections[:0] // 重置长度但保留容量

	for _, conn := range p.connections {
		connections = append(connections, conn)
	}

	// 注意：调用者负责在使用完后归还slice
	// 或者在这里使用defer（但需要确保调用者不会持有slice的引用）
	return connections
}

// ReturnConnectionSlice 归还Connection slice到对象池
func ReturnConnectionSlice(slice []*Connection) {
	if cap(slice) < 1000 { // 只归还小slice，避免内存占用过大
		connectionSlicePool.Put(slice[:0]) // 重置长度
	}
}

// ============================================================================
// 修复7: checkConnection - 使用context控制健康检查
// ============================================================================

// FixedCheckConnection 修复后的检查单个连接（使用context）
func (h *HealthCheckManager) fixedCheckConnection(ctx context.Context, conn *Connection) {
	// 跳过正在使用中的连接
	if conn.IsInUse() {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, h.config.HealthCheckTimeout)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- false
			}
		}()
		// 使用context控制健康检查
		healthy := h.performCheckWithContext(checkCtx, conn.GetConn())
		select {
		case done <- healthy:
		case <-checkCtx.Done():
			// Context已取消，goroutine可以安全退出
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
	case <-checkCtx.Done():
		// 健康检查超时，标记为不健康
		conn.UpdateHealth(false)
		if h.pool.statsCollector != nil {
			h.pool.statsCollector.IncrementHealthCheckAttempts()
			h.pool.statsCollector.IncrementHealthCheckFailures()
		}
		// 等待goroutine退出
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			// 给goroutine一点时间退出
		}
	}
}

// performCheckWithContext 带context的健康检查
func (h *HealthCheckManager) performCheckWithContext(ctx context.Context, conn net.Conn) bool {
	// 检查context是否已取消
	select {
	case <-ctx.Done():
		return false
	default:
	}

	// 如果配置了自定义健康检查函数，使用它
	if h.config.HealthChecker != nil {
		return h.config.HealthChecker(conn)
	}

	// 默认健康检查逻辑...
	// （这里省略具体实现，与原代码类似）
	return true
}
