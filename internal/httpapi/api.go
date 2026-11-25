package httpapi

import (
	"context"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type Store interface {
	AddProduct(ctx context.Context, p inventory.Product) error
	GetProduct(ctx context.Context, sku string) (inventory.Product, error)
	ReserveOnce(ctx context.Context, req inventory.ReservationRequest) (inventory.Reservation, bool, error)
	ConfirmReservation(ctx context.Context, id string) (inventory.Reservation, error)
	CancelReservation(ctx context.Context, id string) (inventory.Reservation, error)
}

type API struct {
	store          Store
	reservationTTL time.Duration
}

func NewAPI(store Store, reservationTTL time.Duration) *API {
	return &API{store: store, reservationTTL: reservationTTL}
}
