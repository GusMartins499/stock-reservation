package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ReserveOnce(ctx context.Context, req inventory.ReservationRequest) (inventory.Reservation, bool, error) {
	if strings.TrimSpace(req.Key) == "" {
		return inventory.Reservation{}, false, inventory.ErrInvalidIdempotencyKey
	}
	if req.Quantity <= 0 {
		return inventory.Reservation{}, false, inventory.ErrInvalidQuantity
	}

	var r inventory.Reservation
	var replayed bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		claimed, err := claimKey(ctx, tx, req.Key)
		if err != nil {
			return err
		}
		if !claimed {
			replayed = true
			r, err = reservationForKey(ctx, tx, req)
			return err
		}

		r, err = reserve(ctx, tx, req.SKU, req.Quantity, req.TTL)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE idempotency_keys SET reservation_id = $2 WHERE key = $1`,
			req.Key, r.ID,
		)
		return err
	})
	if err != nil {
		return inventory.Reservation{}, false, err
	}
	return r, replayed, nil
}

func claimKey(ctx context.Context, tx pgx.Tx, key string) (bool, error) {
	tag, err := tx.Exec(ctx,
		`INSERT INTO idempotency_keys (key) VALUES ($1) ON CONFLICT (key) DO NOTHING`,
		key,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func reservationForKey(ctx context.Context, tx pgx.Tx, req inventory.ReservationRequest) (inventory.Reservation, error) {
	var r inventory.Reservation
	err := tx.QueryRow(ctx, `
		SELECT `+qualifiedReservationColumns+`
		FROM idempotency_keys k
		JOIN reservations r ON r.id = k.reservation_id
		WHERE k.key = $1`,
		req.Key,
	).Scan(reservationFields(&r)...)
	if err != nil {
		return inventory.Reservation{}, fmt.Errorf("reservation for idempotency key: %w", err)
	}
	if r.SKU != req.SKU || r.Quantity != req.Quantity {
		return inventory.Reservation{}, inventory.ErrIdempotencyKeyReused
	}
	return r, nil
}

func (s *Store) DeleteIdempotencyKeys(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		WITH old AS (
			SELECT key FROM idempotency_keys
			WHERE created_at <= now() - make_interval(secs => $1)
			ORDER BY created_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM idempotency_keys k
		USING old
		WHERE k.key = old.key`,
		olderThan.Seconds(), limit,
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
