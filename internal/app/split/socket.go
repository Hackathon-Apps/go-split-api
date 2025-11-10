package split

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Hackathon-Apps/go-split-api/internal/app/metrics"
	"github.com/gorilla/websocket"
)

type WsHub struct {
	mu      sync.RWMutex
	conns   map[string]map[*websocket.Conn]struct{} // billID -> set of conns
	upgr    websocket.Upgrader
	metrics *metrics.Metrics
}

func NewWSHub(m *metrics.Metrics) *WsHub {
	return &WsHub{
		conns: make(map[string]map[*websocket.Conn]struct{}),
		upgr: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		metrics: m,
	}
}

func (h *WsHub) subscribe(billID string, w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgr.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	if _, ok := h.conns[billID]; !ok {
		h.conns[billID] = make(map[*websocket.Conn]struct{})
	}
	h.conns[billID][conn] = struct{}{}
	h.mu.Unlock()
	if h.metrics != nil {
		h.metrics.WsConnections.Inc()
	}

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.conns[billID], conn)
			if len(h.conns[billID]) == 0 {
				delete(h.conns, billID)
			}
			h.mu.Unlock()
			if h.metrics != nil {
				h.metrics.WsConnections.Dec()
			}
			conn.Close()
		}()
		conn.SetReadLimit(512)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()
}

func (h *WsHub) broadcastBill(billID string, payload any) {
	data, _ := json.Marshal(payload)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.conns[billID] {
		_ = c.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = c.WriteMessage(websocket.TextMessage, data)
	}
}
