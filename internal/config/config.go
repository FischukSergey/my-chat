// Package config содержит конфигурационные компоненты.
package config

// Config хранит конфигурацию приложения.
type Config struct {
	Global             GlobalConfig             `yaml:"global"`
	Log                LogConfig                `yaml:"log"`
	Servers            ServersConfig            `yaml:"servers"`
	Database           DatabaseConfig           `yaml:"database"`
	JWT                JWTConfig                `yaml:"jwt"`
	NotificationWorker NotificationWorkerConfig `yaml:"notification_worker"`
	Chat               ChatConfig               `yaml:"chat"`
	Expirer            ExpirerConfig            `yaml:"expirer"`
	CORS               CORSConfig               `yaml:"cors"`
}

// GlobalConfig хранит глобальные параметры окружения.
type GlobalConfig struct {
	Env string `yaml:"env" validate:"required,oneof=local dev stage prod"`
}

// LogConfig хранит настройки логирования.
type LogConfig struct {
	Level       string `yaml:"level" validate:"required,oneof=debug info warn error"`
	Format      string `yaml:"format" validate:"required,oneof=json text"`
	ServiceName string `yaml:"service_name" validate:"required"`
}

// ServersConfig хранит настройки сетевых серверов.
type ServersConfig struct {
	Client ClientServerConfig `yaml:"client"`
}

// ClientServerConfig хранит настройки HTTP API сервера.
type ClientServerConfig struct {
	Addr string `yaml:"addr" validate:"omitempty,hostname_port"`
}

// IsConfigured проверяет, задан ли адрес клиентского сервера.
func (c ClientServerConfig) IsConfigured() bool {
	return c.Addr != ""
}

// DatabaseConfig хранит параметры подключения к PostgreSQL.
type DatabaseConfig struct {
	DSN         string `yaml:"dsn"`
	AutoMigrate bool   `yaml:"auto_migrate"`
}

// IsConfigured проверяет, задана ли строка подключения к БД.
func (d DatabaseConfig) IsConfigured() bool {
	return d.DSN != ""
}

// JWTConfig хранит параметры для подписи и валидации JWT.
// Поля необязательны глобально — обязательность проверяется вручную через IsConfigured() в каждом App.New.
type JWTConfig struct {
	Secret          string `yaml:"secret"`
	AccessTokenTTL  int    `yaml:"access_token_ttl_seconds" validate:"omitempty,min=60"`
	RefreshTokenTTL int    `yaml:"refresh_token_ttl_seconds" validate:"omitempty,min=60"`
}

// IsConfigured проверяет, задан ли секрет.
func (j JWTConfig) IsConfigured() bool {
	return j.Secret != ""
}

// NotificationWorkerConfig хранит параметры notification-worker.
// Нулевые значения означают использование дефолтов в App.New.
type NotificationWorkerConfig struct {
	PollIntervalSeconds int    `yaml:"poll_interval_seconds" validate:"omitempty,min=1"`
	BatchSize           int    `yaml:"batch_size" validate:"omitempty,min=1"`
	MaxAttempts         int    `yaml:"max_attempts" validate:"omitempty,min=1"`
	BackoffBaseSeconds  int    `yaml:"backoff_base_seconds" validate:"omitempty,min=1"`
	Provider            string `yaml:"provider" validate:"omitempty,oneof=dev-log noop"`
}

// ChatConfig хранит параметры сервиса чата.
type ChatConfig struct {
	// MessageTTLSeconds — время жизни сообщения в секундах; 0 = TTL не используется.
	MessageTTLSeconds int `yaml:"message_ttl_seconds" validate:"omitempty,min=0"`
}

// CORSConfig хранит параметры CORS-политики.
type CORSConfig struct {
	// AllowedOrigins — список разрешённых источников.
	// Если список пуст или содержит единственный элемент "*", разрешаются все origins (только для local/dev).
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// ExpirerConfig хранит параметры сервиса message-expirer.
// Нулевые значения означают использование дефолтов в App.New.
type ExpirerConfig struct {
	// IntervalSeconds — интервал между запусками тикера в секундах; 0 = дефолт (10 сек).
	IntervalSeconds int `yaml:"interval_seconds" validate:"omitempty,min=1"`
	// BatchSize — максимальное число сообщений, обрабатываемых за одну итерацию; 0 = дефолт (100).
	BatchSize int `yaml:"batch_size" validate:"omitempty,min=1"`
}
