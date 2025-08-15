package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SingBox配置结构
type SingBoxConfig struct {
	Log       *LogConfig       `json:"log,omitempty"`
	DNS       *DNSConfig       `json:"dns,omitempty"`
	Inbounds  []InboundConfig  `json:"inbounds,omitempty"`
	Outbounds []OutboundConfig `json:"outbounds"`
	Route     *RouteConfig     `json:"route,omitempty"`
}

type LogConfig struct {
	Level string `json:"level,omitempty"`
}

type DNSConfig struct {
	Servers []interface{} `json:"servers,omitempty"`
	Rules   []interface{} `json:"rules,omitempty"`
}

type InboundConfig struct {
	Type   string      `json:"type"`
	Tag    string      `json:"tag,omitempty"`
	Listen string      `json:"listen,omitempty"`
	Port   int         `json:"listen_port,omitempty"`
	Users  interface{} `json:"users,omitempty"`
}

type OutboundConfig struct {
	Type           string           `json:"type"`
	Tag            string           `json:"tag"`
	Server         string           `json:"server,omitempty"`
	ServerPort     int              `json:"server_port,omitempty"`
	Method         string           `json:"method,omitempty"`
	Password       string           `json:"password,omitempty"`
	UUID           string           `json:"uuid,omitempty"`
	Security       string           `json:"security,omitempty"`
	AlterId        int              `json:"alter_id,omitempty"`
	Network        string           `json:"network,omitempty"`
	Path           string           `json:"path,omitempty"`
	Host           string           `json:"host,omitempty"`
	ServiceName    string           `json:"service_name,omitempty"`
	TLS            *TLSConfig       `json:"tls,omitempty"`
	Transport      *TransportConfig `json:"transport,omitempty"`
	PacketEncoding string           `json:"packet_encoding,omitempty"`
	Outbounds      interface{}      `json:"outbounds,omitempty"` // 新增字段，用于存储selector类型的outbounds
}

type TLSConfig struct {
	Enabled    bool   `json:"enabled,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
}

type TransportConfig struct {
	Type    string                 `json:"type,omitempty"`
	Path    string                 `json:"path,omitempty"`
	Headers map[string]interface{} `json:"headers,omitempty"`
}

type RouteConfig struct {
	Rules []interface{} `json:"rules,omitempty"`
}

// Clash配置结构
type ClashProxy struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Server         string            `yaml:"server"`
	Port           string            `yaml:"port"` // 修改为 string
	Cipher         string            `yaml:"cipher,omitempty"`
	Password       string            `yaml:"password,omitempty"`
	UUID           string            `yaml:"uuid,omitempty"`
	AlterId        string            `yaml:"alterId,omitempty"` // 修改为 string
	Security       string            `yaml:"security,omitempty"`
	Network        string            `yaml:"network,omitempty"`
	WSPath         string            `yaml:"ws-path,omitempty"`
	WSHeaders      map[string]string `yaml:"ws-headers,omitempty"`
	TLS            bool              `yaml:"tls,omitempty"`
	SkipCertVerify bool              `yaml:"skip-cert-verify,omitempty"`
	ServerName     string            `yaml:"servername,omitempty"`
	UDP            bool              `yaml:"udp,omitempty"` // 新增支持 udp 字段
}

type ClashConfig struct {
	Proxies []ClashProxy `yaml:"proxies"`
}

// 配置文件
type Config struct {
	Subscriptions   []Subscription `json:"subscriptions"`
	ConfigDir       string         `json:"config_dir"`
	InsertMarker    string         `json:"insert_marker"`
	UpdateInterval  int            `json:"update_interval_hours"`
	ExcludeKeywords []string       `json:"exclude_keywords,omitempty"` // 新增：排除包含指定关键词的节点
}

type Subscription struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Type   string `json:"type"` // "clash" or "singbox"
	Enable bool   `json:"enable"`
}

// NodeManager 节点管理器
type NodeManager struct {
	config *Config
}

func NewNodeManager(configPath string) (*NodeManager, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return &NodeManager{config: &config}, nil
}

// 获取订阅内容
func (nm *NodeManager) fetchSubscription(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取订阅失败: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取订阅内容失败: %v", err)
	}

	return data, nil
}

// 解析Clash订阅
func (nm *NodeManager) parseClashSubscription(data []byte) ([]OutboundConfig, error) {
	var clashConfig ClashConfig
	if err := yaml.Unmarshal(data, &clashConfig); err != nil {
		return nil, fmt.Errorf("解析Clash配置失败: %v", err)
	}

	var outbounds []OutboundConfig
	for _, proxy := range clashConfig.Proxies {
		outbound := nm.convertClashToSingBox(proxy)
		if outbound != nil {
			outbounds = append(outbounds, *outbound)
		}
	}

	return outbounds, nil
}

// 解析SingBox订阅
func (nm *NodeManager) parseSingBoxSubscription(data []byte) ([]OutboundConfig, error) {
	var config SingBoxConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析SingBox配置失败: %v", err)
	}

	var outbounds []OutboundConfig
	for _, outbound := range config.Outbounds {
		if outbound.Type != "direct" && outbound.Type != "block" && outbound.Type != "selector" {
			outbounds = append(outbounds, outbound)
		}
	}

	return outbounds, nil
}

// 转换Clash代理到SingBox格式
func (nm *NodeManager) convertClashToSingBox(proxy ClashProxy) *OutboundConfig {
	outbound := &OutboundConfig{
		Tag:    proxy.Name,
		Type:   strings.ToLower(proxy.Type),
		Server: proxy.Server,
	}

	// 转换 port
	port, err := strconv.Atoi(proxy.Port)
	if err != nil {
		log.Printf("转换 port 失败: %v", err)
		return nil
	}
	outbound.ServerPort = port

	// 统一处理 UDP 相关的 packet_encoding（对 vmess 和 vless 都生效）
	if proxy.UDP {
		if proxy.Type == "vmess" || proxy.Type == "vless" {
			outbound.PacketEncoding = "xudp"
		}
	}

	// 统一处理 WebSocket 传输配置
	if proxy.Network == "ws" {
		outbound.Transport = &TransportConfig{
			Type: "ws",
			Path: proxy.WSPath,
		}
		if len(proxy.WSHeaders) > 0 {
			headers := make(map[string]interface{})
			for k, v := range proxy.WSHeaders {
				headers[k] = v
			}
			outbound.Transport.Headers = headers
		}
	}

	// 统一处理 TLS 配置
	if proxy.TLS {
		outbound.TLS = &TLSConfig{
			Enabled:    true,
			ServerName: proxy.ServerName,
			Insecure:   proxy.SkipCertVerify,
		}
	}

	switch strings.ToLower(proxy.Type) {
	case "ss":
		outbound.Type = "shadowsocks"
		outbound.Method = proxy.Cipher
		outbound.Password = proxy.Password

	case "vmess":
		outbound.Type = "vmess"
		outbound.UUID = proxy.UUID
		// 转换 alterId
		alterId, err := strconv.Atoi(proxy.AlterId)
		if err != nil {
			log.Printf("转换 alterId 失败: %v", err)
			return nil
		}
		outbound.AlterId = alterId
		outbound.Security = proxy.Cipher // 映射 cipher 到 security

	case "vless":
		outbound.Type = "vless"
		outbound.UUID = proxy.UUID
		outbound.Security = proxy.Cipher // 映射 cipher 到 security

	case "trojan":
		outbound.Type = "trojan"
		outbound.Password = proxy.Password

	default:
		log.Printf("不支持的代理类型: %s", proxy.Type)
		return nil
	}

	return outbound
}

// 获取所有节点
func (nm *NodeManager) fetchAllNodes() ([]OutboundConfig, error) {
	var allNodes []OutboundConfig

	for _, sub := range nm.config.Subscriptions {
		if !sub.Enable {
			continue
		}

		log.Printf("获取订阅: %s", sub.Name)
		data, err := nm.fetchSubscription(sub.URL)
		if err != nil {
			log.Printf("获取订阅 %s 失败: %v", sub.Name, err)
			continue
		}

		var nodes []OutboundConfig
		switch strings.ToLower(sub.Type) {
		case "clash":
			nodes, err = nm.parseClashSubscription(data)
		case "singbox":
			nodes, err = nm.parseSingBoxSubscription(data)
		default:
			log.Printf("不支持的订阅类型: %s", sub.Type)
			continue
		}

		if err != nil {
			log.Printf("解析订阅 %s 失败: %v", sub.Name, err)
			continue
		}

		// 为节点添加订阅前缀
		for i := range nodes {
			nodes[i].Tag = fmt.Sprintf("[%s] %s", sub.Name, nodes[i].Tag)
		}

		// 过滤排除包含指定关键词的节点
		var filteredNodes []OutboundConfig
		for _, node := range nodes {
			shouldExclude := false
			for _, keyword := range nm.config.ExcludeKeywords {
				if strings.Contains(strings.ToLower(node.Tag), strings.ToLower(keyword)) {
					log.Printf("排除节点: %s (包含关键词: %s)", node.Tag, keyword)
					shouldExclude = true
					break
				}
			}
			if !shouldExclude {
				filteredNodes = append(filteredNodes, node)
			}
		}

		allNodes = append(allNodes, filteredNodes...)
		log.Printf("从订阅 %s 获取到 %d 个节点，过滤后剩余 %d 个节点", sub.Name, len(nodes), len(filteredNodes))
	}

	return allNodes, nil
}

// 扫描配置文件夹
func (nm *NodeManager) scanConfigFiles() ([]string, error) {
	var configFiles []string

	err := filepath.Walk(nm.config.ConfigDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".json") {
			configFiles = append(configFiles, path)
		}

		return nil
	})

	return configFiles, err
}

// 更新配置文件
func (nm *NodeManager) updateConfigFile(configPath string, nodes []OutboundConfig) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config SingBoxConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 查找插入标记（selector类型的outbound）
	var markerOutbound *OutboundConfig
	for i, outbound := range config.Outbounds {
		if outbound.Tag == nm.config.InsertMarker {
			markerOutbound = &config.Outbounds[i]
			break
		}
	}

	if markerOutbound == nil {
		return fmt.Errorf("未找到插入标记: %s", nm.config.InsertMarker)
	}

	// 检查插入标记是否为selector类型
	if markerOutbound.Type != "selector" {
		return fmt.Errorf("插入标记 %s 不是selector类型", nm.config.InsertMarker)
	}

	// 移除旧的订阅节点（通过标签识别）
	var newOutbounds []OutboundConfig
	for _, outbound := range config.Outbounds {
		// 保留非订阅节点
		isSubscriptionNode := false
		for _, sub := range nm.config.Subscriptions {
			if strings.Contains(outbound.Tag, fmt.Sprintf("[%s]", sub.Name)) {
				isSubscriptionNode = true
				break
			}
		}
		if !isSubscriptionNode {
			newOutbounds = append(newOutbounds, outbound)
		}
	}

	// 将新节点添加到outbounds数组
	newOutbounds = append(newOutbounds, nodes...)

	// 更新插入标记的outbounds数组
	var nodeTags []string
	for _, node := range nodes {
		nodeTags = append(nodeTags, node.Tag)
	}

	// 查找更新后的插入标记位置
	var updatedMarkerOutbound *OutboundConfig
	for i := range newOutbounds {
		if newOutbounds[i].Tag == nm.config.InsertMarker {
			updatedMarkerOutbound = &newOutbounds[i]
			break
		}
	}

	if updatedMarkerOutbound == nil {
		return fmt.Errorf("更新后未找到插入标记: %s", nm.config.InsertMarker)
	}

	// 将节点标签添加到selector的outbounds数组中
	// 注意：这里需要处理interface{}类型，因为outbounds字段可能是[]string或[]interface{}
	if outboundList, ok := updatedMarkerOutbound.Outbounds.([]string); ok {
		// 移除旧的订阅节点标签
		var newOutboundList []string
		for _, tag := range outboundList {
			isSubscriptionTag := false
			for _, sub := range nm.config.Subscriptions {
				if strings.Contains(tag, fmt.Sprintf("[%s]", sub.Name)) {
					isSubscriptionTag = true
					break
				}
			}
			if !isSubscriptionTag {
				newOutboundList = append(newOutboundList, tag)
			}
		}
		// 添加新的节点标签
		newOutboundList = append(newOutboundList, nodeTags...)
		updatedMarkerOutbound.Outbounds = newOutboundList
	} else if outboundList, ok := updatedMarkerOutbound.Outbounds.([]interface{}); ok {
		// 移除旧的订阅节点标签
		var newOutboundList []interface{}
		for _, tag := range outboundList {
			if tagStr, ok := tag.(string); ok {
				isSubscriptionTag := false
				for _, sub := range nm.config.Subscriptions {
					if strings.Contains(tagStr, fmt.Sprintf("[%s]", sub.Name)) {
						isSubscriptionTag = true
						break
					}
				}
				if !isSubscriptionTag {
					newOutboundList = append(newOutboundList, tag)
				}
			} else {
				newOutboundList = append(newOutboundList, tag)
			}
		}
		// 添加新的节点标签
		for _, tag := range nodeTags {
			newOutboundList = append(newOutboundList, tag)
		}
		updatedMarkerOutbound.Outbounds = newOutboundList
	} else {
		// 如果outbounds字段不存在或类型不匹配，直接设置为节点标签数组
		updatedMarkerOutbound.Outbounds = nodeTags
	}

	config.Outbounds = newOutbounds

	// 写回文件
	updatedData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, updatedData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// 更新所有配置文件
func (nm *NodeManager) updateAllConfigs() error {
	log.Println("开始更新配置...")

	// 获取所有节点
	nodes, err := nm.fetchAllNodes()
	if err != nil {
		return fmt.Errorf("获取节点失败: %v", err)
	}

	log.Printf("总共获取到 %d 个节点", len(nodes))

	// 扫描配置文件
	configFiles, err := nm.scanConfigFiles()
	if err != nil {
		return fmt.Errorf("扫描配置文件失败: %v", err)
	}

	log.Printf("找到 %d 个配置文件", len(configFiles))

	// 更新每个配置文件
	for _, configFile := range configFiles {
		log.Printf("更新配置文件: %s", configFile)
		if err := nm.updateConfigFile(configFile, nodes); err != nil {
			log.Printf("更新配置文件 %s 失败: %v", configFile, err)
		} else {
			log.Printf("成功更新配置文件: %s", configFile)
		}
	}

	log.Println("配置更新完成")
	return nil
}

// 启动定期更新
func (nm *NodeManager) startScheduler() {
	ticker := time.NewTicker(time.Duration(nm.config.UpdateInterval) * time.Hour)
	defer ticker.Stop()

	log.Printf("启动定期更新，间隔: %d 小时", nm.config.UpdateInterval)

	for range ticker.C {
		log.Println("执行定期更新...")
		if err := nm.updateAllConfigs(); err != nil {
			log.Printf("定期更新失败: %v", err)
		}
	}
}

// 生成示例配置文件
func generateExampleConfig() {
	config := Config{
		Subscriptions: []Subscription{
			{
				Name:   "示例订阅1",
				URL:    "https://example.com/clash-subscription",
				Type:   "clash",
				Enable: true,
			},
			{
				Name:   "示例订阅2",
				URL:    "https://example.com/singbox-subscription",
				Type:   "singbox",
				Enable: true,
			},
		},
		ConfigDir:       "./configs",
		InsertMarker:    "🚀 节点选择",
		UpdateInterval:  6,
		ExcludeKeywords: []string{"故障转移", "流量", "专线"}, // 新增：排除包含这些关键词的节点
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile("config.json", data, 0644)
	fmt.Println("已生成示例配置文件: config.json")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		generateExampleConfig()
		return
	}

	// 初始化节点管理器
	manager, err := NewNodeManager("config.json")
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	// 执行一次更新
	if err := manager.updateAllConfigs(); err != nil {
		log.Printf("初始更新失败: %v", err)
	}

	// 启动定期更新
	manager.startScheduler()
}
