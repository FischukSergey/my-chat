// Package config содержит конфигурационные компоненты.
package config

import (
	"errors"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/ilyakaznacheev/cleanenv"
)

// ParseAndValidate читает YAML-конфиг и валидирует его.
func ParseAndValidate(filename string) (Config, error) {
	if filename == "" {
		return Config{}, errors.New("filename is required")
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return Config{}, errors.New("file does not exist")
	}

	cfg := Config{}
	if err := cleanenv.ReadConfig(filename, &cfg); err != nil {
		return Config{}, err
	}

	if err := validator.New().Struct(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
