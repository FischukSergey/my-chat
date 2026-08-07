// Package user содержит бизнес-логику регистрации пользователей.
package user

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"my-chat/internal/store"
)

// ErrUsernameTaken возвращается, если username уже занят.
var ErrUsernameTaken = errors.New("username already taken")

// ErrInvalidUsername возвращается, если username не соответствует требованиям.
var ErrInvalidUsername = errors.New("username must be 3–50 characters: latin letters, digits and _")

// ErrPasswordTooShort возвращается, если пароль короче минимального.
var ErrPasswordTooShort = errors.New("password must be at least 8 characters")

// ErrInvalidSearchQuery возвращается, если q короче 2 символов.
var ErrInvalidSearchQuery = errors.New("search query must be at least 2 characters")

const (
	bcryptCost       = 12
	minPassword      = 8
	minSearchRunes   = 2
	defaultSearchLim = 20
	maxSearchLimit   = 50
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,50}$`)

// SearchHit — публичный результат поиска (без PII).
type SearchHit struct {
	UserID   string
	Username string
}

type userRepository interface {
	Create(ctx context.Context, u store.User) (store.User, error)
	SearchByUsernamePrefix(ctx context.Context, prefix, excludeUserID string, limit int) ([]store.User, error)
}

// Service управляет регистрацией пользователей.
type Service struct {
	repo userRepository
}

// NewService создаёт Service.
func NewService(repo userRepository) *Service {
	return &Service{repo: repo}
}

// Register создаёт нового пользователя с bcrypt-хешем пароля.
// Username нормализуется к нижнему регистру (логин case-insensitive).
// Возвращает ErrInvalidUsername, ErrPasswordTooShort или ErrUsernameTaken при ошибках валидации/дублей.
func (s *Service) Register(ctx context.Context, username, password string) (store.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))

	if !usernameRe.MatchString(username) {
		return store.User{}, ErrInvalidUsername
	}

	if len(password) < minPassword {
		return store.User{}, ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return store.User{}, fmt.Errorf("hash password: %w", err)
	}

	u := store.User{
		ID:           uuid.NewString(),
		Status:       "active",
		Username:     username,
		PasswordHash: string(hash),
	}

	created, err := s.repo.Create(ctx, u)
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			return store.User{}, ErrUsernameTaken
		}

		return store.User{}, fmt.Errorf("create user: %w", err)
	}

	return created, nil
}

// Search ищет active-пользователей по prefix username.
// excludeUserID — текущий caller (исключается из результатов).
// limit: 0/отрицательный → 20; больше 50 → 50.
func (s *Service) Search(ctx context.Context, excludeUserID, q string, limit int) ([]SearchHit, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if utf8.RuneCountInString(q) < minSearchRunes {
		return nil, ErrInvalidSearchQuery
	}

	if limit <= 0 {
		limit = defaultSearchLim
	}
	limit = min(limit, maxSearchLimit)

	users, err := s.repo.SearchByUsernamePrefix(ctx, q, excludeUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}

	hits := make([]SearchHit, 0, len(users))
	for _, u := range users {
		hits = append(hits, SearchHit{
			UserID:   u.ID,
			Username: u.Username,
		})
	}

	return hits, nil
}
