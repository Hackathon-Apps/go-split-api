package chain

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/xssnick/tonutils-go/address"
)

const (
	pingInterval   = 20 * time.Second
	pongWait       = 60 * time.Second
	reconnectMin   = 500 * time.Millisecond
	reconnectMax   = 10 * time.Second
	writeDeadline  = 5 * time.Second
	readLimitBytes = 1 << 20
)

type TonStream struct {
	log       *logrus.Logger
	apiURL    string
	token     string
	mu        sync.Mutex
	conn      *websocket.Conn
	writeMu   sync.Mutex
	subs      map[string]struct{}
	listeners map[string]map[chan TonEvent]struct{}
	done      chan struct{}
}

type TonEvent struct {
	Account string `json:"account_id"`
	TxHash  string `json:"tx_hash"`
	LT      uint64 `json:"lt"`
}

func NewTonStream(log *logrus.Logger, apiURL, token string) *TonStream {
	return &TonStream{
		log:       log,
		apiURL:    apiURL,
		token:     token,
		subs:      make(map[string]struct{}),
		listeners: make(map[string]map[chan TonEvent]struct{}),
	}
}

func (t *TonStream) Connect() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return nil
	}
	if err := t.dial(); err != nil {
		return err
	}
	go t.readPump()
	go t.pingPump()

	if len(t.subs) > 0 {
		addrs := t.currentSubs()
		go func() { _ = t.Subscribe(addrs...) }()
	}
	return nil
}

func (t *TonStream) dial() error {
	u, _ := url.Parse(t.apiURL)
	h := http.Header{}
	if t.token != "" {
		h.Set("Authorization", "Bearer "+t.token)
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	if err != nil {
		return err
	}
	c.SetReadLimit(readLimitBytes)
	_ = c.SetReadDeadline(time.Now().Add(pongWait))
	c.SetPongHandler(func(_ string) error {
		return c.SetReadDeadline(time.Now().Add(pongWait))
	})
	t.conn = c
	t.done = make(chan struct{})
	return nil
}

func (t *TonStream) readPump() {
	for {
		_, data, err := t.readMessage()
		if err != nil {
			t.log.WithError(err).Warn("chain stream read error")
			t.reconnectLoop()
			return
		}

		var msg struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			t.log.WithError(err).Warn("ws: unmarshal frame failed")
			continue
		}
		if msg.Method != "account_transaction" {
			continue
		}
		var p struct {
			AccountID string      `json:"account_id"`
			TxHash    string      `json:"tx_hash"`
			LT        json.Number `json:"lt"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			t.log.WithError(err).Warn("ws: unmarshal account_transaction params failed")
			continue
		}
		ltU, _ := strconv.ParseUint(p.LT.String(), 10, 64)
		t.dispatchEvent(TonEvent{Account: p.AccountID, TxHash: p.TxHash, LT: ltU})
	}
}

func (t *TonStream) readMessage() (messageType int, data []byte, err error) {
	t.mu.Lock()
	c := t.conn
	t.mu.Unlock()
	if c == nil {
		return 0, nil, websocket.ErrCloseSent
	}
	_ = c.SetReadDeadline(time.Now().Add(pongWait))
	return c.ReadMessage()
}

func (t *TonStream) pingPump() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.writeMu.Lock()
			t.mu.Lock()
			c := t.conn
			t.mu.Unlock()
			if c != nil {
				_ = c.SetWriteDeadline(time.Now().Add(writeDeadline))
				_ = c.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(writeDeadline))
			}
			t.writeMu.Unlock()
		case <-t.done:
			return
		}
	}
}

func (t *TonStream) reconnectLoop() {
	t.mu.Lock()
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	if t.done != nil {
		close(t.done)
	}
	t.mu.Unlock()

	backoff := reconnectMin
	for {
		time.Sleep(backoff)
		t.mu.Lock()
		err := t.dial()
		t.mu.Unlock()
		if err != nil {
			t.log.WithError(err).Warn("tonstream: reconnect failed")
			backoff *= 2
			if backoff > reconnectMax {
				backoff = reconnectMax
			}
			continue
		}
		go t.readPump()
		go t.pingPump()
		addrs := t.currentSubs()
		if len(addrs) > 0 {
			if err := t.Subscribe(addrs...); err != nil {
				t.log.WithError(err).Warn("tonstream: resubscribe failed after reconnect")
			} else {
				t.log.WithField("count", len(addrs)).Info("tonstream: resubscribed after reconnect")
			}
		}
		return
	}
}

func (t *TonStream) currentSubs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.subs))
	for a := range t.subs {
		out = append(out, a)
	}
	return out
}

func (t *TonStream) ensure() error {
	t.mu.Lock()
	need := t.conn == nil
	t.mu.Unlock()
	if need {
		return t.Connect()
	}
	return nil
}

func (t *TonStream) Subscribe(addresses ...string) error {
	if err := t.ensure(); err != nil {
		return err
	}

	norm := make([]string, 0, len(addresses))
	t.mu.Lock()
	for _, a := range addresses {
		key := normalizeAddress(a)
		if key == "" {
			continue
		}
		if _, ok := t.subs[key]; !ok {
			norm = append(norm, key)
		}
	}
	t.mu.Unlock()
	if len(norm) == 0 {
		return nil
	}

	req := map[string]any{
		"id":      time.Now().UnixNano(),
		"jsonrpc": "2.0",
		"method":  "subscribe_account",
		"params":  norm,
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	t.mu.Lock()
	c := t.conn
	t.mu.Unlock()
	if c == nil {
		return websocket.ErrCloseSent
	}
	_ = c.SetWriteDeadline(time.Now().Add(writeDeadline))
	if err := c.WriteJSON(req); err != nil {
		return err
	}

	t.mu.Lock()
	for _, a := range norm {
		t.subs[a] = struct{}{}
	}
	t.mu.Unlock()
	return nil
}

func (t *TonStream) RegisterListener(addr string) (<-chan TonEvent, func()) {
	ch := make(chan TonEvent, 16)
	key := normalizeAddress(addr)

	t.mu.Lock()
	if _, ok := t.listeners[key]; !ok {
		t.listeners[key] = make(map[chan TonEvent]struct{})
	}
	t.listeners[key][ch] = struct{}{}
	t.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			t.mu.Lock()
			if listeners, ok := t.listeners[key]; ok {
				delete(listeners, ch)
				if len(listeners) == 0 {
					delete(t.listeners, key)
				}
			}
			t.mu.Unlock()
		})
	}

	return ch, cancel
}

func (t *TonStream) dispatchEvent(ev TonEvent) {
	key := normalizeAddress(ev.Account)

	t.mu.Lock()
	listenersMap := t.listeners[key]
	targets := make([]chan TonEvent, 0, len(listenersMap))
	for ch := range listenersMap {
		targets = append(targets, ch)
	}
	t.mu.Unlock()

	dropped := 0
	for _, ch := range targets {
		select {
		case ch <- ev:
		default:
			dropped++
		}
	}
	if dropped > 0 {
		t.log.WithField("dropped", dropped).Warn("tonstream: listeners buffers full, events dropped")
	}
}

func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if strings.Contains(addr, ":") {
		return strings.ToLower(addr)
	}
	parsed, err := address.ParseAddr(addr)
	if err != nil {
		return strings.ToLower(addr)
	}
	return strings.ToLower(parsed.StringRaw())
}
