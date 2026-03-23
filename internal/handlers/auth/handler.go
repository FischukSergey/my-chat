// Package auth содержит HTTP-хендлеры auth-proxy.
package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"my-chat/internal/jwt"
)

// Config хранит зависимости хендлеров auth.
type Config struct {
	JWTSecret          string
	AccessTokenTTLSec  int
	RefreshTokenTTLSec int
}

// Handler предоставляет методы login/refresh/logout.
type Handler struct {
	cfg Config
}

// New создает Handler.
func New(cfg Config) *Handler {
	return &Handler{cfg: cfg}
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
}

// Login выдаёт пару токенов для userID.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "user_id is required")
		return
	}

	access, refresh, err := h.issueTokenPair(req.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to issue tokens")
		return
	}

	respondJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    h.cfg.AccessTokenTTLSec,
	})
}

// --- Refresh ---

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh выпускает новую пару токенов по валидному refresh-токену.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "refresh_token is required")
		return
	}

	userID, err := jwt.ParseRefresh(req.RefreshToken, h.cfg.JWTSecret)
	if err != nil {
		if errors.Is(err, jwt.ErrInvalidToken) || errors.Is(err, jwt.ErrWrongTokenType) {
			respondError(w, http.StatusUnauthorized, "unauthenticated", "invalid refresh token")
			return
		}
		respondError(w, http.StatusInternalServerError, "internal", "failed to parse token")
		return
	}

	access, refresh, err := h.issueTokenPair(userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to issue tokens")
		return
	}

	respondJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    h.cfg.AccessTokenTTLSec,
	})
}

// --- Logout ---

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout завершает сессию (MVP: только валидирует refresh, реального revoke нет).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		respondError(w, http.StatusBadRequest, "invalid_argument", "refresh_token is required")
		return
	}

	if _, err := jwt.ParseRefresh(req.RefreshToken, h.cfg.JWTSecret); err != nil {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "invalid refresh token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func (h *Handler) issueTokenPair(userID string) (string, string, error) {
	access, err := jwt.IssueAccess(userID, h.cfg.JWTSecret, time.Duration(h.cfg.AccessTokenTTLSec)*time.Second)
	if err != nil {
		return "", "", err
	}

	refresh, err := jwt.IssueRefresh(userID, h.cfg.JWTSecret, time.Duration(h.cfg.RefreshTokenTTLSec)*time.Second)
	if err != nil {
		return "", "", err
	}

	return access, refresh, nil
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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
