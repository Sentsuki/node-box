package client

import (
	"fmt"
	"log"
)

// Fetcher handles fetching subscription data from remote URLs.
// It uses an HTTPClient to perform the actual HTTP requests and provides
// a higher-level interface for subscription operations.
type Fetcher struct {
	client HTTPClient
}

// NewFetcher creates a new Fetcher instance with the provided HTTPClient.
// The HTTPClient can be configured with or without proxy support.
func NewFetcher(client HTTPClient) *Fetcher {
	return &Fetcher{
		client: client,
	}
}

// FetchSubscription retrieves subscription data from the specified URL.
// It returns the raw subscription data as bytes or an error if fetching fails.
// This method handles logging and error wrapping for better debugging.
func (f *Fetcher) FetchSubscription(url string) ([]byte, error) {
	log.Printf("获取订阅: %s", url)

	data, err := f.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取订阅失败: %v", err)
	}

	log.Printf("成功获取 %d 字节数据: %s", len(data), url)
	return data, nil
}
