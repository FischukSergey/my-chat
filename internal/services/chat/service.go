// Package chat содержит сервисную бизнес-логику сообщений.
package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"my-chat/internal/store"
)

var (
	// ErrForbiddenDialogAccess возвращается, если пользователь не участник диалога.
	ErrForbiddenDialogAccess = errors.New("user does not belong to dialog")
	// ErrInvalidMessageBody возвращается при пустом тексте сообщения.
	ErrInvalidMessageBody = errors.New("message body is empty")
)

// Service оркестрирует операции над сообщениями и receipt-статусами.
type Service struct {
	dialogs  dialogRepository
	messages messageRepository
	receipts receiptRepository
}

type dialogRepository interface {
	GetByID(ctx context.Context, dialogID string) (store.Dialog, error)
}

type messageRepository interface {
	Create(ctx context.Context, message store.Message) (store.Message, error)
	ListByDialog(ctx context.Context, dialogID string, limit int, before *time.Time) ([]store.Message, error)
}

type receiptRepository interface {
	Ensure(ctx context.Context, messageID, userID string) error
	MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error
	CountUnread(ctx context.Context, userID string) (int, error)
}

// NewService создает сервис чата.
func NewService(
	dialogs dialogRepository,
	messages messageRepository,
	receipts receiptRepository,
) *Service {
	return &Service{
		dialogs:  dialogs,
		messages: messages,
		receipts: receipts,
	}
}

// SendMessage создает сообщение и подготавливает receipt для второго участника.
func (s *Service) SendMessage(ctx context.Context, message store.Message) (store.Message, error) {
	if strings.TrimSpace(message.Body) == "" {
		return store.Message{}, ErrInvalidMessageBody
	}

	dialog, err := s.dialogs.GetByID(ctx, message.DialogID)
	if err != nil {
		return store.Message{}, fmt.Errorf("get dialog: %w", err)
	}

	receiverID, ok := receiverID(dialog, message.SenderID)
	if !ok {
		return store.Message{}, ErrForbiddenDialogAccess
	}

	created, err := s.messages.Create(ctx, message)
	if err != nil {
		return store.Message{}, fmt.Errorf("create message: %w", err)
	}

	if err = s.receipts.Ensure(ctx, created.ID, receiverID); err != nil {
		return store.Message{}, fmt.Errorf("ensure message receipt: %w", err)
	}

	return created, nil
}

// ListMessages возвращает историю сообщений диалога.
func (s *Service) ListMessages(ctx context.Context, userID, dialogID string, limit int, before *time.Time) ([]store.Message, error) {
	dialog, err := s.dialogs.GetByID(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("get dialog: %w", err)
	}

	if _, ok := receiverID(dialog, userID); !ok {
		return nil, ErrForbiddenDialogAccess
	}

	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 100)

	items, err := s.messages.ListByDialog(ctx, dialogID, limit, before)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	return items, nil
}

// MarkRead отмечает сообщение как прочитанное пользователем.
func (s *Service) MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error {
	if readAt.IsZero() {
		readAt = time.Now().UTC()
	}

	if err := s.receipts.MarkRead(ctx, messageID, userID, readAt); err != nil {
		return fmt.Errorf("mark message read: %w", err)
	}

	return nil
}

// UnreadCount возвращает количество непрочитанных сообщений пользователя.
func (s *Service) UnreadCount(ctx context.Context, userID string) (int, error) {
	count, err := s.receipts.CountUnread(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count unread messages: %w", err)
	}

	return count, nil
}

func receiverID(dialog store.Dialog, userID string) (string, bool) {
	if dialog.UserAID == userID {
		return dialog.UserBID, true
	}
	if dialog.UserBID == userID {
		return dialog.UserAID, true
	}

	return "", false
}
