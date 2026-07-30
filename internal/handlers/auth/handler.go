// Package auth содержит HTTP-хендлеры auth-proxy.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	authsvc "my-chat/internal/services/auth"
)

type authService interface {
	Login(ctx context.Context, userID string, deviceID *string) (authsvc.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (authsvc.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	RevokeAll(ctx context.Context, userID string) error
}

// Handler предоставляет методы login/refresh/logout.
type Handler struct {
	svc authService
}

// New создает Handler.
func New(svc authService) *Handler {
	return &Handler{svc: svc}
}

// --- Login ---

type loginRequest struct {
	UserID string `json:"user_id"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	SessionID    string `json:"session_id"`
}

// Login выдаёт пару токенов и создаёт серверную сессию.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "user_id is required")
		return
	}

	pair, err := h.svc.Login(r.Context(), req.UserID, nil)
	if err != nil {
		if errors.Is(err, authsvc.ErrUserInactive) {
			respondError(w, http.StatusForbidden, "user_inactive", "user account is inactive")
			return
		}
		respondError(w, http.StatusInternalServerError, "internal", "failed to login")
		return
	}

	respondJSON(w, http.StatusOK, pairToResponse(pair))
}

// --- Refresh ---

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh ротирует refresh-токен и выдаёт новую пару.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "refresh_token is required")
		return
	}

	pair, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		respondAuthError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, pairToResponse(pair))
}

// --- Logout ---

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout отзывает текущую сессию.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "refresh_token is required")
		return
	}

	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		respondAuthError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func pairToResponse(p authsvc.TokenPair) tokenResponse {
	return tokenResponse{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    p.ExpiresIn,
		SessionID:    p.SessionID,
	}
}

// respondAuthError маппирует ошибки auth-сервиса на HTTP-коды согласно api-sprint-3.md.
func respondAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authsvc.ErrSessionExpired):
		respondError(w, http.StatusUnauthorized, "session_expired", "session has expired")
	case errors.Is(err, authsvc.ErrSessionCompromised):
		respondError(w, http.StatusUnauthorized, "session_compromised", "token reuse detected, all sessions revoked")
	case errors.Is(err, authsvc.ErrSessionRevoked):
		respondError(w, http.StatusUnauthorized, "session_revoked", "session has been revoked")
	default:
		respondError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) //nolint:errchkjson // payload is always a concrete response struct
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	type errBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type resp struct {
		Error errBody `json:"error"`
	}

	respondJSON(w, status, resp{Error: errBody{Code: code, Message: message}})
}
