package store

import (
	"context"
	"errors"
	"fmt"
)

var errInvalidDialogUsers = errors.New("dialog requires two different users")

// DialogRepository работает с таблицей dialogs.
type DialogRepository struct {
	poolDB db
}

// NewDialogRepository создает репозиторий диалогов.
func NewDialogRepository(s *Store) *DialogRepository {
	return &DialogRepository{poolDB: s.pool}
}

// GetByID возвращает диалог по идентификатору.
func (r *DialogRepository) GetByID(ctx context.Context, dialogID string) (Dialog, error) {
	const query = `
SELECT id, user_a_id, user_b_id, created_at
FROM dialogs
WHERE id = $1`

	var dialog Dialog
	if err := r.poolDB.QueryRow(ctx, query, dialogID).Scan(
		&dialog.ID,
		&dialog.UserAID,
		&dialog.UserBID,
		&dialog.CreatedAt,
	); err != nil {
		return Dialog{}, fmt.Errorf("select dialog by id: %w", err)
	}

	return dialog, nil
}

// GetOrCreate создает диалог для пары пользователей или возвращает существующий.
func (r *DialogRepository) GetOrCreate(ctx context.Context, dialogID, user1ID, user2ID string) (Dialog, error) {
	userAID, userBID, err := normalizeDialogUsers(user1ID, user2ID)
	if err != nil {
		return Dialog{}, err
	}

	const query = `
INSERT INTO dialogs (id, user_a_id, user_b_id)
VALUES ($1, $2, $3)
ON CONFLICT (user_a_id, user_b_id)
DO UPDATE SET user_a_id = EXCLUDED.user_a_id
RETURNING id, user_a_id, user_b_id, created_at`

	var dialog Dialog
	if err = r.poolDB.QueryRow(ctx, query, dialogID, userAID, userBID).Scan(
		&dialog.ID,
		&dialog.UserAID,
		&dialog.UserBID,
		&dialog.CreatedAt,
	); err != nil {
		return Dialog{}, fmt.Errorf("insert or select dialog: %w", err)
	}

	return dialog, nil
}

// ListByUserID возвращает диалоги пользователя с peer, last message и per-dialog unread.
// Сортировка: updated_at DESC (max(last_message.created_at, dialog.created_at)).
// Soft-deleted сообщения не участвуют в preview и unread.
func (r *DialogRepository) ListByUserID(ctx context.Context, userID string) ([]DialogListItem, error) {
	const query = `
SELECT
    d.id,
    peer.id,
    peer.username,
    lm.id,
    lm.sender_id,
    lm.body,
    lm.created_at,
    COALESCE(uc.unread_count, 0),
    COALESCE(lm.created_at, d.created_at) AS updated_at
FROM dialogs d
JOIN users peer ON peer.id = CASE
    WHEN d.user_a_id = $1 THEN d.user_b_id
    ELSE d.user_a_id
END
LEFT JOIN LATERAL (
    SELECT m.id, m.sender_id, m.body, m.created_at
    FROM messages m
    WHERE m.dialog_id = d.id AND m.deleted_at IS NULL
    ORDER BY m.created_at DESC
    LIMIT 1
) lm ON TRUE
LEFT JOIN LATERAL (
    SELECT COUNT(1)::int AS unread_count
    FROM messages m
    LEFT JOIN message_receipts mr ON mr.message_id = m.id AND mr.user_id = $1
    WHERE m.dialog_id = d.id
      AND m.sender_id <> $1
      AND m.deleted_at IS NULL
      AND mr.read_at IS NULL
) uc ON TRUE
WHERE d.user_a_id = $1 OR d.user_b_id = $1
ORDER BY updated_at DESC`

	rows, err := r.poolDB.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list dialogs by user: %w", err)
	}
	defer rows.Close()

	items := make([]DialogListItem, 0)
	for rows.Next() {
		var item DialogListItem
		if err = rows.Scan(
			&item.DialogID,
			&item.PeerUserID,
			&item.PeerUsername,
			&item.LastMessageID,
			&item.LastMessageSenderID,
			&item.LastMessageBody,
			&item.LastMessageCreatedAt,
			&item.UnreadCount,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dialog list row: %w", err)
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dialog list rows: %w", err)
	}

	return items, nil
}

func normalizeDialogUsers(user1ID, user2ID string) (userAID, userBID string, err error) {
	if user1ID == "" || user2ID == "" || user1ID == user2ID {
		return "", "", errInvalidDialogUsers
	}

	if user1ID < user2ID {
		return user1ID, user2ID, nil
	}

	return user2ID, user1ID, nil
}
