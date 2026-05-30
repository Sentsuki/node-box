package client

import (
	"fmt"
	"node-box/internal/logger"
	"os"
	"time"
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
func (f *Fetcher) FetchSubscription(url string) ([]byte, error) {
	return f.FetchSubscriptionWithUserAgent(url, "")
}

// FetchSubscriptionWithUserAgent retrieves subscription data from the specified URL
// with a custom User-Agent and retry support.
func (f *Fetcher) FetchSubscriptionWithUserAgent(url string, userAgent string) ([]byte, error) {
	if userAgent != "" {
		logger.Debug("获取订阅: %s (User-Agent: %s)", url, userAgent)
	} else {
		logger.Debug("获取订阅: %s", url)
	}

	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * f.retryDelay
			logger.Warn("第 %d 次重试获取订阅 %s，等待 %v...", attempt, url, delay)
			time.Sleep(delay)
		}

		var data []byte
		var err error

		if userAgent != "" {
			data, err = f.client.GetWithUserAgent(url, userAgent)
		} else {
			data, err = f.client.Get(url)
		}

		if err != nil {
			lastErr = err
			logger.Debug("获取订阅失败 (尝试 %d/%d): %v", attempt+1, f.maxRetries+1, err)
			continue
		}

		logger.Debug("成功获取 %d 字节数据: %s", len(data), url)
		return data, nil
	}

	return nil, fmt.Errorf("获取订阅失败，已重试 %d 次: %v", f.maxRetries, lastErr)
}

// FetchSubscriptionFromPath reads subscription data from a local file path with retry support.
func (f *Fetcher) FetchSubscriptionFromPath(path string) ([]byte, error) {
	return f.readFileWithRetry(path, "订阅文件")
}

// FetchModuleFromPath reads module data from a local file path with retry support.
func (f *Fetcher) FetchModuleFromPath(path string) ([]byte, error) {
	return f.readFileWithRetry(path, "模块文件")
}

// readFileWithRetry reads a local file with retry support for transient errors.
// label is used in log messages to distinguish between subscription and module files.
func (f *Fetcher) readFileWithRetry(path, label string) ([]byte, error) {
	logger.Debug("读取本地%s: %s", label, path)

	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * f.retryDelay
			logger.Warn("第 %d 次重试读取%s %s，等待 %v...", attempt, label, path, delay)
			time.Sleep(delay)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			// 文件不存在是非临时性错误，不需要重试
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%s不存在: %s", label, path)
			}

			lastErr = err
			logger.Debug("读取%s失败 (尝试 %d/%d): %s - %v", label, attempt+1, f.maxRetries+1, path, err)
			continue
		}

		logger.Debug("成功读取%s %d 字节数据: %s", label, len(data), path)
		return data, nil
	}

	return nil, fmt.Errorf("读取%s失败，已重试 %d 次: %s - %v", label, f.maxRetries, path, lastErr)
}
