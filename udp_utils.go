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
	"io"
	"net"
	"time"
)

var (
	// udpCleanupSemaphore limits concurrent UDP cleanup to prevent goroutine leak
	udpCleanupSemaphore = make(chan struct{}, 10)
)

// ClearUDPReadBuffer clears UDP connection read buffer
// This is very important to prevent data confusion when UDP connections are reused in the pool
func ClearUDPReadBuffer(conn net.Conn, timeout time.Duration, maxPackets int) error {
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		// Not a UDP connection, no need to clear
		return nil
	}

	// Set read timeout to avoid blocking forever
	readTimeout := timeout
	if readTimeout <= 0 {
		readTimeout = 100 * time.Millisecond
	}

	// Set overall timeout deadline
	deadline := time.Now().Add(readTimeout)
	udpConn.SetReadDeadline(deadline)
	defer udpConn.SetReadDeadline(time.Time{}) // Reset timeout

	// Continuously read until buffer is empty or timeout
	buf := make([]byte, 65507) // UDP max packet size
	if maxPackets <= 0 {
		maxPackets = 100 // Default max 100 packets
	}

	for i := 0; i < maxPackets; i++ {
		// Check if overall timeout has been exceeded
		if time.Now().After(deadline) {
			return nil // Timeout, buffer should be cleared or unable to continue clearing
		}

		// Set timeout for this read (using remaining time)
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil // Timeout
		}
		// Use short timeout for each read (max 50ms) to avoid long blocking
		readDeadline := remaining
		if readDeadline > 50*time.Millisecond {
			readDeadline = 50 * time.Millisecond
		}
		udpConn.SetReadDeadline(time.Now().Add(readDeadline))

		_, err := udpConn.Read(buf)
		if err != nil {
			// If timeout error, buffer is empty
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil // Buffer cleared
			}
			// EOF or other error, connection may be closed
			if err == io.EOF {
				return err
			}
			// Other errors (like connection closed), return nil indicating cleanup complete
			return nil
		}
		// Successfully read a packet, continue reading next
	}

	return nil
}

// ClearUDPReadBufferNonBlocking clears UDP read buffer in non-blocking way
// Uses goroutine to clean in background without blocking caller
// Uses semaphore to limit concurrency and prevent goroutine leak
func ClearUDPReadBufferNonBlocking(conn net.Conn, timeout time.Duration, maxPackets int) {
	// First check if connection is valid
	if conn == nil {
		return
	}

	select {
	case udpCleanupSemaphore <- struct{}{}:
		// Acquired semaphore, start goroutine to clean
		go func(c net.Conn) {
			defer func() {
				// Ensure semaphore is released to prevent leak
				<-udpCleanupSemaphore
			}()
			// Prevent panic from causing abnormal goroutine exit
			defer func() {
				if r := recover(); r != nil {
					// Log but ignore panic
				}
			}()
			// Check connection validity again in goroutine
			if c == nil {
				return
			}
			ClearUDPReadBuffer(c, timeout, maxPackets)
		}(conn)
	default:
		// If cleanup goroutines are full, use shorter timeout to clean synchronously
		// Avoid unlimited goroutine growth
		shortTimeout := timeout
		if shortTimeout <= 0 {
			shortTimeout = 100 * time.Millisecond
		}
		ClearUDPReadBuffer(conn, shortTimeout/2, maxPackets)
	}
}

// HasUDPDataInBuffer checks if UDP connection read buffer has data
// Returns true if there may be data, false if buffer is empty
func HasUDPDataInBuffer(conn net.Conn) bool {
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		return false
	}

	// Set very short read timeout for probe
	udpConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	defer udpConn.SetReadDeadline(time.Time{})

	buf := make([]byte, 1)
	_, err := udpConn.Read(buf)
	if err != nil {
		// Timeout error means buffer is empty
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return false
		}
		// Other errors also mean no data
		return false
	}

	// If can read data, buffer is not empty
	// Note: This data packet has been consumed, caller should consider this
	return true
}
