// Package launch provides the shared, restart-time CLI options for the
// gomeboy binaries (desktop, web, and agent).
//
// Options are registered on a flag.FlagSet and parsed into a validated
// Options value. The binaries translate Options into core (internal/gameboy)
// or public (pkg/gomeboy) options, run the emulator behind a run(args) error
// boundary, and let main log the final error and exit non-zero.
package launch

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/types"
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
	"github.com/thelolagemann/gomeboy/pkg/log"
	"github.com/thelolagemann/gomeboy/pkg/utils"
)

// Options holds the shared CLI options. Every field is a restart-time
// setting: changing one requires re-running the binary.
type Options struct {
	ROM       string        // path to the ROM to load
	BootROM   string        // path to a boot ROM (.gbr)
	Model     gomeboy.Model // hardware model (ModelAuto keeps inference)
	Printer   bool          // attach the Game Boy Printer
	Cheats    string        // explicit cheats file path
	SaveDir   string        // directory for .sav / .state files ("" = working directory)
	NoSaves   bool          // disable save file I/O entirely
	LogLevel  log.Level     // log severity
	Driver    string        // display driver name
	PProfAddr string        // pprof listen address ("" = disabled)
}

// Register registers the core shared options on fs: -rom, -boot, -model,
// -printer, -cheats, -log-level, and -pprof.
func Register(fs *flag.FlagSet) {
	fs.String("rom", "", "path to a .gb / .gbc ROM to load")
	fs.String("boot", "", "path to a boot ROM (.gbr) to use instead of the emulated boot process")
	fs.String("model", string(gomeboy.ModelAuto), "hardware model to emulate: auto, DMG0, DMG, CGB0, CGB, MGB, SGB, SGB2, or AGB (case-insensitive); auto infers the model from the cartridge")
	fs.Bool("printer", false, "attach the Game Boy Printer serial device")
	fs.String("cheats", "", "path to a cheats file (GameShark / GameGenie) to load; the working directory is never probed")
	fs.String("log-level", log.InfoLevel.String(), "log level: debug, info, or error")
	fs.String("pprof", "", "address (host:port) to serve net/http/pprof on; empty disables profiling")
}

// RegisterSaves registers the persistence options on fs: -save-dir and
// -no-saves. Binaries that historically persist saves (desktop, web)
// register them; diskless binaries (agent) do not.
func RegisterSaves(fs *flag.FlagSet) {
	fs.String("save-dir", "", "directory for battery saves (.sav) and quick-save states (.state); empty keeps the working directory")
	fs.Bool("no-saves", false, "disable all save file I/O; conflicts with -save-dir")
}

// RegisterDriver registers the -driver option with the given default.
func RegisterDriver(fs *flag.FlagSet, def string) {
	fs.String("driver", def, "display driver to use")
}

// modelValues maps the accepted spellings of -model (case-insensitive) to
// the public model.
var modelValues = map[string]gomeboy.Model{
	"auto": gomeboy.ModelAuto,
	"dmg0": gomeboy.ModelDMG0,
	"dmg":  gomeboy.ModelDMG,
	"cgb0": gomeboy.ModelCGB0,
	"cgb":  gomeboy.ModelCGB,
	"mgb":  gomeboy.ModelMGB,
	"sgb":  gomeboy.ModelSGB,
	"sgb2": gomeboy.ModelSGB2,
	"agb":  gomeboy.ModelAGB,
}

func parseModel(s string) (gomeboy.Model, error) {
	m, ok := modelValues[strings.ToLower(strings.TrimSpace(s))]
	if !ok {
		return "", fmt.Errorf("launch: invalid -model %q: use auto, DMG0, DMG, CGB0, CGB, MGB, SGB, SGB2, or AGB", s)
	}
	return m, nil
}

// Parse parses args with fs and returns the validated launch options. fs
// must already carry the registered launch options (Register, RegisterSaves,
// RegisterDriver) and may also carry display driver options.
func Parse(fs *flag.FlagSet, args []string) (*Options, error) {
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	o := &Options{}
	if err := o.read(fs); err != nil {
		return nil, err
	}
	return o, nil
}

// read fills o from the flags registered on fs. Unregistered options keep
// their zero values, so each binary only sees the flags it registered.
func (o *Options) read(fs *flag.FlagSet) error {
	o.ROM = flagString(fs, "rom")
	o.BootROM = flagString(fs, "boot")

	m, err := parseModel(flagString(fs, "model"))
	if err != nil {
		return err
	}
	o.Model = m

	o.Printer = flagBool(fs, "printer")
	o.Cheats = flagString(fs, "cheats")
	o.SaveDir = flagString(fs, "save-dir")
	o.NoSaves = flagBool(fs, "no-saves")

	lv, err := log.ParseLevel(flagString(fs, "log-level"))
	if err != nil {
		return err
	}
	o.LogLevel = lv

	o.Driver = flagString(fs, "driver")
	o.PProfAddr = flagString(fs, "pprof")

	return o.validate()
}

func flagString(fs *flag.FlagSet, name string) string {
	if f := fs.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

func flagBool(fs *flag.FlagSet, name string) bool {
	if f := fs.Lookup(name); f != nil {
		return f.Value.String() == "true"
	}
	return false
}

// validate rejects contradictory or malformed option combinations.
func (o *Options) validate() error {
	if o.NoSaves && o.SaveDir != "" {
		return fmt.Errorf("launch: -no-saves conflicts with -save-dir %q: pick one", o.SaveDir)
	}
	if o.PProfAddr != "" {
		if _, _, err := net.SplitHostPort(o.PProfAddr); err != nil {
			return fmt.Errorf("launch: invalid -pprof address %q: use host:port, e.g. 127.0.0.1:6060", o.PProfAddr)
		}
	}
	return nil
}

// CheatsPath returns the cheats file the desktop and web binaries load: the
// explicit -cheats path if given, otherwise the historical <romname>.cheats
// in the working directory when a ROM is set.
func (o *Options) CheatsPath() string {
	if o.Cheats != "" {
		return o.Cheats
	}
	if o.ROM == "" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(o.ROM), filepath.Ext(o.ROM))
	return base + ".cheats"
}

// coreModel maps the public model to the internal hardware model.
var coreModel = map[gomeboy.Model]types.Model{
	gomeboy.ModelDMG0: types.DMG0,
	gomeboy.ModelDMG:  types.DMGABC,
	gomeboy.ModelCGB0: types.CGB0,
	gomeboy.ModelCGB:  types.CGBABC,
	gomeboy.ModelMGB:  types.MGB,
	gomeboy.ModelSGB:  types.SGB,
	gomeboy.ModelSGB2: types.SGB2,
	gomeboy.ModelAGB:  types.AGB,
}

// CoreOptions translates these options into internal gameboy options for the
// desktop and web binaries, which drive internal/gameboy directly. The
// historical defaults are preserved: cheats come from <romname>.cheats and
// saves live in the working directory unless disabled or redirected.
func (o *Options) CoreOptions() ([]gameboy.Opt, error) {
	var opts []gameboy.Opt

	if o.BootROM != "" {
		boot, err := utils.LoadFile(o.BootROM)
		if err != nil {
			return nil, fmt.Errorf("launch: load boot ROM %s: %w", o.BootROM, err)
		}
		opts = append(opts, gameboy.WithBootROM(boot))
	}

	// appended after WithBootROM so an explicit model overrides the model
	// detected from the boot ROM
	if o.Model != gomeboy.ModelAuto {
		m, ok := coreModel[o.Model]
		if !ok {
			return nil, fmt.Errorf("launch: unsupported model %q", o.Model)
		}
		opts = append(opts, gameboy.AsModel(m))
	}

	if o.Printer {
		opts = append(opts, gameboy.WithPrinter())
	}

	if cheats := o.CheatsPath(); cheats != "" {
		opts = append(opts, gameboy.WithCheats(cheats))
	}

	switch {
	case o.NoSaves:
		opts = append(opts, gameboy.WithoutSaves())
	case o.SaveDir != "":
		opts = append(opts, gameboy.WithSaveDir(o.SaveDir))
	}

	return opts, nil
}

// PublicOptions translates these options into pkg/gomeboy options for the
// agent binary. The agent stays diskless: no save directory is passed and
// cheats are only loaded from an explicit -cheats path.
func (o *Options) PublicOptions() []gomeboy.Option {
	var opts []gomeboy.Option

	if o.BootROM != "" {
		opts = append(opts, gomeboy.WithBootROM(o.BootROM))
	}
	if o.Model != gomeboy.ModelAuto {
		opts = append(opts, gomeboy.WithModel(o.Model))
	}
	if o.Printer {
		opts = append(opts, gomeboy.WithPrinter())
	}
	if o.Cheats != "" {
		opts = append(opts, gomeboy.WithCheats(o.Cheats))
	}

	return opts
}

// StartPProf serves net/http/pprof on addr in a background goroutine and
// returns a stop function that closes the listener. An empty addr disables
// profiling. A bind failure is returned so the caller can exit with a
// contextual error; a later serve failure is reported through logger instead
// of being discarded.
func StartPProf(addr string, logger log.Logger) (func(), error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("launch: pprof listen on %s: %w", addr, err)
	}
	go func() {
		if err := http.Serve(ln, nil); err != nil && !errors.Is(err, net.ErrClosed) {
			logger.Errorf("pprof: serve on %s: %v", addr, err)
		}
	}()
	return func() { ln.Close() }, nil
}
