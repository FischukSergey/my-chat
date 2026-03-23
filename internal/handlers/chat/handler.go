// Package chat содержит HTTP-хендлеры для работы с сообщениями.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"my-chat/internal/middleware"
	chatservice "my-chat/internal/services/chat"
	"my-chat/internal/store"
)

// chatService описывает зависимость хендлера от сервисного слоя.
type chatService interface {
	SendMessage(ctx context.Context, message store.Message) (store.Message, error)
	ListMessages(ctx context.Context, userID, dialogID string, limit int, before *time.Time) ([]store.Message, error)
	MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error
	UnreadCount(ctx context.Context, userID string) (int, error)
}

// Handler предоставляет методы для работы с сообщениями.
type Handler struct {
	svc chatService
}

// New создает Handler.
func New(svc chatService) *Handler {
	return &Handler{svc: svc}
}

// --- Send message ---

type sendMessageRequest struct {
	Body string `json:"body"`
}

type messageResponse struct {
	ID        string `json:"id"`
	DialogID  string `json:"dialog_id"`
	SenderID  string `json:"sender_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type sendMessageResponse struct {
	Message messageResponse `json:"message"`
}

// SendMessage обрабатывает POST /api/v1/dialogs/{id}/messages.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	dialogID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid_argument", "invalid request body")
		return
	}

	msg := store.Message{
		ID:       uuid.New().String(),
		DialogID: dialogID,
		SenderID: userID,
		Body:     req.Body,
	}

	created, err := h.svc.SendMessage(r.Context(), msg)
	if err != nil {
		if errors.Is(err, chatservice.ErrInvalidMessageBody) {
			respondError(w, http.StatusBadRequest, "invalid_argument", "message body is empty")
			return
		}
		if errors.Is(err, chatservice.ErrForbiddenDialogAccess) {
			respondError(w, http.StatusForbidden, "forbidden", "you are not a member of this dialog")
			return
		}
		respondError(w, http.StatusInternalServerError, "internal", "failed to send message")
		return
	}

	respondJSON(w, http.StatusCreated, sendMessageResponse{
		Message: toMessageResponse(created),
	})
}

// --- List messages ---

type listMessagesResponse struct {
	Items      []messageResponse `json:"items"`
	NextBefore *string           `json:"next_before,omitempty"`
}

// ListMessages обрабатывает GET /api/v1/dialogs/{id}/messages.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	dialogID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 || parsed > 100 {
			respondError(w, http.StatusBadRequest, "invalid_argument", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	var before *time.Time
	if b := r.URL.Query().Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339, b)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid_argument", "before must be RFC3339 time")
			return
		}
		before = &t
	}

	items, err := h.svc.ListMessages(r.Context(), userID, dialogID, limit, before)
	if err != nil {
		if errors.Is(err, chatservice.ErrForbiddenDialogAccess) {
			respondError(w, http.StatusForbidden, "forbidden", "you are not a member of this dialog")
			return
		}
		respondError(w, http.StatusInternalServerError, "internal", "failed to list messages")
		return
	}

	resp := listMessagesResponse{
		Items: make([]messageResponse, len(items)),
	}
	for i, m := range items {
		resp.Items[i] = toMessageResponse(m)
	}
	if len(items) == limit {
		t := items[len(items)-1].CreatedAt.Format(time.RFC3339)
		resp.NextBefore = &t
	}

	respondJSON(w, http.StatusOK, resp)
}

// --- Mark read ---

// MarkRead обрабатывает POST /api/v1/messages/{id}/read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	messageID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	if err := h.svc.MarkRead(r.Context(), messageID, userID, time.Now().UTC()); err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to mark message as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Unread count ---

type unreadCountResponse struct {
	UnreadCount int `json:"unread_count"`
}

// UnreadCount обрабатывает GET /api/v1/me/unread-count.
func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	count, err := h.svc.UnreadCount(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to get unread count")
		return
	}

	respondJSON(w, http.StatusOK, unreadCountResponse{UnreadCount: count})
}

// --- helpers ---

func parseUUIDParam(w http.ResponseWriter, r *http.Request, param string) (string, bool) {
	raw := chi.URLParam(r, param)
	if _, err := uuid.Parse(raw); err != nil {
		respondError(w, http.StatusBadRequest, "invalid_argument", param+" is not a valid UUID")
		return "", false
	}
	return raw, true
}

func toMessageResponse(m store.Message) messageResponse {
	return messageResponse{
		ID:        m.ID,
		DialogID:  m.DialogID,
		SenderID:  m.SenderID,
		Body:      m.Body,
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}
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
