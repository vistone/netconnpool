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
)

// IPVersion IP version type
type IPVersion int

const (
	// IPVersionUnknown unknown IP version
	IPVersionUnknown IPVersion = iota
	// IPVersionIPv4 IPv4
	IPVersionIPv4
	// IPVersionIPv6 IPv6
	IPVersionIPv6
)

// String returns IP version string representation
func (v IPVersion) String() string {
	switch v {
	case IPVersionIPv4:
		return "IPv4"
	case IPVersionIPv6:
		return "IPv6"
	default:
		return "Unknown"
	}
}

// DetectIPVersion detects connection IP version
// If conn is net.Conn type, detects IP version of its remote address
// Otherwise returns IPVersionUnknown
func DetectIPVersion(conn net.Conn) IPVersion {
	if conn == nil {
		return IPVersionUnknown
	}

	remoteAddr := conn.RemoteAddr()
	if remoteAddr == nil {
		return IPVersionUnknown
	}

	switch addr := remoteAddr.(type) {
	case *net.TCPAddr:
		if addr.IP != nil {
			if addr.IP.To4() != nil {
				return IPVersionIPv4
			}
			return IPVersionIPv6
		}
	case *net.UDPAddr:
		if addr.IP != nil {
			if addr.IP.To4() != nil {
				return IPVersionIPv4
			}
			return IPVersionIPv6
		}
	default:
		// Try to parse from address string
		host, _, err := net.SplitHostPort(addr.String())
		if err == nil && host != "" {
			ip := net.ParseIP(host)
			if ip != nil {
				if ip.To4() != nil {
					return IPVersionIPv4
				}
				return IPVersionIPv6
			}
		}
	}

	return IPVersionUnknown
}

// ParseIPVersion parses IP version from string
func ParseIPVersion(s string) IPVersion {
	switch s {
	case "IPv4", "ipv4", "4":
		return IPVersionIPv4
	case "IPv6", "ipv6", "6":
		return IPVersionIPv6
	default:
		return IPVersionUnknown
	}
}
