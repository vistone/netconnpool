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
	"sync/atomic"
	"time"
)

// Stats connection pool statistics
type Stats struct {
	// Connection related statistics
	TotalConnectionsCreated  int64 // total connections created
	TotalConnectionsClosed   int64 // total connections closed
	CurrentConnections       int64 // current connection count
	CurrentIdleConnections   int64 // current idle connection count
	CurrentActiveConnections int64 // current active connection count

	// IP version related statistics
	CurrentIPv4Connections     int64 // current IPv4 connection count
	CurrentIPv6Connections     int64 // current IPv6 connection count
	CurrentIPv4IdleConnections int64 // current IPv4 idle connection count
	CurrentIPv6IdleConnections int64 // current IPv6 idle connection count

	// Protocol related statistics
	CurrentTCPConnections     int64 // current TCP connection count
	CurrentUDPConnections     int64 // current UDP connection count
	CurrentTCPIdleConnections int64 // current TCP idle connection count
	CurrentUDPIdleConnections int64 // current UDP idle connection count

	// Request related statistics
	TotalGetRequests int64 // total get connection requests
	SuccessfulGets   int64 // successful get connections
	FailedGets       int64 // failed get connections
	TimeoutGets      int64 // timeout get connections

	// Health check related statistics
	HealthCheckAttempts  int64 // health check attempts
	HealthCheckFailures  int64 // health check failures
	UnhealthyConnections int64 // unhealthy connection count

	// Error related statistics
	ConnectionErrors  int64 // connection errors
	LeakedConnections int64 // leaked connections

	// Connection reuse related statistics
	TotalConnectionsReused int64   // total connection reuse count (times obtained from idle pool)
	AverageReuseCount      float64 // average reuse count per connection

	// Time related statistics
	AverageGetTime time.Duration // average get connection time
	TotalGetTime   time.Duration // total get connection time

	// Last update time
	LastUpdateTime time.Time
}

// StatsCollector statistics collector
type StatsCollector struct {
	stats Stats
	mu    sync.RWMutex // protects LastUpdateTime
}

// NewStatsCollector creates a statistics collector
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		stats: Stats{
			LastUpdateTime: time.Now(),
		},
	}
}

// IncrementTotalConnectionsCreated increments created connection count
func (s *StatsCollector) IncrementTotalConnectionsCreated() {
	atomic.AddInt64(&s.stats.TotalConnectionsCreated, 1)
	atomic.AddInt64(&s.stats.CurrentConnections, 1)
	s.updateTime()
}

// IncrementTotalConnectionsClosed increments closed connection count
func (s *StatsCollector) IncrementTotalConnectionsClosed() {
	atomic.AddInt64(&s.stats.TotalConnectionsClosed, 1)
	atomic.AddInt64(&s.stats.CurrentConnections, -1)
	s.updateTime()
}

// IncrementCurrentIdleConnections increments idle connection count
func (s *StatsCollector) IncrementCurrentIdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentActiveConnections increments active connection count
func (s *StatsCollector) IncrementCurrentActiveConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentActiveConnections, delta)
	s.updateTime()
}

// IncrementTotalGetRequests increments get request count
func (s *StatsCollector) IncrementTotalGetRequests() {
	atomic.AddInt64(&s.stats.TotalGetRequests, 1)
	s.updateTime()
}

// IncrementSuccessfulGets increments successful get count
func (s *StatsCollector) IncrementSuccessfulGets() {
	atomic.AddInt64(&s.stats.SuccessfulGets, 1)
	s.updateTime()
}

// IncrementFailedGets increments failed get count
func (s *StatsCollector) IncrementFailedGets() {
	atomic.AddInt64(&s.stats.FailedGets, 1)
	s.updateTime()
}

// IncrementTimeoutGets increments timeout get count
func (s *StatsCollector) IncrementTimeoutGets() {
	atomic.AddInt64(&s.stats.TimeoutGets, 1)
	s.updateTime()
}

// IncrementHealthCheckAttempts increments health check attempt count
func (s *StatsCollector) IncrementHealthCheckAttempts() {
	atomic.AddInt64(&s.stats.HealthCheckAttempts, 1)
	s.updateTime()
}

// IncrementHealthCheckFailures increments health check failure count
func (s *StatsCollector) IncrementHealthCheckFailures() {
	atomic.AddInt64(&s.stats.HealthCheckFailures, 1)
	s.updateTime()
}

// IncrementUnhealthyConnections increments unhealthy connection count
func (s *StatsCollector) IncrementUnhealthyConnections() {
	atomic.AddInt64(&s.stats.UnhealthyConnections, 1)
	s.updateTime()
}

// IncrementConnectionErrors increments connection error count
func (s *StatsCollector) IncrementConnectionErrors() {
	atomic.AddInt64(&s.stats.ConnectionErrors, 1)
	s.updateTime()
}

// IncrementLeakedConnections increments leaked connection count
func (s *StatsCollector) IncrementLeakedConnections() {
	atomic.AddInt64(&s.stats.LeakedConnections, 1)
	s.updateTime()
}

// RecordGetTime records get connection time
func (s *StatsCollector) RecordGetTime(duration time.Duration) {
	atomic.AddInt64((*int64)(&s.stats.TotalGetTime), int64(duration))
	// Calculate average time
	// Use retry mechanism to reduce race condition impact
	for i := 0; i < 3; i++ {
		totalGets := atomic.LoadInt64(&s.stats.SuccessfulGets)
		if totalGets > 0 {
			totalTime := atomic.LoadInt64((*int64)(&s.stats.TotalGetTime))
			// Check totalGets again, if not much changed then use it
			totalGets2 := atomic.LoadInt64(&s.stats.SuccessfulGets)
			if totalGets == totalGets2 || totalGets2 == 0 {
				if totalGets2 > 0 {
					avgTime := time.Duration(totalTime / totalGets2)
					atomic.StoreInt64((*int64)(&s.stats.AverageGetTime), int64(avgTime))
				}
				break
			}
			// If totalGets changed significantly, retry
		} else {
			break
		}
	}
	s.updateTime()
}

// GetStats gets current statistics snapshot
func (s *StatsCollector) GetStats() Stats {
	return Stats{
		TotalConnectionsCreated:    atomic.LoadInt64(&s.stats.TotalConnectionsCreated),
		TotalConnectionsClosed:     atomic.LoadInt64(&s.stats.TotalConnectionsClosed),
		CurrentConnections:         atomic.LoadInt64(&s.stats.CurrentConnections),
		CurrentIdleConnections:     atomic.LoadInt64(&s.stats.CurrentIdleConnections),
		CurrentActiveConnections:   atomic.LoadInt64(&s.stats.CurrentActiveConnections),
		CurrentIPv4Connections:     atomic.LoadInt64(&s.stats.CurrentIPv4Connections),
		CurrentIPv6Connections:     atomic.LoadInt64(&s.stats.CurrentIPv6Connections),
		CurrentIPv4IdleConnections: atomic.LoadInt64(&s.stats.CurrentIPv4IdleConnections),
		CurrentIPv6IdleConnections: atomic.LoadInt64(&s.stats.CurrentIPv6IdleConnections),
		CurrentTCPConnections:      atomic.LoadInt64(&s.stats.CurrentTCPConnections),
		CurrentUDPConnections:      atomic.LoadInt64(&s.stats.CurrentUDPConnections),
		CurrentTCPIdleConnections:  atomic.LoadInt64(&s.stats.CurrentTCPIdleConnections),
		CurrentUDPIdleConnections:  atomic.LoadInt64(&s.stats.CurrentUDPIdleConnections),
		TotalGetRequests:           atomic.LoadInt64(&s.stats.TotalGetRequests),
		SuccessfulGets:             atomic.LoadInt64(&s.stats.SuccessfulGets),
		FailedGets:                 atomic.LoadInt64(&s.stats.FailedGets),
		TimeoutGets:                atomic.LoadInt64(&s.stats.TimeoutGets),
		HealthCheckAttempts:        atomic.LoadInt64(&s.stats.HealthCheckAttempts),
		HealthCheckFailures:        atomic.LoadInt64(&s.stats.HealthCheckFailures),
		UnhealthyConnections:       atomic.LoadInt64(&s.stats.UnhealthyConnections),
		ConnectionErrors:           atomic.LoadInt64(&s.stats.ConnectionErrors),
		LeakedConnections:          atomic.LoadInt64(&s.stats.LeakedConnections),
		TotalConnectionsReused:     atomic.LoadInt64(&s.stats.TotalConnectionsReused),
		AverageReuseCount:          s.calculateAverageReuseCount(),
		AverageGetTime:             time.Duration(atomic.LoadInt64((*int64)(&s.stats.AverageGetTime))),
		TotalGetTime:               time.Duration(atomic.LoadInt64((*int64)(&s.stats.TotalGetTime))),
		LastUpdateTime:             s.getLastUpdateTime(),
	}
}

// IncrementCurrentIPv4Connections increments IPv4 connection count
func (s *StatsCollector) IncrementCurrentIPv4Connections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv4Connections, delta)
	s.updateTime()
}

// IncrementCurrentIPv6Connections increments IPv6 connection count
func (s *StatsCollector) IncrementCurrentIPv6Connections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv6Connections, delta)
	s.updateTime()
}

// IncrementCurrentIPv4IdleConnections increments IPv4 idle connection count
func (s *StatsCollector) IncrementCurrentIPv4IdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv4IdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentIPv6IdleConnections increments IPv6 idle connection count
func (s *StatsCollector) IncrementCurrentIPv6IdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv6IdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentTCPConnections increments TCP connection count
func (s *StatsCollector) IncrementCurrentTCPConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentTCPConnections, delta)
	s.updateTime()
}

// IncrementCurrentUDPConnections increments UDP connection count
func (s *StatsCollector) IncrementCurrentUDPConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentUDPConnections, delta)
	s.updateTime()
}

// IncrementCurrentTCPIdleConnections increments TCP idle connection count
func (s *StatsCollector) IncrementCurrentTCPIdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentTCPIdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentUDPIdleConnections increments UDP idle connection count
func (s *StatsCollector) IncrementCurrentUDPIdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentUDPIdleConnections, delta)
	s.updateTime()
}

// IncrementTotalConnectionsReused increments connection reuse count
func (s *StatsCollector) IncrementTotalConnectionsReused() {
	atomic.AddInt64(&s.stats.TotalConnectionsReused, 1)
	s.updateTime()
}

// calculateAverageReuseCount calculates average reuse count
func (s *StatsCollector) calculateAverageReuseCount() float64 {
	totalCreated := atomic.LoadInt64(&s.stats.TotalConnectionsCreated)
	if totalCreated == 0 {
		return 0
	}
	totalReused := atomic.LoadInt64(&s.stats.TotalConnectionsReused)
	return float64(totalReused) / float64(totalCreated)
}

func (s *StatsCollector) updateTime() {
	s.mu.RLock()
	oldTime := s.stats.LastUpdateTime
	s.mu.RUnlock()

	now := time.Now()
	// Reduce time update frequency, update every 100ms
	if now.Sub(oldTime) < 100*time.Millisecond {
		return
	}

	s.mu.Lock()
	s.stats.LastUpdateTime = now
	s.mu.Unlock()
}

func (s *StatsCollector) getLastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats.LastUpdateTime
}
