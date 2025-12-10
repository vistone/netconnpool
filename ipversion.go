package netconnpool

import (
	"net"
)

// IPVersion IP版本类型
type IPVersion int

const (
	// IPVersionUnknown 未知IP版本
	IPVersionUnknown IPVersion = iota
	// IPVersionIPv4 IPv4
	IPVersionIPv4
	// IPVersionIPv6 IPv6
	IPVersionIPv6
)

// String 返回IP版本的字符串表示
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

// DetectIPVersion 检测连接的IP版本
// 如果conn是net.Conn类型，则检测其远程地址的IP版本
// 否则返回IPVersionUnknown
func DetectIPVersion(conn any) IPVersion {
	netConn, ok := conn.(net.Conn)
	if !ok {
		return IPVersionUnknown
	}

	remoteAddr := netConn.RemoteAddr()
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
		// 尝试从地址字符串解析
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

// ParseIPVersion 从字符串解析IP版本
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
