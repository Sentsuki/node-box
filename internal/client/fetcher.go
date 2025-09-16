package client

import (
	"fmt"
	"log"
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
// This method handles logging, error wrapping, and automatic retry with exponential backoff.
func (f *Fetcher) FetchSubscription(url string) ([]byte, error) {
	return f.FetchSubscriptionWithUserAgent(url, "")
}

// FetchSubscriptionWithUserAgent retrieves subscription data from the specified URL with custom User-Agent and retry support.
// It returns the raw subscription data as bytes or an error if all retry attempts fail.
// This method handles logging, error wrapping, and automatic retry with exponential backoff.
func (f *Fetcher) FetchSubscriptionWithUserAgent(url string, userAgent string) ([]byte, error) {
	if userAgent != "" {
		log.Printf("获取订阅: %s (User-Agent: %s)", url, userAgent)
	} else {
		log.Printf("获取订阅: %s", url)
	}

	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * f.retryDelay
			log.Printf("第 %d 次重试获取订阅 %s，等待 %v...", attempt, url, delay)
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
			log.Printf("获取订阅失败 (尝试 %d/%d): %v", attempt+1, f.maxRetries+1, err)
			continue
		}

		log.Printf("成功获取 %d 字节数据: %s", len(data), url)
		return data, nil
	}

	return nil, fmt.Errorf("获取订阅失败，已重试 %d 次: %v", f.maxRetries, lastErr)
}
