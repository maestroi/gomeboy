package linkcable

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

type Server struct {
	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	name       string
	peers      map[string]*peer
	nextReqID  uint64
	virtualReq map[uint64]virtualRequest
	metadata   map[string]map[string]json.RawMessage
}

type virtualRequest struct {
	emulator *peer
	seq      uint64
}

type pendingTransfer struct {
	seq           uint64
	out           byte
	internalClock bool
}

type peer struct {
	endpoint string
	role     Role
	conn     net.Conn
	enc      *json.Encoder
	sendMu   sync.Mutex
	pending  *pendingTransfer
}

func NewServer() *Server {
	return &Server{sessions: make(map[string]*session)}
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if ln == nil {
		return errors.New("linkcable: nil listener")
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)
	var hello message
	if err := dec.Decode(&hello); err != nil {
		return
	}
	if hello.Type != msgHello || hello.Version != ProtocolVersion || hello.Session == "" || hello.Endpoint == "" {
		_ = enc.Encode(message{Type: msgError, Error: "linkcable: invalid hello"})
		return
	}
	if hello.Role != RoleEmulator && hello.Role != RoleVirtual {
		_ = enc.Encode(message{Type: msgError, Error: "linkcable: unsupported role"})
		return
	}
	p := &peer{endpoint: hello.Endpoint, role: hello.Role, conn: conn, enc: enc}
	ss, err := s.join(hello.Session, p)
	if err != nil {
		_ = enc.Encode(message{Type: msgError, Error: err.Error()})
		return
	}
	defer s.leave(ss, p)
	if err := p.send(message{Type: msgWelcome, Version: ProtocolVersion, Session: hello.Session, Endpoint: hello.Endpoint, Role: hello.Role}); err != nil {
		return
	}
	s.tryMatch(ss)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var m message
		if err := dec.Decode(&m); err != nil {
			if !errors.Is(err, io.EOF) {
				_ = err
			}
			return
		}
		switch m.Type {
		case msgReady:
			if p.role != RoleEmulator {
				_ = p.send(message{Type: msgError, Error: "linkcable: ready requires emulator role"})
				return
			}
			s.handleReady(ss, p, m)
		case msgExchangeResult:
			if p.role != RoleVirtual {
				_ = p.send(message{Type: msgError, Error: "linkcable: exchange_result requires virtual role"})
				return
			}
			s.handleVirtualResult(ss, p, m)
		case msgMetadata:
			s.handleMetadata(ss, p, m.Metadata)
		default:
			_ = p.send(message{Type: msgError, Error: fmt.Sprintf("linkcable: unsupported message %q", m.Type)})
			return
		}
	}
}

func (s *Server) join(name string, p *peer) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.sessions[name]
	if ss == nil {
		ss = &session{name: name, peers: make(map[string]*peer), virtualReq: make(map[uint64]virtualRequest), metadata: make(map[string]map[string]json.RawMessage)}
		s.sessions[name] = ss
	}
	if old := ss.peers[p.endpoint]; old != nil {
		return nil, fmt.Errorf("linkcable: endpoint %q already connected to session %q", p.endpoint, name)
	}
	if len(ss.peers) >= 2 {
		return nil, fmt.Errorf("linkcable: session %q already has two endpoints", name)
	}
	ss.peers[p.endpoint] = p
	return ss, nil
}

func (s *Server) leave(ss *session, p *peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ss.peers[p.endpoint] == p {
		delete(ss.peers, p.endpoint)
		delete(ss.metadata, p.endpoint)
	}
	for id, req := range ss.virtualReq {
		if req.emulator == p || p.role == RoleVirtual {
			delete(ss.virtualReq, id)
		}
	}
	if len(ss.peers) == 0 {
		delete(s.sessions, ss.name)
	}
}

func (s *Server) handleReady(ss *session, p *peer, m message) {
	s.mu.Lock()
	p.pending = &pendingTransfer{seq: m.Seq, out: m.Out, internalClock: m.InternalClock}
	s.mu.Unlock()
	s.tryMatch(ss)
}

func (s *Server) tryMatch(ss *session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var a, b *peer
	for _, candidate := range ss.peers {
		if a == nil {
			a = candidate
		} else {
			b = candidate
			break
		}
	}
	if a == nil || b == nil {
		return
	}

	var emulator, virtual *peer
	if a.role == RoleVirtual && b.role == RoleEmulator {
		virtual, emulator = a, b
	} else if b.role == RoleVirtual && a.role == RoleEmulator {
		virtual, emulator = b, a
	}
	if virtual != nil {
		if emulator.pending == nil || !emulator.pending.internalClock {
			return
		}
		ss.nextReqID++
		requestID := ss.nextReqID
		ss.virtualReq[requestID] = virtualRequest{emulator: emulator, seq: emulator.pending.seq}
		out := emulator.pending.out
		emulator.pending = nil
		go func() {
			if err := virtual.send(message{Type: msgExchange, RequestID: requestID, Out: out}); err != nil {
				s.failVirtualRequest(ss, requestID, err)
			}
		}()
		return
	}

	if a.role != RoleEmulator || b.role != RoleEmulator || a.pending == nil || b.pending == nil {
		return
	}
	if a.pending.internalClock == b.pending.internalClock {
		if a.pending.internalClock {
			a.pending, b.pending = nil, nil
			go a.send(message{Type: msgError, Error: "linkcable: both endpoints requested internal clock"})
			go b.send(message{Type: msgError, Error: "linkcable: both endpoints requested internal clock"})
		}
		return
	}
	pa, pb := a.pending, b.pending
	a.pending, b.pending = nil, nil
	go a.send(message{Type: msgResult, Seq: pa.seq, In: pb.out})
	go b.send(message{Type: msgResult, Seq: pb.seq, In: pa.out})
}

func (s *Server) handleVirtualResult(ss *session, p *peer, m message) {
	s.mu.Lock()
	req, ok := ss.virtualReq[m.RequestID]
	if ok {
		delete(ss.virtualReq, m.RequestID)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if m.Error != "" {
		_ = req.emulator.send(message{Type: msgError, Error: m.Error})
		return
	}
	_ = req.emulator.send(message{Type: msgResult, Seq: req.seq, In: m.In})
}

func (s *Server) failVirtualRequest(ss *session, requestID uint64, err error) {
	s.mu.Lock()
	req, ok := ss.virtualReq[requestID]
	if ok {
		delete(ss.virtualReq, requestID)
	}
	s.mu.Unlock()
	if ok {
		_ = req.emulator.send(message{Type: msgError, Error: err.Error()})
	}
}

func (s *Server) handleMetadata(ss *session, p *peer, md map[string]json.RawMessage) {
	s.mu.Lock()
	ss.metadata[p.endpoint] = md
	var other *peer
	for _, candidate := range ss.peers {
		if candidate != p {
			other = candidate
			break
		}
	}
	s.mu.Unlock()
	if other != nil {
		_ = other.send(message{Type: msgPeerMetadata, Endpoint: p.endpoint, Metadata: md})
	}
}

func (p *peer) send(m message) error {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	return p.enc.Encode(m)
}
