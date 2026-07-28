// Package store содержит работу с PostgreSQL для main-service.
package store

import (
	"context"
	"fmt"
	"time"
)

// NotificationOutboxRepository реализует операции с таблицей notification_outbox.
type NotificationOutboxRepository struct {
	db db
}

// NewNotificationOutboxRepository создает NotificationOutboxRepository.
func NewNotificationOutboxRepository(s *Store) *NotificationOutboxRepository {
	return &NotificationOutboxRepository{db: s.pool}
}

// Enqueue добавляет новую outbox-задачу.
// При совпадении dedup_key (UNIQUE constraint) молча игнорирует дубль.
func (r *NotificationOutboxRepository) Enqueue(ctx context.Context, task NotificationOutbox) error {
	const q = `
		INSERT INTO notification_outbox
		            (id, event_type, user_id, payload, dedup_key, status, next_attempt_at, created_at, updated_at)
		VALUES      ($1, $2, $3, $4, $5, 'pending', NOW(), NOW(), NOW())
		ON CONFLICT (dedup_key) DO NOTHING`

	if _, err := r.db.Exec(ctx, q, task.ID, task.EventType, task.UserID, task.Payload, task.DedupKey); err != nil {
		return fmt.Errorf("enqueue outbox task: %w", err)
	}

	return nil
}

// ClaimBatch атомарно захватывает до batchSize задач, готовых к обработке
// (status IN ('pending', 'failed') AND next_attempt_at <= now).
// Захваченным задачам выставляется next_attempt_at = now + 5 минут (processing lease):
// если воркер упадёт не вызвав MarkSent/MarkFailed, задача автоматически
// вернётся в обработку после истечения lease.
func (r *NotificationOutboxRepository) ClaimBatch(ctx context.Context, batchSize int) ([]NotificationOutbox, error) {
	const q = `
		UPDATE notification_outbox
		   SET status          = 'pending',
		       attempt         = attempt + 1,
		       next_attempt_at = NOW() + INTERVAL '5 minutes',
		       updated_at      = NOW()
		 WHERE id IN (
		     SELECT id
		       FROM notification_outbox
		      WHERE status IN ('pending', 'failed')
		        AND next_attempt_at <= NOW()
		      ORDER BY next_attempt_at ASC
		      LIMIT $1
		      FOR UPDATE SKIP LOCKED
		 )
		RETURNING id, event_type, user_id, payload, dedup_key, attempt, status,
		          next_attempt_at, last_error, created_at, updated_at`

	rows, err := r.db.Query(ctx, q, batchSize)
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}
	defer rows.Close()

	var tasks []NotificationOutbox
	for rows.Next() {
		var t NotificationOutbox
		var rawPayload []byte

		err = rows.Scan(
			&t.ID,
			&t.EventType,
			&t.UserID,
			&rawPayload,
			&t.DedupKey,
			&t.Attempt,
			&t.Status,
			&t.NextAttemptAt,
			&t.LastError,
			&t.CreatedAt,
			&t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan outbox task: %w", err)
		}

		t.Payload = rawPayload
		tasks = append(tasks, t)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox tasks: %w", err)
	}

	return tasks, nil
}

// MarkSent переводит задачу в статус 'sent'.
func (r *NotificationOutboxRepository) MarkSent(ctx context.Context, id string) error {
	const q = `
		UPDATE notification_outbox
		   SET status     = 'sent',
		       updated_at = NOW()
		 WHERE id = $1`

	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark outbox task sent: %w", err)
	}

	return nil
}

// DeleteSent удаляет задачи со статусом 'sent', чей updated_at старше olderThan.
// Возвращает количество удалённых строк.
func (r *NotificationOutboxRepository) DeleteSent(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	const q = `
		DELETE FROM notification_outbox
		 WHERE status = 'sent'
		   AND updated_at < $1`

	tag, err := r.db.Exec(ctx, q, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete sent outbox tasks: %w", err)
	}

	return tag.RowsAffected(), nil
}

// MarkFailed переводит задачу в статус 'failed' с ошибкой и задержкой до следующей попытки.
func (r *NotificationOutboxRepository) MarkFailed(ctx context.Context, id string, lastErr string, nextAttemptAt time.Time) error {
	const q = `
		UPDATE notification_outbox
		   SET status          = 'failed',
		       last_error      = $2,
		       next_attempt_at = $3,
		       updated_at      = NOW()
		 WHERE id = $1`

	_, err := r.db.Exec(ctx, q, id, lastErr, nextAttemptAt.UTC())
	if err != nil {
		return fmt.Errorf("mark outbox task failed: %w", err)
	}

	return nil
}
