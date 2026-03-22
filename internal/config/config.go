// Package config содержит конфигурационные компоненты.
package config

// Config хранит конфигурацию приложения.
type Config struct {
	Global  GlobalConfig  `yaml:"global"`
	Log     LogConfig     `yaml:"log"`
	Servers ServersConfig `yaml:"servers"`
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
