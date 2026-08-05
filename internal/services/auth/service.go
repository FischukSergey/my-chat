// Package auth содержит бизнес-логику управления сессиями и выдачи токенов.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	internaljwt "my-chat/internal/jwt"
	"my-chat/internal/store"
)

// Ошибки, возвращаемые сервисом при невалидных или скомпрометированных сессиях.
var (
	// ErrSessionRevoked возвращается, если refresh-токен отозван или не найден.
	ErrSessionRevoked = errors.New("session revoked")
	// ErrSessionExpired возвращается, если сессия просрочена.
	ErrSessionExpired = errors.New("session expired")
	// ErrSessionCompromised возвращается при обнаружении повторного использования
	// отозванного токена (reuse detection). Вся family отзывается.
	ErrSessionCompromised = errors.New("session compromised: token reuse detected")
	// ErrUserInactive возвращается, если аккаунт пользователя заблокирован (status != "active").
	ErrUserInactive = errors.New("user account is inactive")
	// ErrInvalidCredentials возвращается при неверном username или пароле.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrDeviceMismatch возвращается, если X-Device-ID запроса не совпадает
	// с device_id, сохранённым при создании сессии.
	ErrDeviceMismatch = errors.New("device mismatch")
)

// Config хранит настройки сервиса аутентификации.
type Config struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// TokenPair содержит выданную пару access/refresh токенов.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	SessionID    string
	ExpiresIn    int // в секундах, равно AccessTokenTTL
}

type sessionRepository interface {
	Create(ctx context.Context, s store.AuthSession) error
	FindByTokenHash(ctx context.Context, hash string) (store.AuthSession, error)
	RevokeSession(ctx context.Context, sessionID string) error
	RevokeFamily(ctx context.Context, familyID string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	RotateSession(ctx context.Context, oldSessionID string, newSession store.AuthSession) error
}

type userRepository interface {
	FindByUsername(ctx context.Context, username string) (store.User, error)
}

// Service управляет жизненным циклом сессий: login, refresh rotation, logout, reuse detection.
type Service struct {
	repo     sessionRepository
	userRepo userRepository
	cfg      Config
	log      *slog.Logger
}

// NewService создаёт Service.
func NewService(repo sessionRepository, userRepo userRepository, cfg Config, log *slog.Logger) *Service {
	return &Service{repo: repo, userRepo: userRepo, cfg: cfg, log: log}
}

// Login создаёт новую сессию и выдаёт пару токенов.
// Возвращает ErrInvalidCredentials при неверном username или пароле.
// Возвращает ErrUserInactive, если аккаунт пользователя заблокирован.
// deviceID опционален — передаётся при наличии зарегистрированного устройства.
func (s *Service) Login(ctx context.Context, username, password string, deviceID *string) (TokenPair, error) {
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, fmt.Errorf("find user: %w", err)
	}
	if user.Status != "active" {
		s.log.Warn("auth_login_blocked",
			slog.String("username", username),
			slog.String("status", user.Status),
		)
		return TokenPair{}, ErrUserInactive
	}
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}

	sessionID := uuid.NewString()
	familyID := uuid.NewString()
	now := time.Now().UTC()

	refreshToken, err := internaljwt.IssueRefreshWithSession(user.ID, sessionID, s.cfg.JWTSecret, s.cfg.RefreshTokenTTL)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue refresh token: %w", err)
	}

	accessToken, err := internaljwt.IssueAccessWithSession(user.ID, sessionID, s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}

	session := store.AuthSession{
		ID:        sessionID,
		UserID:    user.ID,
		FamilyID:  familyID,
		TokenHash: hashToken(refreshToken),
		DeviceID:  deviceID,
		ExpiresAt: now.Add(s.cfg.RefreshTokenTTL),
		CreatedAt: now,
	}

	if err = s.repo.Create(ctx, session); err != nil {
		return TokenPair{}, fmt.Errorf("create auth session: %w", err)
	}

	s.log.Info("auth_login",
		slog.String("user_id", user.ID),
		slog.String("session_id", sessionID),
		slog.String("family_id", familyID),
	)

	return TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    sessionID,
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
	}, nil
}

// Refresh ротирует refresh-токен: инвалидирует старый и выдаёт новую пару.
// При повторном использовании отозванного токена (reuse detection) отзывает всю family.
// deviceID опционален: если сессия была привязана к устройству, значение должно совпадать.
func (s *Service) Refresh(ctx context.Context, refreshToken string, deviceID *string) (TokenPair, error) {
	claims, err := internaljwt.ParseRefreshClaims(refreshToken, s.cfg.JWTSecret)
	if err != nil {
		return TokenPair{}, ErrSessionRevoked
	}

	hash := hashToken(refreshToken)

	session, err := s.repo.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrSessionNotFound) {
			return TokenPair{}, ErrSessionRevoked
		}

		return TokenPair{}, fmt.Errorf("find auth session: %w", err)
	}

	// Reuse detection: токен найден, но уже отозван.
	if session.IsRevoked() {
		s.log.Warn("auth_reuse_detected",
			slog.String("user_id", session.UserID),
			slog.String("session_id", session.ID),
			slog.String("family_id", session.FamilyID),
		)

		if revokeErr := s.repo.RevokeFamily(ctx, session.FamilyID); revokeErr != nil {
			s.log.Error("revoke family after reuse detection",
				slog.String("family_id", session.FamilyID),
				slog.String("error", revokeErr.Error()),
			)
		}

		return TokenPair{}, ErrSessionCompromised
	}

	if session.IsExpired() {
		return TokenPair{}, ErrSessionExpired
	}

	// Device binding: если сессия была создана с device_id, сверяем с запросом.
	if session.DeviceID != nil {
		if deviceID == nil || *session.DeviceID != *deviceID {
			return TokenPair{}, ErrDeviceMismatch
		}
	}

	userID := claims.UserID
	newSessionID := uuid.NewString()
	now := time.Now().UTC()
	rotatedFrom := session.ID

	newRefreshToken, err := internaljwt.IssueRefreshWithSession(userID, newSessionID, s.cfg.JWTSecret, s.cfg.RefreshTokenTTL)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue refresh token: %w", err)
	}

	newAccessToken, err := internaljwt.IssueAccessWithSession(userID, newSessionID, s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}

	newSession := store.AuthSession{
		ID:          newSessionID,
		UserID:      userID,
		FamilyID:    session.FamilyID,
		TokenHash:   hashToken(newRefreshToken),
		DeviceID:    session.DeviceID,
		ExpiresAt:   now.Add(s.cfg.RefreshTokenTTL),
		CreatedAt:   now,
		RotatedFrom: &rotatedFrom,
	}

	if err = s.repo.RotateSession(ctx, session.ID, newSession); err != nil {
		return TokenPair{}, fmt.Errorf("rotate session: %w", err)
	}

	s.log.Info("auth_refresh",
		slog.String("user_id", userID),
		slog.String("old_session_id", session.ID),
		slog.String("new_session_id", newSessionID),
	)

	return TokenPair{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		SessionID:    newSessionID,
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
	}, nil
}

// Logout отзывает сессию, связанную с refresh-токеном.
// Идемпотентен: если сессия уже отозвана или не найдена — возвращает nil.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := internaljwt.ParseRefreshClaims(refreshToken, s.cfg.JWTSecret)
	if err != nil {
		return ErrSessionRevoked
	}

	hash := hashToken(refreshToken)

	session, err := s.repo.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrSessionNotFound) {
			return nil // идемпотентность: уже не существует
		}

		return fmt.Errorf("find auth session: %w", err)
	}

	if session.IsRevoked() {
		return nil // уже отозвана
	}

	if err = s.repo.RevokeSession(ctx, session.ID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	s.log.Info("auth_logout",
		slog.String("user_id", claims.UserID),
		slog.String("session_id", session.ID),
	)

	return nil
}

// RevokeAll отзывает все активные сессии пользователя (logout everywhere).
func (s *Service) RevokeAll(ctx context.Context, userID string) error {
	if err := s.repo.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}

	s.log.Info("auth_revoke_all", slog.String("user_id", userID))

	return nil
}

// hashToken возвращает SHA-256 хеш токена в hex-кодировке.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
