package web

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/maestroi/gomeboy/internal/types"
	"github.com/maestroi/gomeboy/pkg/log"
	"golang.org/x/sys/unix"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type hub struct {
	// clients is a sync.Map because the registry is mutated by the run
	// loop while handlers and client pumps (which may already hold mu)
	// iterate it; a plain map plus mutex would deadlock there.
	clients          sync.Map
	player1, player2 *Player

	broadcast            chan []byte
	register, unregister chan *Client

	compression      bool
	compressionLevel int
	framePatching    bool
	framePatchRatio  int
	frameSkipping    bool
	currentID        uint8

	// listenAddr points at the configured listen address (the web-listen
	// flag) and is read when the hub starts, not when it is created.
	listenAddr *string
	logger     log.Logger

	listener net.Listener
	server   *http.Server

	stopping  chan struct{} // closed by stop() to end the server and ticker
	stopOnce  sync.Once
	startOnce sync.Once
	startErr  error

	// done is closed once the server and ticker goroutines have exited.
	done chan struct{}
	wg   sync.WaitGroup

	mu sync.Mutex
}

// newHub creates a hub that serves websocket clients at "/" and (if
// GOMEBOY_WEB_STATIC_DIR is set) static assets under "/app/". Nothing is
// bound or started until start is called.
func newHub(listenAddr *string, logger log.Logger) *hub {
	return &hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),

		compression:      true,
		compressionLevel: 2,
		framePatching:    true,
		frameSkipping:    true,
		framePatchRatio:  1,

		listenAddr: listenAddr,
		logger:     logger,
	}
}

// start binds the listen address and starts the HTTP server, the periodic
// info ticker, and the broadcast loop. It is safe to call more than once;
// only the first call has an effect. A bind failure is returned to the
// caller rather than being fatal to the process.
func (w *hub) start() error {
	w.startOnce.Do(func() {
		// a dedicated mux keeps the driver off the global default mux
		mux := http.NewServeMux()

		// serve the pre-built frontend if a static directory is configured.
		// the websocket handler keeps the "/" path, so static assets live
		// under "/app/" (the Svelte client's asset paths are relative).
		if staticDir := os.Getenv("GOMEBOY_WEB_STATIC_DIR"); staticDir != "" {
			mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir(staticDir))))
		}

		// websocket endpoint
		mux.HandleFunc("/", w.handleUpgrade)

		addr := *w.listenAddr
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			w.startErr = fmt.Errorf("web: listen on %s: %w", addr, err)
			return
		}

		w.listener = ln
		w.server = &http.Server{Handler: mux}
		w.stopping = make(chan struct{})
		w.done = make(chan struct{})

		w.wg.Add(2)
		go w.serve()
		go w.tick()
		go w.run()
		go func() {
			w.wg.Wait()
			close(w.done)
		}()
	})
	return w.startErr
}

// stop shuts the HTTP server and the periodic ticker down. It is safe to
// call more than once; only the first call has an effect.
func (w *hub) stop() {
	w.stopOnce.Do(func() {
		if w.stopping == nil {
			return // never started
		}

		close(w.stopping)

		if w.listener != nil {
			// close the listener directly: it releases the address and ends
			// Serve (http.ErrServerClosed). server.Shutdown alone can miss
			// it if stop races ahead of Serve's listener tracking.
			if err := w.listener.Close(); err != nil {
				w.logger.Errorf("web: close listener on %s: %v", *w.listenAddr, err)
			}
		}

		if w.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := w.server.Shutdown(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
				w.logger.Errorf("web: shutdown server on %s: %v", *w.listenAddr, err)
			}
		}
	})
}

// serve runs the HTTP server until it is shut down.
func (w *hub) serve() {
	defer w.wg.Done()
	if err := w.server.Serve(w.listener); err != nil && err != http.ErrServerClosed {
		w.logger.Errorf("web: serve %s: %v", *w.listenAddr, err)
	}
}

// tick sends periodic client latency information to all clients until the
// hub is stopped.
func (w *hub) tick() {
	defer w.wg.Done()

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		// prefer stopping over sending once stop has been requested
		select {
		case <-w.stopping:
			return
		default:
		}

		select {
		case <-w.stopping:
			return
		case <-t.C:
			// build information
			var data []byte
			w.clients.Range(func(k, v any) bool {
				c := k.(*Client)
				latencyBuf := make([]byte, 2)
				c.mu.RLock()
				binary.LittleEndian.PutUint16(latencyBuf, c.avgLatency)
				c.mu.RUnlock()
				data = append(data, c.ID)
				data = append(data, latencyBuf...)
				return true
			})

			// broadcast information
			select {
			case w.broadcast <- append([]byte{ServerInfo}, data...):
			case <-w.stopping:
				return
			}
		}
	}
}

// handleUpgrade upgrades an incoming HTTP request to a websocket
// connection and registers the new client with the hub.
func (w *hub) handleUpgrade(wr http.ResponseWriter, r *http.Request) {
	wr.Header().Set("Access-Control-Allow-Origin", "*")

	// upgrade the connection to a websocket connection
	conn, err := upgrader.Upgrade(wr, r, nil)
	if err != nil {
		// the upgrader already wrote an HTTP error response; log the
		// rejection with enough context to identify the request
		w.logger.Errorf("web: rejected websocket upgrade: %s %s %s (user-agent %q): %v",
			r.RemoteAddr, r.Method, r.URL.Path, r.Header.Get("User-Agent"), err)
		return
	}

	// create new client
	c := w.newClient(conn, r)

	// spawn read/write pumps
	go c.ReadPump()
	go c.WritePump()

	// send initial data information
	c.Send <- []byte{ClientInfo, ClientStatus, w.info(), uint8(w.compressionLevel), uint8(w.framePatchRatio)}

	// inform players of the new clients
	w.player1.clientSync <- c
	if w.player2.c != nil {
		w.player2.clientSync <- c
	}

	// synchronize clients to connecting client
	var data []byte
	w.clients.Range(func(k, v any) bool {
		cl := k.(*Client)
		if c == cl {
			return true // skip self
		}

		data = append(data, c.Metadata.RemoteAddr...)
		data = append(data, 0)
		data = append(data, cl.Metadata.UserAgent...)
		data = append(data, 0)
		data = append(data, cl.Metadata.Username...)
		data = append(data, 0)
		data = append(data, cl.ID)
		data = append(data, byte('\n'))
		return true
	})

	if len(data) > 0 {
		// remove last newline to avoid issues with JS
		data = data[:len(data)-1]
	}

	c.Send <- append([]byte{ClientListSync}, data...)
}

// run handles client registration, unregistration, and broadcasting.
func (w *hub) run() {
	for {
		select {
		case c := <-w.register:
			w.clients.Store(c, true)
		case c := <-w.unregister:
			w.player1.mu.Lock()
			// is this client still registered
			if _, loaded := w.clients.LoadAndDelete(c); loaded {
				// was it one of the players?
				if w.player1 != nil && c == w.player1.c {
					w.player1.clientClose <- struct{}{}
				}
				if w.player2 != nil && c == w.player2.c {
					w.player2.clientClose <- struct{}{}
				}

				id := c.Metadata.RemoteAddr

				// notify connected clients that this client has disconnected
				w.clients.Range(func(k, v any) bool {
					cl := k.(*Client)
					select {
					case cl.Send <- append([]byte{ClientClosing}, id...):
					default:
					}
					return true
				})

				// notify the next client that it can join if there is one available
				if next := w.nextPlayer(); next != nil {
					w.player1.clientConnect <- next
				}
			}
			w.player1.mu.Unlock()
		case msg := <-w.broadcast:
			w.clients.Range(func(k, v any) bool {
				cl := k.(*Client)
				select {
				case cl.Send <- msg:
				default:
					close(cl.Send)
					w.clients.Delete(cl)
				}
				return true
			})
		}
	}
}

// info returns a byte of information containing the various
// hub settings. The byte is constructed as follows:
//
//	Bit 0: Running status of player 1
//	Bit 1: Running status of player 2
//	Bit 2: Compression enabled
//	Bit 3: Frame patching enabled
//	Bit 4: Frame skipping enabled
//	Bit 5: Player 1 paused
//	Bit 6: Player 2 paused
func (w *hub) info() byte {
	info := uint8(0)
	if w.player1.gb != nil {
		if !w.player1.gb.Paused() {
			info |= types.Bit0
		}
		if w.player1.gb.Paused() {
			info |= types.Bit5
		}
	}

	if w.player2.gb != nil {
		if !w.player2.gb.Paused() {
			info |= types.Bit1
		}
		if w.player2.gb.Paused() {
			info |= types.Bit6
		}
	}

	if w.compression {
		info |= types.Bit2
	}
	if w.framePatching {
		info |= types.Bit3
	}
	if w.frameSkipping {
		info |= types.Bit4
	}

	w.logger.Debugf("web: hub info %08b", info)

	return info
}

// nextPlayer returns the next client in the list awaiting
// player upgrade by comparing the value of each connectedAt
// field. Used when a player disconnects and a new player
// is able to take over.
func (w *hub) nextPlayer() *Client {
	var next *Client
	w.clients.Range(func(k, v any) bool {
		c := k.(*Client)
		if next == nil || c.connectedAt.Before(next.connectedAt) {
			next = c
		}
		return true
	})

	return next
}

// newClient creates a new client and registers it to the hub
func (w *hub) newClient(conn *websocket.Conn, r *http.Request) *Client {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.currentID++

	c := &Client{
		hub:    w,
		conn:   conn,
		Send:   make(chan []byte, 256),
		ID:     w.currentID,
		player: make(chan []byte, 256),
		Metadata: struct {
			RemoteAddr string
			UserAgent  string
			Username   string
		}{RemoteAddr: r.RemoteAddr, UserAgent: r.Header.Get("User-Agent")},
		connectedAt: time.Now(),
	}
	w.register <- c
	return c
}

// sendAllButClient sends a message to all connected clients except
// the one specified. Used for events such as username registration,
// where the client is the one that initiated the event, so is already
// aware of the registered username.
func (w *hub) sendAllButClient(client *Client, message []byte) {
	// snapshot first so the (possibly blocking) sends happen without
	// holding the map's internal lock
	clients := make([]*Client, 0, 8)
	w.clients.Range(func(k, v any) bool {
		if c := k.(*Client); c != client {
			clients = append(clients, c)
		}
		return true
	})

	for _, c := range clients {
		c.Send <- message
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 16,
	WriteBufferSize: 1024 * 16,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func tcpInfo(conn *net.TCPConn) (*unix.TCPInfo, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var info *unix.TCPInfo
	ctrlErr := raw.Control(func(fd uintptr) {
		info, err = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	})
	switch {
	case ctrlErr != nil:
		return nil, ctrlErr
	case err != nil:
		return nil, err
	}

	return info, nil
}
