package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/httpapi"
	"github.com/GusMartins499/stock-reservation/internal/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	reservationTTL := 10 * time.Minute
	if v := os.Getenv("RESERVATION_TTL"); v != "" {
		ttl, err := time.ParseDuration(v)
		if err != nil || ttl <= 0 {
			return fmt.Errorf("RESERVATION_TTL must be a positive duration, got %q", v)
		}
		reservationTTL = ttl
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	api := httpapi.NewAPI(postgres.NewStore(pool), reservationTTL)
	log.Printf("listening on %s", addr)
	return http.ListenAndServe(addr, api.Routes())
}
