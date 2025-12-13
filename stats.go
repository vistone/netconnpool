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

// Stats 连接池统计信息
type Stats struct {
	// 连接相关统计
	TotalConnectionsCreated  int64 // 累计创建的连接数
	TotalConnectionsClosed   int64 // 累计关闭的连接数
	CurrentConnections       int64 // 当前连接数
	CurrentIdleConnections   int64 // 当前空闲连接数
	CurrentActiveConnections int64 // 当前活跃连接数

	// IP版本相关统计
	CurrentIPv4Connections     int64 // 当前IPv4连接数
	CurrentIPv6Connections     int64 // 当前IPv6连接数
	CurrentIPv4IdleConnections int64 // 当前IPv4空闲连接数
	CurrentIPv6IdleConnections int64 // 当前IPv6空闲连接数

	// 协议相关统计
	CurrentTCPConnections     int64 // 当前TCP连接数
	CurrentUDPConnections     int64 // 当前UDP连接数
	CurrentTCPIdleConnections int64 // 当前TCP空闲连接数
	CurrentUDPIdleConnections int64 // 当前UDP空闲连接数

	// 请求相关统计
	TotalGetRequests int64 // 累计获取连接请求数
	SuccessfulGets   int64 // 成功获取连接数
	FailedGets       int64 // 失败获取连接数
	TimeoutGets      int64 // 超时获取连接数

	// 健康检查相关统计
	HealthCheckAttempts  int64 // 健康检查尝试次数
	HealthCheckFailures  int64 // 健康检查失败次数
	UnhealthyConnections int64 // 不健康连接数

	// 错误相关统计
	ConnectionErrors  int64 // 连接错误数
	LeakedConnections int64 // 泄漏的连接数

	// 连接复用相关统计
	TotalConnectionsReused int64   // 累计连接复用次数（从空闲池获取的次数）
	AverageReuseCount      float64 // 平均每个连接的复用次数

	// 时间相关统计
	AverageGetTime time.Duration // 平均获取连接时间
	TotalGetTime   time.Duration // 总获取连接时间

	// 最后更新时间
	LastUpdateTime time.Time
}

// StatsCollector 统计收集器
type StatsCollector struct {
	stats Stats
	mu    sync.RWMutex // 保护 LastUpdateTime
}

// NewStatsCollector 创建统计收集器
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		stats: Stats{
			LastUpdateTime: time.Now(),
		},
	}
}

// IncrementTotalConnectionsCreated 增加创建连接计数
func (s *StatsCollector) IncrementTotalConnectionsCreated() {
	atomic.AddInt64(&s.stats.TotalConnectionsCreated, 1)
	atomic.AddInt64(&s.stats.CurrentConnections, 1)
	s.updateTime()
}

// IncrementTotalConnectionsClosed 增加关闭连接计数
func (s *StatsCollector) IncrementTotalConnectionsClosed() {
	atomic.AddInt64(&s.stats.TotalConnectionsClosed, 1)
	atomic.AddInt64(&s.stats.CurrentConnections, -1)
	s.updateTime()
}

// IncrementCurrentIdleConnections 增加空闲连接计数
func (s *StatsCollector) IncrementCurrentIdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentActiveConnections 增加活跃连接计数
func (s *StatsCollector) IncrementCurrentActiveConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentActiveConnections, delta)
	s.updateTime()
}

// IncrementTotalGetRequests 增加获取请求计数
func (s *StatsCollector) IncrementTotalGetRequests() {
	atomic.AddInt64(&s.stats.TotalGetRequests, 1)
	s.updateTime()
}

// IncrementSuccessfulGets 增加成功获取计数
func (s *StatsCollector) IncrementSuccessfulGets() {
	atomic.AddInt64(&s.stats.SuccessfulGets, 1)
	s.updateTime()
}

// IncrementFailedGets 增加失败获取计数
func (s *StatsCollector) IncrementFailedGets() {
	atomic.AddInt64(&s.stats.FailedGets, 1)
	s.updateTime()
}

// IncrementTimeoutGets 增加超时获取计数
func (s *StatsCollector) IncrementTimeoutGets() {
	atomic.AddInt64(&s.stats.TimeoutGets, 1)
	s.updateTime()
}

// IncrementHealthCheckAttempts 增加健康检查尝试计数
func (s *StatsCollector) IncrementHealthCheckAttempts() {
	atomic.AddInt64(&s.stats.HealthCheckAttempts, 1)
	s.updateTime()
}

// IncrementHealthCheckFailures 增加健康检查失败计数
func (s *StatsCollector) IncrementHealthCheckFailures() {
	atomic.AddInt64(&s.stats.HealthCheckFailures, 1)
	s.updateTime()
}

// IncrementUnhealthyConnections 增加不健康连接计数
func (s *StatsCollector) IncrementUnhealthyConnections() {
	atomic.AddInt64(&s.stats.UnhealthyConnections, 1)
	s.updateTime()
}

// IncrementConnectionErrors 增加连接错误计数
func (s *StatsCollector) IncrementConnectionErrors() {
	atomic.AddInt64(&s.stats.ConnectionErrors, 1)
	s.updateTime()
}

// IncrementLeakedConnections 增加泄漏连接计数
func (s *StatsCollector) IncrementLeakedConnections() {
	atomic.AddInt64(&s.stats.LeakedConnections, 1)
	s.updateTime()
}

// RecordGetTime 记录获取连接的时间
func (s *StatsCollector) RecordGetTime(duration time.Duration) {
	atomic.AddInt64((*int64)(&s.stats.TotalGetTime), int64(duration))
	// 计算平均时间
	// 使用重试机制减少竞态条件的影响
	for i := 0; i < 3; i++ {
		totalGets := atomic.LoadInt64(&s.stats.SuccessfulGets)
		if totalGets > 0 {
			totalTime := atomic.LoadInt64((*int64)(&s.stats.TotalGetTime))
			// 再次检查 totalGets，如果变化不大则使用
			totalGets2 := atomic.LoadInt64(&s.stats.SuccessfulGets)
			if totalGets == totalGets2 || totalGets2 == 0 {
				if totalGets2 > 0 {
					avgTime := time.Duration(totalTime / totalGets2)
					atomic.StoreInt64((*int64)(&s.stats.AverageGetTime), int64(avgTime))
				}
				break
			}
			// 如果 totalGets 变化很大，重试
		} else {
			break
		}
	}
	s.updateTime()
}

// GetStats 获取当前统计信息快照
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

// IncrementCurrentIPv4Connections 增加IPv4连接计数
func (s *StatsCollector) IncrementCurrentIPv4Connections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv4Connections, delta)
	s.updateTime()
}

// IncrementCurrentIPv6Connections 增加IPv6连接计数
func (s *StatsCollector) IncrementCurrentIPv6Connections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv6Connections, delta)
	s.updateTime()
}

// IncrementCurrentIPv4IdleConnections 增加IPv4空闲连接计数
func (s *StatsCollector) IncrementCurrentIPv4IdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv4IdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentIPv6IdleConnections 增加IPv6空闲连接计数
func (s *StatsCollector) IncrementCurrentIPv6IdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentIPv6IdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentTCPConnections 增加TCP连接计数
func (s *StatsCollector) IncrementCurrentTCPConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentTCPConnections, delta)
	s.updateTime()
}

// IncrementCurrentUDPConnections 增加UDP连接计数
func (s *StatsCollector) IncrementCurrentUDPConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentUDPConnections, delta)
	s.updateTime()
}

// IncrementCurrentTCPIdleConnections 增加TCP空闲连接计数
func (s *StatsCollector) IncrementCurrentTCPIdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentTCPIdleConnections, delta)
	s.updateTime()
}

// IncrementCurrentUDPIdleConnections 增加UDP空闲连接计数
func (s *StatsCollector) IncrementCurrentUDPIdleConnections(delta int64) {
	atomic.AddInt64(&s.stats.CurrentUDPIdleConnections, delta)
	s.updateTime()
}

// IncrementTotalConnectionsReused 增加连接复用计数
func (s *StatsCollector) IncrementTotalConnectionsReused() {
	atomic.AddInt64(&s.stats.TotalConnectionsReused, 1)
	s.updateTime()
}

// calculateAverageReuseCount 计算平均复用次数
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
	// 减少时间更新频率，每100ms更新一次
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
