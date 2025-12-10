package netconnpool

// PoolMode 连接池模式
type PoolMode int

const (
	// PoolModeClient 客户端模式：主动连接到服务器
	PoolModeClient PoolMode = iota
	// PoolModeServer 服务器端模式：接受客户端连接
	PoolModeServer
)

// String 返回模式字符串表示
func (m PoolMode) String() string {
	switch m {
	case PoolModeClient:
		return "client"
	case PoolModeServer:
		return "server"
	default:
		return "unknown"
	}
}

// ParsePoolMode 从字符串解析连接池模式
func ParsePoolMode(s string) PoolMode {
	switch s {
	case "client":
		return PoolModeClient
	case "server":
		return PoolModeServer
	default:
		return PoolModeClient // 默认客户端模式
	}
}
