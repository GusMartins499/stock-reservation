package postgres

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

func reserveOrFail(t *testing.T, store *Store, sku string, qty int, ttl time.Duration) inventory.Reservation {
	t.Helper()

	r, err := store.Reserve(t.Context(), sku, qty, ttl)
	if err != nil {
		t.Fatalf("setup: Reserve(%q, %d): %v", sku, qty, err)
	}
	return r
}

func statusOf(t *testing.T, store *Store, id string) inventory.ReservationStatus {
	t.Helper()

	var status inventory.ReservationStatus
	if err := store.pool.QueryRow(t.Context(), `SELECT status FROM reservations WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("status of %s: %v", id, err)
	}
	return status
}

func TestExpireDue_ExpiresOnlyDuePendingReservations(t *testing.T) {
	store := newTestStore(t)
	if err := store.AddProduct(t.Context(), inventory.Product{SKU: "PS", Name: "Playstation 5", Stock: 10}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	due := reserveOrFail(t, store, "PS", 2, -time.Minute)
	alsoDue := reserveOrFail(t, store, "PS", 1, -time.Second)
	notDue := reserveOrFail(t, store, "PS", 3, testTTL)
	confirmed := reserveOrFail(t, store, "PS", 1, testTTL)
	if _, err := store.ConfirmReservation(t.Context(), confirmed.ID); err != nil {
		t.Fatalf("setup: ConfirmReservation: %v", err)
	}

	got, err := store.ExpireDue(t.Context(), 100)
	if err != nil {
		t.Fatalf("ExpireDue() error = %v", err)
	}
	if got != 2 {
		t.Errorf("ExpireDue() = %d, want 2", got)
	}

	wantStatus := map[string]inventory.ReservationStatus{
		due.ID:       inventory.StatusExpired,
		alsoDue.ID:   inventory.StatusExpired,
		notDue.ID:    inventory.StatusPending,
		confirmed.ID: inventory.StatusConfirmed,
	}
	for id, want := range wantStatus {
		if got := statusOf(t, store, id); got != want {
			t.Errorf("status of %s = %q, want %q", id, got, want)
		}
	}
	assertStock(t, store, 6)
}

func TestExpireDue_RespectsTheBatchLimit(t *testing.T) {
	store := newTestStore(t)
	if err := store.AddProduct(t.Context(), inventory.Product{SKU: "PS", Name: "Playstation 5", Stock: 5}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for range 5 {
		reserveOrFail(t, store, "PS", 1, -time.Minute)
	}

	for _, want := range []int{2, 2, 1, 0} {
		got, err := store.ExpireDue(t.Context(), 2)
		if err != nil {
			t.Fatalf("ExpireDue() error = %v", err)
		}
		if got != want {
			t.Errorf("ExpireDue() = %d, want %d", got, want)
		}
	}
	assertStock(t, store, 5)
}

func TestExpireDue_ConcurrentExpirersReturnEachUnitOnce(t *testing.T) {
	first := newTestPool(t)
	truncateProducts(t, first)
	stores := []*Store{NewStore(first), NewStore(newTestPool(t))}

	const skus, perSKU = 5, 40
	for i := range skus {
		sku := fmt.Sprintf("SKU-%d", i)
		if err := stores[0].AddProduct(t.Context(), inventory.Product{SKU: sku, Name: sku, Stock: perSKU}); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	for n := range skus * perSKU {
		reserveOrFail(t, stores[0], fmt.Sprintf("SKU-%d", n%skus), 1, -time.Minute)
	}

	var wg sync.WaitGroup
	var expired atomic.Int64
	errs := make(chan error, 8)
	for i := range 8 {
		store := stores[i%len(stores)]
		wg.Go(func() {
			for {
				n, err := store.ExpireDue(t.Context(), 7)
				if err != nil {
					errs <- err
					return
				}
				if n == 0 {
					return
				}
				expired.Add(int64(n))
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("ExpireDue() error = %v", err)
	}
	if got := expired.Load(); got != skus*perSKU {
		t.Errorf("expired reservations = %d, want %d", got, skus*perSKU)
	}
	for i := range skus {
		sku := fmt.Sprintf("SKU-%d", i)
		p, err := stores[0].GetProduct(t.Context(), sku)
		if err != nil {
			t.Fatalf("GetProduct(%q): %v", sku, err)
		}
		if p.Stock != perSKU {
			t.Errorf("stock of %s = %d, want %d", sku, p.Stock, perSKU)
		}
	}
}

func TestExpireDue_RacingCancelReturnsStockOnce(t *testing.T) {
	for range 20 {
		first, r := newReservedStore(t, 5, 2, -time.Minute)
		second := NewStore(newTestPool(t))

		var wg sync.WaitGroup
		var expired int
		var expireErr, cancelErr error
		wg.Go(func() { expired, expireErr = first.ExpireDue(t.Context(), 10) })
		wg.Go(func() { _, cancelErr = second.CancelReservation(t.Context(), r.ID) })
		wg.Wait()

		if expireErr != nil {
			t.Fatalf("ExpireDue() error = %v", expireErr)
		}
		switch {
		case expired == 1 && errors.Is(cancelErr, inventory.ErrReservationNotPending):
			if got := statusOf(t, first, r.ID); got != inventory.StatusExpired {
				t.Errorf("status = %q, want %q", got, inventory.StatusExpired)
			}
		case expired == 0 && cancelErr == nil:
			if got := statusOf(t, first, r.ID); got != inventory.StatusCancelled {
				t.Errorf("status = %q, want %q", got, inventory.StatusCancelled)
			}
		default:
			t.Fatalf("expired = %d, cancel error = %v, want exactly one to win", expired, cancelErr)
		}
		assertStock(t, first, 5)
	}
}

func TestExpireDue_RacingConfirmHasOneWinner(t *testing.T) {
	for i := range 20 {
		first, r := newReservedStore(t, 5, 2, 50*time.Millisecond)
		second := NewStore(newTestPool(t))
		if err := second.pool.Ping(t.Context()); err != nil {
			t.Fatalf("setup: Ping: %v", err)
		}
		time.Sleep(time.Until(r.ExpiresAt) - time.Duration(i%10)*time.Millisecond)

		var wg sync.WaitGroup
		var expired int
		var expireErr, confirmErr error
		wg.Go(func() { expired, expireErr = first.ExpireDue(t.Context(), 10) })
		wg.Go(func() { _, confirmErr = second.ConfirmReservation(t.Context(), r.ID) })
		wg.Wait()

		if expireErr != nil {
			t.Fatalf("ExpireDue() error = %v", expireErr)
		}
		time.Sleep(time.Until(r.ExpiresAt))
		again, err := first.ExpireDue(t.Context(), 10)
		if err != nil {
			t.Fatalf("second ExpireDue() error = %v", err)
		}

		switch {
		case confirmErr == nil:
			if expired+again != 0 {
				t.Errorf("expirations = %d, want 0 after a successful confirm", expired+again)
			}
			if got := statusOf(t, first, r.ID); got != inventory.StatusConfirmed {
				t.Errorf("status = %q, want %q", got, inventory.StatusConfirmed)
			}
			assertStock(t, first, 3)
		case errors.Is(confirmErr, inventory.ErrReservationNotPending), errors.Is(confirmErr, inventory.ErrReservationExpired):
			if expired+again != 1 {
				t.Errorf("expirations = %d, want 1 after a failed confirm", expired+again)
			}
			if got := statusOf(t, first, r.ID); got != inventory.StatusExpired {
				t.Errorf("status = %q, want %q", got, inventory.StatusExpired)
			}
			assertStock(t, first, 5)
		default:
			t.Fatalf("confirm error = %v, want success, %v or %v", confirmErr, inventory.ErrReservationNotPending, inventory.ErrReservationExpired)
		}
	}
}
