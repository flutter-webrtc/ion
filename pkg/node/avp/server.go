package avp

import (
	"errors"
	"sync"

	"github.com/nats-io/nats.go"
	iavp "github.com/pion/ion-avp/pkg"
	log "github.com/pion/ion-log"
	"github.com/pion/ion/pkg/proto"
)

// server represents an server instance
type server struct {
	config  iavp.Config
	clients map[string]*sfu
	mu      sync.RWMutex
	sub     *nats.Subscription
	nid     string
	nrpc    *proto.NatsRPC
}

// newServer creates a new avp server instance
func newServer(conf iavp.Config, elems map[string]iavp.ElementFun, nid string, nrpc *proto.NatsRPC) *server {
	s := &server{
		config:  conf,
		clients: make(map[string]*sfu),
		nid:     nid,
		nrpc:    nrpc,
	}

	iavp.Init(elems)

	return s
}

func (s *server) start() error {
	var err error
	if s.sub, err = s.nrpc.Subscribe(s.nid, s.handle); err != nil {
		return err
	}
	return nil
}

func (s *server) close() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(); err != nil {
			log.Errorf("unsubscribe %s error: %v", s.sub.Subject, err)
		}
	}
}

func (s *server) handle(msg interface{}) (interface{}, error) {
	log.Infof("handleRequest: %T, %+v", msg, msg)

	switch v := msg.(type) {
	case *proto.ToAvpProcessMsg:
		if err := s.process(v.Addr, v.PID, v.RID, v.TID, v.EID, v.Config); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unkonw message")
	}

	return nil, nil
}

// process starts a process for a track.
func (s *server) process(addr, pid, rid, tid, eid string, config []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.clients[addr]
	// no client yet, create one
	if c == nil {
		var err error
		log.Infof("create a sfu client, addr=%s", addr)
		if c, err = newSFU(addr, s.config, s.nrpc); err != nil {
			return err
		}
		c.onClose(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			log.Infof("sfu client close, addr=%s", addr)
			delete(s.clients, addr)
		})
		s.clients[addr] = c
	} else {
		log.Infof("sfu client exist, addr=%s", addr)
	}

	t, err := c.getTransport(proto.RID(rid))
	if err != nil {
		return err
	}

	return t.Process(pid, tid, eid, config)
}
