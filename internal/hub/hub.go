// Package hub содержит реестр активных WebSocket-соединений и механизм отправки событий.
package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Event представляет WebSocket-событие, отправляемое клиенту.
type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
	TS    string `json:"ts"`
}

// NewEvent создаёт Event с текущим временем.
func NewEvent(name string, data any) Event {
	return Event{
		Event: name,
		Data:  data,
		TS:    time.Now().UTC().Format(time.RFC3339),
	}
}

// Conn — обёртка над одним WebSocket-соединением.
type Conn struct {
	mu  sync.Mutex
	raw *websocket.Conn
}

// write потокобезопасно отправляет JSON-сообщение.
func (c *Conn) write(ctx context.Context, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.raw.Write(ctx, websocket.MessageText, data)
}

// Hub хранит активные соединения по user_id.
type Hub struct {
	mu     sync.RWMutex
	conns  map[string][]*Conn
	logger *slog.Logger
}

// New создаёт Hub.
func New(logger *slog.Logger) *Hub {
	return &Hub{
		conns:  make(map[string][]*Conn),
		logger: logger,
	}
}

// NewConn оборачивает websocket.Conn.
func NewConn(raw *websocket.Conn) *Conn {
	return &Conn{raw: raw}
}

// Register регистрирует соединение для пользователя.
func (h *Hub) Register(userID string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.conns[userID] = append(h.conns[userID], c)
}

// Unregister удаляет соединение из реестра.
func (h *Hub) Unregister(userID string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	list := h.conns[userID]
	filtered := list[:0]
	for _, existing := range list {
		if existing != c {
			filtered = append(filtered, existing)
		}
	}

	if len(filtered) == 0 {
		delete(h.conns, userID)
	} else {
		h.conns[userID] = filtered
	}
}

// Send отправляет событие всем активным соединениям пользователя.
// Возвращает true, если у пользователя есть хотя бы одно активное соединение.
// Отправка best-effort: ошибки логируются, но не возвращаются.
func (h *Hub) Send(ctx context.Context, userID string, event Event) bool {
	h.mu.RLock()
	list := make([]*Conn, len(h.conns[userID]))
	copy(list, h.conns[userID])
	h.mu.RUnlock()

	if len(list) == 0 {
		return false
	}

	for _, c := range list {
		if err := c.write(ctx, event); err != nil {
			h.logger.Warn(
				"ws send failed",
				slog.String("user_id", userID),
				slog.String("event", event.Event),
				slog.Any("err", err),
			)
		}
	}

	return true
}
