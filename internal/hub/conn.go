package hub

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
)

const (
	// DefaultHeartbeatInterval — интервал между ping-кадрами.
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultPingTimeout — максимальное ожидание pong от клиента.
	DefaultPingTimeout = 5 * time.Second
)

// wsConn — минимальный интерфейс над *websocket.Conn, достаточный для read-loop и heartbeat.
// Все методы реализованы *websocket.Conn из пакета coder/websocket.
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Ping(ctx context.Context) error
}

// ConnConfig задаёт параметры heartbeat соединения.
type ConnConfig struct {
	HeartbeatInterval time.Duration
	PingTimeout       time.Duration
}

// DefaultConnConfig возвращает production-конфигурацию heartbeat (30s/5s).
func DefaultConnConfig() ConnConfig {
	return ConnConfig{
		HeartbeatInterval: DefaultHeartbeatInterval,
		PingTimeout:       DefaultPingTimeout,
	}
}

// RunConn запускает read-loop с heartbeat для одного WS-соединения.
// Блокируется до закрытия соединения или отмены ctx.
//
// Heartbeat: каждые cfg.HeartbeatInterval отправляется ping-кадр.
// Если pong не вернулся за cfg.PingTimeout — соединение признаётся мёртвым,
// контекст отменяется и read-loop завершается.
func RunConn(ctx context.Context, conn wsConn, cfg ConnConfig) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	go runHeartbeat(ctx, conn, cfg, cancel)

	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			return err
		}
	}
}

func runHeartbeat(
	ctx context.Context,
	conn wsConn,
	cfg ConnConfig,
	cancel context.CancelCauseFunc,
) {
	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, cfg.PingTimeout)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				cancel(fmt.Errorf("heartbeat ping: %w", err))
				return
			}
		}
	}
}
