package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/expiry"
	"github.com/GusMartins499/stock-reservation/internal/httpapi"
	"github.com/GusMartins499/stock-reservation/internal/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const shutdownTimeout = 10 * time.Second

type config struct {
	databaseURL     string
	addr            string
	reservationTTL  time.Duration
	expiryInterval  time.Duration
	expiryBatchSize int
	keyTTL          time.Duration
	keyInterval     time.Duration
}

func main() {
	if err := run(); err != nil {
		slog.Error("stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	store := postgres.NewStore(pool)
	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           httpapi.NewAPI(store, cfg.reservationTTL).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	reservations := expiry.Worker{
		Name:      "reservations",
		Expire:    store.ExpireDue,
		Interval:  cfg.expiryInterval,
		BatchSize: cfg.expiryBatchSize,
		Logger:    slog.Default(),
	}
	keys := expiry.Worker{
		Name: "idempotency_keys",
		Expire: func(ctx context.Context, limit int) (int, error) {
			return store.DeleteIdempotencyKeys(ctx, cfg.keyTTL, limit)
		},
		Interval:  cfg.keyInterval,
		BatchSize: cfg.expiryBatchSize,
		Logger:    slog.Default(),
	}

	var wg sync.WaitGroup
	wg.Go(func() { reservations.Run(ctx) })
	wg.Go(func() { keys.Run(ctx) })

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	slog.Info("listening", "addr", cfg.addr)

	select {
	case err := <-serveErr:
		stop()
		wg.Wait()
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	wg.Wait()
	return err
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL: os.Getenv("DATABASE_URL"),
		addr:        envOr("ADDR", ":8080"),
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}

	var err error
	if cfg.reservationTTL, err = positiveDuration("RESERVATION_TTL", 10*time.Minute); err != nil {
		return config{}, err
	}
	if cfg.expiryInterval, err = positiveDuration("EXPIRY_INTERVAL", 5*time.Second); err != nil {
		return config{}, err
	}
	if cfg.expiryBatchSize, err = positiveInt("EXPIRY_BATCH_SIZE", 100); err != nil {
		return config{}, err
	}
	if cfg.keyTTL, err = positiveDuration("IDEMPOTENCY_KEY_TTL", 24*time.Hour); err != nil {
		return config{}, err
	}
	if cfg.keyInterval, err = positiveDuration("IDEMPOTENCY_KEY_CLEANUP_INTERVAL", time.Minute); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func positiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", name, v)
	}
	return d, nil
}

func positiveInt(name string, fallback int) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", name, v)
	}
	return n, nil
}
