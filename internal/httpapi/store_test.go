package httpapi

import (
	"context"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type fakeStore struct {
	product     inventory.Product
	reservation inventory.Reservation
	err         error

	added       inventory.Product
	reservedSKU string
	reservedQty int
	reservedTTL time.Duration
}

func (f *fakeStore) AddProduct(_ context.Context, p inventory.Product) error {
	f.added = p
	return f.err
}

func (f *fakeStore) GetProduct(context.Context, string) (inventory.Product, error) {
	return f.product, f.err
}

func (f *fakeStore) Reserve(_ context.Context, sku string, qty int, ttl time.Duration) (inventory.Reservation, error) {
	f.reservedSKU, f.reservedQty, f.reservedTTL = sku, qty, ttl
	return f.reservation, f.err
}
