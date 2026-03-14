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

import "errors"

// Connection pool related error definitions
var (
	// ErrPoolClosed pool is closed
	ErrPoolClosed = errors.New("pool is closed")

	// ErrConnectionClosed connection is closed
	ErrConnectionClosed = errors.New("connection is closed")

	// ErrGetConnectionTimeout get connection timeout
	ErrGetConnectionTimeout = errors.New("get connection timeout")

	// ErrMaxConnectionsReached max connections reached
	ErrMaxConnectionsReached = errors.New("max connections limit reached")

	// ErrInvalidConnection invalid connection
	ErrInvalidConnection = errors.New("invalid connection")

	// ErrConnectionUnhealthy connection unhealthy
	ErrConnectionUnhealthy = errors.New("connection unhealthy")

	// ErrInvalidConfig invalid config
	ErrInvalidConfig = errors.New("invalid config parameters")

	// ErrConnectionLeaked connection leak detected
	ErrConnectionLeaked = errors.New("connection leak detected: connection not returned within timeout")

	// ErrPoolExhausted pool exhausted
	ErrPoolExhausted = errors.New("pool exhausted, cannot create new connection")

	// ErrUnsupportedIPVersion unsupported IP version
	ErrUnsupportedIPVersion = errors.New("unsupported IP version")

	// ErrNoConnectionForIPVersion no available connection for specified IP version
	ErrNoConnectionForIPVersion = errors.New("no available connection for specified IP version")

	// ErrUnsupportedProtocol unsupported protocol
	ErrUnsupportedProtocol = errors.New("unsupported protocol type")

	// ErrNoConnectionForProtocol no available connection for specified protocol
	ErrNoConnectionForProtocol = errors.New("no available connection for specified protocol")
)
