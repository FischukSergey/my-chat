package push_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	webpushlib "github.com/SherClockHolmes/webpush-go"

	"my-chat/internal/clients/push"
	"my-chat/internal/store"
)

const (
	testEndpoint       = "https://push.example.com/push/test"
	testPlatform       = "web"
	testSubject        = "test@example.com"
	eventTypeMessage   = "message_new"
	eventTypeBadgeSync = "badge_sync"
)

// newTestSubscriptionJSON возвращает JSON-строку валидной Web Push подписки с тестовыми ключами.
func newTestSubscriptionJSON(t *testing.T) string {
	t.Helper()

	curve := ecdh.P256()
	key, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdh key: %v", err)
	}
	p256dh := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())

	authBytes := make([]byte, 16)
	if _, readErr := rand.Read(authBytes); readErr != nil {
		t.Fatalf("generate auth bytes: %v", readErr)
	}
	auth := base64.RawURLEncoding.EncodeToString(authBytes)

	sub := webpushlib.Subscription{
		Endpoint: testEndpoint,
		Keys:     webpushlib.Keys{P256dh: p256dh, Auth: auth},
	}
	data, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("marshal subscription: %v", err)
	}
	return string(data)
}

// testVAPIDKeys генерирует тестовую пару VAPID-ключей.
func testVAPIDKeys(t *testing.T) (private, public string) {
	t.Helper()
	priv, pub, err := webpushlib.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate VAPID keys: %v", err)
	}
	return priv, pub
}

// mockHTTPClient — простой HTTPClient, возвращающий заданный HTTP-статус.
type mockHTTPClient struct {
	statusCode int
}

func (m *mockHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

// --- buildPayload tests ---

func TestBuildPayload_Alert(t *testing.T) {
	msg := push.Message{
		EventType:      eventTypeMessage,
		SenderUsername: "alice",
		Preview:        "секретный текст",
		Badge:          3,
		DialogID:       "dialog-1",
		MessageID:      "msg-42",
	}

	data, err := push.BuildPayload(msg)
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}

	var got map[string]any
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]any{
		"title":      "alice",
		"body":       "Новое сообщение",
		"dialog_id":  "dialog-1",
		"message_id": "msg-42",
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("payload[%q]: got %v, want %v", k, got[k], want)
		}
	}
	if got["badge"].(float64) != 3 {
		t.Errorf("badge: got %v, want 3", got["badge"])
	}
	if got["title"] == msg.Preview {
		t.Error("title must not be message preview")
	}
}

func TestBuildPayload_Alert_UsernameFallback(t *testing.T) {
	msg := push.Message{
		EventType: eventTypeMessage,
		Preview:   "привет",
		Badge:     1,
	}

	data, err := push.BuildPayload(msg)
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}

	var got map[string]any
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["title"] != "user" {
		t.Errorf("title: got %v, want user (fallback)", got["title"])
	}
	if got["body"] != "Новое сообщение" {
		t.Errorf("body: got %v", got["body"])
	}
}

func TestBuildPayload_BadgeSync(t *testing.T) {
	msg := push.Message{
		EventType: eventTypeBadgeSync,
		Badge:     7,
	}

	data, err := push.BuildPayload(msg)
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}

	var got map[string]any
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["type"] != "badge_sync" {
		t.Errorf("type: got %v, want badge_sync", got["type"])
	}
	if got["badge"].(float64) != 7 {
		t.Errorf("badge: got %v, want 7", got["badge"])
	}
	if _, ok := got["title"]; ok {
		t.Error("badge_sync payload не должен содержать поле title")
	}
	if _, ok := got["body"]; ok {
		t.Error("badge_sync payload не должен содержать поле body")
	}
}

// --- Send tests ---

func newTestProvider(t *testing.T, statusCode int) *push.WebPushProvider {
	t.Helper()
	priv, pub := testVAPIDKeys(t)
	return push.NewWebPushProviderWithClient(
		push.WebPushConfig{VAPIDPrivateKey: priv, VAPIDPublicKey: pub, Subject: testSubject},
		&mockHTTPClient{statusCode: statusCode},
	)
}

func TestWebPushProvider_Send_SubscriptionGone_on_410(t *testing.T) {
	provider := newTestProvider(t, http.StatusGone)
	msg := push.Message{
		Device:    store.Device{ID: "dev-1", Platform: testPlatform, PushSubscription: newTestSubscriptionJSON(t)},
		EventType: eventTypeMessage,
		Preview:   "test",
		Badge:     1,
	}

	err := provider.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("ожидалась ошибка при HTTP 410, получили nil")
	}
	if !errors.Is(err, push.ErrSubscriptionGone) {
		t.Errorf("ошибка должна быть ErrSubscriptionGone, получили: %v", err)
	}
}

func TestWebPushProvider_Send_SubscriptionGone_on_404(t *testing.T) {
	provider := newTestProvider(t, http.StatusNotFound)
	msg := push.Message{
		Device:    store.Device{ID: "dev-2", Platform: testPlatform, PushSubscription: newTestSubscriptionJSON(t)},
		EventType: eventTypeBadgeSync,
		Badge:     0,
	}

	err := provider.Send(context.Background(), msg)
	if !errors.Is(err, push.ErrSubscriptionGone) {
		t.Errorf("ожидался ErrSubscriptionGone при 404, получили: %v", err)
	}
}

func TestWebPushProvider_Send_ServerError(t *testing.T) {
	provider := newTestProvider(t, http.StatusInternalServerError)
	msg := push.Message{
		Device:    store.Device{ID: "dev-3", Platform: testPlatform, PushSubscription: newTestSubscriptionJSON(t)},
		EventType: eventTypeMessage,
		Badge:     2,
	}

	err := provider.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("ожидалась ошибка при HTTP 500, получили nil")
	}
	if errors.Is(err, push.ErrSubscriptionGone) {
		t.Error("HTTP 500 не должен возвращать ErrSubscriptionGone")
	}
}

func TestWebPushProvider_Send_Success(t *testing.T) {
	provider := newTestProvider(t, http.StatusCreated)
	msg := push.Message{
		Device:    store.Device{ID: "dev-4", Platform: testPlatform, PushSubscription: newTestSubscriptionJSON(t)},
		EventType: eventTypeMessage,
		Preview:   "hello",
		Badge:     1,
		DialogID:  "d1",
		MessageID: "m1",
	}

	if err := provider.Send(context.Background(), msg); err != nil {
		t.Errorf("неожиданная ошибка при успешной отправке: %v", err)
	}
}

func TestNormalizeVAPIDSubject(t *testing.T) {
	const email = "admin@beepru.ru"
	cases := map[string]string{
		email:               email,
		"mailto:" + email:   email,
		"https://beepru.ru": "https://beepru.ru",
		"  mailto:a@b.c  ":  "a@b.c",
	}
	for in, want := range cases {
		got := push.NormalizeVAPIDSubject(in)
		if got != want {
			t.Errorf("NormalizeVAPIDSubject(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestWebPushProvider_Send_EmptySubscription(t *testing.T) {
	provider := newTestProvider(t, http.StatusCreated)
	msg := push.Message{
		Device:    store.Device{ID: "dev-5", Platform: testPlatform, PushSubscription: ""},
		EventType: eventTypeMessage,
	}

	if err := provider.Send(context.Background(), msg); err == nil {
		t.Error("ожидалась ошибка при пустой push_subscription")
	}
}
