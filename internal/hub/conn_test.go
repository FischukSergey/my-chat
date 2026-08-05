package hub_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"my-chat/internal/hub"
)

// testConn — mock реализация wsConn интерфейса для unit-тестов.
type testConn struct {
	readFn func(ctx context.Context) (websocket.MessageType, []byte, error)
	pingFn func(ctx context.Context) error
}

func (c *testConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	return c.readFn(ctx)
}

func (c *testConn) Ping(ctx context.Context) error {
	return c.pingFn(ctx)
}

// fastCfg возвращает конфигурацию с малыми интервалами для ускорения тестов.
func fastCfg() hub.ConnConfig {
	return hub.ConnConfig{
		HeartbeatInterval: 5 * time.Millisecond,
		PingTimeout:       5 * time.Millisecond,
	}
}

// TestRunConn_DeadConn_ClosesAfterPingFailure проверяет, что соединение,
// не отвечающее на ping, закрывается и RunConn возвращает ошибку.
func TestRunConn_DeadConn_ClosesAfterPingFailure(t *testing.T) {
	t.Parallel()

	pingErr := errors.New("connection reset by peer")

	conn := &testConn{
		pingFn: func(_ context.Context) error { return pingErr },
		readFn: func(ctx context.Context) (websocket.MessageType, []byte, error) {
			// Блокируемся до отмены контекста (симулируем ожидающее чтение).
			<-ctx.Done()
			return 0, nil, ctx.Err()
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- hub.RunConn(context.Background(), conn, fastCfg())
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from dead connection, got nil")
		}
		// Ошибка должна содержать "heartbeat ping"
		if !errors.Is(err, pingErr) {
			t.Errorf("expected underlying pingErr, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunConn did not close dead connection within 500ms")
	}
}

// TestRunConn_PongReceived_ConnectionStaysAlive проверяет, что соединение,
// отвечающее на ping, остаётся живым до явного закрытия.
func TestRunConn_PongReceived_ConnectionStaysAlive(t *testing.T) {
	t.Parallel()

	cfg := hub.ConnConfig{
		HeartbeatInterval: 10 * time.Millisecond,
		PingTimeout:       10 * time.Millisecond,
	}

	var pingCount atomic.Int64
	closeRead := make(chan struct{})

	conn := &testConn{
		pingFn: func(_ context.Context) error {
			pingCount.Add(1)
			return nil // pong получен — соединение живо
		},
		readFn: func(ctx context.Context) (websocket.MessageType, []byte, error) {
			select {
			case <-closeRead:
				return 0, nil, errors.New("connection closed by client")
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			}
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- hub.RunConn(context.Background(), conn, cfg)
	}()

	// Даём время пройти нескольким ping-циклам.
	time.Sleep(60 * time.Millisecond)

	if pingCount.Load() < 2 {
		t.Errorf("expected at least 2 successful pings, got %d", pingCount.Load())
	}

	// Закрываем соединение с «клиентской» стороны.
	close(closeRead)

	select {
	case <-done:
		// RunConn завершился после ошибки Read — ожидаемо.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunConn did not return after connection close")
	}
}
