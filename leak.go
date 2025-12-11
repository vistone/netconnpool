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
		// 使用线程安全的方法检查连接是否在使用中
		if !conn.IsInUse() {
			continue
		}

		// 检查是否泄漏
		if conn.IsLeaked(l.config.ConnectionLeakTimeout) {
			conn.mu.Lock()
			conn.LeakDetected = true
			conn.mu.Unlock()

			if l.pool.statsCollector != nil {
				l.pool.statsCollector.IncrementLeakedConnections()
			}

			// 注意：这里只记录泄漏，不自动关闭连接
			// 因为连接可能正在被正常使用，只是时间较长
			// 实际应用中可以考虑记录日志或触发告警
		}
	}
}
