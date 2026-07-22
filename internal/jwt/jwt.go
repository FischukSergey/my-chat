// Package jwt предоставляет вспомогательные функции для работы с JWT-токенами.
package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

// Claims содержит стандартные и пользовательские поля JWT.
// SessionID добавлен в Sprint 3: идентифицирует server-side запись в auth_sessions.
// Для токенов, выпущенных до Sprint 3, SessionID будет пустой строкой.
type Claims struct {
	jwt.RegisteredClaims
	UserID    string `json:"user_id"`
	TokenType string `json:"token_type"`
	SessionID string `json:"session_id,omitempty"`
}

var (
	// ErrInvalidToken возвращается при невалидном токене.
	ErrInvalidToken = errors.New("invalid token")
	// ErrWrongTokenType возвращается, если тип токена не совпадает с ожидаемым.
	ErrWrongTokenType = errors.New("wrong token type")
)

// IssueAccess выпускает access-токен для userID с заданным TTL.
// Для токенов с session_id используйте IssueAccessWithSession.
func IssueAccess(userID, secret string, ttl time.Duration) (string, error) {
	return issue(userID, "", tokenTypeAccess, secret, ttl)
}

// IssueRefresh выпускает refresh-токен для userID с заданным TTL.
// Для токенов с session_id используйте IssueRefreshWithSession.
func IssueRefresh(userID, secret string, ttl time.Duration) (string, error) {
	return issue(userID, "", tokenTypeRefresh, secret, ttl)
}

// IssueAccessWithSession выпускает access-токен с привязкой к серверной сессии (Sprint 3+).
func IssueAccessWithSession(userID, sessionID, secret string, ttl time.Duration) (string, error) {
	return issue(userID, sessionID, tokenTypeAccess, secret, ttl)
}

// IssueRefreshWithSession выпускает refresh-токен с привязкой к серверной сессии (Sprint 3+).
func IssueRefreshWithSession(userID, sessionID, secret string, ttl time.Duration) (string, error) {
	return issue(userID, sessionID, tokenTypeRefresh, secret, ttl)
}

// ParseAccess парсит и валидирует access-токен, возвращает userID.
func ParseAccess(tokenString, secret string) (string, error) {
	claims, err := parseClaims(tokenString, secret, tokenTypeAccess)
	if err != nil {
		return "", err
	}

	return claims.UserID, nil
}

// ParseRefresh парсит и валидирует refresh-токен, возвращает userID.
func ParseRefresh(tokenString, secret string) (string, error) {
	claims, err := parseClaims(tokenString, secret, tokenTypeRefresh)
	if err != nil {
		return "", err
	}

	return claims.UserID, nil
}

// ParseRefreshClaims парсит refresh-токен и возвращает полные Claims, включая SessionID.
// Используется в auth-сервисе Sprint 3 для получения session_id при ротации токена.
func ParseRefreshClaims(tokenString, secret string) (Claims, error) {
	return parseClaims(tokenString, secret, tokenTypeRefresh)
}

func issue(userID, sessionID, tokenType, secret string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		UserID:    userID,
		TokenType: tokenType,
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}

	return signed, nil
}

func parseClaims(tokenString, secret, expectedType string) (Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return Claims{}, ErrInvalidToken
	}

	if claims.TokenType != expectedType {
		return Claims{}, ErrWrongTokenType
	}

	return *claims, nil
}
