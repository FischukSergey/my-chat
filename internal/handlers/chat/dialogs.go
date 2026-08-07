package chat

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"my-chat/internal/middleware"
	chatservice "my-chat/internal/services/chat"
)

type peerResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type lastMessageResponse struct {
	MessageID   string `json:"message_id"`
	SenderID    string `json:"sender_id"`
	BodyPreview string `json:"body_preview"`
	CreatedAt   string `json:"created_at"`
}

type dialogItemResponse struct {
	DialogID    string               `json:"dialog_id"`
	Peer        peerResponse         `json:"peer"`
	LastMessage *lastMessageResponse `json:"last_message"`
	UnreadCount int                  `json:"unread_count"`
	UpdatedAt   string               `json:"updated_at"`
}

type listDialogsResponse struct {
	Dialogs []dialogItemResponse `json:"dialogs"`
}

type createDialogRequest struct {
	Username string `json:"username"`
}

// ListDialogs обрабатывает GET /api/v1/dialogs.
func (h *Handler) ListDialogs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	items, err := h.svc.ListDialogs(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal", "failed to list dialogs")
		return
	}

	resp := listDialogsResponse{
		Dialogs: make([]dialogItemResponse, 0, len(items)),
	}
	for _, item := range items {
		resp.Dialogs = append(resp.Dialogs, toDialogItemResponse(item))
	}

	respondJSON(w, http.StatusOK, resp)
}

// CreateDialog обрабатывает POST /api/v1/dialogs.
func (h *Handler) CreateDialog(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	var req createDialogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid_argument", "invalid request body")
		return
	}

	item, err := h.svc.CreateDialogByUsername(r.Context(), userID, req.Username)
	if err != nil {
		switch {
		case errors.Is(err, chatservice.ErrInvalidDialogUsername):
			respondError(w, http.StatusBadRequest, "invalid_argument", "username is required")
		case errors.Is(err, chatservice.ErrCannotDialogWithSelf):
			respondError(w, http.StatusBadRequest, "cannot_dialog_with_self", "cannot create dialog with yourself")
		case errors.Is(err, chatservice.ErrDialogUserNotFound):
			respondError(w, http.StatusNotFound, "user_not_found", "user not found")
		default:
			respondError(w, http.StatusInternalServerError, "internal", "failed to create dialog")
		}
		return
	}

	respondJSON(w, http.StatusOK, toDialogItemResponse(item))
}

func toDialogItemResponse(item chatservice.DialogItem) dialogItemResponse {
	resp := dialogItemResponse{
		DialogID: item.DialogID,
		Peer: peerResponse{
			UserID:   item.Peer.UserID,
			Username: item.Peer.Username,
		},
		LastMessage: nil,
		UnreadCount: item.UnreadCount,
		UpdatedAt:   item.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if item.LastMessage != nil {
		resp.LastMessage = &lastMessageResponse{
			MessageID:   item.LastMessage.MessageID,
			SenderID:    item.LastMessage.SenderID,
			BodyPreview: item.LastMessage.BodyPreview,
			CreatedAt:   item.LastMessage.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	return resp
}
