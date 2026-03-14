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

// Pool is a network connection pool
type Pool struct {
	config      *Config
	connections map[uint64]*Connection // all connections (O(1) lookup using map)

	// Separate idle connection channels to avoid protocol confusion and performance jitter
	idleTCPConnections chan *Connection
	idleUDPConnections chan *Connection

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	closeChan chan struct{}

	// Managers
	healthCheckManager *HealthCheckManager
	cleanupManager     *CleanupManager
	leakDetector       *LeakDetector

	// Statistics
	statsCollector *StatsCollector

	// Wait queue (when pool is exhausted)
	waitQueue chan chan *Connection

	// IP version specific wait queues
	waitQueueIPv4 chan chan *Connection
	waitQueueIPv6 chan chan *Connection

	// Protocol specific wait queues
	waitQueueTCP chan chan *Connection
	waitQueueUDP chan chan *Connection

	// Synchronization primitives
	createSemaphore chan struct{} // limits concurrent connection creation
}

// NewPool creates a new connection pool
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
		waitQueue:          make(chan chan *Connection, 100), // wait queue buffer
		waitQueueIPv4:      make(chan chan *Connection, 100), // IPv4 wait queue
		waitQueueIPv6:      make(chan chan *Connection, 100), // IPv6 wait queue
		waitQueueTCP:       make(chan chan *Connection, 100), // TCP wait queue
		waitQueueUDP:       make(chan chan *Connection, 100), // UDP wait queue
		createSemaphore:    make(chan struct{}, 5),           // max 5 concurrent connection creations
	}

	// Initialize stats collector
	if config.EnableStats {
		pool.statsCollector = NewStatsCollector()
	}

	// Initialize health check manager
	pool.healthCheckManager = NewHealthCheckManager(pool, config)

	// Initialize cleanup manager
	pool.cleanupManager = NewCleanupManager(pool, config)

	// Initialize leak detector
	pool.leakDetector = NewLeakDetector(pool, config)

	// Warm up the connection pool
	if err := pool.warmUp(); err != nil {
		return nil, err
	}

	// Start background tasks
	pool.startBackgroundTasks()

	return pool, nil
}

// Get gets a connection (automatically selects IP version)
func (p *Pool) Get(ctx context.Context) (*Connection, error) {
	return p.GetWithTimeout(ctx, p.config.GetConnectionTimeout)
}

// GetIPv4 gets an IPv4 connection
func (p *Pool) GetIPv4(ctx context.Context) (*Connection, error) {
	return p.GetWithIPVersion(ctx, IPVersionIPv4, p.config.GetConnectionTimeout)
}

// GetIPv6 gets an IPv6 connection
func (p *Pool) GetIPv6(ctx context.Context) (*Connection, error) {
	return p.GetWithIPVersion(ctx, IPVersionIPv6, p.config.GetConnectionTimeout)
}

// GetTCP gets a TCP connection
func (p *Pool) GetTCP(ctx context.Context) (*Connection, error) {
	return p.GetWithProtocol(ctx, ProtocolTCP, p.config.GetConnectionTimeout)
}

// GetUDP gets a UDP connection
func (p *Pool) GetUDP(ctx context.Context) (*Connection, error) {
	return p.GetWithProtocol(ctx, ProtocolUDP, p.config.GetConnectionTimeout)
}

// GetWithProtocol gets a connection with specified protocol
func (p *Pool) GetWithProtocol(ctx context.Context, protocol Protocol, timeout time.Duration) (*Connection, error) {
	if protocol == ProtocolUnknown {
		// Fall back to default behavior if unknown protocol is specified
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

	// Create context with timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Determine which idle channel to use
	var idleChan chan *Connection
	if protocol == ProtocolTCP {
		idleChan = p.idleTCPConnections
	} else {
		idleChan = p.idleUDPConnections
	}

	// Try to get a connection of the specified protocol
	maxAttempts := 10          // max 10 attempts
	protocolMismatchCount := 0 // track protocol mismatch count
	createFailedCount := 0     // track connection creation failure count

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Check if pool is closed
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// Try to get a connection of the specified protocol from idle pool
		select {
		case conn := <-idleChan:
			if conn == nil {
				continue
			}

			// Check if connection is valid
			if !p.isConnectionValid(conn) {
				p.closeConnection(conn)
				continue
			}

			// Check if protocol matches
			if conn.GetProtocol() != protocol {
				// Protocol mismatch, return to correct pool and continue
				p.Put(conn)
				continue
			}

			// Mark as in use and record connection reuse
			conn.MarkInUse()
			conn.IncrementReuseCount() // increment reuse count

			// Call borrow hook
			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
				p.statsCollector.IncrementCurrentIdleConnections(-1)
				p.statsCollector.IncrementTotalConnectionsReused() // record connection reuse
				// Update protocol idle connection statistics
				switch conn.GetProtocol() {
				case ProtocolTCP:
					p.statsCollector.IncrementCurrentTCPIdleConnections(-1)
				case ProtocolUDP:
					p.statsCollector.IncrementCurrentUDPIdleConnections(-1)
				}
			}

			return conn, nil
		default:
			// Idle pool is empty, try to create new connection
		}

		// Check if max connections reached
		if p.config.MaxConnections > 0 {
			current := p.getCurrentConnectionsCount()
			if current >= p.config.MaxConnections {
				// Wait for available connection or timeout
				return p.waitForConnectionWithProtocol(ctx, protocol)
			}
		}

		// Create new connection
		conn, err := p.createConnection(ctx)
		if err != nil {
			createFailedCount++
			// Return error if connection creation fails multiple times to avoid infinite loop
			if createFailedCount >= 3 {
				if p.statsCollector != nil {
					p.statsCollector.IncrementFailedGets()
					p.statsCollector.IncrementConnectionErrors()
				}
				return nil, err
			}
			// Retry after short delay
			select {
			case <-time.After(10 * time.Millisecond):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Connection created successfully but returned nil (shouldn't happen), add protection
		if conn == nil {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
			}
			return nil, ErrInvalidConnection
		}

		// Check if protocol matches (theoretically createConnection should return correct protocol, but for safety)
		if conn.GetProtocol() == protocol {
			conn.MarkInUse()

			// Call borrow hook
			if p.config.OnBorrow != nil {
				p.config.OnBorrow(conn.GetConn())
			}

			if p.statsCollector != nil {
				p.statsCollector.IncrementSuccessfulGets()
				p.statsCollector.IncrementCurrentActiveConnections(1)
			}

			return conn, nil
		}

		// Protocol mismatch, return connection
		protocolMismatchCount++
		p.Put(conn)

		// Return error if multiple consecutive mismatches to avoid infinite loop
		if protocolMismatchCount >= 3 {
			if p.statsCollector != nil {
				p.statsCollector.IncrementFailedGets()
			}
			return nil, ErrNoConnectionForProtocol
		}
		// Continue loop attempt
	}

	if p.statsCollector != nil {
		p.statsCollector.IncrementFailedGets()
	}
	return nil, ErrNoConnectionForProtocol
}

// GetWithIPVersion gets a connection with specified IP version
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
	ipVersionMismatchCount := 0 // track IP version mismatch count
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if p.isClosed() {
			return nil, ErrPoolClosed
		}

		// Try to get from both idle pools
		var conn *Connection
		select {
		case conn = <-p.idleTCPConnections:
		case conn = <-p.idleUDPConnections:
		default:
			// No idle connections available
		}

		if conn != nil {
			// Check if IP version matches
			if conn.GetIPVersion() != ipVersion {
				p.Put(conn)
				ipVersionMismatchCount++
				// Return error if multiple consecutive mismatches to avoid infinite loop
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

		// Try to create
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
			// Shouldn't happen, but add protection to prevent infinite loop
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

		// IP version mismatch, return connection
		ipVersionMismatchCount++
		p.Put(conn)

		// Return error if multiple consecutive mismatches to avoid infinite loop
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

// GetWithTimeout gets a connection (with timeout, automatically selects IP version)
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

		// Try to get from any idle pool
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
			// Shouldn't happen, but add protection to prevent infinite loop
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

// Put returns a connection
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

	// Call return hook
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

	// Determine which channel to return to
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

// Close closes the connection pool
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

		// Clean up wait queues
		closeWaitQueue := func(q chan chan *Connection) {
			defer func() {
				// Recover from possible panic (e.g., closing closed channel)
				// This is expected behavior because channel may have been closed
				if r := recover(); r != nil {
					// Expected panic: closing closed channel
					// Don't rethrow, ensure other resources can be cleaned up
					_ = r
				}
			}()
			for {
				select {
				case waitChan := <-q:
					// Safely close waitChan
					func() {
						defer func() {
							// Expected panic: sending to closed channel or closing closed channel
							if r := recover(); r != nil {
								_ = r
							}
						}()
						close(waitChan)
					}()
				default:
					// Try to close queue channel
					func() {
						defer func() {
							if r := recover(); r != nil {
								// Expected panic: closing closed channel
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

		// Clean up idle connection channels
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

// Stats gets statistics
func (p *Pool) Stats() Stats {
	if p.statsCollector == nil {
		return Stats{}
	}
	return p.statsCollector.GetStats()
}

// createConnection creates a new connection
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

	// Call creation hook
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

// createCloseFunc creates close function
func (p *Pool) createCloseFunc(conn net.Conn) func() error {
	return func() error {
		if p.config.CloseConn != nil {
			return p.config.CloseConn(conn)
		}
		return conn.Close()
	}
}

// closeConnection closes a connection
func (p *Pool) closeConnection(conn *Connection) error {
	if conn == nil {
		return nil
	}

	// Mark connection as unhealthy first to prevent reuse
	conn.UpdateHealth(false)

	// Delete from map - delete before closing connection to avoid race condition
	p.mu.Lock()
	_, exists := p.connections[conn.ID]
	if exists {
		delete(p.connections, conn.ID)
	}
	p.mu.Unlock()

	// Then close connection to ensure it's unavailable
	// Note: close connection after deleting from map to prevent other goroutines from getting it during closing
	err := conn.Close()

	// Update statistics
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

// isConnectionValid checks if connection is valid
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

// waitForConnection waits for an available connection
func (p *Pool) waitForConnection(ctx context.Context) (*Connection, error) {
	waitChan := make(chan *Connection, 1)
	defer close(waitChan)

	select {
	case p.waitQueue <- waitChan:
		// Successfully joined wait queue, wait for connection
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
				// Connection invalid, close it and continue waiting for next connection
				// Note: don't return error here, continue waiting as other connections may be returned
				if conn != nil {
					p.closeConnection(conn)
				}
				continue
			case <-ctx.Done():
				// Check if there's data in waitChan to avoid blocking notifier
				select {
				case <-waitChan:
				default:
				}
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				// Check if there's data in waitChan to avoid blocking notifier
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

// waitForConnectionWithProtocol waits for an available connection with specified protocol
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
		// Successfully joined wait queue, wait for connection
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
				// Connection invalid or protocol mismatch, close it and continue waiting
				if conn != nil {
					p.closeConnection(conn)
				}
				continue
			case <-ctx.Done():
				// Check if there's data in waitChan to avoid blocking notifier
				select {
				case <-waitChan:
				default:
				}
				if p.statsCollector != nil {
					p.statsCollector.IncrementTimeoutGets()
				}
				return nil, ErrGetConnectionTimeout
			case <-p.closeChan:
				// Check if there's data in waitChan to avoid blocking notifier
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

// notifyWaitQueue notifies wait queue
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

// notifySpecificWaitQueue notifies a specific wait queue
func (p *Pool) notifySpecificWaitQueue(waitQueue chan chan *Connection, conn *Connection) bool {
	for {
		select {
		case waitChan := <-waitQueue:
			// Use recover to prevent panic when sending to closed channel
			sent := func() (success bool) {
				defer func() {
					if r := recover(); r != nil {
						// Channel closed, ignore this waiter
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
			// Send failed, try next waiter
		default:
			// No waiting requests
			return false
		}
	}
}

// waitForConnectionWithIPVersion waits for an available connection with specified IP version
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
		// Successfully joined wait queue, wait for connection
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
				// Connection invalid or IP version mismatch, close it and continue waiting
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

// warmUp warms up the connection pool
func (p *Pool) warmUp() error {
	if p.config.Mode == PoolModeServer {
		return nil
	}

	if p.config.MinConnections <= 0 {
		return nil
	}

	// Prevent timeout overflow, set max timeout to 10 minutes
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
			// Warm-up failed, update statistics
			if p.statsCollector != nil {
				p.statsCollector.IncrementConnectionErrors()
			}
			continue
		}

		// Put connection into idle pool
		if err := p.Put(conn); err != nil {
			// Put failed (possibly pool is full), connection will be closed
			// Note: Put method calls closeConnection on failure,
			// but no extra action needed here to ensure resource release
			// because Put already handles the close logic internally
		} else {
			successCount++
		}
	}

	// Consider returning error if all warm-ups failed (but not returning for compatibility)
	// Statistics have recorded errors, caller can check warm-up status via statistics
	return nil
}

// startBackgroundTasks starts background tasks
func (p *Pool) startBackgroundTasks() {
	if p.config.EnableHealthCheck {
		p.healthCheckManager.Start()
	}
	p.cleanupManager.Start()
	p.leakDetector.Start()
}

// isClosed checks if pool is closed
func (p *Pool) isClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// getCurrentConnectionsCount gets current connection count
func (p *Pool) getCurrentConnectionsCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections)
}

// getAllConnections gets all connections (for health check and cleanup)
func (p *Pool) getAllConnections() []*Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	connections := make([]*Connection, 0, len(p.connections))
	for _, conn := range p.connections {
		connections = append(connections, conn)
	}
	return connections
}
