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

// HealthCheckManager 健康检查管理器
type HealthCheckManager struct {
	pool     *Pool
	config   *Config
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// NewHealthCheckManager 创建健康检查管理器
func NewHealthCheckManager(pool *Pool, config *Config) *HealthCheckManager {
	return &HealthCheckManager{
		pool:     pool,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start 启动健康检查
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

// Stop 停止健康检查
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

// healthCheckLoop 健康检查循环
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

// performHealthCheck 执行健康检查
func (h *HealthCheckManager) performHealthCheck() {
	connections := h.pool.getAllConnections()

	// 并发执行健康检查，但限制并发数
	const maxConcurrentChecks = 10
	semaphore := make(chan struct{}, maxConcurrentChecks)
	var wg sync.WaitGroup

	for _, conn := range connections {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()
			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			h.checkConnection(c)
		}(conn)
	}

	wg.Wait()
}

// checkConnection 检查单个连接
func (h *HealthCheckManager) checkConnection(conn *Connection) {
	// 跳过正在使用中的连接
	conn.mu.RLock()
	inUse := conn.InUse
	conn.mu.RUnlock()

	if inUse {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.config.HealthCheckTimeout)
	defer cancel()

	done := make(chan bool, 1)
	// 使用带缓冲的channel，确保goroutine不会永远阻塞
	go func() {
		defer func() {
			// 防止panic
			if r := recover(); r != nil {
				select {
				case done <- false:
				case <-ctx.Done():
					// Context已取消，goroutine可以安全退出
				}
			}
		}()
		// 检查context是否已取消
		select {
		case <-ctx.Done():
			// Context已取消，goroutine可以安全退出
			return
		default:
		}
		healthy := h.performCheck(conn.GetConn())
		select {
		case done <- healthy:
		case <-ctx.Done():
			// 上下文已取消，goroutine可以安全退出
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
	case <-ctx.Done():
		// 健康检查超时，标记为不健康
		conn.UpdateHealth(false)
		if h.pool.statsCollector != nil {
			h.pool.statsCollector.IncrementHealthCheckAttempts()
			h.pool.statsCollector.IncrementHealthCheckFailures()
		}
		// 等待goroutine退出或超时
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			// 给goroutine一点时间退出
		}
	}
}

// performCheck 执行实际的健康检查
func (h *HealthCheckManager) performCheck(conn net.Conn) bool {
	// 如果配置了自定义健康检查函数，使用它
	if h.config.HealthChecker != nil {
		return h.config.HealthChecker(conn)
	}

	// 检查是否是UDP连接
	if udpConn, ok := conn.(*net.UDPConn); ok {
		// UDP连接的特殊健康检查
		// UDP是无连接的，不能像TCP那样读取数据来判断健康状态
		// 我们通过检查连接是否已关闭来判断

		// 设置极短的读取超时进行探测
		udpConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
		defer udpConn.SetReadDeadline(time.Time{})

		// 尝试读取一个字节，如果能读到说明连接正常（即使没有数据也会超时）
		buf := make([]byte, 1)
		_, err := udpConn.Read(buf)
		if err != nil {
			// 超时错误是正常的，说明连接还活着（只是没有数据）
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return true
			}
			// EOF错误表示连接已关闭
			if err == io.EOF {
				return false
			}
			// 对于UDP，其他错误（如连接关闭、网络错误等）应该认为不健康
			// 只有超时错误才表示连接正常（只是没有数据）
			return false
		}
		// 如果能读到数据，说明连接正常
		return true
	}

	// 默认健康检查：通过带超时的读取探测连接状态
	// 超时错误表示连接存活（无数据可读但连接正常）
	// EOF表示对端关闭了连接
	// 注意：如果连接有缓冲数据，Read会消耗一个字节
	return h.readProbeCheck(conn)
}

// readProbeCheck 通过带超时的读取探测连接健康状态
func (h *HealthCheckManager) readProbeCheck(conn net.Conn) bool {
	conn.SetReadDeadline(time.Now().Add(h.config.HealthCheckTimeout))
	one := make([]byte, 1)
	_, err := conn.Read(one)
	conn.SetReadDeadline(time.Time{})
	if err == io.EOF {
		return false
	}
	// 超时表示连接存活但没有数据，属于正常
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	// 其他错误（非超时、非EOF），连接可能已断开
	if err != nil {
		return false
	}
	return true
}
