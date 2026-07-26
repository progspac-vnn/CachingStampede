// Package config loads and validates application configuration from
// environment variables (optionally sourced from a .env file).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration required to start the service.
type Config struct {
	Env      string
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// PostgresConfig holds PostgreSQL connection configuration.
type PostgresConfig struct {
	Host           string
	Port           string
	User           string
	Password       string
	Database       string
	SSLMode        string
	MaxConns       int32
	MinConns       int32
	ConnectTimeout time.Duration
}

// DSN builds a PostgreSQL connection string from the configuration.
func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode,
	)
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Host        string
	Port        string
	Password    string
	DB          int
	DialTimeout time.Duration
}

// Addr returns the Redis host:port address.
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// Load reads configuration from a .env file (if present) and the process
// environment, validates required variables, and returns a populated Config.
// It returns an error if any required variable is missing or malformed.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("config: failed to load .env file: %w", err)
	}

	required := []string{
		"POSTGRES_HOST",
		"POSTGRES_PORT",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"REDIS_HOST",
		"REDIS_PORT",
	}
	if err := validateRequired(required); err != nil {
		return nil, err
	}

	serverReadTimeout, err := parseDuration("SERVER_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	serverWriteTimeout, err := parseDuration("SERVER_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}
	serverIdleTimeout, err := parseDuration("SERVER_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, err
	}
	serverShutdownTimeout, err := parseDuration("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}

	postgresMaxConns, err := parseInt32("POSTGRES_MAX_CONNS", 10)
	if err != nil {
		return nil, err
	}
	postgresMinConns, err := parseInt32("POSTGRES_MIN_CONNS", 2)
	if err != nil {
		return nil, err
	}
	postgresConnectTimeout, err := parseDuration("POSTGRES_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}

	redisDB, err := parseInt("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}
	redisDialTimeout, err := parseDuration("REDIS_DIAL_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Env: getEnvOrDefault("APP_ENV", "development"),
		Server: ServerConfig{
			Port:            getEnvOrDefault("SERVER_PORT", "8080"),
			ReadTimeout:     serverReadTimeout,
			WriteTimeout:    serverWriteTimeout,
			IdleTimeout:     serverIdleTimeout,
			ShutdownTimeout: serverShutdownTimeout,
		},
		Postgres: PostgresConfig{
			Host:           os.Getenv("POSTGRES_HOST"),
			Port:           os.Getenv("POSTGRES_PORT"),
			User:           os.Getenv("POSTGRES_USER"),
			Password:       os.Getenv("POSTGRES_PASSWORD"),
			Database:       os.Getenv("POSTGRES_DB"),
			SSLMode:        getEnvOrDefault("POSTGRES_SSLMODE", "disable"),
			MaxConns:       postgresMaxConns,
			MinConns:       postgresMinConns,
			ConnectTimeout: postgresConnectTimeout,
		},
		Redis: RedisConfig{
			Host:        os.Getenv("REDIS_HOST"),
			Port:        os.Getenv("REDIS_PORT"),
			Password:    os.Getenv("REDIS_PASSWORD"),
			DB:          redisDB,
			DialTimeout: redisDialTimeout,
		},
	}

	return cfg, nil
}

func validateRequired(keys []string) error {
	var missing []string
	for _, key := range keys {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required environment variables: %v", missing)
	}
	return nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid duration for %s: %w", key, err)
	}
	return d, nil
}

func parseInt32(key string, fallback int32) (int32, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("config: invalid integer for %s: %w", key, err)
	}
	return int32(n), nil
}

func parseInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid integer for %s: %w", key, err)
	}
	return n, nil
}
