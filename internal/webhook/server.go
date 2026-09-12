// Package webhook exposes the small HTTP surface that lets the configuration
// repository ask node-box to update immediately.
//
// The server binds a loopback address and expects TLS to be terminated by a
// reverse proxy, so it deliberately has no certificate handling of its own.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"node-box/internal/control"
	"node-box/internal/logx"
)

const (
	// maxBody caps the request body. The payload is a single commit sha.
	maxBody = 64 << 10
	// signatureHeader carries the HMAC of the raw request body.
	signatureHeader = "X-NodeBox-Signature-256"
	// shutdownGrace is how long in-flight requests get to finish.
	shutdownGrace = 5 * time.Second
)

// Updater is the part of the runner this server needs. Keeping it an interface
// lets the handlers be tested without a real snapshot store behind them.
type Updater interface {
	Trigger(control.Trigger) bool
	Status() control.Status
}

// Server serves the webhook and status endpoints.
type Server struct {
	http    *http.Server
	secret  []byte
	updater Updater
	limiter *limiter
}

// New creates a server listening on addr.
func New(addr string, secret []byte, u Updater) *Server {
	s := &Server{
		secret:  secret,
		updater: u,
		limiter: newLimiter(30, time.Minute),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/github", s.handleHook)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /status", s.handleStatus)

	s.http = &http.Server{
		Addr:    addr,
		Handler: mux,
		// Go's zero values mean "no timeout", which leaves a public-facing
		// listener open to connections that never finish sending.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s
}

// Run serves until the context is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() {
		logx.Infof("webhook listening on %s", s.http.Addr)
		err := s.http.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			logx.Warnf("webhook shutdown: %v", err)
		}
		return nil
	}
}

type hookRequest struct {
	Ref string `json:"ref"`
}

func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	// Rate limiting is global rather than per-IP: every request arrives from
	// the reverse proxy, so the source address carries no information unless
	// a forwarded-for header is explicitly trusted.
	if !s.limiter.allow() {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limited"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read body"})
		return
	}
	if len(body) > maxBody {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "body too large"})
		return
	}

	if !s.verify(r.Header.Get(signatureHeader), body) {
		logx.Warnf("webhook rejected: bad signature")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	var req hookRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON object"})
		return
	}

	logx.Infof("webhook accepted for ref %s", shortRef(req.Ref))
	// Queue and return immediately. The sender should not wait for a full
	// update, and GitHub Actions has its own timeout to respect.
	s.updater.Trigger(control.Trigger{Kind: control.KindWebhook, Ref: req.Ref})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

// verify checks the HMAC over the exact bytes received.
func (s *Server) verify(header string, body []byte) bool {
	value, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(body)
	// Constant time, so a wrong signature reveals nothing about how wrong.
	return hmac.Equal(got, mac.Sum(nil))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.updater.Status())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logx.Debugf("write response: %v", err)
	}
}

func shortRef(ref string) string {
	if len(ref) <= 8 {
		if ref == "" {
			return "(latest)"
		}
		return ref
	}
	return ref[:8]
}

// ComputeSignature returns the header value for a body, for tests and tooling.
func ComputeSignature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}
