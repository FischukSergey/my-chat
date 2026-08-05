package store

import "time"

// User представляет пользователя системы.
type User struct {
	ID           string
	Status       string // "active" | "blocked"
	Username     string // уникальный логин; пустая строка у legacy-пользователей без credentials
	PasswordHash string // bcrypt(cost=12); пустая строка у legacy-пользователей
	CreatedAt    time.Time
}

// AuthSession представляет server-side запись refresh-сессии пользователя.
type AuthSession struct {
	ID          string
	UserID      string
	FamilyID    string
	TokenHash   string  // SHA-256(refresh_token), hex-encoded
	DeviceID    *string // клиентский X-Device-ID (привязка refresh); не FK на devices
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
	ExpiresAt *time.Time // NULL — сообщение не истекает
	DeletedAt *time.Time // NULL — сообщение активно; soft delete
}

// Device представляет зарегистрированное устройство пользователя для push-уведомлений.
type Device struct {
	ID               string
	UserID           string
	Platform         string
	PushToken        string // legacy APNs/FCM; пустая строка = NULL в БД (platform=web)
	PushSubscription string // JSON Web Push subscription; пустая строка = NULL в БД (platform=ios/android)
	Enabled          bool
	LastSeenAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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

// WSEventOutbox представляет WS-событие, ожидающее доставки конкретному пользователю.
type WSEventOutbox struct {
	ID          string
	EventType   string
	UserID      string
	Payload     []byte     // JSONB
	ProcessedAt *time.Time // NULL — событие ещё не обработано
	CreatedAt   time.Time
}

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
