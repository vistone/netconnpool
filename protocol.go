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
	"net"
	"strings"
)

// Protocol protocol type
type Protocol int

const (
	// ProtocolUnknown unknown protocol
	ProtocolUnknown Protocol = iota
	// ProtocolTCP TCP protocol
	ProtocolTCP
	// ProtocolUDP UDP protocol
	ProtocolUDP
)

// String returns protocol string representation
func (p Protocol) String() string {
	switch p {
	case ProtocolTCP:
		return "TCP"
	case ProtocolUDP:
		return "UDP"
	default:
		return "Unknown"
	}
}

// DetectProtocol detects connection protocol type
// Supports TCP and UDP connections
func DetectProtocol(conn net.Conn) Protocol {
	if conn == nil {
		return ProtocolUnknown
	}

	// Determine protocol by address type
	remoteAddr := conn.RemoteAddr()
	if remoteAddr == nil {
		localAddr := conn.LocalAddr()
		if localAddr != nil {
			return detectProtocolFromAddr(localAddr)
		}
		return ProtocolUnknown
	}

	return detectProtocolFromAddr(remoteAddr)
}

// detectProtocolFromAddr determines protocol type from address
func detectProtocolFromAddr(addr net.Addr) Protocol {
	switch addr.(type) {
	case *net.TCPAddr:
		return ProtocolTCP
	case *net.UDPAddr:
		return ProtocolUDP
	default:
		// Try to determine from network string
		network := addr.Network()
		switch strings.ToLower(network) {
		case "tcp", "tcp4", "tcp6":
			return ProtocolTCP
		case "udp", "udp4", "udp6":
			return ProtocolUDP
		default:
			// Check if address string contains protocol identifier
			addrStr := addr.String()
			if strings.Contains(addrStr, "tcp") {
				return ProtocolTCP
			}
			if strings.Contains(addrStr, "udp") {
				return ProtocolUDP
			}
		}
	}
	return ProtocolUnknown
}

// ParseProtocol parses protocol type from string
func ParseProtocol(s string) Protocol {
	switch strings.ToUpper(s) {
	case "TCP":
		return ProtocolTCP
	case "UDP":
		return ProtocolUDP
	default:
		return ProtocolUnknown
	}
}

// IsTCP checks if TCP protocol
func (p Protocol) IsTCP() bool {
	return p == ProtocolTCP
}

// IsUDP checks if UDP protocol
func (p Protocol) IsUDP() bool {
	return p == ProtocolUDP
}
