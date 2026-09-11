// Package logx provides leveled logging for node-box.
//
// Output goes to stderr so systemd/journald captures it. The level is stored
// atomically because the webhook server and the runner log from different
// goroutines.
package logx

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// Level controls how much is logged. Higher values are more verbose.
type Level int32

const (
	Silent Level = iota
	Error
	Warn
	Info
	Debug
)

// String returns the uppercase name of the level.
func (l Level) String() string {
	switch l {
	case Silent:
		return "SILENT"
	case Error:
		return "ERROR"
	case Warn:
		return "WARN"
	case Info:
		return "INFO"
	case Debug:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a level name to a Level. Unknown names are an error so
// a typo in the config surfaces instead of silently falling back to Info.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "silent":
		return Silent, nil
	case "error":
		return Error, nil
	case "warn", "warning":
		return Warn, nil
	case "info":
		return Info, nil
	case "debug":
		return Debug, nil
	default:
		return Info, fmt.Errorf("unknown log level %q (want silent/error/warn/info/debug)", s)
	}
}

var (
	level atomic.Int32

	mu  sync.Mutex
	out io.Writer = os.Stderr
)

func init() { level.Store(int32(Info)) }

// SetLevel sets the global log level.
func SetLevel(l Level) { level.Store(int32(l)) }

// CurrentLevel reports the global log level.
func CurrentLevel() Level { return Level(level.Load()) }

// SetOutput redirects log output. Used by tests.
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	out = w
}

func logf(l Level, format string, args ...any) {
	if Level(level.Load()) < l {
		return
	}
	msg := fmt.Sprintf(format, args...)
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(out, "[%s] %s\n", l, msg)
}

// Errorf logs at error level.
func Errorf(format string, args ...any) { logf(Error, format, args...) }

// Warnf logs at warn level.
func Warnf(format string, args ...any) { logf(Warn, format, args...) }

// Infof logs at info level.
func Infof(format string, args ...any) { logf(Info, format, args...) }

// Debugf logs at debug level.
func Debugf(format string, args ...any) { logf(Debug, format, args...) }
