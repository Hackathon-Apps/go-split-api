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

type TonStream struct {
	log       *logrus.Logger
	apiURL    string
	token     string
	conn      *websocket.Conn
	mu        sync.Mutex
	subs      map[string]struct{}
	stopped   chan struct{}
	listeners map[string]map[chan TonEvent]struct{}
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
		stopped:   make(chan struct{}),
		listeners: make(map[string]map[chan TonEvent]struct{}),
	}
}

func (t *TonStream) Connect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn != nil {
		return nil
	}

	u, _ := url.Parse(t.apiURL)
	h := http.Header{}
	if t.token != "" {
		h.Set("Authorization", "Bearer "+t.token)
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	if err != nil {
		return err
	}
	t.conn = c

	go func() {
		defer close(t.stopped)
		for {
			_, data, err := t.conn.ReadMessage()
			if err != nil {
				t.log.WithError(err).Warn("chain stream read error")
				_ = t.close()
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

	}()

	if len(t.subs) > 0 {
		go func(addrs []string) {
			_ = t.Subscribe(addrs...)
		}(t.currentSubs())
	}

	return nil
}

func (t *TonStream) close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	return nil
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
	if t.conn == nil {
		return t.Connect()
	}
	return nil
}

func (t *TonStream) Subscribe(addresses ...string) error {
	if err := t.ensure(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	req := map[string]any{
		"id":      time.Now().UnixNano(),
		"jsonrpc": "2.0",
		"method":  "subscribe_account",
		"params":  addresses,
	}
	if err := t.conn.WriteJSON(req); err != nil {
		return err
	}
	for _, a := range addresses {
		t.subs[a] = struct{}{}
	}
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
			defer t.mu.Unlock()
			if listeners, ok := t.listeners[key]; ok {
				if _, exists := listeners[ch]; exists {
					delete(listeners, ch)
				}
				if len(listeners) == 0 {
					delete(t.listeners, key)
				}
			}
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
	delivered := 0
	dropped := 0
	t.mu.Unlock()

	for _, ch := range targets {
		select {
		case ch <- ev:
			delivered++
		default:
			dropped++
		}
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
