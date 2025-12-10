package netconnpool

import (
	"sync"
	"time"
)

// CleanupManager 清理管理器
type CleanupManager struct {
	pool     *Pool
	config   *Config
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// NewCleanupManager 创建清理管理器
func NewCleanupManager(pool *Pool, config *Config) *CleanupManager {
	return &CleanupManager{
		pool:     pool,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start 启动清理管理器
func (c *CleanupManager) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return
	}

	c.running = true
	// 每30秒清理一次
	c.ticker = time.NewTicker(30 * time.Second)

	c.wg.Add(1)
	go c.cleanupLoop()
}

// Stop 停止清理管理器
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

// cleanupLoop 清理循环
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

// performCleanup 执行清理
func (c *CleanupManager) performCleanup() {
	connections := c.pool.getAllConnections()

	for _, conn := range connections {
		// 跳过正在使用中的连接
		if conn.InUse {
			continue
		}

		shouldClose := false

		// 检查是否过期
		if conn.IsExpired(c.config.MaxLifetime) {
			shouldClose = true
		}

		// 检查是否空闲太久
		if !shouldClose && conn.IsIdleTooLong(c.config.IdleTimeout) {
			shouldClose = true
		}

		// 检查是否不健康
		if !shouldClose && c.config.EnableHealthCheck && !conn.IsHealthy {
			shouldClose = true
		}

		if shouldClose {
			// 尝试从空闲连接池中移除
			removed := c.removeFromIdlePool(conn)
			if removed || !conn.InUse {
				c.pool.closeConnection(conn)
			}
		}
	}

	// 确保空闲连接数不超过MaxIdleConnections
	c.enforceMaxIdleConnections()
}

// removeFromIdlePool 从空闲连接池中移除连接
// 注意：由于通道的特性，我们无法直接移除通道中的特定连接
// 这个方法标记连接无效，当从通道取出时会被验证并关闭
// 返回false表示连接可能在通道中，需要在取出时验证
func (c *CleanupManager) removeFromIdlePool(conn *Connection) bool {
	// 标记连接为不健康，这样在Get时会被验证并关闭
	conn.UpdateHealth(false)
	// 由于无法直接从通道中移除，返回false
	// 连接会在下次从通道取出时因验证失败而被关闭
	return false
}

// enforceMaxIdleConnections 强制限制空闲连接数
func (c *CleanupManager) enforceMaxIdleConnections() {
	if c.pool.statsCollector == nil {
		return
	}
	
	idleCount := int(c.pool.statsCollector.GetStats().CurrentIdleConnections)
	if idleCount <= c.config.MaxIdleConnections {
		return
	}

	// 计算需要关闭的连接数
	excess := idleCount - c.config.MaxIdleConnections

	connections := c.pool.getAllConnections()
	
	// 找出空闲时间最长的连接，优先关闭
	idleConns := make([]*Connection, 0)
	for _, conn := range connections {
		conn.mu.RLock()
		if !conn.InUse {
			idleConns = append(idleConns, conn)
		}
		conn.mu.RUnlock()
	}
	
	// 简单选择排序：找到空闲时间最长的连接
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
		// 移除已选中的连接
		idleConns = append(idleConns[:maxIdx], idleConns[maxIdx+1:]...)
	}
	
	// 标记为不健康并关闭
	for _, conn := range toClose {
		conn.UpdateHealth(false)
		// 尝试关闭连接（可能在通道中，会在下次取出时验证失败而被关闭）
		c.pool.closeConnection(conn)
	}
}

