package display

import (
	"strings"
	"testing"

	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/pkg/emulator"
)

// fakeDriver is a display-server-free stand-in for a display driver.
type fakeDriver struct {
	name string
}

func (f *fakeDriver) Start(c emulator.Controller, fb <-chan []byte, pressed, released chan<- io.Button) error {
	return nil
}

func (f *fakeDriver) Stop() error { return nil }

// installForTest replaces the installed driver registry with fakes for the
// given names, in that order, and restores the original registry at test end.
func installForTest(t *testing.T, names ...string) {
	t.Helper()
	saved := InstalledDrivers
	InstalledDrivers = nil
	for _, name := range names {
		Install(name, &fakeDriver{name: name}, nil)
	}
	t.Cleanup(func() { InstalledDrivers = saved })
}

// driverByName returns the fake driver registered under the given name.
func driverByName(t *testing.T, name string) *fakeDriver {
	t.Helper()
	for _, d := range InstalledDrivers {
		if d.Name == name {
			if f, ok := d.Driver.(*fakeDriver); ok {
				return f
			}
		}
	}
	t.Fatalf("driver %q not installed", name)
	return nil
}

// WEB-AUTO: auto chooses the preferred installed desktop driver regardless
// of registration order. GLFW is preferred when both desktop frontends are
// available; web is never selected implicitly.
func TestAutoSelectsDesktopDriverRegardlessOfOrder(t *testing.T) {
	tests := []struct {
		name  string
		order []string
		want  string // name of the driver auto must select
	}{
		{name: "web first", order: []string{"web", "fyne", "glfw"}, want: "glfw"},
		{name: "web middle", order: []string{"fyne", "web", "glfw"}, want: "glfw"},
		{name: "web last", order: []string{"fyne", "glfw", "web"}, want: "glfw"},
		{name: "glfw before fyne", order: []string{"glfw", "fyne", "web"}, want: "glfw"},
		{name: "desktop reversed", order: []string{"web", "glfw", "fyne"}, want: "glfw"},
		{name: "only glfw after web", order: []string{"web", "glfw"}, want: "glfw"},
		{name: "only glfw before web", order: []string{"glfw", "web"}, want: "glfw"},
		{name: "only fyne after web", order: []string{"web", "fyne"}, want: "fyne"},
		{name: "only fyne before web", order: []string{"fyne", "web"}, want: "fyne"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			installForTest(t, tc.order...)

			want := driverByName(t, tc.want)
			got := GetDriver("auto")
			if got == nil {
				t.Fatalf("GetDriver(auto) = nil with %v installed, want %q", tc.order, tc.want)
			}
			if got == driverByName(t, "web") {
				t.Fatalf("GetDriver(auto) selected the web driver with %v installed", tc.order)
			}
			if got != want {
				t.Fatalf("GetDriver(auto) with %v installed = %v, want %v", tc.order, got, want)
			}
		})
	}
}

// WEB-DEFAULT-OFF: auto never resolves to web, including a web-only
// registry: it returns nil so the caller reports an actionable error
// instead of silently opening a network listener.
func TestAutoNeverResolvesToWeb(t *testing.T) {
	installForTest(t, "web")

	if got := GetDriver("auto"); got != nil {
		t.Fatalf("GetDriver(auto) in a web-only registry = %v, want nil", got)
	}
}

// WEB-EXPLICIT: explicit web selection still returns the registered web
// driver, regardless of registration order.
func TestExplicitWebSelection(t *testing.T) {
	for _, order := range [][]string{
		{"web", "fyne", "glfw"},
		{"fyne", "web", "glfw"},
		{"fyne", "glfw", "web"},
	} {
		t.Run(strings.Join(order, "+"), func(t *testing.T) {
			installForTest(t, order...)

			want := driverByName(t, "web")
			if got := GetDriver("web"); got != want {
				t.Fatalf("GetDriver(web) with %v installed = %v, want the registered web driver", order, got)
			}
		})
	}
}

// WEB-EXPLICIT: explicit selection of a driver that is not installed
// still returns nil.
func TestExplicitUnknownDriver(t *testing.T) {
	installForTest(t, "fyne", "web")

	if got := GetDriver("glfw"); got != nil {
		t.Fatalf("GetDriver(glfw) with only fyne and web installed = %v, want nil", got)
	}
}
