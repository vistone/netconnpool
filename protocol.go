package netconnpool

import (
	"net"
	"strings"
)

// Protocol 协议类型
type Protocol int

const (
	// ProtocolUnknown 未知协议
	ProtocolUnknown Protocol = iota
	// ProtocolTCP TCP协议
	ProtocolTCP
	// ProtocolUDP UDP协议
	ProtocolUDP
)

// String 返回协议的字符串表示
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

// DetectProtocol 检测连接的协议类型
// 支持TCP和UDP连接
func DetectProtocol(conn any) Protocol {
	// 尝试类型断言为net.Conn
	netConn, ok := conn.(net.Conn)
	if !ok {
		return ProtocolUnknown
	}

	// 通过地址类型判断协议
	remoteAddr := netConn.RemoteAddr()
	if remoteAddr == nil {
		localAddr := netConn.LocalAddr()
		if localAddr != nil {
			return detectProtocolFromAddr(localAddr)
		}
		return ProtocolUnknown
	}

	return detectProtocolFromAddr(remoteAddr)
}

// detectProtocolFromAddr 从地址判断协议类型
func detectProtocolFromAddr(addr net.Addr) Protocol {
	switch addr.(type) {
	case *net.TCPAddr:
		return ProtocolTCP
	case *net.UDPAddr:
		return ProtocolUDP
	default:
		// 尝试从网络字符串判断
		network := addr.Network()
		switch strings.ToLower(network) {
		case "tcp", "tcp4", "tcp6":
			return ProtocolTCP
		case "udp", "udp4", "udp6":
			return ProtocolUDP
		default:
			// 检查地址字符串中是否包含协议标识
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

// ParseProtocol 从字符串解析协议类型
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

// IsTCP 检查是否为TCP协议
func (p Protocol) IsTCP() bool {
	return p == ProtocolTCP
}

// IsUDP 检查是否为UDP协议
func (p Protocol) IsUDP() bool {
	return p == ProtocolUDP
}
