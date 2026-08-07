package chat_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"my-chat/internal/handlers/chat"
	"my-chat/internal/middleware"
	chatservice "my-chat/internal/services/chat"
	"my-chat/internal/store"
)

const (
	testUserID     = "11111111-1111-1111-1111-111111111111"
	testPeerID     = "22222222-2222-2222-2222-222222222222"
	testDialogID   = "33333333-3333-3333-3333-333333333333"
	testPeerName   = "bob"
	testCreatePath = "/api/v1/dialogs"
)

type mockChatSvc struct {
	listDialogsFn            func(ctx context.Context, userID string) ([]chatservice.DialogItem, error)
	createDialogByUsernameFn func(ctx context.Context, userID, username string) (chatservice.DialogItem, error)
	sendMessageFn            func(ctx context.Context, message store.Message) (store.Message, error)
	listMessagesFn           func(
		ctx context.Context,
		userID, dialogID string,
		limit int,
		before *time.Time,
	) ([]store.Message, error)
	markReadFn    func(ctx context.Context, messageID, userID string, readAt time.Time) error
	unreadCountFn func(ctx context.Context, userID string) (int, error)
}

func (m *mockChatSvc) SendMessage(ctx context.Context, message store.Message) (store.Message, error) {
	return m.sendMessageFn(ctx, message)
}

func (m *mockChatSvc) ListMessages(
	ctx context.Context,
	userID, dialogID string,
	limit int,
	before *time.Time,
) ([]store.Message, error) {
	return m.listMessagesFn(ctx, userID, dialogID, limit, before)
}

func (m *mockChatSvc) MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error {
	return m.markReadFn(ctx, messageID, userID, readAt)
}

func (m *mockChatSvc) UnreadCount(ctx context.Context, userID string) (int, error) {
	return m.unreadCountFn(ctx, userID)
}

func (m *mockChatSvc) ListDialogs(ctx context.Context, userID string) ([]chatservice.DialogItem, error) {
	return m.listDialogsFn(ctx, userID)
}

func (m *mockChatSvc) CreateDialogByUsername(
	ctx context.Context,
	userID, username string,
) (chatservice.DialogItem, error) {
	return m.createDialogByUsernameFn(ctx, userID, username)
}

func decodeErrorCode(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	return resp.Error.Code
}

func withUser(req *http.Request) *http.Request {
	return req.WithContext(middleware.ContextWithUserID(req.Context(), testUserID))
}

func sampleDialogItem() chatservice.DialogItem {
	return chatservice.DialogItem{
		DialogID: testDialogID,
		Peer: chatservice.Peer{
			UserID:   testPeerID,
			Username: testPeerName,
		},
		LastMessage: nil,
		UnreadCount: 0,
		UpdatedAt:   time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
	}
}

func TestListDialogs_200(t *testing.T) {
	t.Parallel()

	svc := &mockChatSvc{
		listDialogsFn: func(_ context.Context, userID string) ([]chatservice.DialogItem, error) {
			if userID != testUserID {
				t.Errorf("userID: want %q, got %q", testUserID, userID)
			}
			return []chatservice.DialogItem{sampleDialogItem()}, nil
		},
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCreatePath, nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	chat.New(svc).ListDialogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Dialogs []struct {
			DialogID string `json:"dialog_id"`
			Peer     struct {
				Username string `json:"username"`
			} `json:"peer"`
			LastMessage any `json:"last_message"`
		} `json:"dialogs"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Dialogs) != 1 || resp.Dialogs[0].DialogID != testDialogID {
		t.Fatalf("unexpected dialogs: %+v", resp.Dialogs)
	}
	if resp.Dialogs[0].Peer.Username != testPeerName {
		t.Errorf("peer username: want %q, got %q", testPeerName, resp.Dialogs[0].Peer.Username)
	}
	if resp.Dialogs[0].LastMessage != nil {
		t.Errorf("last_message: want null, got %v", resp.Dialogs[0].LastMessage)
	}
}

func TestListDialogs_401(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/dialogs", nil)
	rec := httptest.NewRecorder()

	chat.New(&mockChatSvc{}).ListDialogs(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body); code != "unauthenticated" {
		t.Errorf("code: want unauthenticated, got %q", code)
	}
}

func TestCreateDialog_200(t *testing.T) {
	t.Parallel()

	svc := &mockChatSvc{
		createDialogByUsernameFn: func(_ context.Context, userID, username string) (chatservice.DialogItem, error) {
			if userID != testUserID || username != testPeerName {
				t.Errorf("args: userID=%q username=%q", userID, username)
			}
			return sampleDialogItem(), nil
		},
	}

	body := bytes.NewBufferString(`{"username":"bob"}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testCreatePath, body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	chat.New(svc).CreateDialog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DialogID string `json:"dialog_id"`
		Peer     struct {
			Username string `json:"username"`
		} `json:"peer"`
		LastMessage any `json:"last_message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DialogID != testDialogID || resp.Peer.Username != testPeerName {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.LastMessage != nil {
		t.Errorf("last_message: want null, got %v", resp.LastMessage)
	}
}

func TestCreateDialog_401(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		testCreatePath,
		bytes.NewBufferString(`{"username":"bob"}`),
	)
	rec := httptest.NewRecorder()

	chat.New(&mockChatSvc{}).CreateDialog(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
}

func TestCreateDialog_400_Self(t *testing.T) {
	t.Parallel()

	svc := &mockChatSvc{
		createDialogByUsernameFn: func(_ context.Context, _, _ string) (chatservice.DialogItem, error) {
			return chatservice.DialogItem{}, chatservice.ErrCannotDialogWithSelf
		},
	}

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		testCreatePath,
		bytes.NewBufferString(`{"username":"alice"}`),
	)
	req = withUser(req)
	rec := httptest.NewRecorder()

	chat.New(svc).CreateDialog(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body); code != "cannot_dialog_with_self" {
		t.Errorf("code: want cannot_dialog_with_self, got %q", code)
	}
}

func TestCreateDialog_404_MissingUser(t *testing.T) {
	t.Parallel()

	svc := &mockChatSvc{
		createDialogByUsernameFn: func(_ context.Context, _, _ string) (chatservice.DialogItem, error) {
			return chatservice.DialogItem{}, chatservice.ErrDialogUserNotFound
		},
	}

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		testCreatePath,
		bytes.NewBufferString(`{"username":"nobody"}`),
	)
	req = withUser(req)
	rec := httptest.NewRecorder()

	chat.New(svc).CreateDialog(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body); code != "user_not_found" {
		t.Errorf("code: want user_not_found, got %q", code)
	}
}

func TestCreateDialog_Idempotent(t *testing.T) {
	t.Parallel()

	calls := 0
	svc := &mockChatSvc{
		createDialogByUsernameFn: func(_ context.Context, _, _ string) (chatservice.DialogItem, error) {
			calls++
			return sampleDialogItem(), nil
		},
	}
	h := chat.New(svc)

	for range 2 {
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			testCreatePath,
			bytes.NewBufferString(`{"username":"bob"}`),
		)
		req = withUser(req)
		rec := httptest.NewRecorder()
		h.CreateDialog(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: want 200, got %d", rec.Code)
		}
		var resp struct {
			DialogID string `json:"dialog_id"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.DialogID != testDialogID {
			t.Errorf("dialog_id: want %q, got %q", testDialogID, resp.DialogID)
		}
	}

	if calls != 2 {
		t.Errorf("expected 2 service calls, got %d", calls)
	}
}

func TestCreateDialog_400_EmptyUsername(t *testing.T) {
	t.Parallel()

	svc := &mockChatSvc{
		createDialogByUsernameFn: func(_ context.Context, _, _ string) (chatservice.DialogItem, error) {
			return chatservice.DialogItem{}, chatservice.ErrInvalidDialogUsername
		},
	}

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		testCreatePath,
		bytes.NewBufferString(`{"username":"  "}`),
	)
	req = withUser(req)
	rec := httptest.NewRecorder()

	chat.New(svc).CreateDialog(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body); code != "invalid_argument" {
		t.Errorf("code: want invalid_argument, got %q", code)
	}
}
