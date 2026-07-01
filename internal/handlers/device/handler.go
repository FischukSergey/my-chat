// Package device содержит HTTP-хендлеры управления push-устройствами.
package device

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"my-chat/internal/middleware"
	"my-chat/internal/store"
)

var allowedPlatforms = map[string]struct{}{
	"ios":     {},
	"android": {},
	"web":     {},
}

type deviceService interface {
	Register(ctx context.Context, d store.Device) (store.Device, error)
	Unregister(ctx context.Context, userID, pushToken string) error
}

// Handler предоставляет методы регистрации и отключения push-устройств.
type Handler struct {
	svc deviceService
}

// New создаёт Handler.
func New(svc deviceService) *Handler {
	return &Handler{svc: svc}
}

// --- Register ---

type registerRequest struct {
	Platform  string  `json:"platform"`
	PushToken string  `json:"push_token"`
	DeviceID  *string `json:"device_id"`
}

type deviceResponse struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Platform   string `json:"platform"`
	PushToken  string `json:"push_token"`
	Enabled    bool   `json:"enabled"`
	LastSeenAt string `json:"last_seen_at"`
}

type registerResponse struct {
	Device deviceResponse `json:"device"`
}

// Register обрабатывает POST /api/v1/devices/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id", nil)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid_argument", "invalid request body", nil)
		return
	}

	if _, ok := allowedPlatforms[req.Platform]; !ok {
		respondError(w, http.StatusBadRequest, "invalid_argument", "platform is invalid", map[string]string{"field": "platform"})
		return
	}

	token := strings.TrimSpace(req.PushToken)
	if token == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "push_token is required", map[string]string{"field": "push_token"})
		return
	}
	if len(token) > 1024 {
		respondError(w, http.StatusBadRequest, "invalid_argument", "push_token exceeds 1024 characters", map[string]string{"field": "push_token"})
		return
	}

	d := store.Device{
		ID:        uuid.NewString(),
		UserID:    userID,
		Platform:  req.Platform,
		PushToken: token,
	}

	created, err := h.svc.Register(r.Context(), d)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to register device", nil)
		return
	}

	respondJSON(w, http.StatusOK, registerResponse{Device: toDeviceResponse(created)})
}

// --- Unregister ---

type unregisterRequest struct {
	Platform  string `json:"platform"`
	PushToken string `json:"push_token"`
}

// Unregister обрабатывает POST /api/v1/devices/unregister.
func (h *Handler) Unregister(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id", nil)
		return
	}

	var req unregisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid_argument", "invalid request body", nil)
		return
	}

	if _, ok := allowedPlatforms[req.Platform]; !ok {
		respondError(w, http.StatusBadRequest, "invalid_argument", "platform is invalid", map[string]string{"field": "platform"})
		return
	}

	token := strings.TrimSpace(req.PushToken)
	if token == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "push_token is required", map[string]string{"field": "push_token"})
		return
	}
	if len(token) > 1024 {
		respondError(w, http.StatusBadRequest, "invalid_argument", "push_token exceeds 1024 characters", map[string]string{"field": "push_token"})
		return
	}

	if err := h.svc.Unregister(r.Context(), userID, token); err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to unregister device", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func toDeviceResponse(d store.Device) deviceResponse {
	return deviceResponse{
		ID:         d.ID,
		UserID:     d.UserID,
		Platform:   d.Platform,
		PushToken:  d.PushToken,
		Enabled:    d.Enabled,
		LastSeenAt: d.LastSeenAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) //nolint:errchkjson // payload is always a concrete response struct
}

func respondError(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	type errBody struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details map[string]string `json:"details,omitempty"`
	}
	type resp struct {
		Error errBody `json:"error"`
	}

	respondJSON(w, status, resp{Error: errBody{Code: code, Message: message, Details: details}})
}
