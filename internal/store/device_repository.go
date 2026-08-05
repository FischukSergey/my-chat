// Package store содержит работу с PostgreSQL для main-service.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeviceRepository реализует операции с таблицей devices.
type DeviceRepository struct {
	db db
}

// NewDeviceRepository создает DeviceRepository.
func NewDeviceRepository(s *Store) *DeviceRepository {
	return &DeviceRepository{db: s.pool}
}

// Upsert регистрирует устройство или обновляет существующее.
// Для platform="web" выполняет upsert по (user_id, endpoint) из push_subscription.
// Для ios/android — по (user_id, platform, push_token).
func (r *DeviceRepository) Upsert(ctx context.Context, d Device) (Device, error) {
	if d.PushSubscription != "" {
		return r.upsertWeb(ctx, d)
	}

	return r.upsertToken(ctx, d)
}

// upsertToken — путь для ios/android: ON CONFLICT по (user_id, platform, push_token).
func (r *DeviceRepository) upsertToken(ctx context.Context, d Device) (Device, error) {
	const q = `
		INSERT INTO devices (id, user_id, platform, push_token, enabled, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, $5, $5, $5)
		ON CONFLICT (user_id, platform, push_token) DO UPDATE
		    SET enabled      = TRUE,
		        last_seen_at = EXCLUDED.last_seen_at,
		        updated_at   = EXCLUDED.updated_at
		RETURNING id, user_id, platform,
		          COALESCE(push_token, '')              AS push_token,
		          COALESCE(push_subscription::text, '') AS push_subscription,
		          enabled, last_seen_at, created_at, updated_at`

	now := time.Now().UTC()
	row := r.db.QueryRow(ctx, q, d.ID, d.UserID, d.Platform, d.PushToken, now)

	return scanDevice(row)
}

// upsertWeb — путь для platform="web": ON CONFLICT по (user_id, endpoint).
// Использует partial unique index devices_web_endpoint_unique.
func (r *DeviceRepository) upsertWeb(ctx context.Context, d Device) (Device, error) {
	const q = `
		INSERT INTO devices (id, user_id, platform, push_subscription, enabled, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, TRUE, $5, $5, $5)
		ON CONFLICT (user_id, (push_subscription->>'endpoint')) WHERE platform = 'web'
		DO UPDATE
		    SET enabled           = TRUE,
		        push_subscription = EXCLUDED.push_subscription,
		        last_seen_at      = EXCLUDED.last_seen_at,
		        updated_at        = EXCLUDED.updated_at
		RETURNING id, user_id, platform,
		          COALESCE(push_token, '')              AS push_token,
		          COALESCE(push_subscription::text, '') AS push_subscription,
		          enabled, last_seen_at, created_at, updated_at`

	now := time.Now().UTC()
	row := r.db.QueryRow(ctx, q, d.ID, d.UserID, d.Platform, d.PushSubscription, now)

	return scanDevice(row)
}

// Disable деактивирует устройство по push_token и user_id (unregister).
// Не возвращает ошибку, если устройство не найдено.
func (r *DeviceRepository) Disable(ctx context.Context, userID, pushToken string) error {
	const q = `
		UPDATE devices
		   SET enabled    = FALSE,
		       updated_at = NOW()
		 WHERE user_id    = $1
		   AND push_token = $2`

	_, err := r.db.Exec(ctx, q, userID, pushToken)
	if err != nil {
		return fmt.Errorf("disable device: %w", err)
	}

	return nil
}

// DisableByID деактивирует устройство по его ID.
// Используется при получении HTTP 404/410 от push-сервера (устаревшая Web Push подписка).
// Не возвращает ошибку, если устройство не найдено.
func (r *DeviceRepository) DisableByID(ctx context.Context, id string) error {
	const q = `
		UPDATE devices
		   SET enabled    = FALSE,
		       updated_at = NOW()
		 WHERE id = $1`

	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("disable device by id: %w", err)
	}

	return nil
}

// ListActive возвращает все активные устройства пользователя.
func (r *DeviceRepository) ListActive(ctx context.Context, userID string) ([]Device, error) {
	const q = `
		SELECT id, user_id, platform,
		       COALESCE(push_token, '')              AS push_token,
		       COALESCE(push_subscription::text, '') AS push_subscription,
		       enabled, last_seen_at, created_at, updated_at
		  FROM devices
		 WHERE user_id = $1
		   AND enabled = TRUE
		 ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list active devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}

	return devices, nil
}

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(
		&d.ID,
		&d.UserID,
		&d.Platform,
		&d.PushToken,
		&d.PushSubscription,
		&d.Enabled,
		&d.LastSeenAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return Device{}, fmt.Errorf("scan device: %w", err)
	}

	return d, nil
}
