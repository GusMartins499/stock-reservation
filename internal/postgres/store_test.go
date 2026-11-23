package postgres

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testTTL = 10 * time.Minute

func countReservations(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM reservations`).Scan(&n); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	return n
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func truncateProducts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(t.Context(), `TRUNCATE reservations, products`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	pool := newTestPool(t)
	truncateProducts(t, pool)

	return NewStore(pool)
}

func TestReserve_ConcurrentBuyersNeverOversell(t *testing.T) {
	first := newTestPool(t)
	second := newTestPool(t)
	truncateProducts(t, first)

	stores := []*Store{NewStore(first), NewStore(second)}
	if err := stores[0].AddProduct(t.Context(), inventory.Product{SKU: "MAC", Name: "Macbook pro", Stock: 100}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var wg sync.WaitGroup
	var counter atomic.Int64

	for i := range 1000 {
		store := stores[i%len(stores)]
		wg.Go(func() {
			if _, err := store.Reserve(t.Context(), "MAC", 1, testTTL); err == nil {
				counter.Add(1)
			}
		})
	}
	wg.Wait()

	if got := counter.Load(); got != 100 {
		t.Errorf("successful reservations = %d, want 100", got)
	}
	p, err := stores[0].GetProduct(t.Context(), "MAC")
	if err != nil {
		t.Fatalf("GetProduct(%q) error = %v", "MAC", err)
	}
	if p.Stock != 0 {
		t.Errorf("stock after concurrent reservations = %d, want 0", p.Stock)
	}
	if got := countReservations(t, first); got != 100 {
		t.Errorf("reservations created = %d, want 100", got)
	}
}

func TestAddProduct(t *testing.T) {
	tests := []struct {
		name     string
		product  inventory.Product
		existing []inventory.Product
		wantErr  error
	}{
		{
			name:    "adds a new product",
			product: inventory.Product{SKU: "PS", Name: "Playstation", Stock: 2},
		},
		{
			name:    "accepts zero stock",
			product: inventory.Product{SKU: "MAC", Name: "Macbook pro", Stock: 0},
		},
		{
			name:    "rejects negative stock",
			product: inventory.Product{SKU: "ANDROID", Name: "Galaxy s23", Stock: -1},
			wantErr: inventory.ErrInvalidQuantity,
		},
		{
			name:    "rejects blank SKU",
			product: inventory.Product{SKU: "  ", Name: "No SKU", Stock: 1},
			wantErr: inventory.ErrInvalidSKU,
		},
		{
			name:     "rejects duplicate SKU",
			product:  inventory.Product{SKU: "MAC", Name: "Macbook air", Stock: 1},
			existing: []inventory.Product{{SKU: "MAC", Name: "Macbook pro", Stock: 1}},
			wantErr:  inventory.ErrProductAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			for _, p := range tt.existing {
				if err := store.AddProduct(t.Context(), p); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}

			err := store.AddProduct(t.Context(), tt.product)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("AddProduct() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				got, err := store.GetProduct(t.Context(), tt.product.SKU)
				if err != nil {
					t.Fatalf("GetProduct() error = %v", err)
				}
				if got != tt.product {
					t.Errorf("GetProduct() = %v, want %v", got, tt.product)
				}
			}
		})
	}
}

func TestGetProduct(t *testing.T) {
	existing := inventory.Product{SKU: "PS", Name: "Playstation 5", Stock: 3}

	tests := []struct {
		name    string
		sku     string
		want    inventory.Product
		wantErr error
	}{
		{name: "returns an existing product", sku: "PS", want: existing},
		{name: "rejects unknown SKU", sku: "XBOX", wantErr: inventory.ErrProductNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			if err := store.AddProduct(t.Context(), existing); err != nil {
				t.Fatalf("setup: %v", err)
			}

			got, err := store.GetProduct(t.Context(), tt.sku)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("GetProduct(%q) error = %v, want %v", tt.sku, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("GetProduct(%q) = %v, want %v", tt.sku, got, tt.want)
			}
		})
	}
}

func TestReserve(t *testing.T) {
	const registeredSKU = "PS"

	tests := []struct {
		name      string
		stock     int
		sku       string
		qty       int
		wantErr   error
		wantStock int
	}{
		{name: "reserves available units", stock: 5, sku: registeredSKU, qty: 1, wantStock: 4},
		{name: "reserves entire stock", stock: 5, sku: registeredSKU, qty: 5, wantStock: 0},
		{name: "rejects quantity above stock", stock: 1, sku: registeredSKU, qty: 2, wantErr: inventory.ErrInsufficientStock, wantStock: 1},
		{name: "rejects sold out product", stock: 0, sku: registeredSKU, qty: 1, wantErr: inventory.ErrInsufficientStock, wantStock: 0},
		{name: "rejects negative quantity", stock: 1, sku: registeredSKU, qty: -1, wantErr: inventory.ErrInvalidQuantity, wantStock: 1},
		{name: "rejects unknown SKU", stock: 2, sku: "XBOX", qty: 1, wantErr: inventory.ErrProductNotFound, wantStock: 2},
		{name: "rejects zero quantity", stock: 2, sku: registeredSKU, qty: 0, wantErr: inventory.ErrInvalidQuantity, wantStock: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newTestPool(t)
			truncateProducts(t, pool)
			store := NewStore(pool)
			if err := store.AddProduct(t.Context(), inventory.Product{SKU: registeredSKU, Name: "Playstation 5", Stock: tt.stock}); err != nil {
				t.Fatalf("setup: %v", err)
			}

			r, err := store.Reserve(t.Context(), tt.sku, tt.qty, testTTL)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Reserve() error = %v, want %v", err, tt.wantErr)
			}
			wantReservations := 0
			if tt.wantErr == nil {
				wantReservations = 1
				if r.ID == "" {
					t.Error("Reserve() returned an empty ID")
				}
				if r.SKU != tt.sku || r.Quantity != tt.qty || r.Status != inventory.StatusPending {
					t.Errorf("Reserve() = %+v, want SKU %q, quantity %d, status %q", r, tt.sku, tt.qty, inventory.StatusPending)
				}
				if got := r.ExpiresAt.Sub(r.CreatedAt); got != testTTL {
					t.Errorf("ExpiresAt - CreatedAt = %v, want %v", got, testTTL)
				}
			}
			if got := countReservations(t, pool); got != wantReservations {
				t.Errorf("reservations created = %d, want %d", got, wantReservations)
			}
			got, err := store.GetProduct(t.Context(), registeredSKU)
			if err != nil {
				t.Fatalf("GetProduct() error = %v", err)
			}
			if got.Stock != tt.wantStock {
				t.Errorf("Stock = %d, want %d", got.Stock, tt.wantStock)
			}
		})
	}
}
