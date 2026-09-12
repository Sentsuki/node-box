package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"node-box/internal/control"
)

var secret = []byte("test-secret")

// fakeUpdater records what a handler queued.
type fakeUpdater struct{ got []control.Trigger }

func (f *fakeUpdater) Trigger(t control.Trigger) bool {
	f.got = append(f.got, t)
	return true
}

func (f *fakeUpdater) Status() control.Status { return control.Status{} }

// post sends a body to the hook handler. signature is used verbatim when
// non-empty; pass signed to have the correct one computed instead.
func post(t *testing.T, body string, signed bool, signature string) (*httptest.ResponseRecorder, *fakeUpdater) {
	t.Helper()

	up := &fakeUpdater{}
	s := &Server{secret: secret, updater: up, limiter: newLimiter(1000, time.Minute)}

	req := httptest.NewRequest(http.MethodPost, "/hooks/github", strings.NewReader(body))
	if signed {
		signature = ComputeSignature(secret, []byte(body))
	}
	if signature != "" {
		req.Header.Set(signatureHeader, signature)
	}

	w := httptest.NewRecorder()
	s.handleHook(w, req)
	return w, up
}

// assertRejected checks the status and that nothing was queued.
func assertRejected(t *testing.T, w *httptest.ResponseRecorder, up *fakeUpdater, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d", w.Code, want)
	}
	if len(up.got) != 0 {
		t.Errorf("a rejected request queued %d update(s)", len(up.got))
	}
}

func TestHandleHook_SignedRequestQueuesUpdate(t *testing.T) {
	w, up := post(t, `{"ref":"abc123"}`, true, "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if len(up.got) != 1 {
		t.Fatalf("queued %d updates, want 1", len(up.got))
	}
	if up.got[0].Ref != "abc123" || up.got[0].Kind != control.KindWebhook {
		t.Errorf("trigger = %+v", up.got[0])
	}
}

func TestVerify_AcceptsCorrectSignature(t *testing.T) {
	s := &Server{secret: secret}
	body := []byte(`{"ref":"abc"}`)
	if !s.verify(ComputeSignature(secret, body), body) {
		t.Fatal("a correctly signed body was rejected")
	}
}

func TestVerify_Rejects(t *testing.T) {
	s := &Server{secret: secret}
	body := []byte(`{"ref":"abc"}`)
	good := ComputeSignature(secret, body)

	tests := map[string]struct {
		header string
		body   []byte
	}{
		"missing header":      {"", body},
		"missing prefix":      {strings.TrimPrefix(good, "sha256="), body},
		"wrong algorithm tag": {"sha1=" + strings.TrimPrefix(good, "sha256="), body},
		"not hex":             {"sha256=zzzz", body},
		"truncated":           {good[:len(good)-2], body},
		"wrong secret":        {ComputeSignature([]byte("other"), body), body},
		"tampered body":       {good, []byte(`{"ref":"evil"}`)},
		"empty signature":     {"sha256=", body},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if s.verify(tc.header, tc.body) {
				t.Error("an invalid signature was accepted")
			}
		})
	}
}

func TestHandleHook_UnsignedIsUnauthorized(t *testing.T) {
	w, up := post(t, `{"ref":"abc"}`, false, "")
	assertRejected(t, w, up, http.StatusUnauthorized)
}

func TestHandleHook_TamperedBodyIsUnauthorized(t *testing.T) {
	// Sign one body, send a different one.
	sig := ComputeSignature(secret, []byte(`{"ref":"good"}`))
	w, up := post(t, `{"ref":"evil"}`, false, sig)
	assertRejected(t, w, up, http.StatusUnauthorized)
}

func TestHandleHook_WrongSecretIsUnauthorized(t *testing.T) {
	body := `{"ref":"abc"}`
	sig := ComputeSignature([]byte("not-the-secret"), []byte(body))
	w, up := post(t, body, false, sig)
	assertRejected(t, w, up, http.StatusUnauthorized)
}

func TestHandleHook_OversizedBodyIsRejected(t *testing.T) {
	w, up := post(t, strings.Repeat("x", maxBody+10), true, "")
	assertRejected(t, w, up, http.StatusRequestEntityTooLarge)
}

func TestHandleHook_SignatureCheckedBeforeParsing(t *testing.T) {
	// Malformed JSON without a signature must fail as unauthorized rather than
	// as a parse error: an unauthenticated caller learns nothing either way.
	w, up := post(t, `{not json`, false, "")
	assertRejected(t, w, up, http.StatusUnauthorized)
}

func TestHandleHook_SignedMalformedJSONIsBadRequest(t *testing.T) {
	w, up := post(t, `{not json`, true, "")
	assertRejected(t, w, up, http.StatusBadRequest)
}

func TestHandleHook_RateLimited(t *testing.T) {
	up := &fakeUpdater{}
	s := &Server{secret: secret, updater: up, limiter: newLimiter(2, time.Minute)}
	body := `{"ref":"abc"}`

	send := func() int {
		req := httptest.NewRequest(http.MethodPost, "/hooks/github", strings.NewReader(body))
		req.Header.Set(signatureHeader, ComputeSignature(secret, []byte(body)))
		w := httptest.NewRecorder()
		s.handleHook(w, req)
		return w.Code
	}

	for i := range 2 {
		if code := send(); code != http.StatusAccepted {
			t.Fatalf("request %d: status = %d, want %d", i+1, code, http.StatusAccepted)
		}
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Errorf("third request: status = %d, want %d", code, http.StatusTooManyRequests)
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter(3, time.Minute)
	for i := range 3 {
		if !l.allow() {
			t.Fatalf("request %d should have been allowed", i+1)
		}
	}
	if l.allow() {
		t.Error("the fourth request should have been limited")
	}
}
