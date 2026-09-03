package launch

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/serial/accessories"
	"github.com/maestroi/gomeboy/internal/types"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/gomeboy/pkg/log"
	"github.com/maestroi/gomeboy/pkg/utils"
)

var (
	romOnce sync.Once
	romErr  error
	// pkgDir is the package directory at test start, so ROM paths stay
	// valid even after a subtest calls t.Chdir.
	pkgDir = func() string {
		dir, err := os.Getwd()
		if err != nil {
			return "."
		}
		return dir
	}()
)

// testROM returns the absolute path of a known-good test ROM. The first call
// extracts tests/roms.zip into tests/roms, which is gitignored.
func testROM(t *testing.T) string {
	t.Helper()
	romOnce.Do(func() {
		romErr = utils.Unzip(
			filepath.Join(pkgDir, "..", "..", "tests", "roms.zip"),
			filepath.Join(pkgDir, "..", "..", "tests", "roms"),
		)
	})
	if romErr != nil {
		t.Fatalf("extracting test ROMs: %v", romErr)
	}
	return filepath.Join(pkgDir, "..", "..", "tests", "roms", "little-things-gb", "firstwhite.gb")
}

// batteryROM returns a copy of the test ROM flagged as a cartridge with
// battery-backed RAM (MBC3RAMBATT) so the emulator performs save file I/O.
func batteryROM(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(testROM(t))
	if err != nil {
		t.Fatalf("reading test ROM: %v", err)
	}
	data[0x147] = 0x13 // MBC3RAMBATT
	dst := filepath.Join(dir, "battery.gb")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("writing battery ROM: %v", err)
	}
	return dst
}

// newFlagSet builds a flag set matching the option profile of the named
// binary: the agent registers only the core options, the desktop and web
// binaries also register the persistence options and their driver default.
func newFlagSet(t *testing.T, profile string) *flag.FlagSet {
	t.Helper()
	fs := flag.NewFlagSet(profile, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	Register(fs)
	switch profile {
	case "desktop":
		RegisterSaves(fs)
		RegisterDriver(fs, "auto")
	case "web":
		RegisterSaves(fs)
		RegisterDriver(fs, "web")
	}
	return fs
}

func TestParseValid(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Options
	}{
		{
			name: "defaults",
			args: nil,
			want: Options{Model: gomeboy.ModelAuto, LogLevel: log.InfoLevel},
		},
		{
			name: "all core flags",
			args: []string{
				"-rom", "game.gb",
				"-boot", "boot.gbr",
				"-model", "DMG",
				"-printer",
				"-cheats", "cheats.txt",
				"-log-level", "debug",
				"-pprof", "127.0.0.1:6060",
			},
			want: Options{
				ROM:       "game.gb",
				BootROM:   "boot.gbr",
				Model:     gomeboy.ModelDMG,
				Printer:   true,
				Cheats:    "cheats.txt",
				LogLevel:  log.DebugLevel,
				PProfAddr: "127.0.0.1:6060",
			},
		},
		{
			name: "model is case-insensitive",
			args: []string{"-model", "cgb"},
			want: Options{Model: gomeboy.ModelCGB, LogLevel: log.InfoLevel},
		},
		{
			name: "log level is case-insensitive",
			args: []string{"-log-level", "ERROR"},
			want: Options{Model: gomeboy.ModelAuto, LogLevel: log.ErrorLevel},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(newFlagSet(t, "agent"), tc.args)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if *got != tc.want {
				t.Errorf("Parse = %+v, want %+v", *got, tc.want)
			}
		})
	}
}

func TestPerBinaryDefaults(t *testing.T) {
	cases := []struct {
		name   string
		driver string
	}{
		{"desktop", "auto"},
		{"web", "web"},
		{"agent", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(newFlagSet(t, tc.name), nil)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			want := Options{Model: gomeboy.ModelAuto, LogLevel: log.InfoLevel, Driver: tc.driver}
			if *got != want {
				t.Errorf("defaults = %+v, want %+v", *got, want)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		args    []string
		wantSub string
	}{
		{"invalid model", "agent", []string{"-model", "bogus"}, `invalid -model "bogus"`},
		{"empty model", "agent", []string{"-model", ""}, `invalid -model ""`},
		{"invalid log level", "agent", []string{"-log-level", "warn"}, `invalid level "warn"`},
		{"pprof without port", "agent", []string{"-pprof", "6060"}, `invalid -pprof address "6060"`},
		{"pprof with extra colon", "agent", []string{"-pprof", "a:b:c"}, `invalid -pprof address "a:b:c"`},
		{"conflicting persistence", "desktop", []string{"-no-saves", "-save-dir", "saves"}, `-no-saves conflicts with -save-dir "saves"`},
		{"unknown flag", "agent", []string{"-bogus"}, "flag provided but not defined: -bogus"},
		{"invalid bool", "agent", []string{"-printer=notabool"}, `invalid boolean value "notabool"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(newFlagSet(t, tc.profile), tc.args)
			if err == nil {
				t.Fatalf("Parse: expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

func TestCheatsPath(t *testing.T) {
	cases := []struct {
		name string
		o    Options
		want string
	}{
		{"explicit path wins", Options{ROM: "game.gb", Cheats: "explicit.txt"}, "explicit.txt"},
		{"derived from ROM", Options{ROM: "/dir/game.gb"}, "game.cheats"},
		{"derived from extensionless ROM", Options{ROM: "game"}, "game.cheats"},
		{"no ROM", Options{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.o.CheatsPath(); got != tc.want {
				t.Errorf("CheatsPath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCoreOptionsSaveBehavior(t *testing.T) {
	t.Run("defaults persist saves in the working directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		romPath := batteryROM(t, dir)
		o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", romPath})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		opts, err := o.CoreOptions()
		if err != nil {
			t.Fatalf("CoreOptions: %v", err)
		}
		gb := gameboy.NewGameBoy(opts...)
		if err := gb.LoadROM(romPath); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "battery.sav")); err != nil {
			t.Errorf("expected battery.sav in the working directory: %v", err)
		}
	})

	t.Run("no-saves disables save I/O", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		romPath := batteryROM(t, dir)
		o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", romPath, "-no-saves"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		opts, err := o.CoreOptions()
		if err != nil {
			t.Fatalf("CoreOptions: %v", err)
		}
		gb := gameboy.NewGameBoy(opts...)
		if err := gb.LoadROM(romPath); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "battery.sav")); !os.IsNotExist(err) {
			t.Errorf("expected no battery.sav with -no-saves, stat err = %v", err)
		}
	})

	t.Run("save-dir redirects saves", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		saveDir := filepath.Join(dir, "saves")
		if err := os.MkdirAll(saveDir, 0o755); err != nil {
			t.Fatalf("creating save dir: %v", err)
		}
		romPath := batteryROM(t, dir)
		o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", romPath, "-save-dir", saveDir})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		opts, err := o.CoreOptions()
		if err != nil {
			t.Fatalf("CoreOptions: %v", err)
		}
		gb := gameboy.NewGameBoy(opts...)
		if err := gb.LoadROM(romPath); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
		if _, err := os.Stat(filepath.Join(saveDir, "battery.sav")); err != nil {
			t.Errorf("expected battery.sav in the save dir: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "battery.sav")); !os.IsNotExist(err) {
			t.Errorf("expected no battery.sav in the working directory, stat err = %v", err)
		}
	})
}

func TestCoreOptionsModel(t *testing.T) {
	rom := testROM(t)
	cases := []struct {
		flag string
		want types.Model
	}{
		{"sgb2", types.SGB2},
		{"cgb", types.CGBABC},
		{"dmg", types.DMGABC},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", rom, "-model", tc.flag})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			opts, err := o.CoreOptions()
			if err != nil {
				t.Fatalf("CoreOptions: %v", err)
			}
			gb := gameboy.NewGameBoy(opts...)
			if err := gb.LoadROM(rom); err != nil {
				t.Fatalf("LoadROM: %v", err)
			}
			if got := gb.Bus.Model(); got != tc.want {
				t.Errorf("model = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCoreOptionsPrinter(t *testing.T) {
	rom := testROM(t)
	dir := t.TempDir()
	t.Chdir(dir)
	o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", rom, "-printer", "-no-saves"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	opts, err := o.CoreOptions()
	if err != nil {
		t.Fatalf("CoreOptions: %v", err)
	}
	gb := gameboy.NewGameBoy(opts...)
	if err := gb.LoadROM(rom); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}
	if _, ok := gb.Serial.AttachedDevice.(*accessories.Printer); !ok {
		t.Errorf("AttachedDevice = %T, want *accessories.Printer", gb.Serial.AttachedDevice)
	}
}

func TestCoreOptionsCheats(t *testing.T) {
	rom := testROM(t)

	t.Run("explicit file is loaded", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cheats := filepath.Join(dir, "game.cheats")
		if err := os.WriteFile(cheats, []byte("#Test\n01FF00-0000\n"), 0o644); err != nil {
			t.Fatalf("writing cheats file: %v", err)
		}
		o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", rom, "-cheats", cheats, "-no-saves"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		opts, err := o.CoreOptions()
		if err != nil {
			t.Fatalf("CoreOptions: %v", err)
		}
		gb := gameboy.NewGameBoy(opts...)
		if err := gb.LoadROM(rom); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
		if len(gb.Bus.LoadedCheats) != 1 {
			t.Fatalf("LoadedCheats = %d entries, want 1", len(gb.Bus.LoadedCheats))
		}
		if len(gb.Bus.GameGenieCodes) != 1 {
			t.Errorf("GameGenieCodes = %d entries, want 1", len(gb.Bus.GameGenieCodes))
		}
	})

	t.Run("missing file is ignored", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		missing := filepath.Join(dir, "absent.cheats")
		o, err := Parse(newFlagSet(t, "desktop"), []string{"-rom", rom, "-cheats", missing, "-no-saves"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		opts, err := o.CoreOptions()
		if err != nil {
			t.Fatalf("CoreOptions: %v", err)
		}
		gb := gameboy.NewGameBoy(opts...)
		if err := gb.LoadROM(rom); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
		if len(gb.Bus.LoadedCheats) != 0 {
			t.Errorf("LoadedCheats = %d entries, want 0", len(gb.Bus.LoadedCheats))
		}
	})
}

func TestCoreOptionsBootROM(t *testing.T) {
	rom := testROM(t)

	t.Run("missing file is a contextual error", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent.gbr")
		o := &Options{BootROM: missing, Model: gomeboy.ModelAuto}
		if _, err := o.CoreOptions(); err == nil {
			t.Fatalf("CoreOptions: expected an error")
		} else if !strings.Contains(err.Error(), missing) {
			t.Errorf("error %q does not contain the boot ROM path", err)
		}
	})

	t.Run("valid file loads", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		boot := filepath.Join(dir, "boot.gbr")
		if err := os.WriteFile(boot, make([]byte, 256), 0o644); err != nil {
			t.Fatalf("writing boot ROM: %v", err)
		}
		o := &Options{BootROM: boot, Model: gomeboy.ModelAuto}
		opts, err := o.CoreOptions()
		if err != nil {
			t.Fatalf("CoreOptions: %v", err)
		}
		gb := gameboy.NewGameBoy(opts...)
		if err := gb.LoadROM(rom); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
	})
}

func TestPublicOptionsDiskless(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	romPath := batteryROM(t, dir)
	o, err := Parse(newFlagSet(t, "agent"), []string{"-rom", romPath})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := gomeboy.New(append(o.PublicOptions(), gomeboy.WithROM(romPath), gomeboy.Headless())...); err != nil {
		t.Fatalf("gomeboy.New: %v", err)
	}
	// the agent never touches the disk, even for battery-backed cartridges
	if _, err := os.Stat(filepath.Join(dir, "battery.sav")); !os.IsNotExist(err) {
		t.Errorf("expected no battery.sav for the agent, stat err = %v", err)
	}
}

func TestPublicOptionsModelAndBoot(t *testing.T) {
	dir := t.TempDir()
	boot := filepath.Join(dir, "boot.gbr")
	if err := os.WriteFile(boot, make([]byte, 256), 0o644); err != nil {
		t.Fatalf("writing boot ROM: %v", err)
	}
	o := &Options{BootROM: boot, Model: gomeboy.ModelSGB2}
	if opts := o.PublicOptions(); len(opts) != 2 {
		t.Errorf("PublicOptions = %d options, want 2 (boot ROM and model)", len(opts))
	}
}
