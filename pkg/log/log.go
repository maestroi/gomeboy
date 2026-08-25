package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level is a log severity. Higher means more severe.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	ErrorLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case ErrorLevel:
		return "error"
	}
	return fmt.Sprintf("Level(%d)", int(l))
}

// ParseLevel parses a level name. It accepts "debug", "info", and "error"
// case-insensitively and returns an actionable error for anything else.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "error":
		return ErrorLevel, nil
	}
	return 0, fmt.Errorf("log: invalid level %q: must be one of debug, info, error", s)
}

// Logger is the stable logging interface used by existing callers.
type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Fatal(str string)
}

// ContextualLogger is a Logger that can carry key/value context on every
// record it emits.
type ContextualLogger interface {
	Logger
	With(key, value interface{}) ContextualLogger
}

// osStderr resolves the process stderr at write time so redirection in tests
// and the process's real stderr stay consistent.
type osStderr struct{}

func (osStderr) Write(p []byte) (int, error) { return os.Stderr.Write(p) }

type logger struct {
	mu     *sync.Mutex
	out    io.Writer
	level  Level
	fields []string
}

// New returns the default logger: it writes to the process stderr at info
// level, so debug output is suppressed unless the level is raised.
func New() Logger {
	return &logger{mu: &sync.Mutex{}, out: osStderr{}, level: InfoLevel}
}

// NewWithWriter returns a logger that writes to w at level. It validates its
// inputs so behavior is unit-testable without global process state.
func NewWithWriter(w io.Writer, level Level) (ContextualLogger, error) {
	if w == nil {
		return nil, fmt.Errorf("log: writer must not be nil")
	}
	if level < DebugLevel || level > ErrorLevel {
		return nil, fmt.Errorf("log: level %v out of range: must be one of debug, info, error", level)
	}
	return &logger{mu: &sync.Mutex{}, out: w, level: level}, nil
}

// With returns a logger that appends key=value to every record it emits.
// The receiver is left unchanged.
func (l *logger) With(key, value interface{}) ContextualLogger {
	fields := make([]string, 0, len(l.fields)+1)
	fields = append(fields, l.fields...)
	fields = append(fields, fmt.Sprintf("%v=%v", key, value))
	return &logger{mu: l.mu, out: l.out, level: l.level, fields: fields}
}

func (l *logger) Infof(format string, args ...interface{}) {
	l.log(InfoLevel, "INFO", format, args...)
}

func (l *logger) Errorf(format string, args ...interface{}) {
	l.log(ErrorLevel, "ERROR", format, args...)
}

func (l *logger) Debugf(format string, args ...interface{}) {
	l.log(DebugLevel, "DEBUG", format, args...)
}

// Fatal emits a FATAL record and exits the process. It is a temporary
// compatibility boundary; do not add new call sites.
func (l *logger) Fatal(str string) {
	l.write("FATAL", str)
	os.Exit(1)
}

func (l *logger) log(level Level, label, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	l.write(label, formatMessage(format, args...))
}

// formatMessage renders a format string with its arguments. The local copy
// of format keeps the public *f methods out of vet's printf-wrapper
// inference, matching pre-facade behavior.
func formatMessage(format string, args ...interface{}) string {
	f := format
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}

func (l *logger) write(label, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var b strings.Builder
	b.WriteString(time.Now().Format("2006-01-02T15:04:05.000Z07:00"))
	b.WriteString("\t[" + label + "]\t")
	b.WriteString(msg)
	for _, f := range l.fields {
		b.WriteByte('\t')
		b.WriteString(f)
	}
	b.WriteByte('\n')
	io.WriteString(l.out, b.String())
}

var l = New()

func Infof(format string, args ...interface{}) {
	l.Infof(format, args...)
}

func Errorf(format string, args ...interface{}) {
	l.Errorf(format, args...)
}

func Debugf(format string, args ...interface{}) {
	l.Debugf(format, args...)
}

func Fatal(str string) {
	l.Fatal(str)
}
