//go:build integration

package chat_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"my-chat/internal/hub"
	chat "my-chat/internal/services/chat"
	"my-chat/internal/services/expirer"
	"my-chat/internal/services/wsdelivery"
	"my-chat/internal/store"
)

// --- helpers ---

// captureHub перехватывает hub.Send-вызовы для проверки в тестах.
type captureHub struct {
	mu   sync.Mutex
	sent []capturedEvent
}

type capturedEvent struct {
	userID string
	event  hub.Event
}

func (h *captureHub) Send(_ context.Context, userID string, event hub.Event) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, capturedEvent{userID: userID, event: event})
	return true
}

func (h *captureHub) byType(eventType string) []capturedEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []capturedEvent
	for _, e := range h.sent {
		if e.event.Event == eventType {
			out = append(out, e)
		}
	}
	return out
}

// testDB открывает соединение и накатывает миграции.
func testDB(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	s, err := store.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to db: %v", err)
	}
	t.Cleanup(s.Close)
	if _, err = s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// insertUsers создаёт двух пользователей и регистрирует удаление.
func insertUsers(t *testing.T, s *store.Store, userAID, userBID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.DB().Exec(ctx, "INSERT INTO users (id) VALUES ($1), ($2)", userAID, userBID); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = s.DB().Exec(c, "DELETE FROM ws_event_outbox WHERE user_id IN ($1,$2)", userAID, userBID)
		_, _ = s.DB().Exec(c, "DELETE FROM dialogs WHERE user_a_id IN ($1,$2) OR user_b_id IN ($1,$2)", userAID, userBID)
		_, _ = s.DB().Exec(c, "DELETE FROM users WHERE id IN ($1,$2)", userAID, userBID)
	})
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newServiceWithTTL создаёт chat.Service с реальными репозиториями и заданным TTL.
// Использует noopNotifier и noopOutbox из service_test.go.
func newServiceWithTTL(
	dialogRepo *store.DialogRepository,
	messageRepo *store.MessageRepository,
	receiptRepo *store.ReceiptRepository,
	ttl time.Duration,
) *chat.Service {
	return chat.NewService(dialogRepo, messageRepo, receiptRepo, noopNotifier(), noopOutbox(), ttl)
}

// TestIntegration_TTL_ExpireMessages_ListEmpty проверяет пункт 14 чеклиста:
// SendMessage → MarkRead (TTL истёк) → expirer.Tick → ListMessages возвращает пустой список.
func TestIntegration_TTL_ExpireMessages_ListEmpty(t *testing.T) {
	t.Parallel()

	s := testDB(t)

	userAID, userBID := uuid.NewString(), uuid.NewString()
	insertUsers(t, s, userAID, userBID)

	ctx := context.Background()
	dialogRepo := store.NewDialogRepository(s)
	messageRepo := store.NewMessageRepository(s)
	receiptRepo := store.NewReceiptRepository(s)
	wsOutboxRepo := store.NewWSEventOutboxRepository(s)

	dialog, err := dialogRepo.GetOrCreate(ctx, uuid.NewString(), userAID, userBID)
	if err != nil {
		t.Fatalf("GetOrCreate dialog: %v", err)
	}

	const ttl = time.Second
	chatSvc := newServiceWithTTL(dialogRepo, messageRepo, receiptRepo, ttl)

	msg, err := chatSvc.SendMessage(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialog.ID,
		SenderID: userAID,
		Body:     "ttl-expire-list test",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// readAt в прошлом → expires_at = readAt + ttl < now.
	if err = chatSvc.MarkRead(ctx, msg.ID, userBID, time.Now().UTC().Add(-ttl-time.Second)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	exp := expirer.New(messageRepo, wsOutboxRepo, silentLogger(), 100)
	n, err := exp.Tick(ctx)
	if err != nil {
		t.Fatalf("expirer.Tick: %v", err)
	}
	if n == 0 {
		t.Fatal("expected expirer to find at least 1 expired message")
	}

	messages, err := chatSvc.ListMessages(ctx, userAID, dialog.ID, 50, nil)
	if err != nil {
		t.Fatalf("ListMessages after expiry: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after TTL expiry, got %d", len(messages))
	}
}

// TestIntegration_TTL_WsDelivery_MessageDeletedEvent проверяет пункт 14 чеклиста:
// после expirer.Tick оба участника диалога получают message_deleted через wsdelivery.
// Тест намеренно не параллельный: ClaimBatch работает с глобальной таблицей ws_event_outbox,
// параллельный запуск создаёт гонку на владение записями.
func TestIntegration_TTL_WsDelivery_MessageDeletedEvent(t *testing.T) {
	s := testDB(t)

	userAID, userBID := uuid.NewString(), uuid.NewString()
	insertUsers(t, s, userAID, userBID)

	ctx := context.Background()
	dialogRepo := store.NewDialogRepository(s)
	messageRepo := store.NewMessageRepository(s)
	receiptRepo := store.NewReceiptRepository(s)
	wsOutboxRepo := store.NewWSEventOutboxRepository(s)

	dialog, err := dialogRepo.GetOrCreate(ctx, uuid.NewString(), userAID, userBID)
	if err != nil {
		t.Fatalf("GetOrCreate dialog: %v", err)
	}

	const ttl = time.Second
	chatSvc := newServiceWithTTL(dialogRepo, messageRepo, receiptRepo, ttl)

	msg, err := chatSvc.SendMessage(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialog.ID,
		SenderID: userAID,
		Body:     "ws-delivery test",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if err = chatSvc.MarkRead(ctx, msg.ID, userBID, time.Now().UTC().Add(-ttl-time.Second)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	exp := expirer.New(messageRepo, wsOutboxRepo, silentLogger(), 100)
	if _, err = exp.Tick(ctx); err != nil {
		t.Fatalf("expirer.Tick: %v", err)
	}

	h := &captureHub{}
	delivery := wsdelivery.New(wsOutboxRepo, h, silentLogger(), 100)
	n, err := delivery.RunOnce(ctx)
	if err != nil {
		t.Fatalf("wsdelivery.RunOnce: %v", err)
	}
	if n == 0 {
		t.Fatal("wsdelivery processed 0 events; expected at least 2 (one per participant)")
	}

	// Тесты работают параллельно на одной БД: ClaimBatch может забрать события
	// от других тестов. Фильтруем только события для конкретного msg.ID этого теста.
	var targetEvents []capturedEvent
	for _, ev := range h.byType(hub.EventMessageDeleted) {
		rawData, marshalErr := json.Marshal(ev.event.Data)
		if marshalErr != nil {
			continue
		}
		var payload struct {
			MessageID string `json:"message_id"`
			DialogID  string `json:"dialog_id"`
		}
		if json.Unmarshal(rawData, &payload) != nil {
			continue
		}
		if payload.MessageID == msg.ID && payload.DialogID == dialog.ID {
			targetEvents = append(targetEvents, ev)
		}
	}

	if len(targetEvents) < 2 {
		t.Fatalf("expected ≥2 message_deleted events for msg %q (one per participant), got %d",
			msg.ID, len(targetEvents))
	}

	recipients := map[string]bool{}
	for _, ev := range targetEvents {
		recipients[ev.userID] = true
	}
	if !recipients[userAID] {
		t.Errorf("userA (%s) did not receive message_deleted", userAID)
	}
	if !recipients[userBID] {
		t.Errorf("userB (%s) did not receive message_deleted", userBID)
	}
}

// TestIntegration_TTL_Reconnect_NoDeletedMessages проверяет пункт 14 чеклиста:
// после истечения TTL повторный ListMessages (reconnect) не возвращает удалённых сообщений.
func TestIntegration_TTL_Reconnect_NoDeletedMessages(t *testing.T) {
	t.Parallel()

	s := testDB(t)

	userAID, userBID := uuid.NewString(), uuid.NewString()
	insertUsers(t, s, userAID, userBID)

	ctx := context.Background()
	dialogRepo := store.NewDialogRepository(s)
	messageRepo := store.NewMessageRepository(s)
	receiptRepo := store.NewReceiptRepository(s)
	wsOutboxRepo := store.NewWSEventOutboxRepository(s)

	dialog, err := dialogRepo.GetOrCreate(ctx, uuid.NewString(), userAID, userBID)
	if err != nil {
		t.Fatalf("GetOrCreate dialog: %v", err)
	}

	const ttl = time.Second
	chatSvc := newServiceWithTTL(dialogRepo, messageRepo, receiptRepo, ttl)

	msg, err := chatSvc.SendMessage(ctx, store.Message{
		ID:       uuid.NewString(),
		DialogID: dialog.ID,
		SenderID: userAID,
		Body:     "reconnect test",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if err = chatSvc.MarkRead(ctx, msg.ID, userBID, time.Now().UTC().Add(-ttl-time.Second)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	exp := expirer.New(messageRepo, wsOutboxRepo, silentLogger(), 100)
	if _, err = exp.Tick(ctx); err != nil {
		t.Fatalf("expirer.Tick: %v", err)
	}

	// Simulates client reconnect: fresh ListMessages call for both participants.
	for _, uid := range []string{userAID, userBID} {
		msgs, listErr := chatSvc.ListMessages(ctx, uid, dialog.ID, 50, nil)
		if listErr != nil {
			t.Fatalf("ListMessages (%s): %v", uid, listErr)
		}
		for _, m := range msgs {
			if m.ID == msg.ID {
				t.Errorf("user %s sees expired message %q after reconnect", uid, msg.ID)
			}
		}
	}
}
