package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) AddProduct(ctx context.Context, p inventory.Product) error {
	if p.Stock < 0 {
		return inventory.ErrInvalidQuantity
	}
	if strings.TrimSpace(p.SKU) == "" {
		return inventory.ErrInvalidSKU
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO products (sku, name, stock) VALUES ($1, $2, $3)`,
		p.SKU, p.Name, p.Stock,
	)
	if isUniqueViolation(err) {
		return inventory.ErrProductAlreadyExists
	}
	return err
}

func (s *Store) GetProduct(ctx context.Context, sku string) (inventory.Product, error) {
	var p inventory.Product
	err := s.pool.QueryRow(ctx,
		`SELECT sku, name, stock FROM products WHERE sku = $1`,
		sku,
	).Scan(&p.SKU, &p.Name, &p.Stock)
	if err != nil {
		if isNoRows(err) {
			return inventory.Product{}, inventory.ErrProductNotFound
		}
		return inventory.Product{}, err
	}
	return p, nil
}

func (s *Store) Reserve(ctx context.Context, sku string, qty int, ttl time.Duration) (inventory.Reservation, error) {
	if qty <= 0 {
		return inventory.Reservation{}, inventory.ErrInvalidQuantity
	}

	var r inventory.Reservation
	err := s.pool.QueryRow(ctx, `
		WITH reserved AS (
			UPDATE products SET stock = stock - $2
			WHERE sku = $1 AND stock >= $2
			RETURNING sku
		)
		INSERT INTO reservations (sku, quantity, expires_at)
		SELECT sku, $2, now() + make_interval(secs => $3) FROM reserved
		RETURNING `+reservationColumns,
		sku, qty, ttl.Seconds(),
	).Scan(reservationFields(&r)...)
	if err == nil {
		return r, nil
	}
	if !isNoRows(err) {
		return inventory.Reservation{}, err
	}

	if _, err := s.GetProduct(ctx, sku); err != nil {
		return inventory.Reservation{}, err
	}
	return inventory.Reservation{}, inventory.ErrInsufficientStock
}

func (s *Store) ConfirmReservation(ctx context.Context, id string) (inventory.Reservation, error) {
	var r inventory.Reservation
	err := s.pool.QueryRow(ctx, `
		UPDATE reservations SET status = 'confirmed'
		WHERE id = $1 AND status = 'pending' AND expires_at > now()
		RETURNING `+reservationColumns,
		id,
	).Scan(reservationFields(&r)...)
	if err == nil {
		return r, nil
	}
	if !isNoRows(err) {
		return inventory.Reservation{}, reservationLookupError(err)
	}
	return inventory.Reservation{}, s.whyNotPending(ctx, id)
}

func (s *Store) CancelReservation(ctx context.Context, id string) (inventory.Reservation, error) {
	var r inventory.Reservation
	err := s.pool.QueryRow(ctx, `
		WITH cancelled AS (
			UPDATE reservations SET status = 'cancelled'
			WHERE id = $1 AND status = 'pending'
			RETURNING *
		), restocked AS (
			UPDATE products p SET stock = p.stock + c.quantity
			FROM cancelled c
			WHERE p.sku = c.sku
		)
		SELECT `+reservationColumns+` FROM cancelled`,
		id,
	).Scan(reservationFields(&r)...)
	if err == nil {
		return r, nil
	}
	if !isNoRows(err) {
		return inventory.Reservation{}, reservationLookupError(err)
	}
	return inventory.Reservation{}, s.whyNotPending(ctx, id)
}

func (s *Store) whyNotPending(ctx context.Context, id string) error {
	var r inventory.Reservation
	var expired bool
	err := s.pool.QueryRow(ctx,
		`SELECT `+reservationColumns+`, expires_at <= now() FROM reservations WHERE id = $1`,
		id,
	).Scan(append(reservationFields(&r), &expired)...)
	if err != nil {
		if isNoRows(err) {
			return inventory.ErrReservationNotFound
		}
		return reservationLookupError(err)
	}
	if r.Status == inventory.StatusPending && expired {
		return inventory.ErrReservationExpired
	}
	return inventory.ErrReservationNotPending
}

const reservationColumns = `id::text, sku, quantity, status, expires_at, created_at`

func reservationFields(r *inventory.Reservation) []any {
	return []any{&r.ID, &r.SKU, &r.Quantity, &r.Status, &r.ExpiresAt, &r.CreatedAt}
}

func reservationLookupError(err error) error {
	if isInvalidText(err) {
		return inventory.ErrReservationNotFound
	}
	return err
}
