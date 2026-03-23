package store

import "time"

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
