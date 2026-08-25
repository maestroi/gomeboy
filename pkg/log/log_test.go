package log

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// recordRe defines the observable line format:
// <timestamp>\t[LEVEL]\t<formatted message>[\tkey=value...]
// Timestamp is RFC3339 with millisecond precision; level labels are stable.
var recordRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}(?:Z|[+-]\d{2}:\d{2}))\t\[(DEBUG|INFO|ERROR|FATAL)\]\t(.+)$`)

func newTestLogger(t *testing.T, level Level) (*bytes.Buffer, ContextualLogger) {
	t.Helper()
	var buf bytes.Buffer
	lg, err := NewWithWriter(&buf, level)
	if err != nil {
		t.Fatalf("NewWithWriter: %v", err)
	}
	return &buf, lg
}

func lines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	s := strings.TrimSpace(buf.String())
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// LOG-LEVELS: debug is suppressed at info and emitted at debug;
// info and error filtering follows a deterministic threshold:
// a record is emitted iff its severity is >= the configured level.
func TestLevelFiltering(t *testing.T) {
	cases := []struct {
		name  string
		level Level
		want  map[string]bool // method -> emitted
	}{
		{"debug level emits all", DebugLevel, map[string]bool{"Debugf": true, "Infof": true, "Errorf": true}},
		{"info level suppresses debug", InfoLevel, map[string]bool{"Debugf": false, "Infof": true, "Errorf": true}},
		{"error level suppresses debug and info", ErrorLevel, map[string]bool{"Debugf": false, "Infof": false, "Errorf": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf, lg := newTestLogger(t, tc.level)
			lg.Debugf("dbg %d", 1)
			lg.Infof("inf %d", 2)
			lg.Errorf("err %d", 3)
			for method, want := range tc.want {
				msg := map[string]string{"Debugf": "dbg 1", "Infof": "inf 2", "Errorf": "err 3"}[method]
				got := strings.Contains(buf.String(), msg)
				if got != want {
					t.Errorf("%s at level %s: emitted=%v want=%v (output %q)", method, tc.level, got, want, buf.String())
				}
			}
		})
	}
}

// LOG-FORMAT: each captured line carries a timestamp, a stable level label,
// and the formatted message with its supplied arguments.
func TestFormat(t *testing.T) {
	buf, lg := newTestLogger(t, DebugLevel)
	lg.Infof("hello %s, attempt %d", "world", 3)
	lglines := lines(t, buf)
	if len(lglines) != 1 {
		t.Fatalf("want 1 line, got %d: %q", len(lglines), buf.String())
	}
	m := recordRe.FindStringSubmatch(lglines[0])
	if m == nil {
		t.Fatalf("line %q does not match <timestamp>\\t[LEVEL]\\t<message>", lglines[0])
	}
	if m[2] != "INFO" {
		t.Errorf("level label = %q, want INFO", m[2])
	}
	if m[3] != "hello world, attempt 3" {
		t.Errorf("message = %q, want %q", m[3], "hello world, attempt 3")
	}

	buf.Reset()
	lg.Errorf("boom %v", errBoom{})
	m = recordRe.FindStringSubmatch(lines(t, buf)[0])
	if m == nil {
		t.Fatalf("error line %q does not match format", lines(t, buf)[0])
	}
	if m[2] != "ERROR" || m[3] != "boom oops" {
		t.Errorf("error line = %q, want [ERROR] boom oops", lines(t, buf)[0])
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "oops" }

// LOG-FORMAT: context supplied via With is appended as key=value fields
// after the message, and does not leak back to the parent logger.
func TestContext(t *testing.T) {
	buf, lg := newTestLogger(t, InfoLevel)
	lg.With("user", "alice").With("attempt", 2).Infof("login failed")
	first := lines(t, buf)[0]
	if !strings.Contains(first, "login failed") {
		t.Errorf("line %q missing message", first)
	}
	if !strings.Contains(first, "user=alice") {
		t.Errorf("line %q missing context user=alice", first)
	}
	if !strings.Contains(first, "attempt=2") {
		t.Errorf("line %q missing context attempt=2", first)
	}
	if m := recordRe.FindStringSubmatch(first); m == nil {
		t.Errorf("context line %q does not match <timestamp>\\t[LEVEL]\\t<message>\\t<fields>", first)
	}

	lg.Infof("no context here")
	second := lines(t, buf)[1]
	if strings.Contains(second, "user=") || strings.Contains(second, "attempt=") {
		t.Errorf("context leaked to parent logger: %q", second)
	}
	if !strings.HasSuffix(second, "no context here") {
		t.Errorf("parent line %q missing plain message", second)
	}
}

// LOG-FORMAT / destination: the default package logger writes to the process
// stderr at write time and never to stdout.
func TestDefaultDestination(t *testing.T) {
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	Infof("dest %s", "check")
	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var outBuf, errBuf bytes.Buffer
	io.Copy(&outBuf, outR)
	io.Copy(&errBuf, errR)
	if outBuf.Len() != 0 {
		t.Errorf("stdout received %q; normal logs must not write to stdout", outBuf.String())
	}
	if !strings.Contains(errBuf.String(), "dest check") {
		t.Errorf("stderr missing default log; got %q", errBuf.String())
	}
	if m := recordRe.FindStringSubmatch(strings.TrimSpace(errBuf.String())); m == nil {
		t.Errorf("default log line %q does not match format", errBuf.String())
	}
}

// LOG-CONFIG: level names are explicit and validated; unknown names return
// an actionable error naming the valid levels.
func TestParseLevel(t *testing.T) {
	for name, want := range map[string]Level{
		"debug":  DebugLevel,
		"info":   InfoLevel,
		"error":  ErrorLevel,
		"DEBUG":  DebugLevel,
		" Info ": InfoLevel,
	} {
		got, err := ParseLevel(name)
		if err != nil {
			t.Errorf("ParseLevel(%q) unexpected error: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", name, got, want)
		}
	}
	for _, bad := range []string{"", "verbose", "warn", "fatal", "Debug2"} {
		_, err := ParseLevel(bad)
		if err == nil {
			t.Errorf("ParseLevel(%q): expected error, got nil", bad)
			continue
		}
		for _, valid := range []string{"debug", "info", "error"} {
			if !strings.Contains(err.Error(), valid) {
				t.Errorf("ParseLevel(%q) error %q is not actionable: must mention %q", bad, err, valid)
			}
		}
	}
}

// LOG-CONFIG: the constructor validates its inputs instead of silently
// accepting them.
func TestNewWithWriterValidation(t *testing.T) {
	if _, err := NewWithWriter(nil, InfoLevel); err == nil {
		t.Error("NewWithWriter(nil, InfoLevel): expected error, got nil")
	}
	if _, err := NewWithWriter(&bytes.Buffer{}, Level(42)); err == nil {
		t.Error("NewWithWriter(buf, Level(42)): expected error, got nil")
	}
	if _, err := NewWithWriter(&bytes.Buffer{}, Level(-1)); err == nil {
		t.Error("NewWithWriter(buf, Level(-1)): expected error, got nil")
	}
}

// LOG-COMPAT: the pre-existing package-level and Logger format methods keep
// their signatures and behavior.
func TestCompatAPI(t *testing.T) {
	var _ Logger = New()
	var _ Logger = NewNullLogger()

	var lg Logger = New()
	lg.Infof("%s", "a")
	lg.Errorf("%s", "b")
	lg.Debugf("%s", "c")

	Infof("compat %d", 1)
	Errorf("compat %d", 2)
	Debugf("compat %d", 3)
}
