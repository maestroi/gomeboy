package web

import (
	"bytes"
	"encoding/binary"
	"github.com/cespare/xxhash"
	"github.com/google/brotli/go/cbrotli"
	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/ppu"
	"github.com/thelolagemann/gomeboy/pkg/display"
	"github.com/thelolagemann/gomeboy/pkg/emulator"
	"github.com/thelolagemann/gomeboy/pkg/log"
	"sync"
)

// webListenAddr is the address the web driver listens on. It is exposed
// as the web-listen flag and read when the driver starts.
var webListenAddr = ":8090"

// encode compresses frame data with the given quality. It is a variable so
// tests can substitute a failing encoder.
var encode = func(data []byte, quality int) ([]byte, error) {
	return cbrotli.Encode(data, cbrotli.WriterOptions{Quality: quality})
}

// init registers the web display driver. It does not open a listener or
// start any goroutine: the driver's Start method brings the hub up and its
// Stop method takes it down.
func init() {
	h := newHub(&webListenAddr, log.New())
	p1 := newPlayer(h, 10)
	p2 := newPlayer(h, 0)
	h.player1 = p1
	h.player2 = p2

	display.Install("web", p1, []display.DriverOption{{
		Name:        "listen",
		Default:     ":8090",
		Value:       &webListenAddr,
		Description: "The address the web driver listens on",
		Type:        "string",
	}})
}

// newPlayer creates a player attached to the given hub. closeBuf is the
// buffer size of the clientClose channel.
func newPlayer(h *hub, closeBuf int) *Player {
	return &Player{
		hub:           h,
		clientClose:   make(chan struct{}, closeBuf),
		clientConnect: make(chan *Client, 10),
		clientSync:    make(chan *Client, 10),
		patchCache:    newCache(16384),
		frameCache:    newCache(1024),
		currentFrame:  make([]byte, 92160),
	}
}

type Player struct {
	c             *Client
	hub           *hub
	clientClose   chan struct{}
	clientConnect chan *Client
	clientSync    chan *Client

	gb                     *gameboy.GameBoy
	pressed, release       chan<- io.Button
	patchCache, frameCache *cache
	currentFrame           []byte

	playerByte byte

	mu sync.Mutex
}

func (p *Player) AttachGameboy(gb *gameboy.GameBoy) {
	p.gb = gb
}

func (p *Player) Start(c emulator.Controller, fb <-chan []byte, pressed, released chan<- io.Button) error {
	// bring the shared web hub up (listener, ticker, broadcast loop). a
	// bind failure is returned to the caller rather than being fatal.
	if err := p.hub.start(); err != nil {
		return err
	}

	// setup keys
	p.pressed = pressed
	p.release = released

	// determine which player byte to use
	var playerByte byte = 0
	if p.hub.player1 == p {
		playerByte = 1
	} else if p.hub.player2 == p {
		playerByte = 2
	}
	p.playerByte = playerByte

	// setup vars
	var dirtied = false
	var dirtiedPixelCount, framesSkipped = 0, 0
	dirtiedPixels := make([]byte, ppu.ScreenWidth*ppu.ScreenHeight*4)
	emptyDirtiedPixels := make([]byte, ppu.ScreenWidth*ppu.ScreenHeight*4)

	var frameSkipBuf = make([]byte, 4)

	for {
		select {
		case f := <-fb:
			// process incoming framebuffer
			for i := 0; i < ppu.ScreenWidth*ppu.ScreenHeight; i++ {
				r, g, b := f[i*3], f[i*3+1], f[i*3+2]
				if p.currentFrame[i*4] != r || p.currentFrame[i*4+1] != g || p.currentFrame[i*4+2] != b {
					dirtied = true

					dirtiedPixels[i*4] = r
					dirtiedPixels[i*4+1] = g
					dirtiedPixels[i*4+2] = b
					dirtiedPixels[i*4+3] = 255

					dirtiedPixelCount++
				}

				p.currentFrame[i*4] = r
				p.currentFrame[i*4+1] = g
				p.currentFrame[i*4+2] = b
				p.currentFrame[i*4+3] = 255
			}

			// did the framebuffer get dirtied (or has the hub disabled frameSkipping)
			if dirtied || !p.hub.frameSkipping {
				// handle frame skips
				if framesSkipped > 0 && p.hub.frameSkipping {
					binary.LittleEndian.PutUint32(frameSkipBuf, uint32(framesSkipped))

					// send frames skipped to clients
					p.hub.broadcast <- p.createMessage(FrameSkip, bytes.TrimRight(frameSkipBuf, "\x00"))
				}

				// can we patch the framebuffer?
				if dirtiedPixelCount < (p.hub.framePatchRatio*4608) && p.hub.framePatching {
					p.broadcastFrame(FramePatch, dirtiedPixels)
				} else {
					p.broadcastFrame(Frame, p.currentFrame)
				}
			} else if p.hub.frameSkipping { // if not dirtied, but frame skipping is enabled, increment count
				framesSkipped++
			}

			// reset various flags
			dirtied = false
			dirtiedPixelCount = 0
			copy(dirtiedPixels, emptyDirtiedPixels)
		case <-p.clientClose:
			p.c = nil
		case c := <-p.clientConnect:
			// is there is already a client attached, or the client
			// is the one connecting, then ignore
			if p.c != nil || c == p.c {
				continue
			}

			p.c = c
			c.Send <- p.createMessage(PlayerIdentify, []byte{p.playerByte})
			go p.ReadPump(c.player)
		case c := <-p.clientSync:
			p.Sync(c)
		}
	}
}

// broadcastFrame encodes the frame or patch (if compression is enabled)
// and broadcasts it to all clients, using the patch/frame caches to avoid
// resending data the clients already have. A frame that cannot be encoded
// is logged and dropped rather than fatal.
func (p *Player) broadcastFrame(e Type, buffer []byte) {
	var output []byte

	// handle compression (if enabled)
	if p.hub.compression {
		var err error
		output, err = encode(buffer, p.hub.compressionLevel)
		if err != nil {
			p.hub.logger.Errorf("web: encode frame for player %d: %v", p.playerByte, err)
			return
		}
	} else {
		output = buffer
	}

	// calculate the hash of the data to see if it exists in cache
	hash := xxhash.Sum64(output)

	cacheBuf := make([]byte, 2)

	// should we be looking in frame of patch cache
	if e == FramePatch {
		p.patchCache.Lock()

		if idx := p.patchCache.index(hash); idx != -1 { // found in cache
			binary.LittleEndian.PutUint16(cacheBuf, uint16(idx))
			p.hub.broadcast <- p.createMessage(PatchCache, bytes.TrimRight(cacheBuf, "\x00"))
		} else { // not found in cache
			p.patchCache.add(hash, output)
			binary.LittleEndian.PutUint16(cacheBuf, uint16(p.patchCache.index(hash)))
			p.hub.broadcast <- p.createMessage(FramePatch, append(cacheBuf, output...))
		}

		p.patchCache.Unlock()
	} else { // full frame
		p.frameCache.Lock()

		if idx := p.frameCache.index(hash); idx != -1 { // found in cache
			binary.LittleEndian.PutUint16(cacheBuf, uint16(idx))
			p.hub.broadcast <- p.createMessage(FrameCache, bytes.TrimRight(cacheBuf, "\x00"))
		} else { // not found in cache
			p.frameCache.add(hash, output)
			binary.LittleEndian.PutUint16(cacheBuf, uint16(p.frameCache.index(hash)))
			p.hub.broadcast <- p.createMessage(Frame, append(cacheBuf, output...))
		}

		p.frameCache.Unlock()
	}
}

// sendToClient sends a message to the player's attached client, if any.
func (p *Player) sendToClient(message []byte) {
	if p.c == nil {
		return
	}
	p.c.Send <- message
}

func (p *Player) Stop() error {
	// idempotent: the hub ignores repeated stops
	p.hub.stop()
	return nil
}

func (p *Player) ReadPump(from <-chan []byte) {
	for {
		select {
		case message, ok := <-from:
			// check if the client has been closed
			if !ok {
				return
			}

			// ignore empty messages
			if len(message) == 0 {
				continue
			}

			// handle special case of pause/play
			if len(message) == 1 {
				if p.gb == nil {
					p.hub.logger.Debugf("web: ignoring pause/play from player %d: no gameboy attached", p.playerByte)
					continue
				}
				if message[0] == 0 {
					p.gb.Pause()
					p.hub.sendAllButClient(p.c, p.createMessage(PlayerInfo, []byte{PausePlay, 0}))
				} else {
					p.gb.Resume()
					p.hub.sendAllButClient(p.c, p.createMessage(PlayerInfo, []byte{PausePlay, 1}))
				}

				continue // skip further processing
			}

			// messages with a payload need at least 2 bytes
			if len(message) < 2 {
				continue
			}

			switch message[0] {
			case 9: // PPU related control
				if p.gb == nil || len(message) < 3 {
					continue
				}

				switch message[1] {
				case 0: // background
					p.gb.PPU.Debug.BackgroundDisabled = message[2] == 0
					p.hub.sendAllButClient(p.c, []byte{PlayerInfo, BackgroundDisabled, message[2]})
				case 1: // window
					p.gb.PPU.Debug.WindowDisabled = message[2] == 0
					p.hub.sendAllButClient(p.c, []byte{PlayerInfo, WindowDisabled, message[2]})
				case 2: // sprites
					p.gb.PPU.Debug.OBJDisabled = message[2] == 0
					p.hub.sendAllButClient(p.c, []byte{PlayerInfo, SpritesDisabled, message[2]})
				}

				continue // skip further processing
			case SaveState:
				if p.gb == nil {
					continue
				}
				err := p.gb.QuickSave()
				result := byte(1)
				if err != nil {
					result = 0
				}
				p.sendToClient(p.createMessage(PlayerInfo, []byte{SaveStateResult, result}))
			case LoadState:
				if p.gb == nil {
					continue
				}
				err := p.gb.QuickLoad()
				result := byte(1)
				if err != nil {
					result = 0
				}
				p.sendToClient(p.createMessage(PlayerInfo, []byte{LoadStateResult, result}))
			case SetSpeed:
				if p.gb == nil {
					continue
				}
				level := int(message[1])
				p.gb.SetSpeed(level)
				p.hub.broadcast <- p.createMessage(PlayerInfo, []byte{SpeedChanged, byte(p.gb.Speed())})
			default:
				button := message[0]
				state := message[1]

				if state == 0 {
					p.release <- button
				} else {
					p.pressed <- button
				}
			}
		}
	}
}

// Sync sends the current state of the Game Boy and various
// Player information to the provided client.
func (p *Player) Sync(c *Client) {
	if p.c == nil {
		// sync
		p.clientConnect <- c
	}

	frameData, err := encode(p.currentFrame, 9)
	if err != nil {
		p.hub.logger.Errorf("web: encode sync frame for player %d: %v", p.playerByte, err)
		return
	}

	c.Send <- p.createMessage(FrameSync, frameData)

	// send caches
	var data []byte
	for i, c := range p.patchCache.cache {
		// this shouldn't happen in practice, but will throw a panic if it does
		if len(c.data) == 0 {
			continue
		}

		// calculate length of cache item and index
		var length, idx = make([]byte, 2), make([]byte, 2)
		binary.LittleEndian.PutUint16(length, uint16(len(c.data)))
		binary.LittleEndian.PutUint16(idx, uint16(i))

		data = append(data, append(append(length, idx...), c.data...)...)
	}

	c.Send <- p.createMessage(PatchCacheSync, data)

	data = []byte{}
	for i, c := range p.frameCache.cache {
		// this shouldn't happen in practice, but will throw a panic if it does
		if len(c.data) == 0 {
			continue
		}

		length := make([]byte, 2)
		idx := make([]byte, 2)
		binary.LittleEndian.PutUint16(length, uint16(len(c.data)))
		binary.LittleEndian.PutUint16(idx, uint16(i))

		data = append(data, append(append(length, idx...), c.data...)...)
	}

	c.Send <- p.createMessage(FrameCacheSync, data)
}

func (p *Player) createMessage(messageType Type, data []byte) []byte {
	return append([]byte{messageType, p.playerByte}, data...)
}
