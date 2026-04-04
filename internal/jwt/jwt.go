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
type Claims struct {
	jwt.RegisteredClaims
	UserID    string `json:"user_id"`
	TokenType string `json:"token_type"`
}

var (
	// ErrInvalidToken возвращается при невалидном токене.
	ErrInvalidToken = errors.New("invalid token")
	// ErrWrongTokenType возвращается, если тип токена не совпадает с ожидаемым.
	ErrWrongTokenType = errors.New("wrong token type")
)

// IssueAccess выпускает access-токен для userID с заданным TTL.
func IssueAccess(userID, secret string, ttl time.Duration) (string, error) {
	return issue(userID, tokenTypeAccess, secret, ttl)
}

// IssueRefresh выпускает refresh-токен для userID с заданным TTL.
func IssueRefresh(userID, secret string, ttl time.Duration) (string, error) {
	return issue(userID, tokenTypeRefresh, secret, ttl)
}

// ParseAccess парсит и валидирует access-токен, возвращает userID.
func ParseAccess(tokenString, secret string) (string, error) {
	return parseTokenType(tokenString, secret, tokenTypeAccess)
}

// ParseRefresh парсит и валидирует refresh-токен, возвращает userID.
func ParseRefresh(tokenString, secret string) (string, error) {
	return parseTokenType(tokenString, secret, tokenTypeRefresh)
}

func issue(userID, tokenType, secret string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		UserID:    userID,
		TokenType: tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}

	return signed, nil
}

func parseTokenType(tokenString, secret, expectedType string) (string, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return "", ErrInvalidToken
	}

	if claims.TokenType != expectedType {
		return "", ErrWrongTokenType
	}

	return claims.UserID, nil
}
