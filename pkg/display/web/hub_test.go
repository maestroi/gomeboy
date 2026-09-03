package web

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/maestroi/gomeboy/pkg/display"
	"github.com/maestroi/gomeboy/pkg/log"
)

// freeAddr returns an ephemeral loopback address that is currently free.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve ephemeral address: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// testLogger returns a logger that writes to a buffer the test can inspect.
func testLogger(t *testing.T) (log.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger, err := log.NewWithWriter(buf, log.DebugLevel)
	if err != nil {
		t.Fatalf("new test logger: %v", err)
	}
	return logger, buf
}

// newTestHub wires a hub up with both players, like package init does.
func newTestHub(t *testing.T, addr string, logger log.Logger) *hub {
	t.Helper()
	h := newHub(&addr, logger)
	h.player1 = newPlayer(h, 10)
	h.player2 = newPlayer(h, 0)
	return h
}

// WEB-IMPORT: importing/registering the driver does not open a listener.
// The test binary imported the package at load time, so the installed
// driver's hub must not have started a server or bound a listener yet.
func TestImportDoesNotOpenListener(t *testing.T) {
	var driver *Player
	for _, d := range display.InstalledDrivers {
		if d.Name == "web" {
			driver, _ = d.Driver.(*Player)
			break
		}
	}
	if driver == nil {
		t.Fatal("web driver not installed by package init")
	}
	if driver.hub.server != nil {
		t.Fatal("package import started an HTTP server")
	}
	if driver.hub.listener != nil {
		t.Fatal("package import opened a listener")
	}
}

// WEB-BIND: two servers on one address produce a returned contextual bind
// error (naming the address and the operation) without terminating the
// test process.
func TestBindFailureIsReturned(t *testing.T) {
	addr := freeAddr(t)
	logger, _ := testLogger(t)

	h1 := newTestHub(t, addr, logger)
	if err := h1.start(); err != nil {
		t.Fatalf("first server should bind %s: %v", addr, err)
	}
	defer h1.stop()

	h2 := newTestHub(t, addr, logger)
	err := h2.start()
	if err == nil {
		h2.stop()
		t.Fatal("expected a bind error starting a second server on the same address")
	}
	if !strings.Contains(err.Error(), addr) {
		t.Fatalf("bind error does not name the address %s: %v", addr, err)
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("bind error does not name the operation: %v", err)
	}
	if h2.listener != nil || h2.server != nil {
		t.Fatal("a failed start left server state behind")
	}
}

// WEB-STOP: Stop is safe to call repeatedly and releases the listener and
// the periodic ticker.
func TestStopIsIdempotentAndReleasesResources(t *testing.T) {
	addr := freeAddr(t)
	logger, _ := testLogger(t)

	h := newTestHub(t, addr, logger)
	if err := h.start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	h.stop()
	h.stop() // must be safe to call again
	h.stop()

	// the listener must be released
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listener not released after stop: %v", err)
	}
	ln.Close()

	// the server and ticker goroutines must have exited
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		t.Fatal("server/ticker goroutines did not exit after stop")
	}

	// the ticker must be released: no further ServerInfo broadcasts. One
	// in-flight broadcast may still arrive after stop.
	select {
	case <-h.broadcast:
	case <-time.After(1500 * time.Millisecond):
	}
	select {
	case msg := <-h.broadcast:
		t.Fatalf("ticker still broadcasting after stop: %q", msg[0])
	case <-time.After(2 * time.Second):
	}
}

// WEB-STATIC + WEB-ERRORS: static files remain mounted below /app/, the
// WebSocket route remains at /, and a rejected upgrade is logged with
// remote/request context instead of panicking or exiting.
func TestStaticMountWebSocketRouteAndUpgradeLogging(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>gomeboy</html>"), 0o644); err != nil {
		t.Fatalf("write static file: %v", err)
	}
	t.Setenv("GOMEBOY_WEB_STATIC_DIR", dir)

	addr := freeAddr(t)
	logger, logged := testLogger(t)

	h := newTestHub(t, addr, logger)
	if err := h.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer h.stop()

	base := "http://" + addr

	// static files are mounted below /app/
	resp, err := http.Get(base + "/app/index.html")
	if err != nil {
		t.Fatalf("GET /app/index.html: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read /app/index.html: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /app/index.html = %d, want 200", resp.StatusCode)
	}
	if string(body) != "<html>gomeboy</html>" {
		t.Fatalf("unexpected static body: %q", body)
	}

	// a non-websocket request to / is rejected with an upgrade error...
	resp, err = http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET / = %d, want 400 rejected upgrade", resp.StatusCode)
	}
	// ...and the rejection is logged with remote/request context
	if !strings.Contains(logged.String(), "rejected websocket upgrade") {
		t.Fatalf("rejected upgrade not logged: %q", logged.String())
	}
	if !strings.Contains(logged.String(), "GET /") {
		t.Fatalf("upgrade log missing request context: %q", logged.String())
	}
	if !strings.Contains(logged.String(), "127.0.0.1") {
		t.Fatalf("upgrade log missing remote address: %q", logged.String())
	}

	// the websocket endpoint remains at /
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/", nil)
	if err != nil {
		t.Fatalf("dial ws:///: %v", err)
	}
	defer conn.Close()

	// the server sends the initial ClientInfo message on connect
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read initial message: %v", err)
	}
	if len(msg) == 0 || msg[0] != byte(ClientInfo) {
		t.Fatalf("initial message is not ClientInfo: % x", msg)
	}
}

// WEB-ERRORS: a recoverable encode failure is logged with context and does
// not panic or broadcast a broken frame.
func TestEncodeFailureIsLoggedNotPanic(t *testing.T) {
	addr := "127.0.0.1:0"
	logger, logged := testLogger(t)
	h := newHub(&addr, logger)
	h.player1 = newPlayer(h, 10)
	h.player2 = newPlayer(h, 0)

	old := encode
	encode = func(data []byte, quality int) ([]byte, error) {
		return nil, errors.New("encoder exploded")
	}
	defer func() { encode = old }()

	// must log the failure and return without panicking
	h.player1.broadcastFrame(Frame, []byte{0, 1, 2, 3})

	if !strings.Contains(logged.String(), "encoder exploded") {
		t.Fatalf("encode failure not logged: %q", logged.String())
	}
	select {
	case msg := <-h.broadcast:
		t.Fatalf("a failed encode was still broadcast: % x", msg)
	default:
	}
}

// a successful encode is still broadcast (regression guard for the
// broadcastFrame refactor).
func TestBroadcastFrameEncodesAndBroadcasts(t *testing.T) {
	addr := "127.0.0.1:0"
	logger, _ := testLogger(t)
	h := newHub(&addr, logger)
	h.player1 = newPlayer(h, 10)
	h.player2 = newPlayer(h, 0)

	// the broadcast channel is unbuffered and normally consumed by the
	// hub's run loop, which is not started here
	got := make(chan []byte, 1)
	go func() { got <- <-h.broadcast }()

	h.player1.broadcastFrame(Frame, []byte{0, 1, 2, 3})

	select {
	case msg := <-got:
		if msg[0] != byte(Frame) {
			t.Fatalf("broadcast message type = %d, want Frame", msg[0])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("frame was not broadcast")
	}
}
