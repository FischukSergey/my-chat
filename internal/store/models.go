package store

import "time"

// AuthSession представляет server-side запись refresh-сессии пользователя.
type AuthSession struct {
	ID          string
	UserID      string
	FamilyID    string
	TokenHash   string  // SHA-256(refresh_token), hex-encoded
	DeviceID    *string // опциональная связь с devices
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	RotatedFrom *string // ID предыдущей сессии в цепочке ротации
	CreatedAt   time.Time
}

// Dialog представляет диалог между двумя пользователями.
type Dialog struct {
	ID        string
	UserAID   string
	UserBID   string
	CreatedAt time.Time
}

// Message представляет сообщение в диалоге.
type Message struct {
	ID        string
	DialogID  string
	SenderID  string
	Body      string
	CreatedAt time.Time
}

// Device представляет зарегистрированное устройство пользователя для push-уведомлений.
type Device struct {
	ID         string
	UserID     string
	Platform   string
	PushToken  string
	Enabled    bool
	LastSeenAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NotificationOutboxStatus — допустимые статусы outbox-задачи.
type NotificationOutboxStatus string

const (
	// OutboxStatusPending — задача ожидает обработки или повторной попытки.
	OutboxStatusPending NotificationOutboxStatus = "pending"
	// OutboxStatusSent — задача успешно отправлена.
	OutboxStatusSent NotificationOutboxStatus = "sent"
	// OutboxStatusFailed — задача превысила лимит попыток или отложена до next_attempt_at.
	OutboxStatusFailed NotificationOutboxStatus = "failed"
)

// NotificationOutbox представляет задачу на отправку push-уведомления.
type NotificationOutbox struct {
	ID            string
	EventType     string
	UserID        string
	Payload       []byte
	DedupKey      string
	Attempt       int
	Status        NotificationOutboxStatus
	NextAttemptAt time.Time
	LastError     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
