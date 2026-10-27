package inventory

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReserve_ConcurrentBuyersNeverOversell(t *testing.T) {
	store := NewStore()
	if err := store.AddProduct(Product{SKU: "MAC", Name: "Macbook pro", Stock: 100}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	var wg sync.WaitGroup
	var counter atomic.Int64

	for range 1000 {
		wg.Go(func() {
			if err := store.Reserve("MAC", 1); err == nil {
				counter.Add(1)
			}
		})
	}
	wg.Wait()
	if got := counter.Load(); got != 100 {
		t.Errorf("successful reservations = %d, want 100", got)
	}
	p, err := store.GetProduct("MAC")
	if err != nil {
		t.Fatalf("GetProduct(%q) error = %v", "MAC", err)
	}
	if p.Stock != 0 {
		t.Errorf("stock after concurrent reservations = %d, want 0", p.Stock)
	}
}

func TestAddProduct(t *testing.T) {
	tests := []struct {
		name     string
		product  Product
		existing []Product
		wantErr  error
	}{
		{
			name:    "adds a new product",
			product: Product{SKU: "PS", Name: "Playstation", Stock: 2},
		},
		{
			name:    "accepts zero stock",
			product: Product{SKU: "MAC", Name: "Macbook pro", Stock: 0},
		},
		{
			name:    "rejects negative stock",
			product: Product{SKU: "ANDROID", Name: "Galaxy s23", Stock: -1},
			wantErr: ErrInvalidQuantity,
		},
		{
			name:     "rejects duplicate SKU",
			product:  Product{SKU: "MAC", Name: "Macbook air", Stock: 1},
			existing: []Product{{SKU: "MAC", Name: "Macbook pro", Stock: 1}},
			wantErr:  ErrProductAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore()
			for _, p := range tt.existing {
				if err := store.AddProduct(p); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			err := store.AddProduct(tt.product)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("AddProduct() error = %+v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				got, err := store.GetProduct(tt.product.SKU)
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
		{name: "rejects quantity above stock", stock: 1, sku: registeredSKU, qty: 2, wantErr: ErrInsufficientStock, wantStock: 1},
		{name: "rejects sold out product", stock: 0, sku: registeredSKU, qty: 1, wantErr: ErrInsufficientStock, wantStock: 0},
		{name: "rejects negative quantity", stock: 1, sku: registeredSKU, qty: -1, wantErr: ErrInvalidQuantity, wantStock: 1},
		{name: "rejects unknown SKU", stock: 2, sku: "XBOX", qty: 1, wantErr: ErrProductNotFound, wantStock: 2},
		{name: "rejects zero quantity", stock: 2, sku: registeredSKU, qty: 0, wantErr: ErrInvalidQuantity, wantStock: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore()
			if err := store.AddProduct(Product{Name: "Playstation 5", SKU: registeredSKU, Stock: tt.stock}); err != nil {
				t.Fatalf("setup: %v", err)
			}
			err := store.Reserve(tt.sku, tt.qty)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Reserve() error = %v, want %v", err, tt.wantErr)
			}
			got, err := store.GetProduct(registeredSKU)
			if err != nil {
				t.Fatalf("GetProduct() error = %v", err)
			}
			if got.Stock != tt.wantStock {
				t.Errorf("Stock = %d, want %d", got.Stock, tt.wantStock)
			}
		})
	}
}
