package postgres

import (
	"errors"
	"os"
	"testing"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestStore(t *testing.T) *Store {
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

	if _, err := pool.Exec(t.Context(), `TRUNCATE products`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	return NewStore(pool)
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
