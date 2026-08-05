package push

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	webpushlib "github.com/SherClockHolmes/webpush-go"
)

// WebPushConfig содержит VAPID-параметры для Web Push провайдера.
type WebPushConfig struct {
	VAPIDPrivateKey string
	VAPIDPublicKey  string
	// Subject — mailto: или https: URL; обязателен по спецификации VAPID.
	Subject string
}

// IsConfigured проверяет, заполнены ли все обязательные поля.
func (c WebPushConfig) IsConfigured() bool {
	return c.VAPIDPrivateKey != "" && c.VAPIDPublicKey != "" && c.Subject != ""
}

// WebPushProvider отправляет Web Push уведомления через VAPID.
// Реализует интерфейс Provider.
type WebPushProvider struct {
	cfg        WebPushConfig
	httpClient webpushlib.HTTPClient // nil = default http.Client; переопределяется в тестах
}

// NewWebPushProvider создаёт WebPushProvider с дефолтным HTTP-клиентом.
func NewWebPushProvider(cfg WebPushConfig) *WebPushProvider {
	return &WebPushProvider{cfg: cfg}
}

// newWebPushProviderWithClient создаёт WebPushProvider с кастомным HTTP-клиентом (для тестов).
func newWebPushProviderWithClient(cfg WebPushConfig, client webpushlib.HTTPClient) *WebPushProvider {
	return &WebPushProvider{cfg: cfg, httpClient: client}
}

// Name возвращает идентификатор провайдера.
func (p *WebPushProvider) Name() string { return "webpush" }

// alertPayload — JSON-структура обычного push-уведомления.
type alertPayload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Badge     int    `json:"badge"`
	DialogID  string `json:"dialog_id"`
	MessageID string `json:"message_id"`
}

// badgeSyncPayload — JSON-структура silent badge-sync уведомления.
type badgeSyncPayload struct {
	Type  string `json:"type"`
	Badge int    `json:"badge"`
}

// buildPayload формирует JSON-payload в зависимости от типа события.
func buildPayload(msg Message) ([]byte, error) {
	if msg.EventType == "badge_sync" {
		return json.Marshal(badgeSyncPayload{
			Type:  "badge_sync",
			Badge: msg.Badge,
		})
	}
	return json.Marshal(alertPayload{
		Title:     msg.Preview,
		Body:      "Новое сообщение",
		Badge:     msg.Badge,
		DialogID:  msg.DialogID,
		MessageID: msg.MessageID,
	})
}

// Send отправляет push-уведомление на одно Web Push устройство.
// Возвращает ErrSubscriptionGone если push-сервер ответил 404 или 410.
func (p *WebPushProvider) Send(ctx context.Context, msg Message) error {
	if msg.Device.PushSubscription == "" {
		return fmt.Errorf("device %s: пустая push_subscription", msg.Device.ID)
	}

	var sub webpushlib.Subscription
	if err := json.Unmarshal([]byte(msg.Device.PushSubscription), &sub); err != nil {
		return fmt.Errorf("parse push_subscription (device %s): %w", msg.Device.ID, err)
	}

	payload, err := buildPayload(msg)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	opts := &webpushlib.Options{
		HTTPClient:      p.httpClient,
		VAPIDPrivateKey: p.cfg.VAPIDPrivateKey,
		VAPIDPublicKey:  p.cfg.VAPIDPublicKey,
		Subscriber:      p.cfg.Subject,
		TTL:             60,
	}

	resp, err := webpushlib.SendNotificationWithContext(ctx, payload, &sub, opts)
	if err != nil {
		return fmt.Errorf("send web push (device %s): %w", msg.Device.ID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return nil
	case http.StatusNotFound, http.StatusGone:
		return fmt.Errorf("device %s HTTP %d: %w", msg.Device.ID, resp.StatusCode, ErrSubscriptionGone)
	default:
		return fmt.Errorf("web push server HTTP %d (device %s)", resp.StatusCode, msg.Device.ID)
	}
}
