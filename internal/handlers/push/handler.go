// Package push содержит HTTP-хендлер для VAPID публичного ключа.
package push

import (
	"encoding/json"
	"net/http"
)

// Handler обслуживает push-related endpoints.
type Handler struct {
	vapidPublicKey string
}

// New создаёт Handler с заданным VAPID публичным ключом.
func New(vapidPublicKey string) *Handler {
	return &Handler{vapidPublicKey: vapidPublicKey}
}

type vapidKeyResponse struct {
	PublicKey string `json:"public_key"`
}

// VapidPublicKey обрабатывает GET /api/v1/push/vapid-public-key.
func (h *Handler) VapidPublicKey(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(vapidKeyResponse{PublicKey: h.vapidPublicKey}) //nolint:errchkjson // concrete response struct
}
