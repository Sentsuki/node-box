// Package subscription 提供订阅数据处理功能
package subscription

// Node 表示统一的节点数据结构，使用any替代interface{}
type Node map[string]any

// Processor 订阅处理器接口，用于处理不同类型的订阅数据
type Processor interface {
	// Process 处理订阅数据并返回节点列表
	Process(data []byte) ([]Node, error)
}

// ClashProxy Clash代理结构体，用于解析Clash格式的订阅
type ClashProxy struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Server         string            `yaml:"server"`
	Port           string            `yaml:"port"`
	Cipher         string            `yaml:"cipher,omitempty"`
	Password       string            `yaml:"password,omitempty"`
	UUID           string            `yaml:"uuid,omitempty"`
	AlterId        string            `yaml:"alterId,omitempty"`
	Security       string            `yaml:"security,omitempty"`
	Network        string            `yaml:"network,omitempty"`
	WSPath         string            `yaml:"ws-path,omitempty"`
	WSHeaders      map[string]string `yaml:"ws-headers,omitempty"`
	TLS            bool              `yaml:"tls,omitempty"`
	SkipCertVerify bool              `yaml:"skip-cert-verify,omitempty"`
	ServerName     string            `yaml:"servername,omitempty"`
	UDP            bool              `yaml:"udp,omitempty"`
}

// ClashConfig Clash配置结构体
type ClashConfig struct {
	Proxies []ClashProxy `yaml:"proxies"`
}
