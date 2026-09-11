// Package logger provides a structured logging system with configurable levels
// to replace the scattered log.Printf statements throughout the application.
package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel represents the severity level of log messages
type LogLevel int

const (
	// SILENT suppresses all log output
	SILENT LogLevel = iota
	// ERROR shows only error messages
	ERROR
	// WARN shows warnings and errors
	WARN
	// INFO shows informational messages, warnings, and errors (default)
	INFO
	// DEBUG shows all messages including debug information
	DEBUG
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case SILENT:
		return "SILENT"
	case ERROR:
		return "ERROR"
	case WARN:
		return "WARN"
	case INFO:
		return "INFO"
	case DEBUG:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging with configurable levels
type Logger struct {
	level     LogLevel
	showTime  bool
	showLevel bool
	prefix    string
}

// Global logger instance
var globalLogger = &Logger{
	level:     INFO,
	showTime:  false,
	showLevel: true,
	prefix:    "",
}

// SetLevel sets the global logging level
func SetLevel(level LogLevel) {
	globalLogger.level = level
}

// SetShowTime enables or disables timestamp in log messages
func SetShowTime(show bool) {
	globalLogger.showTime = show
}

// SetShowLevel enables or disables log level in messages
func SetShowLevel(show bool) {
	globalLogger.showLevel = show
}

// SetPrefix sets a prefix for all log messages
func SetPrefix(prefix string) {
	globalLogger.prefix = prefix
}

// ParseLevel parses a string into a LogLevel
func ParseLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "SILENT":
		return SILENT
	case "ERROR":
		return ERROR
	case "WARN", "WARNING":
		return WARN
	case "INFO":
		return INFO
	case "DEBUG":
		return DEBUG
	default:
		return INFO
	}
}

// formatMessage formats a log message with optional timestamp, level, and prefix
func (l *Logger) formatMessage(level LogLevel, msg string) string {
	var parts []string

	if l.showTime {
		parts = append(parts, time.Now().Format("15:04:05"))
	}

	if l.showLevel {
		parts = append(parts, fmt.Sprintf("[%s]", level.String()))
	}

	if l.prefix != "" {
		parts = append(parts, l.prefix)
	}

	parts = append(parts, msg)
	return strings.Join(parts, " ")
}

// shouldLog checks if a message should be logged based on the current level
func (l *Logger) shouldLog(level LogLevel) bool {
	return l.level >= level
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	if l.shouldLog(ERROR) {
		msg := fmt.Sprintf(format, args...)
		log.Print(l.formatMessage(ERROR, msg))
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.shouldLog(WARN) {
		msg := fmt.Sprintf(format, args...)
		log.Print(l.formatMessage(WARN, msg))
	}
}

// Info logs an informational message
func (l *Logger) Info(format string, args ...interface{}) {
	if l.shouldLog(INFO) {
		msg := fmt.Sprintf(format, args...)
		log.Print(l.formatMessage(INFO, msg))
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.shouldLog(DEBUG) {
		msg := fmt.Sprintf(format, args...)
		log.Print(l.formatMessage(DEBUG, msg))
	}
}

// Fatal logs an error message and exits the program
func (l *Logger) Fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Fatal(l.formatMessage(ERROR, msg))
}

// Global convenience functions

// Error logs an error message using the global logger
func Error(format string, args ...interface{}) {
	globalLogger.Error(format, args...)
}

// Warn logs a warning message using the global logger
func Warn(format string, args ...interface{}) {
	globalLogger.Warn(format, args...)
}

// Info logs an informational message using the global logger
func Info(format string, args ...interface{}) {
	globalLogger.Info(format, args...)
}

// Debug logs a debug message using the global logger
func Debug(format string, args ...interface{}) {
	globalLogger.Debug(format, args...)
}

// Fatal logs an error message and exits the program using the global logger
func Fatal(format string, args ...interface{}) {
	globalLogger.Fatal(format, args...)
}

// InitFromEnv initializes the logger from environment variables
func InitFromEnv() {
	// Check for LOG_LEVEL environment variable
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		SetLevel(ParseLevel(level))
	}

	// Check for LOG_TIME environment variable
	if showTime := os.Getenv("LOG_TIME"); showTime == "true" || showTime == "1" {
		SetShowTime(true)
	}

	// Check for LOG_PREFIX environment variable
	if prefix := os.Getenv("LOG_PREFIX"); prefix != "" {
		SetPrefix(prefix)
	}
}
