package user

import (
	"errors"
	"net/http"
	"strconv"

	"my-chat/internal/middleware"
	usersvc "my-chat/internal/services/user"
)

type searchUserResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type searchUsersResponse struct {
	Users []searchUserResponse `json:"users"`
}

// Search обрабатывает GET /api/v1/users/search?q=&limit=.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthenticated", "missing user id")
		return
	}

	q := r.URL.Query().Get("q")

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			respondError(w, http.StatusBadRequest, "invalid_argument", "limit must be a positive integer")
			return
		}
		limit = parsed
	}

	hits, err := h.svc.Search(r.Context(), userID, q, limit)
	if err != nil {
		if errors.Is(err, usersvc.ErrInvalidSearchQuery) {
			respondError(w, http.StatusBadRequest, "invalid_argument",
				"q must be at least 2 characters")
			return
		}
		respondError(w, http.StatusInternalServerError, "internal", "failed to search users")
		return
	}

	resp := searchUsersResponse{
		Users: make([]searchUserResponse, 0, len(hits)),
	}
	for _, hit := range hits {
		resp.Users = append(resp.Users, searchUserResponse{
			UserID:   hit.UserID,
			Username: hit.Username,
		})
	}

	respondJSON(w, http.StatusOK, resp)
}
