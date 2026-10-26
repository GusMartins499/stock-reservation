package httpapi

import (
	"encoding/json"
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
		existing    []inventory.Product
		wantProduct productJSON
		wantStatus  int
	}{
		{
			name:        "returns 201 and the created product",
			body:        `{"sku": "PS", "name": "Playstation 5", "stock": 2}`,
			wantProduct: productJSON{SKU: "PS", Name: "Playstation 5", Stock: 2},
			wantStatus:  http.StatusCreated,
		},
		{
			name:       "returns 400 for non-numeric stock",
			body:       `{"sku": "PS", "name": "Playstation 5", "stock": "two"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 409 when duplicate SKU",
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			body:       `{"sku": "PS", "name": "Playstation 4", "stock": 2}`,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "returns 422 when negative stock",
			body:       `{"sku": "PS", "name": "Playstation 4", "stock": -1}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "returns 400 for unknown field",
			body:       `{"sku": "PS", "nome": "Playstation 4", "stock": 1}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := inventory.NewStore()
			api := NewAPI(store)
			for _, p := range tt.existing {
				if err := store.AddProduct(p); err != nil {
					t.Fatalf("setup: AddProduct(%+v): %v", p, err)
				}
			}
			req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("POST /products status = %d, want %d; body: %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("POST /products Content-Type = %q, want %q", contentType, "application/json")
			}
			var got productJSON
			if tt.wantStatus == http.StatusCreated {
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
	const registeredSKU = "PS"

	tests := []struct {
		name        string
		sku         string
		existing    []inventory.Product
		wantProduct productJSON
		wantStatus  int
	}{
		{
			name:       "returns 404 for product not found",
			sku:        "xbox",
			existing:   []inventory.Product{{SKU: registeredSKU, Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "returns 200 and return product",
			wantProduct: productJSON{SKU: registeredSKU, Name: "Playstation", Stock: 2},
			existing:    []inventory.Product{{SKU: registeredSKU, Name: "Playstation", Stock: 2}},
			sku:         registeredSKU,
			wantStatus:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := inventory.NewStore()
			api := NewAPI(store)
			for _, p := range tt.existing {
				if err := store.AddProduct(p); err != nil {
					t.Fatalf("setup: AddProduct(%+v): %v", p, err)
				}
			}
			endpoint := "/products/" + tt.sku
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s status = %d, want %d body:%q", endpoint, rec.Code, tt.wantStatus, rec.Body.String())
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("GET %s Content-Type = %q, want %q, body: %q", endpoint, contentType, "application/json", rec.Body.String())
			}

			var got productJSON
			if tt.wantStatus == http.StatusOK {
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode body %q: %v", rec.Body.String(), err)
				}
				if got != tt.wantProduct {
					t.Errorf("GET %s body = %+v, want %+v body: %q", endpoint, got, tt.wantProduct, rec.Body.String())
				}
			}
		})
	}
}
