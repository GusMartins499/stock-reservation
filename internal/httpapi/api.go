package httpapi

import (
	"context"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type Store interface {
	AddProduct(ctx context.Context, p inventory.Product) error
	GetProduct(ctx context.Context, sku string) (inventory.Product, error)
	Reserve(ctx context.Context, sku string, qty int, ttl time.Duration) (inventory.Reservation, error)
}

type API struct {
	store          Store
	reservationTTL time.Duration
}

func NewAPI(store Store, reservationTTL time.Duration) *API {
	return &API{store: store, reservationTTL: reservationTTL}
}
