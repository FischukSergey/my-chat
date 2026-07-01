// Package device содержит сервисную логику регистрации устройств.
package device

import (
	"context"
	"fmt"

	"my-chat/internal/store"
)

type deviceRepository interface {
	Upsert(ctx context.Context, d store.Device) (store.Device, error)
	Disable(ctx context.Context, userID, pushToken string) error
}

// Service управляет регистрацией push-устройств пользователей.
type Service struct {
	repo deviceRepository
}

// NewService создаёт Service.
func NewService(repo deviceRepository) *Service {
	return &Service{repo: repo}
}

// Register регистрирует устройство или обновляет существующее (upsert).
func (s *Service) Register(ctx context.Context, d store.Device) (store.Device, error) {
	result, err := s.repo.Upsert(ctx, d)
	if err != nil {
		return store.Device{}, fmt.Errorf("register device: %w", err)
	}

	return result, nil
}

// Unregister отключает устройство пользователя по push-токену.
func (s *Service) Unregister(ctx context.Context, userID, pushToken string) error {
	if err := s.repo.Disable(ctx, userID, pushToken); err != nil {
		return fmt.Errorf("unregister device: %w", err)
	}

	return nil
}
