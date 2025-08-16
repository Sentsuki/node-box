package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 配置文件
type Config struct {
	Subscriptions   []Subscription `json:"subscriptions"`
	ConfigDir       string         `json:"config_dir"`
	InsertMarker    string         `json:"insert_marker"`
	UpdateInterval  int            `json:"update_interval_hours"`
	ExcludeKeywords []string       `json:"exclude_keywords,omitempty"`
	Proxy           *ProxyConfig   `json:"proxy,omitempty"` // 新增代理配置
}

// 代理配置
type ProxyConfig struct {
	Type     string `json:"type"`     // "http", "https", "socks5"
	Host     string `json:"host"`     // 代理服务器地址
	Port     int    `json:"port"`     // 代理服务器端口
	Username string `json:"username"` // 用户名（可选）
	Password string `json:"password"` // 密码（可选）
}

type Subscription struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Type   string `json:"type"` // "clash" or "singbox"
	Enable bool   `json:"enable"`
}

// Clash代理结构
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

type ClashConfig struct {
	Proxies []ClashProxy `yaml:"proxies"`
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

	// 记录代理配置信息
	if config.Proxy != nil {
		log.Printf("代理配置: %s://%s:%d", config.Proxy.Type, config.Proxy.Host, config.Proxy.Port)
		if config.Proxy.Username != "" {
			log.Printf("代理认证: %s", config.Proxy.Username)
		}
	} else {
		log.Println("未配置代理，将使用直连")
	}

	return &NodeManager{config: &config}, nil
}

// 获取订阅内容
func (nm *NodeManager) fetchSubscription(url string) ([]byte, error) {
	client, err := nm.createHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("创建HTTP客户端失败: %v", err)
	}

	resp, err := client.Get(url)
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

// 创建支持代理的HTTP客户端
func (nm *NodeManager) createHTTPClient() (*http.Client, error) {
	if nm.config.Proxy == nil {
		// 没有代理配置，使用默认客户端
		return &http.Client{
			Timeout: 30 * time.Second,
		}, nil
	}

	// 构建代理URL
	var proxyURL string
	switch strings.ToLower(nm.config.Proxy.Type) {
	case "http", "https":
		if nm.config.Proxy.Username != "" && nm.config.Proxy.Password != "" {
			proxyURL = fmt.Sprintf("%s://%s:%s@%s:%d",
				nm.config.Proxy.Type,
				nm.config.Proxy.Username,
				nm.config.Proxy.Password,
				nm.config.Proxy.Host,
				nm.config.Proxy.Port)
		} else {
			proxyURL = fmt.Sprintf("%s://%s:%d",
				nm.config.Proxy.Type,
				nm.config.Proxy.Host,
				nm.config.Proxy.Port)
		}
	case "socks5":
		if nm.config.Proxy.Username != "" && nm.config.Proxy.Password != "" {
			proxyURL = fmt.Sprintf("socks5://%s:%s@%s:%d",
				nm.config.Proxy.Username,
				nm.config.Proxy.Password,
				nm.config.Proxy.Host,
				nm.config.Proxy.Port)
		} else {
			proxyURL = fmt.Sprintf("socks5://%s:%d",
				nm.config.Proxy.Host,
				nm.config.Proxy.Port)
		}
	default:
		return nil, fmt.Errorf("不支持的代理类型: %s", nm.config.Proxy.Type)
	}

	// 解析代理URL
	proxyURLParsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("解析代理URL失败: %v", err)
	}

	// 创建支持代理的传输层
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURLParsed),
	}

	// 创建HTTP客户端
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return client, nil
}

// 处理订阅数据，根据类型选择不同的处理方式
func (nm *NodeManager) processSubscription(subType string, data []byte) ([]map[string]interface{}, error) {
	switch strings.ToLower(subType) {
	case "singbox":
		return nm.processSingBoxSubscription(data)
	case "clash":
		return nm.processClashSubscription(data)
	default:
		return nil, fmt.Errorf("不支持的订阅类型: %s", subType)
	}
}

// 处理SingBox订阅 - 保留所有原始字段
func (nm *NodeManager) processSingBoxSubscription(data []byte) ([]map[string]interface{}, error) {
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析SingBox配置失败: %v", err)
	}

	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return nil, fmt.Errorf("配置中缺少 outbounds 字段")
	}

	outboundsArray, ok := outboundsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("outbounds 字段格式错误")
	}

	var nodes []map[string]interface{}
	for _, outboundRaw := range outboundsArray {
		outboundMap, ok := outboundRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// 检查类型，排除 direct、block、selector
		outboundType, ok := outboundMap["type"].(string)
		if !ok || outboundType == "direct" || outboundType == "block" || outboundType == "selector" {
			continue
		}

		// 直接保留原始节点数据，不做任何修改
		nodes = append(nodes, outboundMap)
	}

	return nodes, nil
}

// 处理Clash订阅 - 转换为SingBox格式
func (nm *NodeManager) processClashSubscription(data []byte) ([]map[string]interface{}, error) {
	var clashConfig ClashConfig
	if err := yaml.Unmarshal(data, &clashConfig); err != nil {
		return nil, fmt.Errorf("解析Clash配置失败: %v", err)
	}

	var nodes []map[string]interface{}
	for _, proxy := range clashConfig.Proxies {
		node := nm.convertClashToSingBox(proxy)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// 转换Clash代理到SingBox格式
func (nm *NodeManager) convertClashToSingBox(proxy ClashProxy) map[string]interface{} {
	node := map[string]interface{}{
		"type":   strings.ToLower(proxy.Type),
		"tag":    proxy.Name,
		"server": proxy.Server,
	}

	// 转换端口
	port, err := strconv.Atoi(proxy.Port)
	if err != nil {
		log.Printf("转换端口失败: %v", err)
		return nil
	}
	node["server_port"] = port

	// 处理UDP相关的packet_encoding
	if proxy.UDP {
		if proxy.Type == "vmess" || proxy.Type == "vless" {
			node["packet_encoding"] = "xudp"
		}
	}

	// 处理WebSocket传输配置
	if proxy.Network == "ws" {
		transport := map[string]interface{}{
			"type": "ws",
			"path": proxy.WSPath,
		}
		if len(proxy.WSHeaders) > 0 {
			headers := make(map[string]interface{})
			for k, v := range proxy.WSHeaders {
				headers[k] = v
			}
			transport["headers"] = headers
		}
		node["transport"] = transport
	}

	// 处理TLS配置
	if proxy.TLS {
		tls := map[string]interface{}{
			"enabled":     true,
			"server_name": proxy.ServerName,
			"insecure":    proxy.SkipCertVerify,
		}
		node["tls"] = tls
	}

	// 根据不同类型设置特定字段
	switch strings.ToLower(proxy.Type) {
	case "ss":
		node["type"] = "shadowsocks"
		node["method"] = proxy.Cipher
		node["password"] = proxy.Password

	case "vmess":
		node["type"] = "vmess"
		node["uuid"] = proxy.UUID
		if alterId, err := strconv.Atoi(proxy.AlterId); err == nil {
			node["alter_id"] = alterId
		}
		node["security"] = proxy.Cipher

	case "vless":
		node["type"] = "vless"
		node["uuid"] = proxy.UUID
		node["security"] = proxy.Cipher

	case "trojan":
		node["type"] = "trojan"
		node["password"] = proxy.Password

	default:
		log.Printf("不支持的代理类型: %s", proxy.Type)
		return nil
	}

	return node
}

// 统一处理tag转换逻辑
func (nm *NodeManager) addSubscriptionPrefix(nodes []map[string]interface{}, subName string) []map[string]interface{} {
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			node["tag"] = fmt.Sprintf("[%s] %s", subName, tag)
		}
	}
	return nodes
}

// 过滤排除关键词的节点
func (nm *NodeManager) filterNodes(nodes []map[string]interface{}) []map[string]interface{} {
	var filteredNodes []map[string]interface{}
	for _, node := range nodes {
		tag, ok := node["tag"].(string)
		if !ok {
			continue
		}

		shouldExclude := false
		for _, keyword := range nm.config.ExcludeKeywords {
			if strings.Contains(strings.ToLower(tag), strings.ToLower(keyword)) {
				log.Printf("排除节点: %s (包含关键词: %s)", tag, keyword)
				shouldExclude = true
				break
			}
		}
		if !shouldExclude {
			filteredNodes = append(filteredNodes, node)
		}
	}
	return filteredNodes
}

// 获取所有节点
func (nm *NodeManager) fetchAllNodes() ([]map[string]interface{}, error) {
	var allNodes []map[string]interface{}

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

		nodes, err := nm.processSubscription(sub.Type, data)
		if err != nil {
			log.Printf("处理订阅 %s 失败: %v", sub.Name, err)
			continue
		}

		// 统一执行tag转换逻辑
		nodes = nm.addSubscriptionPrefix(nodes, sub.Name)

		// 过滤排除关键词的节点
		originalCount := len(nodes)
		nodes = nm.filterNodes(nodes)
		filteredCount := len(nodes)

		allNodes = append(allNodes, nodes...)
		log.Printf("从订阅 %s 获取到 %d 个节点，过滤后剩余 %d 个节点", sub.Name, originalCount, filteredCount)
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

// 直接修改配置文件，不重组
func (nm *NodeManager) updateConfigFile(configPath string, nodes []map[string]interface{}) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return fmt.Errorf("配置文件中缺少 outbounds 字段")
	}

	outboundsArray, ok := outboundsRaw.([]interface{})
	if !ok {
		return fmt.Errorf("outbounds 字段格式错误")
	}

	// 找到插入标记的位置
	var markerIndex = -1
	var markerOutbound map[string]interface{}
	for i, outboundRaw := range outboundsArray {
		if outboundMap, ok := outboundRaw.(map[string]interface{}); ok {
			if tag, ok := outboundMap["tag"].(string); ok && tag == nm.config.InsertMarker {
				markerIndex = i
				markerOutbound = outboundMap
				break
			}
		}
	}

	if markerIndex == -1 {
		return fmt.Errorf("未找到插入标记: %s", nm.config.InsertMarker)
	}

	// 检查插入标记是否为selector类型
	if markerType, ok := markerOutbound["type"].(string); !ok || markerType != "selector" {
		return fmt.Errorf("插入标记 %s 不是selector类型", nm.config.InsertMarker)
	}

	// 移除旧的订阅节点
	var newOutbounds []interface{}
	for _, outboundRaw := range outboundsArray {
		if outboundMap, ok := outboundRaw.(map[string]interface{}); ok {
			if tag, ok := outboundMap["tag"].(string); ok {
				isSubscriptionNode := false
				for _, sub := range nm.config.Subscriptions {
					if strings.Contains(tag, fmt.Sprintf("[%s]", sub.Name)) {
						isSubscriptionNode = true
						break
					}
				}
				if !isSubscriptionNode {
					newOutbounds = append(newOutbounds, outboundRaw)
				}
			} else {
				newOutbounds = append(newOutbounds, outboundRaw)
			}
		} else {
			newOutbounds = append(newOutbounds, outboundRaw)
		}
	}

	// 添加新节点
	for _, node := range nodes {
		newOutbounds = append(newOutbounds, node)
	}

	// 更新selector的outbounds列表
	var nodeTags []string
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			nodeTags = append(nodeTags, tag)
		}
	}

	// 找到更新后的插入标记位置
	for i, outboundRaw := range newOutbounds {
		if outboundMap, ok := outboundRaw.(map[string]interface{}); ok {
			if tag, ok := outboundMap["tag"].(string); ok && tag == nm.config.InsertMarker {
				// 更新selector的outbounds列表
				if outboundList, ok := outboundMap["outbounds"].([]interface{}); ok {
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
					outboundMap["outbounds"] = newOutboundList
				} else {
					// 如果outbounds字段不存在，直接设置为节点标签数组
					var newOutboundList []interface{}
					for _, tag := range nodeTags {
						newOutboundList = append(newOutboundList, tag)
					}
					outboundMap["outbounds"] = newOutboundList
				}
				newOutbounds[i] = outboundMap
				break
			}
		}
	}

	// 更新配置
	config["outbounds"] = newOutbounds

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
func generateExampleConfig(configPath string) {
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
		ExcludeKeywords: []string{"故障转移", "流量"},
		Proxy: &ProxyConfig{
			Type:     "http",
			Host:     "127.0.0.1",
			Port:     7890,
			Username: "username", // 可选
			Password: "password", // 可选
		},
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)
	fmt.Printf("已生成示例配置文件: %s\n", configPath)
}

func main() {
	configPath := "config.json" // 默认配置文件路径

	if len(os.Args) > 1 && os.Args[1] == "init" {
		if len(os.Args) > 2 {
			configPath = os.Args[2] // 使用用户指定的路径
		}
		generateExampleConfig(configPath)
		return
	}

	// 检查是否有自定义配置文件路径参数
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	// 初始化节点管理器
	manager, err := NewNodeManager(configPath)
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
