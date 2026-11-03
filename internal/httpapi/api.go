package httpapi

import (
	"context"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type Store interface {
	AddProduct(ctx context.Context, p inventory.Product) error
	GetProduct(ctx context.Context, sku string) (inventory.Product, error)
	Reserve(ctx context.Context, sku string, qty int) error
}

type API struct {
	store Store
}

func NewAPI(store Store) *API {
	return &API{store: store}
}
