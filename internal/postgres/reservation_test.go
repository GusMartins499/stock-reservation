package postgres

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

const unknownReservationID = "00000000-0000-0000-0000-000000000000"

func newReservedStore(t *testing.T, stock, qty int, ttl time.Duration) (*Store, inventory.Reservation) {
	t.Helper()

	store := newTestStore(t)
	if err := store.AddProduct(t.Context(), inventory.Product{SKU: "PS", Name: "Playstation 5", Stock: stock}); err != nil {
		t.Fatalf("setup: AddProduct: %v", err)
	}
	r, err := store.Reserve(t.Context(), "PS", qty, ttl)
	if err != nil {
		t.Fatalf("setup: Reserve: %v", err)
	}
	return store, r
}

func assertStock(t *testing.T, store *Store, want int) {
	t.Helper()

	p, err := store.GetProduct(t.Context(), "PS")
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.Stock != want {
		t.Errorf("stock = %d, want %d", p.Stock, want)
	}
}

func TestConfirmReservation_ConfirmsPendingReservationWithoutTouchingStock(t *testing.T) {
	store, r := newReservedStore(t, 5, 2, testTTL)

	got, err := store.ConfirmReservation(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("ConfirmReservation() error = %v", err)
	}
	if got.ID != r.ID || got.Status != inventory.StatusConfirmed {
		t.Errorf("ConfirmReservation() = %+v, want ID %q with status %q", got, r.ID, inventory.StatusConfirmed)
	}
	assertStock(t, store, 3)
}

func TestConfirmReservation_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		ttl     time.Duration
		before  func(t *testing.T, store *Store, id string)
		id      func(id string) string
		wantErr error
	}{
		{
			name:    "unknown id",
			ttl:     testTTL,
			id:      func(string) string { return unknownReservationID },
			wantErr: inventory.ErrReservationNotFound,
		},
		{
			name:    "id that is not a UUID",
			ttl:     testTTL,
			id:      func(string) string { return "not-a-uuid" },
			wantErr: inventory.ErrReservationNotFound,
		},
		{
			name:    "expired reservation",
			ttl:     -time.Minute,
			wantErr: inventory.ErrReservationExpired,
		},
		{
			name: "already confirmed reservation",
			ttl:  testTTL,
			before: func(t *testing.T, store *Store, id string) {
				if _, err := store.ConfirmReservation(t.Context(), id); err != nil {
					t.Fatalf("setup: ConfirmReservation: %v", err)
				}
			},
			wantErr: inventory.ErrReservationNotPending,
		},
		{
			name: "cancelled reservation",
			ttl:  testTTL,
			before: func(t *testing.T, store *Store, id string) {
				if _, err := store.CancelReservation(t.Context(), id); err != nil {
					t.Fatalf("setup: CancelReservation: %v", err)
				}
			},
			wantErr: inventory.ErrReservationNotPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, r := newReservedStore(t, 5, 2, tt.ttl)
			if tt.before != nil {
				tt.before(t, store, r.ID)
			}
			id := r.ID
			if tt.id != nil {
				id = tt.id(r.ID)
			}

			_, err := store.ConfirmReservation(t.Context(), id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ConfirmReservation() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCancelReservation_ReturnsStockOfPendingReservation(t *testing.T) {
	store, r := newReservedStore(t, 5, 2, testTTL)

	got, err := store.CancelReservation(t.Context(), r.ID)
	if err != nil {
		t.Fatalf("CancelReservation() error = %v", err)
	}
	if got.ID != r.ID || got.Status != inventory.StatusCancelled {
		t.Errorf("CancelReservation() = %+v, want ID %q with status %q", got, r.ID, inventory.StatusCancelled)
	}
	assertStock(t, store, 5)
}

func TestCancelReservation_Rejects(t *testing.T) {
	tests := []struct {
		name      string
		before    func(t *testing.T, store *Store, id string)
		id        func(id string) string
		wantErr   error
		wantStock int
	}{
		{
			name:      "unknown id",
			id:        func(string) string { return unknownReservationID },
			wantErr:   inventory.ErrReservationNotFound,
			wantStock: 3,
		},
		{
			name:      "id that is not a UUID",
			id:        func(string) string { return "not-a-uuid" },
			wantErr:   inventory.ErrReservationNotFound,
			wantStock: 3,
		},
		{
			name: "confirmed reservation",
			before: func(t *testing.T, store *Store, id string) {
				if _, err := store.ConfirmReservation(t.Context(), id); err != nil {
					t.Fatalf("setup: ConfirmReservation: %v", err)
				}
			},
			wantErr:   inventory.ErrReservationNotPending,
			wantStock: 3,
		},
		{
			name: "reservation already cancelled",
			before: func(t *testing.T, store *Store, id string) {
				if _, err := store.CancelReservation(t.Context(), id); err != nil {
					t.Fatalf("setup: CancelReservation: %v", err)
				}
			},
			wantErr:   inventory.ErrReservationNotPending,
			wantStock: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, r := newReservedStore(t, 5, 2, testTTL)
			if tt.before != nil {
				tt.before(t, store, r.ID)
			}
			id := r.ID
			if tt.id != nil {
				id = tt.id(r.ID)
			}

			_, err := store.CancelReservation(t.Context(), id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CancelReservation() error = %v, want %v", err, tt.wantErr)
			}
			assertStock(t, store, tt.wantStock)
		})
	}
}

func TestCancelReservation_ConcurrentCancelsReturnStockOnce(t *testing.T) {
	first, r := newReservedStore(t, 5, 2, testTTL)
	stores := []*Store{first, NewStore(newTestPool(t))}

	var wg sync.WaitGroup
	var cancelled atomic.Int64
	for i := range 50 {
		store := stores[i%len(stores)]
		wg.Go(func() {
			if _, err := store.CancelReservation(t.Context(), r.ID); err == nil {
				cancelled.Add(1)
			}
		})
	}
	wg.Wait()

	if got := cancelled.Load(); got != 1 {
		t.Errorf("successful cancels = %d, want 1", got)
	}
	assertStock(t, first, 5)
}

func TestConfirmAndCancel_RacingOnTheSameReservationHaveOneWinner(t *testing.T) {
	for range 20 {
		first, r := newReservedStore(t, 5, 2, testTTL)
		second := NewStore(newTestPool(t))

		var wg sync.WaitGroup
		var confirmErr, cancelErr error
		wg.Go(func() { _, confirmErr = first.ConfirmReservation(t.Context(), r.ID) })
		wg.Go(func() { _, cancelErr = second.CancelReservation(t.Context(), r.ID) })
		wg.Wait()

		switch {
		case confirmErr == nil && errors.Is(cancelErr, inventory.ErrReservationNotPending):
			assertStock(t, first, 3)
		case cancelErr == nil && errors.Is(confirmErr, inventory.ErrReservationNotPending):
			assertStock(t, first, 5)
		default:
			t.Fatalf("confirm error = %v, cancel error = %v, want exactly one to succeed", confirmErr, cancelErr)
		}
	}
}
