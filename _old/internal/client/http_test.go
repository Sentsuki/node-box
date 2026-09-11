package client_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"node-box/internal/client"
	"node-box/internal/config"
)

// ---------------------------------------------------------------------------
// NewHTTPClient
// ---------------------------------------------------------------------------

func TestNewHTTPClient_NoProxy(t *testing.T) {
	c, err := client.NewHTTPClient(nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewHTTPClient_WithHTTPProxy(t *testing.T) {
	proxy := &config.ProxyConfig{
		Type: "http",
		Host: "127.0.0.1",
		Port: 8080,
	}
	c, err := client.NewHTTPClient(proxy, "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewHTTPClient_WithSOCKS5Proxy(t *testing.T) {
	proxy := &config.ProxyConfig{
		Type: "socks5",
		Host: "127.0.0.1",
		Port: 1080,
	}
	c, err := client.NewHTTPClient(proxy, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewHTTPClient_WithProxyAuth(t *testing.T) {
	proxy := &config.ProxyConfig{
		Type:     "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Username: "user",
		Password: "pass",
	}
	c, err := client.NewHTTPClient(proxy, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewHTTPClient_UnsupportedProxyType(t *testing.T) {
	proxy := &config.ProxyConfig{
		Type: "ftp",
		Host: "127.0.0.1",
		Port: 21,
	}
	_, err := client.NewHTTPClient(proxy, "")
	if err == nil {
		t.Error("expected error for unsupported proxy type")
	}
}

// ---------------------------------------------------------------------------
// Client.Get – using a test HTTP server
// ---------------------------------------------------------------------------

func TestClient_Get_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	c, err := client.NewHTTPClient(nil, "test-agent")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	data, err := c.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", data)
	}
}

func TestClient_Get_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c, err := client.NewHTTPClient(nil, "")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	_, err = c.Get(server.URL)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestClient_Get_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c, err := client.NewHTTPClient(nil, "")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	_, err = c.Get(server.URL)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestClient_GetWithUserAgent_SetsHeader(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	c, err := client.NewHTTPClient(nil, "default-agent")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	_, err = c.GetWithUserAgent(server.URL, "custom-agent/1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedUA != "custom-agent/1.0" {
		t.Errorf("expected User-Agent 'custom-agent/1.0', got %q", receivedUA)
	}
}

func TestClient_Get_InvalidURL(t *testing.T) {
	c, err := client.NewHTTPClient(nil, "")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	_, err = c.Get("not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ---------------------------------------------------------------------------
// Fetcher
// ---------------------------------------------------------------------------

func TestFetcher_FetchSubscription_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("subscription data"))
	}))
	defer server.Close()

	c, _ := client.NewHTTPClient(nil, "")
	f := client.NewFetcher(c)

	data, err := f.FetchSubscription(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "subscription data" {
		t.Errorf("expected 'subscription data', got %q", data)
	}
}

func TestFetcher_FetchSubscription_RetriesOnFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success after retries"))
	}))
	defer server.Close()

	c, _ := client.NewHTTPClient(nil, "")
	// 3 retries, minimal delay for test speed
	f := client.NewFetcherWithRetry(c, 3, 1*time.Millisecond)

	data, err := f.FetchSubscription(server.URL)
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if string(data) != "success after retries" {
		t.Errorf("unexpected data: %q", data)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestFetcher_FetchSubscription_ExhaustsRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c, _ := client.NewHTTPClient(nil, "")
	f := client.NewFetcherWithRetry(c, 2, 1*time.Millisecond)

	_, err := f.FetchSubscription(server.URL)
	if err == nil {
		t.Error("expected error after exhausting retries")
	}
}

func TestFetcher_FetchSubscriptionFromPath_Success(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sub.txt"
	if err := writeFile(path, []byte("file content")); err != nil {
		t.Fatal(err)
	}

	c, _ := client.NewHTTPClient(nil, "")
	f := client.NewFetcher(c)

	data, err := f.FetchSubscriptionFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("expected 'file content', got %q", data)
	}
}

func TestFetcher_FetchSubscriptionFromPath_NotFound(t *testing.T) {
	c, _ := client.NewHTTPClient(nil, "")
	f := client.NewFetcher(c)

	_, err := f.FetchSubscriptionFromPath("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFetcher_FetchModuleFromPath_Success(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/module.json"
	if err := writeFile(path, []byte(`{"key":"value"}`)); err != nil {
		t.Fatal(err)
	}

	c, _ := client.NewHTTPClient(nil, "")
	f := client.NewFetcher(c)

	data, err := f.FetchModuleFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"key":"value"}` {
		t.Errorf("unexpected data: %q", data)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
