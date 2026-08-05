// Package device_test contains unit tests for the device HTTP handler.
package device_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"my-chat/internal/handlers/device"
	"my-chat/internal/middleware"
	"my-chat/internal/store"
)

// --- mock ---

const (
	platformIOS       = "ios"
	platformAndroid   = "android"
	testPushToken     = "tok"
	fieldPlatform     = "platform"
	fieldPushTokenKey = "push_token"
)

type mockDeviceSvc struct {
	registerFn   func(ctx context.Context, d store.Device) (store.Device, error)
	unregisterFn func(ctx context.Context, userID, pushToken string) error
}

func (m *mockDeviceSvc) Register(ctx context.Context, d store.Device) (store.Device, error) {
	return m.registerFn(ctx, d)
}

func (m *mockDeviceSvc) Unregister(ctx context.Context, userID, pushToken string) error {
	return m.unregisterFn(ctx, userID, pushToken)
}

// --- helpers ---

func newHandler(svc *mockDeviceSvc) *device.Handler {
	return device.New(svc)
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return bytes.NewReader(b)
}

func withUserID(r *http.Request, userID string) *http.Request {
	ctx := middleware.ContextWithUserID(r.Context(), userID)
	return r.WithContext(ctx)
}

func decodeError(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp.Error.Code
}

// --- Register ---

func TestRegister_Success(t *testing.T) {
	t.Parallel()

	svc := &mockDeviceSvc{
		registerFn: func(_ context.Context, d store.Device) (store.Device, error) {
			d.ID = "dev-1"
			d.Enabled = true
			d.LastSeenAt = time.Now()
			return d, nil
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: "tok-123"}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(svc).Register(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Device struct {
			ID       string `json:"id"`
			Platform string `json:"platform"`
		} `json:"device"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Device.ID != "dev-1" {
		t.Errorf("expected device id=dev-1, got %q", resp.Device.ID)
	}
	if resp.Device.Platform != "ios" {
		t.Errorf("expected platform=ios, got %q", resp.Device.Platform)
	}
}

func TestRegister_Unauthorized_MissingUserID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: testPushToken}))
	// userID NOT injected into context
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Register(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		strings.NewReader("{bad json"))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRegister_InvalidPlatform(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: "fax", fieldPushTokenKey: testPushToken}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if code := decodeError(t, rec.Body); code != "invalid_argument" {
		t.Errorf("expected code=invalid_argument, got %q", code)
	}
}

func TestRegister_EmptyToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: "   "}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRegister_TokenTooLong(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: platformAndroid, fieldPushTokenKey: strings.Repeat("x", 1025)}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRegister_ServiceError(t *testing.T) {
	t.Parallel()

	svc := &mockDeviceSvc{
		registerFn: func(_ context.Context, _ store.Device) (store.Device, error) {
			return store.Device{}, errors.New("db unavailable")
		},
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: testPushToken}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(svc).Register(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	if code := decodeError(t, rec.Body); code != "internal" {
		t.Errorf("expected code=internal, got %q", code)
	}
}

// --- Unregister ---

func TestUnregister_Success(t *testing.T) {
	t.Parallel()

	svc := &mockDeviceSvc{
		unregisterFn: func(_ context.Context, _, _ string) error { return nil },
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/unregister",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: "tok-123"}))
	req = withUserID(req, "user-b")
	rec := httptest.NewRecorder()

	newHandler(svc).Unregister(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnregister_Unauthorized_MissingUserID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/unregister",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: testPushToken}))
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Unregister(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestUnregister_InvalidJSON(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/unregister",
		strings.NewReader("}}"))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Unregister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUnregister_InvalidPlatform(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/unregister",
		jsonBody(t, map[string]string{fieldPlatform: "blackberry", fieldPushTokenKey: testPushToken}))
	req = withUserID(req, "user-b")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Unregister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUnregister_EmptyToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/unregister",
		jsonBody(t, map[string]string{fieldPlatform: platformIOS, fieldPushTokenKey: ""}))
	req = withUserID(req, "user-b")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Unregister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUnregister_ServiceError(t *testing.T) {
	t.Parallel()

	svc := &mockDeviceSvc{
		unregisterFn: func(_ context.Context, _, _ string) error {
			return errors.New("db error")
		},
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/unregister",
		jsonBody(t, map[string]string{fieldPlatform: platformAndroid, fieldPushTokenKey: testPushToken}))
	req = withUserID(req, "user-b")
	rec := httptest.NewRecorder()

	newHandler(svc).Unregister(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// TestRegister_TokenPlatformsAccepted ensures ios and android are valid with push_token.
func TestRegister_TokenPlatformsAccepted(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{"ios", "android"} {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()

			svc := &mockDeviceSvc{
				registerFn: func(_ context.Context, d store.Device) (store.Device, error) {
					d.ID = "dev-id"
					d.Enabled = true
					d.LastSeenAt = time.Now()
					return d, nil
				},
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
				jsonBody(t, map[string]string{fieldPlatform: platform, fieldPushTokenKey: testPushToken}))
			req = withUserID(req, "user-a")
			rec := httptest.NewRecorder()

			newHandler(svc).Register(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("platform %q: expected 200, got %d: %s", platform, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRegister_Web_WithPushSubscription(t *testing.T) {
	t.Parallel()

	sub := map[string]any{
		"endpoint": "https://push.example.com/subscribe/abc123",
		"keys": map[string]string{
			"p256dh": "BNcRdreALRFXTkOOUHK1EtK2wtWelNhztlfMLU4ZN_nNq",
			"auth":   "tBHItJI5svbpez7KI4CCXg",
		},
	}
	svc := &mockDeviceSvc{
		registerFn: func(_ context.Context, d store.Device) (store.Device, error) {
			d.ID = "dev-web-1"
			d.Enabled = true
			d.LastSeenAt = time.Now()
			return d, nil
		},
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]any{fieldPlatform: "web", "push_subscription": sub}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(svc).Register(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegister_Web_MissingPushSubscription(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/devices/register",
		jsonBody(t, map[string]string{fieldPlatform: "web", fieldPushTokenKey: testPushToken}))
	req = withUserID(req, "user-a")
	rec := httptest.NewRecorder()

	newHandler(&mockDeviceSvc{}).Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if code := decodeError(t, rec.Body); code != "invalid_argument" {
		t.Errorf("expected code=invalid_argument, got %q", code)
	}
}
