// Package logx provides leveled logging for node-box.
//
// Output goes to stderr so systemd/journald captures it. The level is stored
// atomically because the webhook server and the runner log from different
// goroutines.
package logx

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	mu    sync.Mutex

	// stamped reports whether each line should carry its own timestamp.
	//
	// systemd sets JOURNAL_STREAM when stderr is the journal, and the journal
	// already records the time of every line, so stamping there would only
	// duplicate it. Everywhere else — stderr redirected to a file, a container
	// log, a terminal — nothing else records when something happened, and a log
	// without times is close to useless for working out what a daemon did.
	stamped = os.Getenv("JOURNAL_STREAM") == ""
)

// timeFormat matches the format the status command prints, so timestamps from
// the log and from `node-box status` can be compared directly.
const timeFormat = "2006-01-02 15:04:05"

func init() { level.Store(int32(Info)) }

// SetLevel sets the global log level.
func SetLevel(l Level) { level.Store(int32(l)) }

func logf(l Level, format string, args ...any) {
	if Level(level.Load()) < l {
		return
	}
	msg := fmt.Sprintf(format, args...)

	mu.Lock()
	defer mu.Unlock()
	if stamped {
		fmt.Fprintf(os.Stderr, "%s [%s] %s\n", time.Now().Format(timeFormat), l, msg)
		return
	}
	fmt.Fprintf(os.Stderr, "[%s] %s\n", l, msg)
}

// Errorf logs at error level.
func Errorf(format string, args ...any) { logf(Error, format, args...) }

// Warnf logs at warn level.
func Warnf(format string, args ...any) { logf(Warn, format, args...) }

// Infof logs at info level.
func Infof(format string, args ...any) { logf(Info, format, args...) }

// Debugf logs at debug level.
func Debugf(format string, args ...any) { logf(Debug, format, args...) }
