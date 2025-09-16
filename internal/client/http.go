// Package client provides HTTP client functionality with proxy support
// for fetching subscription data from remote servers.
package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"node-box/internal/config"
)

// Client package errors
var (
	ErrUnsupportedProxyType = errors.New("unsupported proxy type")
	ErrProxyConfigNil       = errors.New("proxy configuration is nil")
	ErrHTTPRequestFailed    = errors.New("HTTP request failed")
	ErrInvalidStatusCode    = errors.New("invalid HTTP status code")
	ErrReadResponseBody     = errors.New("failed to read response body")
)

// HTTPClient defines the interface for making HTTP requests.
// This interface allows for easy testing and mocking of HTTP operations.
type HTTPClient interface {
	// Get performs an HTTP GET request to the specified URL
	// and returns the response body as bytes or an error.
	Get(url string) ([]byte, error)

	// GetWithUserAgent performs an HTTP GET request with a custom User-Agent
	// and returns the response body as bytes or an error.
	GetWithUserAgent(url string, userAgent string) ([]byte, error)
}

// Client implements HTTPClient interface with proxy support.
// It wraps the standard http.Client and provides additional
// functionality for proxy configuration and timeout handling.
type Client struct {
	httpClient *http.Client
	userAgent  string
}

// NewHTTPClient creates a new HTTP client with optional proxy configuration and user agent.
// If proxy is nil, it creates a client with direct connection.
// Returns an HTTPClient interface implementation or an error if proxy configuration is invalid.
func NewHTTPClient(proxy *config.ProxyConfig, userAgent string) (HTTPClient, error) {
	// Set default user agent if not provided
	if userAgent == "" {
		userAgent = "node-box/1.0"
	}

	if proxy == nil {
		// No proxy configuration, use default client with timeout
		return &Client{
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
			userAgent: userAgent,
		}, nil
	}

	// Build proxy URL based on configuration
	proxyURL, err := buildProxyURL(proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to build proxy URL: %w", err)
	}

	// Parse proxy URL
	proxyURLParsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
	}

	// Create HTTP transport with proxy configuration
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURLParsed),
	}

	// Create HTTP client with proxy transport
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &Client{
		httpClient: httpClient,
		userAgent:  userAgent,
	}, nil
}

// Get performs an HTTP GET request to the specified URL.
// It returns the response body as bytes or an error if the request fails.
func (c *Client) Get(targetURL string) ([]byte, error) {
	return c.GetWithUserAgent(targetURL, c.userAgent)
}

// GetWithUserAgent performs an HTTP GET request with a custom User-Agent.
// It returns the response body as bytes or an error if the request fails.
func (c *Client) GetWithUserAgent(targetURL string, userAgent string) ([]byte, error) {
	// Use default user agent if not provided
	if userAgent == "" {
		userAgent = c.userAgent
	}

	// Create request with custom User-Agent
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHTTPRequestFailed, err)
	}

	// Set custom User-Agent header
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHTTPRequestFailed, err)
	}
	defer resp.Body.Close()

	// Check for HTTP error status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: %d %s", ErrInvalidStatusCode, resp.StatusCode, resp.Status)
	}

	// Read response body using io.ReadAll for safer handling
	// This avoids the makeslice error when ContentLength is invalid
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadResponseBody, err)
	}

	return body, nil
}

// buildProxyURL constructs a proxy URL string from ProxyConfig.
// It handles different proxy types (HTTP, HTTPS, SOCKS5) and optional authentication.
func buildProxyURL(proxy *config.ProxyConfig) (string, error) {
	if proxy == nil {
		return "", ErrProxyConfigNil
	}

	proxyType := strings.ToLower(proxy.Type)

	// Validate proxy type
	switch proxyType {
	case "http", "https", "socks5":
		// Valid proxy types
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedProxyType, proxy.Type)
	}

	// Build proxy URL with or without authentication
	if proxy.Username != "" && proxy.Password != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d",
			proxyType,
			proxy.Username,
			proxy.Password,
			proxy.Host,
			proxy.Port), nil
	}

	return fmt.Sprintf("%s://%s:%d",
		proxyType,
		proxy.Host,
		proxy.Port), nil
}
