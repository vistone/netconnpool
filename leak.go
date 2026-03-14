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

// LeakDetector connection leak detector
type LeakDetector struct {
	pool     *Pool
	config   *Config
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// NewLeakDetector creates leak detector
func NewLeakDetector(pool *Pool, config *Config) *LeakDetector {
	return &LeakDetector{
		pool:     pool,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start starts leak detection
func (l *LeakDetector) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running || l.config.ConnectionLeakTimeout <= 0 {
		return
	}

	l.running = true
	// Use configured detection interval, if not set default to 1 minute
	interval := l.config.LeakDetectionInterval
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	l.ticker = time.NewTicker(interval)

	l.wg.Add(1)
	go l.detectionLoop()
}

// Stop stops leak detection
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

// detectionLoop detection loop
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

// detectLeaks detects leaks
func (l *LeakDetector) detectLeaks() {
	connections := l.pool.getAllConnections()

	for _, conn := range connections {
		// Use thread-safe method to check if connection is in use
		if !conn.IsInUse() {
			continue
		}

		// Check if leaked
		if conn.IsLeaked(l.config.ConnectionLeakTimeout) {
			conn.mu.Lock()
			conn.LeakDetected = true
			conn.mu.Unlock()

			if l.pool.statsCollector != nil {
				l.pool.statsCollector.IncrementLeakedConnections()
			}

			// Note: Only record leak here, do not automatically close connection
			// Because connection may be in normal use, just for a longer time
			// In actual applications, consider logging or triggering alerts
		}
	}
}
