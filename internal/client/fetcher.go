package client

import (
	"fmt"
	"log"
)

// Fetcher 处理从远程URL获取订阅数据。
// 它使用HTTPClient执行实际的HTTP请求，
// 并为订阅操作提供更高级别的接口。
type Fetcher struct {
	client HTTPClient
}

// NewFetcher 使用提供的HTTPClient创建新的Fetcher实例。
// HTTPClient可以配置为支持或不支持代理。
func NewFetcher(client HTTPClient) *Fetcher {
	return &Fetcher{
		client: client,
	}
}

// FetchSubscription 从指定URL获取订阅数据。
// 返回原始订阅数据字节或获取失败时的错误。
// 此方法处理日志记录和错误包装以便更好地调试。
func (f *Fetcher) FetchSubscription(url string) ([]byte, error) {
	log.Printf("获取订阅: %s", url)

	data, err := f.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取订阅失败: %v", err)
	}

	log.Printf("成功获取 %d 字节数据: %s", len(data), url)
	return data, nil
}
