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

func normalizeDialogUsers(user1ID, user2ID string) (string, string, error) {
	if user1ID == "" || user2ID == "" || user1ID == user2ID {
		return "", "", errInvalidDialogUsers
	}

	if user1ID < user2ID {
		return user1ID, user2ID, nil
	}

	return user2ID, user1ID, nil
}
