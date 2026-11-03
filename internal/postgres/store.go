package postgres

import (
	"context"
	"strings"

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
