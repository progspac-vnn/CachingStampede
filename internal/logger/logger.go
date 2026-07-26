// Package logger provides structured logging for the application, built on
// top of Uber's zap. It exposes a constructor rather than a global instance
// so the logger can be injected into dependent packages and mocked in tests.
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// EnvProduction selects a JSON-encoded, production-tuned logger.
	EnvProduction = "production"
	// EnvDevelopment selects a human-readable, development-tuned logger.
	EnvDevelopment = "development"
)

// New builds a *zap.Logger appropriate for the given environment.
//
// EnvProduction yields JSON output at info level with sampling enabled.
// Any other value yields a human-readable console logger at debug level.
func New(env string) (*zap.Logger, error) {
	var cfg zap.Config

	switch env {
	case EnvProduction:
		cfg = zap.NewProductionConfig()
	default:
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("logger: failed to build logger: %w", err)
	}

	return log, nil
}
