package logx

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// capture redirects stderr for the duration of fn and returns what was written.
func capture(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = saved }()

	fn()
	w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	r.Close()
	return b.String()
}

func TestParseLevel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Level
	}{
		{"silent", Silent}, {"error", Error}, {"warn", Warn},
		{"warning", Warn}, {"info", Info}, {"debug", Debug},
		{" DEBUG ", Debug},
	} {
		got, err := ParseLevel(tc.in)
		if err != nil {
			t.Errorf("ParseLevel(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	// A typo must not silently fall back to info.
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("want an error for an unknown level")
	}
}

func TestLevelFiltersQuieterMessages(t *testing.T) {
	saved := Level(level.Load())
	defer SetLevel(saved)

	SetLevel(Warn)
	out := capture(t, func() {
		Errorf("an error")
		Warnf("a warning")
		Infof("some info")
		Debugf("a detail")
	})

	if !strings.Contains(out, "an error") || !strings.Contains(out, "a warning") {
		t.Errorf("error and warn should be logged at Warn, got %q", out)
	}
	if strings.Contains(out, "some info") || strings.Contains(out, "a detail") {
		t.Errorf("info and debug should be suppressed at Warn, got %q", out)
	}
}

func TestTimestamp_PresentUnlessJournaldStampsIt(t *testing.T) {
	saved := Level(level.Load())
	defer SetLevel(saved)
	SetLevel(Info)

	savedStamped := stamped
	defer func() { stamped = savedStamped }()

	// Anywhere but the journal, nothing else records the time, so we must.
	stamped = true
	out := capture(t, func() { Infof("hello") })
	stampedLine := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \[INFO\] hello\n$`)
	if !stampedLine.MatchString(out) {
		t.Errorf("want a timestamped line, got %q", out)
	}

	// Under systemd the journal stamps every line already; a second one would
	// only be noise.
	stamped = false
	out = capture(t, func() { Infof("hello") })
	if out != "[INFO] hello\n" {
		t.Errorf("want an unstamped line, got %q", out)
	}
}
