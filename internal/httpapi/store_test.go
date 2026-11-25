package httpapi

import (
	"context"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type fakeStore struct {
	product     inventory.Product
	reservation inventory.Reservation
	replayed    bool
	err         error

	added         inventory.Product
	reserved      inventory.ReservationRequest
	reserveCalled bool
	confirmedID   string
	cancelledID   string
}

func (f *fakeStore) AddProduct(_ context.Context, p inventory.Product) error {
	f.added = p
	return f.err
}

func (f *fakeStore) GetProduct(context.Context, string) (inventory.Product, error) {
	return f.product, f.err
}

func (f *fakeStore) ReserveOnce(_ context.Context, req inventory.ReservationRequest) (inventory.Reservation, bool, error) {
	f.reserved, f.reserveCalled = req, true
	return f.reservation, f.replayed, f.err
}

func (f *fakeStore) ConfirmReservation(_ context.Context, id string) (inventory.Reservation, error) {
	f.confirmedID = id
	return f.reservation, f.err
}

func (f *fakeStore) CancelReservation(_ context.Context, id string) (inventory.Reservation, error) {
	f.cancelledID = id
	return f.reservation, f.err
}
