// Package fetch performs HTTP requests for configuration sources and
// subscriptions.
//
// Every request takes a context, caps the response body, and retries only the
// failures that are actually transient.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"node-box/internal/model"
)

// DefaultMaxBytes caps a response body. Subscriptions and module files are
// small; an unbounded read turns a misbehaving server into an OOM.
const DefaultMaxBytes = 16 << 20 // 16 MiB

// ErrNotModified is returned when the server answers 304 to a conditional
// request. It is an expected outcome, not a failure.
var ErrNotModified = errors.New("not modified")

// ErrTooLarge is returned when a response body exceeds the configured cap.
var ErrTooLarge = errors.New("response body too large")

// Client performs HTTP GETs with an optional proxy.
type Client struct {
	http      *http.Client
	userAgent string
	maxBytes  int64
}

// Options configures a Client.
type Options struct {
	Proxy     *model.ProxyConfig
	UserAgent string
	Timeout   time.Duration
	MaxBytes  int64
}

// New creates a client.
func New(opts Options) (*Client, error) {
	if opts.UserAgent == "" {
		opts.UserAgent = "node-box"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultMaxBytes
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.Proxy != nil {
		proxyURL, err := proxyURL(opts.Proxy)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &Client{
		http:      &http.Client{Transport: transport, Timeout: opts.Timeout},
		userAgent: opts.UserAgent,
		maxBytes:  opts.MaxBytes,
	}, nil
}

func proxyURL(p *model.ProxyConfig) (*url.URL, error) {
	u := &url.URL{
		Scheme: strings.ToLower(p.Type),
		Host:   net.JoinHostPort(p.Host, fmt.Sprint(p.Port)),
	}
	if p.Username != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u, nil
}

// Request describes a single GET.
type Request struct {
	URL string
	// UserAgent overrides the client default when non-empty.
	UserAgent string
	// Headers are added to the request.
	Headers map[string]string
	// ETag, when set, is sent as If-None-Match so an unchanged resource can
	// answer 304 without transferring (or counting against a rate limit).
	ETag string
	// MaxBytes overrides the client default when non-zero.
	MaxBytes int64
}

// Response is a completed GET.
type Response struct {
	Body []byte
	ETag string
}

// Get performs one GET with no retries.
func (c *Client) Get(ctx context.Context, r Request) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", r.URL, err)
	}

	ua := r.UserAgent
	if ua == "" {
		ua = c.userAgent
	}
	req.Header.Set("User-Agent", ua)
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if r.ETag != "" {
		req.Header.Set("If-None-Match", r.ETag)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", r.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, ErrNotModified
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{URL: r.URL, Status: resp.StatusCode, Reason: resp.Status}
	}

	limit := r.MaxBytes
	if limit == 0 {
		limit = c.maxBytes
	}
	// Read one byte past the cap so exceeding it is detectable rather than
	// silently truncating the payload.
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read body from %s: %w", r.URL, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrTooLarge, r.URL, limit)
	}

	return &Response{Body: body, ETag: resp.Header.Get("ETag")}, nil
}

// StatusError reports a non-2xx response.
type StatusError struct {
	URL    string
	Status int
	Reason string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: %s", e.URL, e.Reason)
}

// Retryable reports whether retrying the request could plausibly succeed.
// Client errors other than 408 and 429 will fail identically every time.
func (e *StatusError) Retryable() bool {
	switch e.Status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return e.Status >= 500
}
