package launch

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	smokeModuleRoot string
	smokeBinDir     string
	smokeROMPlain   string
)

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "smoke: getwd:", err)
		os.Exit(1)
	}
	smokeModuleRoot = filepath.Join(wd, "..", "..")

	smokeBinDir, err = os.MkdirTemp("", "gomeboy-smoke-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "smoke: mkdir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(smokeBinDir)

	if err := smokeBuildBinaries(); err != nil {
		fmt.Fprintln(os.Stderr, "smoke: build binaries:", err)
		os.Exit(1)
	}
	if err := smokeExtractROMs(); err != nil {
		fmt.Fprintln(os.Stderr, "smoke: extract ROMs:", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func smokeBuildBinaries() error {
	targets := map[string]string{
		"gomeboy": ".",
	}
	for name, pkg := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(smokeBinDir, name), pkg)
		cmd.Dir = smokeModuleRoot
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("go build %s: %w: %s", name, err, out)
		}
	}
	return nil
}

func smokeExtractROMs() error {
	zr, err := zip.OpenReader(filepath.Join(smokeModuleRoot, "tests", "roms.zip"))
	if err != nil {
		return err
	}
	defer zr.Close()

	pick := map[string]string{
		"blargg/cpu_instrs/individual/01-special.gb": "plain.gb",
	}
	found := map[string]bool{}
	for _, f := range zr.File {
		want, ok := pick[f.Name]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(smokeBinDir, want))
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		found[f.Name] = true
	}
	for name := range pick {
		if !found[name] {
			return fmt.Errorf("tests/roms.zip: missing %s", name)
		}
	}
	smokeROMPlain = filepath.Join(smokeBinDir, "plain.gb")
	return nil
}

type smokeWriter struct{ p *smokeProc }

func (w *smokeWriter) Write(b []byte) (int, error) {
	w.p.mu.Lock()
	defer w.p.mu.Unlock()
	return w.p.out.Write(b)
}

type smokeProc struct {
	cmd      *exec.Cmd
	mu       sync.Mutex
	out      strings.Builder
	done     chan error
	finished atomic.Bool
	started  time.Time
}

func smokeStart(t *testing.T, bin string, args []string, dir string, extraEnv []string) *smokeProc {
	t.Helper()
	p := &smokeProc{done: make(chan error, 1), started: time.Now()}
	p.cmd = exec.Command(filepath.Join(smokeBinDir, bin), args...)
	if dir != "" {
		p.cmd.Dir = dir
	}
	if len(extraEnv) > 0 {
		p.cmd.Env = append(os.Environ(), extraEnv...)
	}
	w := &smokeWriter{p: p}
	p.cmd.Stdout = w
	p.cmd.Stderr = w
	if err := p.cmd.Start(); err != nil {
		t.Fatalf("smoke: start %s: %v", bin, err)
	}
	go func() {
		err := p.cmd.Wait()
		p.finished.Store(true)
		p.done <- err
	}()
	return p
}

func (p *smokeProc) Output() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.String()
}

func (p *smokeProc) elapsed() time.Duration { return time.Since(p.started) }

func (p *smokeProc) wait(t *testing.T, timeout time.Duration) (int, string) {
	t.Helper()
	select {
	case err := <-p.done:
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("smoke: wait: %v", err)
			}
		}
		return code, p.Output()
	case <-time.After(timeout):
		p.cmd.Process.Kill()
		<-p.done
		t.Fatalf("smoke: process did not exit within %v; output so far:\n%s", timeout, p.Output())
	}
	panic("unreachable")
}

// TestSmokeHelpExitsZero and TestSmokeFailureModes only exercise flag
// parsing and early validation, which run before the GLFW driver tries to
// open a window. There is no headless binary left to smoke-test a full
// startup/shutdown/persistence cycle against in a display-less CI runner
// (gomeboy-web and gomeboy-agent were removed); GLFW-driven startup is
// exercised manually/visually instead.

func TestSmokeHelpExitsZero(t *testing.T) {
	p := smokeStart(t, "gomeboy", []string{"-h"}, "", nil)
	code, out := p.wait(t, 30*time.Second)
	if code != 0 {
		t.Fatalf("gomeboy -h: exit %d, output:\n%s", code, out)
	}
	if !strings.Contains(out, "Usage of") {
		t.Fatalf("gomeboy -h: missing usage text:\n%s", out)
	}
}

func TestSmokeFailureModes(t *testing.T) {
	missingROM := filepath.Join(t.TempDir(), "nope.gb")
	missingBoot := filepath.Join(t.TempDir(), "nope.gbr")
	saveDir := t.TempDir()

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"invalid model", []string{"-model", "bogus"}, `invalid -model "bogus"`},
		{"invalid log level", []string{"-log-level", "bogus"}, `invalid level "bogus"`},
		{"missing rom", []string{"-rom", missingROM}, "load ROM " + missingROM},
		{"missing boot rom", []string{"-rom", smokeROMPlain, "-boot", missingBoot}, "load boot ROM " + missingBoot},
		{"save flags conflict", []string{"-no-saves", "-save-dir", saveDir}, "conflicts with -save-dir"},
		{"unknown flag", []string{"-bogus"}, "flag provided but not defined: -bogus"},
		{"unknown driver", []string{"-driver", "nope", "-rom", smokeROMPlain}, `unknown display driver "nope"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := smokeStart(t, "gomeboy", tc.args, "", nil)
			code, out := p.wait(t, 30*time.Second)
			if code == 0 {
				t.Fatalf("gomeboy %v: exit 0; output:\n%s", tc.args, out)
			}
			if p.elapsed() > 10*time.Second {
				t.Fatalf("gomeboy %v: took %v, expected a fast contextual failure", tc.args, p.elapsed())
			}
			if strings.Contains(out, "panic:") {
				t.Fatalf("gomeboy %v: panic output:\n%s", tc.args, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("gomeboy %v: exit %d, want %q in output:\n%s", tc.args, code, tc.want, out)
			}
		})
	}
}
