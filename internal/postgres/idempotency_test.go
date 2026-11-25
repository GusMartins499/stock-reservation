package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

func newProductStore(t *testing.T, stock int) *Store {
	t.Helper()

	store := newTestStore(t)
	if err := store.AddProduct(t.Context(), inventory.Product{SKU: "PS", Name: "Playstation 5", Stock: stock}); err != nil {
		t.Fatalf("setup: AddProduct: %v", err)
	}
	return store
}

func reserveOnceOrFail(t *testing.T, store *Store, req inventory.ReservationRequest) inventory.Reservation {
	t.Helper()

	r, _, err := store.ReserveOnce(t.Context(), req)
	if err != nil {
		t.Fatalf("setup: ReserveOnce(%+v): %v", req, err)
	}
	return r
}

func countKeys(t *testing.T, store *Store) int {
	t.Helper()

	var n int
	if err := store.pool.QueryRow(t.Context(), `SELECT count(*) FROM idempotency_keys`).Scan(&n); err != nil {
		t.Fatalf("count idempotency keys: %v", err)
	}
	return n
}

func TestReserveOnce_NewKeyReserves(t *testing.T) {
	store := newProductStore(t, 5)

	r, replayed, err := store.ReserveOnce(t.Context(), inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 2, TTL: testTTL})

	if err != nil {
		t.Fatalf("ReserveOnce() error = %v", err)
	}
	if replayed {
		t.Errorf("replayed = true, want false for a new key")
	}
	if r.SKU != "PS" || r.Quantity != 2 || r.Status != inventory.StatusPending {
		t.Errorf("reservation = %+v, want a pending reservation of 2 PS", r)
	}
	assertStock(t, store, 3)
	if got := countKeys(t, store); got != 1 {
		t.Errorf("idempotency keys = %d, want 1", got)
	}
}

func TestReserveOnce_SameKeyReplaysTheReservation(t *testing.T) {
	tests := []struct {
		name  string
		stock int
		qty   int
	}{
		{name: "with stock left", stock: 5, qty: 2},
		{name: "after taking the last unit", stock: 1, qty: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newProductStore(t, tt.stock)
			req := inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: tt.qty, TTL: testTTL}
			first := reserveOnceOrFail(t, store, req)

			again, replayed, err := store.ReserveOnce(t.Context(), req)

			if err != nil {
				t.Fatalf("second ReserveOnce() error = %v", err)
			}
			if !replayed {
				t.Errorf("replayed = false, want true for a known key")
			}
			if again.ID != first.ID {
				t.Errorf("replayed id = %q, want %q", again.ID, first.ID)
			}
			assertStock(t, store, tt.stock-tt.qty)
			if got := countReservations(t, store.pool); got != 1 {
				t.Errorf("reservations = %d, want 1", got)
			}
		})
	}
}

func TestReserveOnce_ReplayReturnsTheCurrentState(t *testing.T) {
	store := newProductStore(t, 5)
	req := inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 2, TTL: testTTL}
	first := reserveOnceOrFail(t, store, req)
	if _, err := store.ConfirmReservation(t.Context(), first.ID); err != nil {
		t.Fatalf("setup: ConfirmReservation: %v", err)
	}

	again, _, err := store.ReserveOnce(t.Context(), req)

	if err != nil {
		t.Fatalf("ReserveOnce() error = %v", err)
	}
	if again.Status != inventory.StatusConfirmed {
		t.Errorf("replayed status = %q, want %q", again.Status, inventory.StatusConfirmed)
	}
}

func TestReserveOnce_RejectsKeyReusedWithADifferentRequest(t *testing.T) {
	tests := []struct {
		name string
		req  inventory.ReservationRequest
	}{
		{name: "different quantity", req: inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 3, TTL: testTTL}},
		{name: "different SKU", req: inventory.ReservationRequest{Key: "k1", SKU: "XBOX", Quantity: 2, TTL: testTTL}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newProductStore(t, 5)
			if err := store.AddProduct(t.Context(), inventory.Product{SKU: "XBOX", Name: "Xbox", Stock: 5}); err != nil {
				t.Fatalf("setup: AddProduct: %v", err)
			}
			reserveOnceOrFail(t, store, inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 2, TTL: testTTL})

			_, _, err := store.ReserveOnce(t.Context(), tt.req)

			if !errors.Is(err, inventory.ErrIdempotencyKeyReused) {
				t.Errorf("ReserveOnce() error = %v, want %v", err, inventory.ErrIdempotencyKeyReused)
			}
			assertStock(t, store, 3)
			if got := countReservations(t, store.pool); got != 1 {
				t.Errorf("reservations = %d, want 1", got)
			}
		})
	}
}

func TestReserveOnce_FailedReservationLeavesNoKey(t *testing.T) {
	tests := []struct {
		name    string
		stock   int
		req     inventory.ReservationRequest
		wantErr error
	}{
		{
			name:    "insufficient stock",
			stock:   1,
			req:     inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 2, TTL: testTTL},
			wantErr: inventory.ErrInsufficientStock,
		},
		{
			name:    "unknown SKU",
			stock:   1,
			req:     inventory.ReservationRequest{Key: "k1", SKU: "XBOX", Quantity: 1, TTL: testTTL},
			wantErr: inventory.ErrProductNotFound,
		},
		{
			name:    "invalid quantity",
			stock:   1,
			req:     inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 0, TTL: testTTL},
			wantErr: inventory.ErrInvalidQuantity,
		},
		{
			name:    "blank key",
			stock:   1,
			req:     inventory.ReservationRequest{Key: " ", SKU: "PS", Quantity: 1, TTL: testTTL},
			wantErr: inventory.ErrInvalidIdempotencyKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newProductStore(t, tt.stock)

			_, _, err := store.ReserveOnce(t.Context(), tt.req)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ReserveOnce() error = %v, want %v", err, tt.wantErr)
			}
			if got := countKeys(t, store); got != 0 {
				t.Errorf("idempotency keys = %d, want 0", got)
			}
			assertStock(t, store, tt.stock)
		})
	}
}

func TestReserveOnce_RetryAfterAFailureCanSucceed(t *testing.T) {
	store := newProductStore(t, 1)
	req := inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 2, TTL: testTTL}
	if _, _, err := store.ReserveOnce(t.Context(), req); !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Fatalf("setup: ReserveOnce() error = %v, want %v", err, inventory.ErrInsufficientStock)
	}
	if _, err := store.pool.Exec(t.Context(), `UPDATE products SET stock = 2 WHERE sku = 'PS'`); err != nil {
		t.Fatalf("setup: restock: %v", err)
	}

	_, replayed, err := store.ReserveOnce(t.Context(), req)

	if err != nil {
		t.Fatalf("ReserveOnce() error = %v", err)
	}
	if replayed {
		t.Errorf("replayed = true, want false after a failed first attempt")
	}
	assertStock(t, store, 0)
}

func TestReserveOnce_ConcurrentRetriesReserveOnce(t *testing.T) {
	const retries = 50
	first := newProductStore(t, 5)
	second := NewStore(newTestPool(t))
	stores := []*Store{first, second}
	req := inventory.ReservationRequest{Key: "k1", SKU: "PS", Quantity: 2, TTL: testTTL}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := make(map[string]int)
	var replays int
	var errs []error
	for i := range retries {
		store := stores[i%len(stores)]
		wg.Go(func() {
			r, replayed, err := store.ReserveOnce(t.Context(), req)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids[r.ID]++
			if replayed {
				replays++
			}
		})
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("ReserveOnce() errors = %v, want none", errs)
	}
	if len(ids) != 1 {
		t.Errorf("distinct reservation ids = %d, want 1", len(ids))
	}
	if replays != retries-1 {
		t.Errorf("replays = %d, want %d", replays, retries-1)
	}
	assertStock(t, first, 3)
	if got := countReservations(t, first.pool); got != 1 {
		t.Errorf("reservations = %d, want 1", got)
	}
}

func TestDeleteIdempotencyKeys(t *testing.T) {
	tests := []struct {
		name        string
		ages        []time.Duration
		limit       int
		wantDeleted int
		wantLeft    int
	}{
		{name: "deletes keys older than the TTL", ages: []time.Duration{25 * time.Hour, 48 * time.Hour, time.Hour}, limit: 10, wantDeleted: 2, wantLeft: 1},
		{name: "respects the batch limit", ages: []time.Duration{25 * time.Hour, 26 * time.Hour, 27 * time.Hour}, limit: 2, wantDeleted: 2, wantLeft: 1},
		{name: "keeps keys within the TTL", ages: []time.Duration{time.Minute, 23 * time.Hour}, limit: 10, wantDeleted: 0, wantLeft: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newProductStore(t, 10)
			for i, age := range tt.ages {
				r := reserveOnceOrFail(t, store, inventory.ReservationRequest{Key: string(rune('a' + i)), SKU: "PS", Quantity: 1, TTL: testTTL})
				if _, err := store.pool.Exec(t.Context(),
					`UPDATE idempotency_keys SET created_at = now() - make_interval(secs => $2) WHERE reservation_id = $1`,
					r.ID, age.Seconds(),
				); err != nil {
					t.Fatalf("setup: age key: %v", err)
				}
			}

			deleted, err := store.DeleteIdempotencyKeys(t.Context(), 24*time.Hour, tt.limit)

			if err != nil {
				t.Fatalf("DeleteIdempotencyKeys() error = %v", err)
			}
			if deleted != tt.wantDeleted {
				t.Errorf("deleted = %d, want %d", deleted, tt.wantDeleted)
			}
			if got := countKeys(t, store); got != tt.wantLeft {
				t.Errorf("keys left = %d, want %d", got, tt.wantLeft)
			}
		})
	}
}
