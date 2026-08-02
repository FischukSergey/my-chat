// Package user содержит HTTP-хендлер регистрации пользователей.
package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	usersvc "my-chat/internal/services/user"
	"my-chat/internal/store"
)

type userService interface {
	Register(ctx context.Context, username, password string) (store.User, error)
}

// Handler предоставляет метод регистрации нового пользователя.
type Handler struct {
	svc userService
}

// New создаёт Handler.
func New(svc userService) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerResponse struct {
	UserID string `json:"user_id"`
}

// Register обрабатывает POST /api/v1/users/register.
// Публичный endpoint: не требует аутентификации.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid_argument", "invalid request body")
		return
	}

	u, err := h.svc.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, usersvc.ErrInvalidUsername):
			respondError(w, http.StatusBadRequest, "invalid_argument",
				"username must be 3–50 characters: latin letters, digits and _")
		case errors.Is(err, usersvc.ErrPasswordTooShort):
			respondError(w, http.StatusBadRequest, "invalid_argument",
				"password must be at least 8 characters")
		case errors.Is(err, usersvc.ErrUsernameTaken):
			respondError(w, http.StatusConflict, "username_taken",
				"username is already taken")
		default:
			respondError(w, http.StatusInternalServerError, "internal", "failed to register user")
		}

		return
	}

	respondJSON(w, http.StatusCreated, registerResponse{UserID: u.ID})
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
