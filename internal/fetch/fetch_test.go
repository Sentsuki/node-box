package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(Options{UserAgent: "node-box-test", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGet_SendsHeaders(t *testing.T) {
	var gotUA, gotETag, gotCustom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotETag = r.Header.Get("If-None-Match")
		gotCustom = r.Header.Get("Authorization")
		w.Header().Set("ETag", `W/"abc"`)
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	resp, err := newTestClient(t).Get(context.Background(), Request{
		URL:       srv.URL,
		UserAgent: "custom-ua",
		Headers:   map[string]string{"Authorization": "Bearer tok"},
		ETag:      `W/"abc"`,
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("body = %q", resp.Body)
	}
	if resp.ETag != `W/"abc"` {
		t.Errorf("ETag = %q", resp.ETag)
	}
	if gotUA != "custom-ua" {
		t.Errorf("User-Agent = %q, want the per-request override", gotUA)
	}
	if gotETag != `W/"abc"` {
		t.Errorf("If-None-Match = %q, want the ETag to be sent as a conditional request", gotETag)
	}
	if gotCustom != "Bearer tok" {
		t.Errorf("Authorization = %q", gotCustom)
	}
}

func TestGet_NotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	// A 304 is an expected outcome, not a failure, and must be distinguishable.
	_, err := newTestClient(t).Get(context.Background(), Request{URL: srv.URL})
	if !errors.Is(err, ErrNotModified) {
		t.Fatalf("err = %v, want ErrNotModified", err)
	}
}

func TestGet_EnforcesSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()

	// Exceeding the cap must be an error, not a silent truncation.
	_, err := newTestClient(t).Get(context.Background(), Request{URL: srv.URL, MaxBytes: 50})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}

	// Exactly at the cap is fine.
	resp, err := newTestClient(t).Get(context.Background(), Request{URL: srv.URL, MaxBytes: 100})
	if err != nil {
		t.Fatalf("a body exactly at the cap was rejected: %v", err)
	}
	if len(resp.Body) != 100 {
		t.Errorf("got %d bytes, want 100", len(resp.Body))
	}
}

func TestGet_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newTestClient(t).Get(context.Background(), Request{URL: srv.URL})
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want a StatusError", err)
	}
	if se.Status != http.StatusForbidden {
		t.Errorf("status = %d", se.Status)
	}
}

func TestGet_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := newTestClient(t).Get(ctx, Request{URL: srv.URL})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRetryable(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"304 is a success":       {ErrNotModified, false},
		"too large is permanent": {ErrTooLarge, false},
		"cancelled":              {context.Canceled, false},
		"deadline":               {context.DeadlineExceeded, false},
		"403":                    {&StatusError{Status: 403}, false},
		"404":                    {&StatusError{Status: 404}, false},
		"401":                    {&StatusError{Status: 401}, false},
		"408":                    {&StatusError{Status: 408}, true},
		"429":                    {&StatusError{Status: 429}, true},
		"500":                    {&StatusError{Status: 500}, true},
		"503":                    {&StatusError{Status: 503}, true},
		"transport failure":      {errors.New("dial tcp: connection reset"), true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := retryable(tc.err); got != tc.want {
				t.Errorf("retryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestGetWithRetry_RetriesThenSucceeds(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			http.Error(w, "later", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resp, err := newTestClient(t).GetWithRetry(context.Background(),
		Request{URL: srv.URL},
		RetryPolicy{Attempts: 3, Base: time.Millisecond})
	if err != nil {
		t.Fatalf("GetWithRetry: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("body = %q", resp.Body)
	}
	if calls != 3 {
		t.Errorf("server saw %d calls, want 3", calls)
	}
}

func TestGetWithRetry_DoesNotRetryClientErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newTestClient(t).GetWithRetry(context.Background(),
		Request{URL: srv.URL},
		RetryPolicy{Attempts: 3, Base: time.Millisecond})
	if err == nil {
		t.Fatal("want an error")
	}
	// Retrying a 404 only delays the run; it will fail identically every time.
	if calls != 1 {
		t.Errorf("server saw %d calls, want 1", calls)
	}
}

func TestGetWithRetry_NotModifiedIsNotRetried(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	_, err := newTestClient(t).GetWithRetry(context.Background(),
		Request{URL: srv.URL, ETag: `"x"`},
		RetryPolicy{Attempts: 3, Base: time.Millisecond})
	if !errors.Is(err, ErrNotModified) {
		t.Fatalf("err = %v, want ErrNotModified", err)
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want 1", calls)
	}
}

func TestGetWithRetry_StopsOnCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "later", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The backoff sleep must observe cancellation rather than holding up
	// shutdown for the rest of its budget.
	start := time.Now()
	_, err := newTestClient(t).GetWithRetry(ctx, Request{URL: srv.URL},
		RetryPolicy{Attempts: 3, Base: time.Hour})
	if err == nil {
		t.Fatal("want an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; the retry backoff ignored the cancelled context", elapsed)
	}
}
