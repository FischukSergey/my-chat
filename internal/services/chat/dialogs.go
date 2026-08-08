package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

var (
	// ErrInvalidDialogUsername — пустой username при создании диалога.
	ErrInvalidDialogUsername = errors.New("username is required")
	// ErrCannotDialogWithSelf — попытка создать диалог с самим собой.
	ErrCannotDialogWithSelf = errors.New("cannot create dialog with yourself")
	// ErrDialogUserNotFound — peer не найден или не active.
	ErrDialogUserNotFound = errors.New("user not found")
)

// Peer — собеседник в списке диалогов.
type Peer struct {
	UserID   string
	Username string
}

// LastMessagePreview — превью последнего сообщения.
type LastMessagePreview struct {
	MessageID   string
	SenderID    string
	BodyPreview string
	CreatedAt   time.Time
}

// DialogItem — элемент списка / ответа create (Sprint 7).
type DialogItem struct {
	DialogID    string
	Peer        Peer
	LastMessage *LastMessagePreview
	UnreadCount int
	UpdatedAt   time.Time
}

type userRepository interface {
	FindByUsername(ctx context.Context, username string) (store.User, error)
	FindByID(ctx context.Context, userID string) (store.User, error)
}

// ListDialogs возвращает диалоги пользователя (updated_at DESC).
func (s *Service) ListDialogs(ctx context.Context, userID string) ([]DialogItem, error) {
	rows, err := s.dialogs.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list dialogs: %w", err)
	}

	items := make([]DialogItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dialogItemFromRow(row))
	}

	return items, nil
}

// CreateDialogByUsername создаёт или возвращает 1:1 диалог по username peer.
func (s *Service) CreateDialogByUsername(ctx context.Context, userID, username string) (DialogItem, error) {
	if s.users == nil {
		return DialogItem{}, errors.New("users repository is not configured")
	}

	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return DialogItem{}, ErrInvalidDialogUsername
	}

	peer, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return DialogItem{}, ErrDialogUserNotFound
		}
		return DialogItem{}, fmt.Errorf("find user by username: %w", err)
	}

	if peer.Status != "active" {
		return DialogItem{}, ErrDialogUserNotFound
	}

	if peer.ID == userID {
		return DialogItem{}, ErrCannotDialogWithSelf
	}

	dialog, err := s.dialogs.GetOrCreate(ctx, uuid.NewString(), userID, peer.ID)
	if err != nil {
		return DialogItem{}, fmt.Errorf("get or create dialog: %w", err)
	}

	rows, err := s.dialogs.ListByUserID(ctx, userID)
	if err != nil {
		return DialogItem{}, fmt.Errorf("list dialogs after create: %w", err)
	}

	for _, row := range rows {
		if row.DialogID == dialog.ID {
			return dialogItemFromRow(row), nil
		}
	}

	// Fallback: только что созданный пустой диалог.
	return DialogItem{
		DialogID: dialog.ID,
		Peer: Peer{
			UserID:   peer.ID,
			Username: peer.Username,
		},
		LastMessage: nil,
		UnreadCount: 0,
		UpdatedAt:   dialog.CreatedAt,
	}, nil
}

func dialogItemFromRow(row store.DialogListItem) DialogItem {
	item := DialogItem{
		DialogID: row.DialogID,
		Peer: Peer{
			UserID:   row.PeerUserID,
			Username: row.PeerUsername,
		},
		UnreadCount: row.UnreadCount,
		UpdatedAt:   row.UpdatedAt,
	}

	if row.LastMessageID != nil && row.LastMessageSenderID != nil &&
		row.LastMessageBody != nil && row.LastMessageCreatedAt != nil {
		item.LastMessage = &LastMessagePreview{
			MessageID:   *row.LastMessageID,
			SenderID:    *row.LastMessageSenderID,
			BodyPreview: buildPreview(*row.LastMessageBody),
			CreatedAt:   *row.LastMessageCreatedAt,
		}
	}

	return item
}
