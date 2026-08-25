package launch

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

var (
	smokeModuleRoot string
	smokeBinDir     string
	smokeROMPlain   string
	smokeROMBattery string
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
		"gomeboy":       ".",
		"gomeboy-web":   "./cmd/gomeboy-web",
		"gomeboy-agent": "./cmd/gomeboy-agent",
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
		"blargg/cpu_instrs/individual/01-special.gb":  "plain.gb",
		"blargg/dmg_sound/rom_singles/01-registers.gb": "battery.gb",
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
	smokeROMBattery = filepath.Join(smokeBinDir, "battery.gb")
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

func (p *smokeProc) exited() bool { return p.finished.Load() }

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

func smokePortOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func smokeFreeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("smoke: reserve port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func smokeWaitForPort(t *testing.T, addr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if smokePortOpen(addr) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return smokePortOpen(addr)
}

func TestSmokeHelpExitsZero(t *testing.T) {
	for _, bin := range []string{"gomeboy", "gomeboy-web", "gomeboy-agent"} {
		t.Run(bin, func(t *testing.T) {
			p := smokeStart(t, bin, []string{"-h"}, "", nil)
			code, out := p.wait(t, 30*time.Second)
			if code != 0 {
				t.Fatalf("%s -h: exit %d, output:\n%s", bin, code, out)
			}
			if !strings.Contains(out, "Usage of") {
				t.Fatalf("%s -h: missing usage text:\n%s", bin, out)
			}
		})
	}
}

func TestSmokeDefaultDriverNeverBindsWeb(t *testing.T) {
	addr := smokeFreeAddr(t)
	p := smokeStart(t, "gomeboy", []string{"-rom", smokeROMPlain, "-web-listen", addr}, "", []string{"DISPLAY=:99"})

	open := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if smokePortOpen(addr) {
			open = true
			break
		}
		if p.exited() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if open {
		p.cmd.Process.Kill()
		t.Fatal("default (auto) driver bound the web listen address")
	}
	code, out := p.wait(t, 30*time.Second)
	if code == 0 {
		t.Fatalf("default headless startup exited 0; output:\n%s", out)
	}
	if smokePortOpen(addr) {
		t.Fatal("web listen address still open after exit")
	}
}

func TestSmokeWebDriverBindsAndShutsDown(t *testing.T) {
	cases := []struct {
		name string
		bin  string
		args []string
	}{
		{"gomeboy-driver-web", "gomeboy", []string{"-driver", "web"}},
		{"gomeboy-web", "gomeboy-web", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr := smokeFreeAddr(t)
			args := append(append([]string{}, tc.args...), "-rom", smokeROMPlain, "-web-listen", addr)
			p := smokeStart(t, tc.bin, args, "", nil)
			if !smokeWaitForPort(t, addr, 15*time.Second) {
				p.cmd.Process.Kill()
				t.Fatalf("%s did not bind %s; output:\n%s", tc.bin, addr, p.Output())
			}
			time.Sleep(time.Second)
			if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatalf("signal: %v", err)
			}
			code, out := p.wait(t, 10*time.Second)
			if code == 0 {
				t.Fatalf("%s exited 0 after SIGTERM; output:\n%s", tc.bin, out)
			}
			if smokePortOpen(addr) {
				t.Fatal("web listen address still open after SIGTERM")
			}
		})
	}
}

func TestSmokeAgentBindsAndShutsDown(t *testing.T) {
	addr := smokeFreeAddr(t)
	p := smokeStart(t, "gomeboy-agent", []string{"-rom", smokeROMPlain, "-web-listen", addr}, "", nil)
	if !smokeWaitForPort(t, addr, 15*time.Second) {
		p.cmd.Process.Kill()
		t.Fatalf("agent did not bind %s; output:\n%s", addr, p.Output())
	}
	time.Sleep(time.Second)
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal: %v", err)
	}
	code, out := p.wait(t, 10*time.Second)
	if code != 0 {
		t.Fatalf("agent exit %d after SIGINT; output:\n%s", code, out)
	}
	if smokePortOpen(addr) {
		t.Fatal("web listen address still open after SIGINT")
	}
}

func TestSmokeFailureModes(t *testing.T) {
	missingROM := filepath.Join(t.TempDir(), "nope.gb")
	missingBoot := filepath.Join(t.TempDir(), "nope.gbr")
	saveDir := t.TempDir()

	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer holder.Close()
	heldAddr := holder.Addr().String()

	cases := []struct {
		name string
		bin  string
		args []string
		want string
	}{
		{"invalid model", "gomeboy", []string{"-model", "bogus"}, `invalid -model "bogus"`},
		{"invalid log level", "gomeboy", []string{"-log-level", "bogus"}, `invalid level "bogus"`},
		{"missing rom", "gomeboy", []string{"-rom", missingROM}, "load ROM " + missingROM},
		{"missing boot rom", "gomeboy", []string{"-rom", smokeROMPlain, "-boot", missingBoot}, "load boot ROM " + missingBoot},
		{"save flags conflict", "gomeboy", []string{"-no-saves", "-save-dir", saveDir}, "conflicts with -save-dir"},
		{"unknown flag", "gomeboy", []string{"-bogus"}, "flag provided but not defined: -bogus"},
		{"unknown driver", "gomeboy", []string{"-driver", "nope", "-rom", smokeROMPlain}, `unknown display driver "nope"`},
		{"web address in use", "gomeboy-web", []string{"-rom", smokeROMPlain, "-web-listen", heldAddr}, "address already in use"},
		{"agent zero fps", "gomeboy-agent", []string{"-fps", "0", "-rom", smokeROMPlain}, "-fps must be positive, got 0"},
		{"agent missing rom", "gomeboy-agent", nil, "-rom is required"},
		{"agent save flag", "gomeboy-agent", []string{"-rom", smokeROMPlain, "-save-dir", saveDir}, "flag provided but not defined: -save-dir"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := smokeStart(t, tc.bin, tc.args, "", nil)
			code, out := p.wait(t, 30*time.Second)
			if code == 0 {
				t.Fatalf("%s %v: exit 0; output:\n%s", tc.bin, tc.args, out)
			}
			if p.elapsed() > 10*time.Second {
				t.Fatalf("%s %v: took %v, expected a fast contextual failure", tc.bin, tc.args, p.elapsed())
			}
			if strings.Contains(out, "panic:") {
				t.Fatalf("%s %v: panic output:\n%s", tc.bin, tc.args, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("%s %v: exit %d, want %q in output:\n%s", tc.bin, tc.args, code, tc.want, out)
			}
		})
	}
}

func TestSmokeMissingCheatsTolerated(t *testing.T) {
	addr := smokeFreeAddr(t)
	missingCheats := filepath.Join(t.TempDir(), "nope.cheats")
	p := smokeStart(t, "gomeboy-web", []string{"-rom", smokeROMPlain, "-cheats", missingCheats, "-web-listen", addr}, "", nil)
	if !smokeWaitForPort(t, addr, 15*time.Second) {
		p.cmd.Process.Kill()
		t.Fatalf("missing cheats file prevented startup; output:\n%s", p.Output())
	}
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	p.wait(t, 10*time.Second)
}

func TestSmokePersistence(t *testing.T) {
	t.Run("default working directory", func(t *testing.T) {
		dir := t.TempDir()
		p := smokeStart(t, "gomeboy-web", []string{"-rom", smokeROMBattery}, dir, nil)
		time.Sleep(2 * time.Second)
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("signal: %v", err)
		}
		p.wait(t, 10*time.Second)
		saves, _ := filepath.Glob(filepath.Join(dir, "*.sav"))
		if len(saves) != 1 {
			t.Fatalf("want exactly one .sav in the working directory, got %v", saves)
		}
	})
	t.Run("save dir", func(t *testing.T) {
		dir := t.TempDir()
		savesDir := filepath.Join(dir, "saves")
		if err := os.Mkdir(savesDir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := smokeStart(t, "gomeboy-web", []string{"-rom", smokeROMBattery, "-save-dir", savesDir}, dir, nil)
		time.Sleep(2 * time.Second)
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("signal: %v", err)
		}
		p.wait(t, 10*time.Second)
		inDir, _ := filepath.Glob(filepath.Join(savesDir, "*.sav"))
		inCwd, _ := filepath.Glob(filepath.Join(dir, "*.sav"))
		if len(inDir) != 1 || len(inCwd) != 0 {
			t.Fatalf("want one .sav in %s and none in %s, got %v / %v", savesDir, dir, inDir, inCwd)
		}
	})
	t.Run("no saves", func(t *testing.T) {
		dir := t.TempDir()
		p := smokeStart(t, "gomeboy-web", []string{"-rom", smokeROMBattery, "-no-saves"}, dir, nil)
		time.Sleep(2 * time.Second)
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("signal: %v", err)
		}
		p.wait(t, 10*time.Second)
		saves, _ := filepath.Glob(filepath.Join(dir, "*.sav"))
		if len(saves) != 0 {
			t.Fatalf("-no-saves left save files: %v", saves)
		}
	})
	t.Run("agent diskless", func(t *testing.T) {
		dir := t.TempDir()
		p := smokeStart(t, "gomeboy-agent", []string{"-rom", smokeROMBattery}, dir, nil)
		time.Sleep(2 * time.Second)
		if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatalf("signal: %v", err)
		}
		code, out := p.wait(t, 10*time.Second)
		if code != 0 {
			t.Fatalf("agent exit %d; output:\n%s", code, out)
		}
		saves, _ := filepath.Glob(filepath.Join(dir, "*.sav"))
		if len(saves) != 0 {
			t.Fatalf("agent left save files: %v", saves)
		}
	})
}

var smokeSummaryRe = regexp.MustCompile(`gomeboy-agent: ROM .+ loaded, web hub on .+`)

func TestSmokeLogging(t *testing.T) {
	t.Run("agent single summary", func(t *testing.T) {
		addr := smokeFreeAddr(t)
		p := smokeStart(t, "gomeboy-agent", []string{"-rom", smokeROMPlain, "-web-listen", addr}, "", nil)
		if !smokeWaitForPort(t, addr, 15*time.Second) {
			p.cmd.Process.Kill()
			t.Fatalf("agent did not bind %s; output:\n%s", addr, p.Output())
		}
		time.Sleep(2 * time.Second)
		if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatalf("signal: %v", err)
		}
		code, out := p.wait(t, 10*time.Second)
		if code != 0 {
			t.Fatalf("agent exit %d; output:\n%s", code, out)
		}
		if matches := smokeSummaryRe.FindAllString(out, -1); len(matches) != 1 {
			t.Fatalf("want exactly one startup summary line, got %d:\n%s", len(matches), out)
		}
		if out != "" && len(strings.Split(strings.TrimSpace(out), "\n")) > 3 {
			t.Fatalf("agent log grew beyond the startup summary:\n%s", out)
		}
	})
	t.Run("web bounded output", func(t *testing.T) {
		addr := smokeFreeAddr(t)
		p := smokeStart(t, "gomeboy-web", []string{"-rom", smokeROMPlain, "-web-listen", addr}, "", nil)
		if !smokeWaitForPort(t, addr, 15*time.Second) {
			p.cmd.Process.Kill()
			t.Fatalf("web did not bind %s; output:\n%s", addr, p.Output())
		}
		time.Sleep(2 * time.Second)
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("signal: %v", err)
		}
		code, out := p.wait(t, 10*time.Second)
		if code == 0 {
			t.Fatalf("web exit 0 after SIGTERM")
		}
		if out != "" && len(strings.Split(strings.TrimSpace(out), "\n")) > 2 {
			t.Fatalf("web log grew per frame:\n%s", out)
		}
	})
}

func TestSmokeHygiene(t *testing.T) {
	addr := smokeFreeAddr(t)
	dir := t.TempDir()
	p := smokeStart(t, "gomeboy-web", []string{"-rom", smokeROMPlain, "-web-listen", addr}, dir, nil)
	if !smokeWaitForPort(t, addr, 15*time.Second) {
		p.cmd.Process.Kill()
		t.Fatalf("web did not bind %s", addr)
	}
	time.Sleep(time.Second)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	code, _ := p.wait(t, 10*time.Second)
	if code == 0 {
		t.Fatal("web exit 0 after SIGTERM")
	}
	if smokePortOpen(addr) {
		t.Fatal("web listen address still open after shutdown")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("working directory not clean after shutdown: %v", names)
	}
	for _, pattern := range []string{filepath.Join(smokeModuleRoot, "*.sav"), filepath.Join(smokeModuleRoot, "*.state")} {
		if hits, _ := filepath.Glob(pattern); len(hits) > 0 {
			t.Fatalf("smoke runs leaked files into the module root: %v", hits)
		}
	}
}
