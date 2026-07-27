package store

import (
	"context"
	"fmt"
	"strings"
)

// WSEventOutboxRepository реализует операции с таблицей ws_event_outbox.
type WSEventOutboxRepository struct {
	db db
}

// NewWSEventOutboxRepository создаёт WSEventOutboxRepository.
func NewWSEventOutboxRepository(s *Store) *WSEventOutboxRepository {
	return &WSEventOutboxRepository{db: s.pool}
}

// EnqueueBatch вставляет пачку WS-событий в очередь.
// При конфликте по id молча игнорирует дубль (идемпотентность).
func (r *WSEventOutboxRepository) EnqueueBatch(ctx context.Context, events []WSEventOutbox) error {
	const query = `
INSERT INTO ws_event_outbox (id, event_type, user_id, payload, created_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (id) DO NOTHING`

	for _, e := range events {
		if _, err := r.db.Exec(ctx, query, e.ID, e.EventType, e.UserID, e.Payload); err != nil {
			return fmt.Errorf("enqueue ws event for user %s: %w", e.UserID, err)
		}
	}

	return nil
}

// ClaimBatch выбирает до batchSize необработанных событий (processed_at IS NULL),
// упорядоченных по времени создания.
func (r *WSEventOutboxRepository) ClaimBatch(ctx context.Context, batchSize int) ([]WSEventOutbox, error) {
	const query = `
SELECT id, event_type, user_id, payload, created_at
FROM ws_event_outbox
WHERE processed_at IS NULL
ORDER BY created_at ASC
LIMIT $1`

	rows, err := r.db.Query(ctx, query, batchSize)
	if err != nil {
		return nil, fmt.Errorf("claim ws event batch: %w", err)
	}
	defer rows.Close()

	var events []WSEventOutbox
	for rows.Next() {
		var e WSEventOutbox
		if err = rows.Scan(&e.ID, &e.EventType, &e.UserID, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ws event row: %w", err)
		}
		events = append(events, e)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ws event rows: %w", err)
	}

	return events, nil
}

// MarkProcessedBatch проставляет processed_at = NOW() для списка событий по id.
func (r *WSEventOutboxRepository) MarkProcessedBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// Строим список плейсхолдеров $1, $2, ...
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(
		"UPDATE ws_event_outbox SET processed_at = NOW() WHERE id IN (%s)",
		strings.Join(placeholders, ", "),
	)

	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("mark ws events processed: %w", err)
	}

	return nil
}
