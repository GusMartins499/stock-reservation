package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

func TestAPI_CreateProduct(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		storeErr    error
		wantAdded   inventory.Product
		wantProduct productJSON
		wantStatus  int
	}{
		{
			name:        "returns 201 and the created product",
			body:        `{"sku": "PS", "name": "Playstation 5", "stock": 2}`,
			wantAdded:   inventory.Product{SKU: "PS", Name: "Playstation 5", Stock: 2},
			wantProduct: productJSON{SKU: "PS", Name: "Playstation 5", Stock: 2},
			wantStatus:  http.StatusCreated,
		},
		{
			name:       "returns 400 for non-numeric stock",
			body:       `{"sku": "PS", "name": "Playstation 5", "stock": "two"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 400 for unknown field",
			body:       `{"sku": "PS", "nome": "Playstation 4", "stock": 1}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 409 when duplicate SKU",
			body:       `{"sku": "PS", "name": "Playstation 4", "stock": 2}`,
			storeErr:   inventory.ErrProductAlreadyExists,
			wantAdded:  inventory.Product{SKU: "PS", Name: "Playstation 4", Stock: 2},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "returns 422 when negative stock",
			body:       `{"sku": "PS", "name": "Playstation 4", "stock": -1}`,
			storeErr:   inventory.ErrInvalidQuantity,
			wantAdded:  inventory.Product{SKU: "PS", Name: "Playstation 4", Stock: -1},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "returns 400 when blank SKU",
			body:       `{"sku": " ", "name": "Playstation 4", "stock": 1}`,
			storeErr:   inventory.ErrInvalidSKU,
			wantAdded:  inventory.Product{SKU: " ", Name: "Playstation 4", Stock: 1},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 500 when the store fails",
			body:       `{"sku": "PS", "name": "Playstation 4", "stock": 1}`,
			storeErr:   errors.New("connection refused"),
			wantAdded:  inventory.Product{SKU: "PS", Name: "Playstation 4", Stock: 1},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{err: tt.storeErr}
			api := NewAPI(store, testTTL)
			req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("POST /products status = %d, want %d; body: %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("POST /products Content-Type = %q, want %q", contentType, "application/json")
			}
			if store.added != tt.wantAdded {
				t.Errorf("AddProduct() got %+v, want %+v", store.added, tt.wantAdded)
			}
			if tt.wantStatus == http.StatusCreated {
				var got productJSON
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode body %q: %v", rec.Body.String(), err)
				}
				if got != tt.wantProduct {
					t.Errorf("POST /products body = %+v, want %+v", got, tt.wantProduct)
				}
			}
		})
	}
}

func TestAPI_GetProduct(t *testing.T) {
	tests := []struct {
		name        string
		product     inventory.Product
		storeErr    error
		wantProduct productJSON
		wantStatus  int
	}{
		{
			name:        "returns 200 and the product",
			product:     inventory.Product{SKU: "PS", Name: "Playstation", Stock: 2},
			wantProduct: productJSON{SKU: "PS", Name: "Playstation", Stock: 2},
			wantStatus:  http.StatusOK,
		},
		{
			name:       "returns 404 for product not found",
			storeErr:   inventory.ErrProductNotFound,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewAPI(&fakeStore{product: tt.product, err: tt.storeErr}, testTTL)
			req := httptest.NewRequest(http.MethodGet, "/products/PS", nil)
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("GET /products/PS status = %d, want %d; body: %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("GET /products/PS Content-Type = %q, want %q", contentType, "application/json")
			}
			if tt.wantStatus == http.StatusOK {
				var got productJSON
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode body %q: %v", rec.Body.String(), err)
				}
				if got != tt.wantProduct {
					t.Errorf("GET /products/PS body = %+v, want %+v", got, tt.wantProduct)
				}
			}
		})
	}
}
