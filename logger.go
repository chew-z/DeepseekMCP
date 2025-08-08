package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel represents the severity level for logging
type LogLevel int

const (
	// LevelDebug is for detailed debugging information
	LevelDebug LogLevel = iota
	// LevelInfo is for general operational information
	LevelInfo
	// LevelWarn is for warning events
	LevelWarn
	// LevelError is for error events
	LevelError
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is a simple logging interface
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// SimpleLogger is a basic implementation of the Logger interface
type SimpleLogger struct {
	level  LogLevel
	logger *log.Logger
}

// NewLogger creates a new logger with the specified level
func NewLogger(level string) Logger {
	logLevel := LevelInfo // Default to Info
	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = LevelDebug
	case "INFO":
		logLevel = LevelInfo
	case "WARN":
		logLevel = LevelWarn
	case "ERROR":
		logLevel = LevelError
	}
	return &SimpleLogger{
		level:  logLevel,
		logger: log.New(os.Stderr, "", log.LstdFlags),
	}
}

// Debug logs a debug message
func (l *SimpleLogger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.log(LevelDebug, format, args...)
	}
}

// Info logs an info message
func (l *SimpleLogger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.log(LevelInfo, format, args...)
	}
}

// Warn logs a warning message
func (l *SimpleLogger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.log(LevelWarn, format, args...)
	}
}

// Error logs an error message
func (l *SimpleLogger) Error(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.log(LevelError, format, args...)
	}
}

// log formats and outputs a log message
func (l *SimpleLogger) log(level LogLevel, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	message := fmt.Sprintf(format, args...)
	l.logger.Printf("[%s] [%s] %s", timestamp, level.String(), message)
}

// Context key for the logger
type contextKey string

const loggerKey contextKey = "logger"
const configKey contextKey = "config"

// LoggerMiddleware wraps a handler with logging functionality
type LoggerMiddleware struct {
	handler  interface{} // This will be a ToolHandler in practice
	logger   Logger
	execTime time.Duration
}

// NewLoggerMiddleware creates a new logger middleware
func NewLoggerMiddleware(handler interface{}, logger Logger) *LoggerMiddleware {
	return &LoggerMiddleware{
		handler: handler,
		logger:  logger,
	}
}
