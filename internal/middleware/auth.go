// Package middleware содержит HTTP middleware для main-service.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"my-chat/internal/jwt"
)

type contextKey string

const userIDKey contextKey = "user_id"

// Authenticate извлекает user_id из Authorization: Bearer <token> и кладёт в контекст.
// При невалидном или отсутствующем токене возвращает 401.
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, ok := bearerToken(r)
			if !ok {
				respondUnauthorized(w)
				return
			}

			userID, err := jwt.ParseAccess(tokenString, jwtSecret)
			if err != nil {
				if errors.Is(err, jwt.ErrInvalidToken) || errors.Is(err, jwt.ErrWrongTokenType) {
					respondUnauthorized(w)
					return
				}
				respondUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext возвращает user_id из контекста запроса.
// Паникует, если middleware не был подключён — намеренно, чтобы не скрывать конфигурационные ошибки.
func UserIDFromContext(ctx context.Context) string {
	val, _ := ctx.Value(userIDKey).(string)
	return val
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
