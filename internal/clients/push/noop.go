package push

import "context"

// NoopProvider — провайдер-заглушка для unit-тестов.
// SendFunc позволяет тестам симулировать ошибки/успех.
type NoopProvider struct {
	SendFunc func(ctx context.Context, msg Message) error
}

// NewNoopProvider создаёт NoopProvider, который по умолчанию возвращает nil.
func NewNoopProvider() *NoopProvider {
	return &NoopProvider{}
}

// Name возвращает идентификатор провайдера.
func (p *NoopProvider) Name() string { return "noop" }

// Send вызывает SendFunc, если задана, иначе возвращает nil.
func (p *NoopProvider) Send(ctx context.Context, msg Message) error {
	if p.SendFunc != nil {
		return p.SendFunc(ctx, msg)
	}

	return nil
}
