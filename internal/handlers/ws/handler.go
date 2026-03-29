// Package ws содержит WebSocket-хендлер для GET /ws/connect.
package ws

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coder/websocket"

	"my-chat/internal/hub"
	"my-chat/internal/jwt"
)

// Handler обслуживает WebSocket-соединения.
type Handler struct {
	hub       *hub.Hub
	jwtSecret string
	logger    *slog.Logger
}

// New создаёт Handler.
func New(h *hub.Hub, jwtSecret string, logger *slog.Logger) *Handler {
	return &Handler{
		hub:       h,
		jwtSecret: jwtSecret,
		logger:    logger,
	}
}

// Connect обрабатывает GET /ws/connect.
// JWT проверяется до апгрейда, чтобы можно было ответить HTTP 401.
func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	tokenString, ok := bearerToken(r)
	if !ok {
		respondUnauthorized(w)
		return
	}

	userID, err := jwt.ParseAccess(tokenString, h.jwtSecret)
	if err != nil {
		if errors.Is(err, jwt.ErrInvalidToken) || errors.Is(err, jwt.ErrWrongTokenType) {
			respondUnauthorized(w)
			return
		}
		respondUnauthorized(w)
		return
	}

	rawConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Warn("ws accept failed", slog.String("user_id", userID), slog.Any("err", err))
		return
	}

	c := hub.NewConn(rawConn)
	h.hub.Register(userID, c)

	h.logger.Info("ws connected", slog.String("user_id", userID))

	defer func() {
		h.hub.Unregister(userID, c)
		rawConn.CloseNow()
		h.logger.Info("ws disconnected", slog.String("user_id", userID))
	}()

	ctx := r.Context()
	for {
		_, _, err := rawConn.Read(ctx)
		if err != nil {
			return
		}
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}

	after, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(after) == "" {
		return "", false
	}

	return after, true
}

func respondUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"missing or invalid token"}}`))
}
