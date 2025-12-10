package netconnpool

import (
	"io"
	"net"
	"time"
)

// ClearUDPReadBuffer 清空UDP连接的读取缓冲区
// 这对于防止UDP连接在连接池中复用时的数据混淆非常重要
func ClearUDPReadBuffer(conn any, timeout time.Duration) error {
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		// 不是UDP连接，无需清理
		return nil
	}

	// 设置读取超时，避免永久阻塞
	readTimeout := timeout
	if readTimeout <= 0 {
		readTimeout = 100 * time.Millisecond
	}
	
	// 设置整体超时时间点
	deadline := time.Now().Add(readTimeout)
	udpConn.SetReadDeadline(deadline)
	defer udpConn.SetReadDeadline(time.Time{}) // 重置超时

	// 持续读取直到缓冲区为空或超时
	buf := make([]byte, 65507) // UDP最大数据包大小
	maxReads := 100            // 最多读取100个数据包，避免无限循环

	for i := 0; i < maxReads; i++ {
		// 检查是否已经超过整体超时时间
		if time.Now().After(deadline) {
			return nil // 超时，缓冲区应该已清空或无法继续清空
		}
		
		// 设置本次读取的超时（使用剩余时间）
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil // 已超时
		}
		// 每次读取使用较短的超时（不超过50ms），避免长时间阻塞
		readDeadline := remaining
		if readDeadline > 50*time.Millisecond {
			readDeadline = 50 * time.Millisecond
		}
		udpConn.SetReadDeadline(time.Now().Add(readDeadline))
		
		_, err := udpConn.Read(buf)
		if err != nil {
			// 如果是超时错误，说明缓冲区已空
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil // 缓冲区已清空
			}
			// EOF或其他错误，连接可能已关闭
			if err == io.EOF {
				return err
			}
			// 其他错误（如连接关闭），返回nil表示清理完成
			return nil
		}
		// 成功读取到一个数据包，继续读取下一个
	}

	return nil
}

// ClearUDPReadBufferNonBlocking 非阻塞方式清空UDP读取缓冲区
// 使用goroutine在后台清理，不阻塞调用者
func ClearUDPReadBufferNonBlocking(conn any, timeout time.Duration) {
	go func() {
		ClearUDPReadBuffer(conn, timeout)
	}()
}

// HasUDPDataInBuffer 检查UDP连接读取缓冲区是否有数据
// 返回true表示可能有数据，false表示缓冲区为空
func HasUDPDataInBuffer(conn any) bool {
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		return false
	}

	// 设置极短的读取超时进行探测
	udpConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	defer udpConn.SetReadDeadline(time.Time{})

	buf := make([]byte, 1)
	_, err := udpConn.Read(buf)
	if err != nil {
		// 超时错误表示缓冲区为空
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return false
		}
		// 其他错误也认为没有数据
		return false
	}

	// 如果能读取到数据，说明缓冲区不为空
	// 注意：这个数据包已经被消耗了，调用者应该考虑这一点
	return true
}
