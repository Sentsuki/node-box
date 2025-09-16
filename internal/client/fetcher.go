package client

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

// Fetcher handles fetching subscription data from remote URLs.
// It uses an HTTPClient to perform the actual HTTP requests and provides
// a higher-level interface for subscription operations with retry support.
type Fetcher struct {
	client     HTTPClient
	maxRetries int
	retryDelay time.Duration
}

// NewFetcher creates a new Fetcher instance with the provided HTTPClient.
// The HTTPClient can be configured with or without proxy support.
// Default retry configuration: 3 retries with 2 second delay between attempts.
func NewFetcher(client HTTPClient) *Fetcher {
	return &Fetcher{
		client:     client,
		maxRetries: 3,
		retryDelay: 2 * time.Second,
	}
}

// NewFetcherWithRetry creates a new Fetcher instance with custom retry configuration.
func NewFetcherWithRetry(client HTTPClient, maxRetries int, retryDelay time.Duration) *Fetcher {
	return &Fetcher{
		client:     client,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

// FetchSubscription retrieves subscription data from the specified URL with retry support.
// It returns the raw subscription data as bytes or an error if all retry attempts fail.
// This method handles logging, error wrapping, and automatic retry with exponential backoff.
func (f *Fetcher) FetchSubscription(url string) ([]byte, error) {
	log.Printf("获取订阅: %s", url)

	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * f.retryDelay
			log.Printf("第 %d 次重试获取订阅 %s，等待 %v...", attempt, url, delay)
			time.Sleep(delay)
		}

		data, err := f.client.Get(url)
		if err != nil {
			lastErr = err
			log.Printf("获取订阅失败 (尝试 %d/%d): %v", attempt+1, f.maxRetries+1, err)
			continue
		}

		log.Printf("成功获取 %d 字节数据: %s", len(data), url)

		// 尝试检测并解码 base64 编码的订阅
		decodedData, err := f.tryDecodeBase64(data)
		if err != nil {
			log.Printf("Base64 解码失败，使用原始数据: %v", err)
			return data, nil
		}

		if decodedData != nil {
			log.Printf("检测到 base64 编码订阅，已解码为 %d 字节", len(decodedData))
			return decodedData, nil
		}

		return data, nil
	}

	return nil, fmt.Errorf("获取订阅失败，已重试 %d 次: %v", f.maxRetries, lastErr)
}

// tryDecodeBase64 尝试检测并解码 base64 编码的订阅数据
// 返回解码后的数据，如果不是 base64 编码则返回 nil
func (f *Fetcher) tryDecodeBase64(data []byte) ([]byte, error) {
	// 去除可能的空白字符
	content := strings.TrimSpace(string(data))

	// 检查是否可能是 base64 编码
	// 1. 长度应该是4的倍数（padding后）
	// 2. 只包含 base64 字符集
	// 3. 不应该包含明显的 YAML/JSON 标识符
	if !f.isLikelyBase64(content) {
		return nil, nil
	}

	// 尝试解码
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		// 尝试 URL safe base64
		decoded, err = base64.URLEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("base64 解码失败: %v", err)
		}
	}

	// 验证解码后的内容是否是有效的文本
	if !utf8.Valid(decoded) {
		return nil, fmt.Errorf("解码后的内容不是有效的 UTF-8 文本")
	}

	// 检查解码后的内容是否像是配置文件
	decodedStr := string(decoded)
	if f.looksLikeConfig(decodedStr) {
		return decoded, nil
	}

	return nil, fmt.Errorf("解码后的内容不像是配置文件")
}

// isLikelyBase64 检查内容是否可能是 base64 编码
func (f *Fetcher) isLikelyBase64(content string) bool {
	// 移除可能的换行符和空格
	content = strings.ReplaceAll(content, "\n", "")
	content = strings.ReplaceAll(content, "\r", "")
	content = strings.ReplaceAll(content, " ", "")

	// 长度检查
	if len(content) == 0 || len(content)%4 != 0 {
		return false
	}

	// 如果内容包含明显的配置文件标识符，则不太可能是 base64
	lowerContent := strings.ToLower(content)
	configIndicators := []string{
		"proxies:", "proxy-groups:", "rules:", // Clash 标识符
		"outbounds:", "inbounds:", "routing:", // SingBox 标识符
		"\"type\":", "\"server\":", "\"port\":", // JSON 格式
		"mixed-port:", "allow-lan:", // Clash 配置
	}

	for _, indicator := range configIndicators {
		if strings.Contains(lowerContent, indicator) {
			return false
		}
	}

	// 检查是否只包含 base64 字符
	for _, char := range content {
		if !((char >= 'A' && char <= 'Z') ||
			(char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '+' || char == '/' || char == '=' ||
			char == '-' || char == '_') { // URL safe base64
			return false
		}
	}

	return true
}

// looksLikeConfig 检查解码后的内容是否像是配置文件
func (f *Fetcher) looksLikeConfig(content string) bool {
	lowerContent := strings.ToLower(content)

	// Clash 配置标识符
	clashIndicators := []string{
		"proxies:", "proxy-groups:", "rules:",
		"mixed-port:", "allow-lan:", "mode:",
		"log-level:", "external-controller:",
	}

	// SingBox 配置标识符
	singboxIndicators := []string{
		"outbounds:", "inbounds:", "routing:",
		"\"type\":", "\"tag\":", "\"server\":",
	}

	// 检查 Clash 标识符
	for _, indicator := range clashIndicators {
		if strings.Contains(lowerContent, indicator) {
			return true
		}
	}

	// 检查 SingBox 标识符
	for _, indicator := range singboxIndicators {
		if strings.Contains(lowerContent, indicator) {
			return true
		}
	}

	// 检查是否包含代理相关的关键词
	proxyKeywords := []string{
		"shadowsocks", "vmess", "vless", "trojan",
		"ss://", "vmess://", "vless://", "trojan://",
		"server:", "port:", "cipher:", "password:",
	}

	for _, keyword := range proxyKeywords {
		if strings.Contains(lowerContent, keyword) {
			return true
		}
	}

	return false
}
