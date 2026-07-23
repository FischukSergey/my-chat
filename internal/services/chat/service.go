// Package chat содержит сервисную бизнес-логику сообщений.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"my-chat/internal/hub"
	"my-chat/internal/store"
)

var (
	// ErrForbiddenDialogAccess возвращается, если пользователь не участник диалога.
	ErrForbiddenDialogAccess = errors.New("user does not belong to dialog")
	// ErrInvalidMessageBody возвращается при пустом тексте сообщения.
	ErrInvalidMessageBody = errors.New("message body is empty")
)

const (
	previewMaxRunes = 120
)

// Service оркестрирует операции над сообщениями и receipt-статусами.
type Service struct {
	dialogs  dialogRepository
	messages messageRepository
	receipts receiptRepository
	notifier notifier
	outbox   outboxPublisher
	ttl      time.Duration // 0 = сообщения не истекают
}

type dialogRepository interface {
	GetByID(ctx context.Context, dialogID string) (store.Dialog, error)
}

type messageRepository interface {
	Create(ctx context.Context, message store.Message) (store.Message, error)
	GetByID(ctx context.Context, messageID string) (store.Message, error)
	ListByDialog(ctx context.Context, dialogID string, limit int, before *time.Time) ([]store.Message, error)
}

type receiptRepository interface {
	Ensure(ctx context.Context, messageID, userID string) error
	MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error
	CountUnread(ctx context.Context, userID string) (int, error)
}

type notifier interface {
	Send(ctx context.Context, userID string, event hub.Event) bool
}

type outboxPublisher interface {
	Enqueue(ctx context.Context, task store.NotificationOutbox) error
}

// NewService создает сервис чата.
// ttl задаёт время жизни сообщений; 0 — без TTL.
func NewService(
	dialogs dialogRepository,
	messages messageRepository,
	receipts receiptRepository,
	n notifier,
	outbox outboxPublisher,
	ttl time.Duration,
) *Service {
	return &Service{
		dialogs:  dialogs,
		messages: messages,
		receipts: receipts,
		notifier: n,
		outbox:   outbox,
		ttl:      ttl,
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

	if s.ttl > 0 {
		exp := time.Now().UTC().Add(s.ttl)
		message.ExpiresAt = &exp
	}

	created, err := s.messages.Create(ctx, message)
	if err != nil {
		return store.Message{}, fmt.Errorf("create message: %w", err)
	}

	if err = s.receipts.Ensure(ctx, created.ID, receiverID); err != nil {
		return store.Message{}, fmt.Errorf("ensure message receipt: %w", err)
	}

	s.notifyNewMessage(ctx, created, receiverID)

	return created, nil
}

func (s *Service) notifyNewMessage(ctx context.Context, msg store.Message, receiverID string) {
	payload := map[string]any{
		"message_id": msg.ID,
		"dialog_id":  msg.DialogID,
		"sender_id":  msg.SenderID,
		"body":       msg.Body,
		"created_at": msg.CreatedAt.UTC().Format(time.RFC3339),
		"expires_at": formatOptionalTime(msg.ExpiresAt),
	}

	newEvent := hub.NewEvent("message_new", payload)

	receiverOnline := s.notifier.Send(ctx, receiverID, newEvent)
	if !receiverOnline {
		s.enqueueOutbox(ctx, msg, receiverID)
		return
	}

	deliveredAt := time.Now().UTC()
	s.notifier.Send(ctx, msg.SenderID, hub.NewEvent("message_delivered", map[string]any{
		"message_id":   msg.ID,
		"dialog_id":    msg.DialogID,
		"user_id":      receiverID,
		"delivered_at": deliveredAt.Format(time.RFC3339),
	}))
}

// enqueueOutbox публикует push-задачу в outbox для offline-получателя.
// Ошибки не возвращаются — операция best-effort; при сбое push не дойдёт,
// но сообщение уже сохранено.
func (s *Service) enqueueOutbox(ctx context.Context, msg store.Message, receiverID string) {
	unreadCount, err := s.receipts.CountUnread(ctx, receiverID)
	if err != nil {
		return
	}

	dedupKey := fmt.Sprintf("message_new:%s:%s", msg.ID, receiverID)

	type outboxPayload struct {
		EventType   string `json:"event_type"`
		UserID      string `json:"user_id"`
		MessageID   string `json:"message_id"`
		DialogID    string `json:"dialog_id"`
		SenderID    string `json:"sender_id"`
		Preview     string `json:"preview"`
		UnreadCount int    `json:"unread_count"`
		CreatedAt   string `json:"created_at"`
		DedupKey    string `json:"dedup_key"`
	}

	payloadBytes, err := json.Marshal(outboxPayload{
		EventType:   "message_new",
		UserID:      receiverID,
		MessageID:   msg.ID,
		DialogID:    msg.DialogID,
		SenderID:    msg.SenderID,
		Preview:     buildPreview(msg.Body),
		UnreadCount: unreadCount,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		DedupKey:    dedupKey,
	})
	if err != nil {
		return
	}

	task := store.NotificationOutbox{
		ID:        uuid.NewString(),
		EventType: "message_new",
		UserID:    receiverID,
		Payload:   payloadBytes,
		DedupKey:  dedupKey,
	}

	_ = s.outbox.Enqueue(ctx, task)
}

// BuildPreview обрезает текст до previewMaxRunes рун и нормализует переносы строк.
func BuildPreview(body string) string {
	return buildPreview(body)
}

// buildPreview — внутренняя реализация.
func buildPreview(body string) string {
	body = strings.ReplaceAll(body, "\r\n", " ")
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\r", " ")

	if utf8.RuneCountInString(body) <= previewMaxRunes {
		return body
	}

	runes := []rune(body)
	return string(runes[:previewMaxRunes])
}

// ListMessages возвращает историю сообщений диалога.
func (s *Service) ListMessages(
	ctx context.Context,
	userID, dialogID string,
	limit int,
	before *time.Time,
) ([]store.Message, error) {
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

	msg, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	if err = s.receipts.MarkRead(ctx, messageID, userID, readAt); err != nil {
		return fmt.Errorf("mark message read: %w", err)
	}

	s.notifier.Send(ctx, msg.SenderID, hub.NewEvent("message_read", map[string]any{
		"message_id": messageID,
		"dialog_id":  msg.DialogID,
		"user_id":    userID,
		"read_at":    readAt.UTC().Format(time.RFC3339),
	}))

	s.sendBadgeUpdated(ctx, userID)

	return nil
}

// sendBadgeUpdated пересчитывает unread и отправляет badge_updated читателю через WS.
// Ошибки best-effort: сбой подсчёта не откатывает MarkRead.
func (s *Service) sendBadgeUpdated(ctx context.Context, userID string) {
	unreadCount, err := s.receipts.CountUnread(ctx, userID)
	if err != nil {
		return
	}

	s.notifier.Send(ctx, userID, hub.NewEvent("badge_updated", map[string]any{
		"user_id":      userID,
		"unread_count": unreadCount,
		"badge":        unreadCount,
		"reason":       "message_read",
	}))
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

// formatOptionalTime форматирует *time.Time в RFC3339 строку или nil.
func formatOptionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}

	return t.UTC().Format(time.RFC3339)
}
