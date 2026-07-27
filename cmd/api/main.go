// Command api starts the CachingStampede HTTP server: it loads
// configuration, initialises logging, connects to PostgreSQL and Redis,
// registers infrastructure routes, and serves until it receives a shutdown
// signal.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/progspac-vnn/CachingStampede/internal/cache"
	"github.com/progspac-vnn/CachingStampede/internal/config"
	"github.com/progspac-vnn/CachingStampede/internal/db"
	"github.com/progspac-vnn/CachingStampede/internal/handler"
	"github.com/progspac-vnn/CachingStampede/internal/logger"
	"github.com/progspac-vnn/CachingStampede/internal/metrics"
	"github.com/progspac-vnn/CachingStampede/internal/repository"
	"github.com/progspac-vnn/CachingStampede/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}

	// 2. Initialise Logger
	log, err := logger.New(cfg.Env)
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 3. Connect PostgreSQL
	postgres, err := db.Connect(ctx, cfg.Postgres, log)
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	defer postgres.Close()

	// 4. Connect Redis
	redisClient, err := cache.Connect(ctx, cfg.Redis, log)
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Error("failed to close redis client", zap.Error(err))
		}
	}()

	appMetrics := metrics.New()
	appMetrics.RegisterPostgresPool(postgres.Pool)

	productRepo := repository.NewProductRepository(postgres.Pool)
	productService := service.NewProductService(productRepo, redisClient.Client, redisClient, cfg.Cache, appMetrics, log)
	productHandler := handler.NewProductHandler(productService, log)

	// 5 & 6. Register Middleware, Register Routes
	router := newRouter(dependencies{
		postgres:       postgres,
		redis:          redisClient,
		metrics:        appMetrics,
		productHandler: productHandler,
		timeout:        cfg.Server.WriteTimeout,
	}, log)

	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// 7. Start HTTP Server
	serverErrCh := make(chan error, 1)
	go func() {
		log.Info("starting http server", zap.String("addr", server.Addr), zap.String("env", cfg.Env))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case err := <-serverErrCh:
		if err != nil {
			return fmt.Errorf("main: server failed: %w", err)
		}
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	// 8. Graceful Shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("main: graceful shutdown failed: %w", err)
	}

	log.Info("server shut down cleanly")
	return nil
}
