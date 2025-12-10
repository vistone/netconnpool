package netconnpool

import (
	"sync"
	"time"
)

// LeakDetector 连接泄漏检测器
type LeakDetector struct {
	pool     *Pool
	config   *Config
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// NewLeakDetector 创建泄漏检测器
func NewLeakDetector(pool *Pool, config *Config) *LeakDetector {
	return &LeakDetector{
		pool:     pool,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start 启动泄漏检测
func (l *LeakDetector) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running || l.config.ConnectionLeakTimeout <= 0 {
		return
	}

	l.running = true
	// 每分钟检查一次
	l.ticker = time.NewTicker(1 * time.Minute)

	l.wg.Add(1)
	go l.detectionLoop()
}

// Stop 停止泄漏检测
func (l *LeakDetector) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return
	}

	l.running = false
	if l.ticker != nil {
		l.ticker.Stop()
	}
	close(l.stopChan)
	l.wg.Wait()
}

// detectionLoop 检测循环
func (l *LeakDetector) detectionLoop() {
	defer l.wg.Done()

	for {
		select {
		case <-l.stopChan:
			return
		case <-l.ticker.C:
			l.detectLeaks()
		}
	}
}

// detectLeaks 检测泄漏
func (l *LeakDetector) detectLeaks() {
	connections := l.pool.getAllConnections()

	for _, conn := range connections {
		if !conn.InUse {
			continue
		}

		// 检查是否泄漏
		if conn.IsLeaked(l.config.ConnectionLeakTimeout) {
			conn.LeakDetected = true

			if l.pool.statsCollector != nil {
				l.pool.statsCollector.IncrementLeakedConnections()
			}

			// 注意：这里只记录泄漏，不自动关闭连接
			// 因为连接可能正在被正常使用，只是时间较长
			// 实际应用中可以考虑记录日志或触发告警
		}
	}
}
