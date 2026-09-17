package link

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// Broker pairs two endpoints per logical session and forwards link traffic
// between them. Endpoints may be emulators or synthetic/virtual peers.
type Broker struct {
	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	peers map[string]*peer
}

type peer struct {
	id   string
	conn net.Conn
	enc  *Encoder
	mu   sync.Mutex
}

func NewBroker() *Broker {
	return &Broker{sessions: make(map[string]*session)}
}

func (b *Broker) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go b.handle(conn)
	}
}

func (b *Broker) handle(conn net.Conn) {
	defer conn.Close()
	dec := NewDecoder(conn)
	enc := NewEncoder(conn)

	hello, err := dec.Decode()
	if err != nil {
		return
	}
	if hello.Type != MessageHello || hello.Session == "" || hello.PeerID == "" {
		_ = enc.Encode(Message{Type: MessageError, Error: "first message must be hello with session and peer_id"})
		return
	}

	p := &peer{id: hello.PeerID, conn: conn, enc: enc}
	if err := b.addPeer(hello.Session, p); err != nil {
		_ = enc.Encode(Message{Type: MessageError, Error: err.Error()})
		return
	}
	defer b.removePeer(hello.Session, hello.PeerID, p)

	_ = p.send(Message{Type: MessageReady, Session: hello.Session, PeerID: hello.PeerID})
	if hello.Metadata != nil {
		b.forward(hello.Session, hello.PeerID, Message{
			Type:     MessageMetadata,
			Session:  hello.Session,
			PeerID:   hello.PeerID,
			Role:     hello.Role,
			Metadata: hello.Metadata,
		})
	}

	for {
		msg, err := dec.Decode()
		if err != nil {
			if err != io.EOF {
				_ = p.send(Message{Type: MessageError, Error: err.Error()})
			}
			return
		}

		switch msg.Type {
		case MessageClock, MessageClockReply, MessageMetadata:
			msg.Session = hello.Session
			msg.PeerID = hello.PeerID
			if !b.forward(hello.Session, hello.PeerID, msg) && msg.Type == MessageClock {
				_ = p.send(Message{Type: MessageError, Sequence: msg.Sequence, Error: "link peer is not connected"})
			}
		default:
			_ = p.send(Message{Type: MessageError, Sequence: msg.Sequence, Error: fmt.Sprintf("unsupported message type %q", msg.Type)})
		}
	}
}

func (b *Broker) addPeer(sessionID string, p *peer) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.sessions[sessionID]
	if s == nil {
		s = &session{peers: make(map[string]*peer)}
		b.sessions[sessionID] = s
	}
	if current := s.peers[p.id]; current != nil {
		return fmt.Errorf("peer %q is already connected to session %q", p.id, sessionID)
	}
	if len(s.peers) >= 2 {
		return fmt.Errorf("session %q already has two peers", sessionID)
	}
	s.peers[p.id] = p
	return nil
}

func (b *Broker) removePeer(sessionID, peerID string, p *peer) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.sessions[sessionID]
	if s == nil || s.peers[peerID] != p {
		return
	}
	delete(s.peers, peerID)
	if len(s.peers) == 0 {
		delete(b.sessions, sessionID)
	}
}

func (b *Broker) forward(sessionID, from string, msg Message) bool {
	b.mu.Lock()
	s := b.sessions[sessionID]
	var target *peer
	if s != nil {
		for id, p := range s.peers {
			if id != from {
				target = p
				break
			}
		}
	}
	b.mu.Unlock()

	if target == nil {
		return false
	}
	return target.send(msg) == nil
}

func (p *peer) send(msg Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enc.Encode(msg)
}
