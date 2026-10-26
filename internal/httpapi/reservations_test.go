package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

func TestAPI_CreateReservation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		existing   []inventory.Product
		wantStatus int
		wantStock  int
	}{
		{
			name:       "returns 201 when create a reservation",
			body:       `{"sku": "PS", "quantity": 1}`,
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusCreated,
			wantStock:  1,
		},
		{
			name:       "returns 409 when insufficient stock",
			body:       `{"sku": "PS", "quantity": 3}`,
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusConflict,
			wantStock:  2,
		},
		{
			name:       "returns 422 when invalid quantity",
			body:       `{"sku": "PS", "quantity": -1}`,
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusUnprocessableEntity,
			wantStock:  2,
		},
		{
			name:       "returns 404 for unknown SKU",
			body:       `{"sku": "xbox", "quantity": 1}`,
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusNotFound,
			wantStock:  2,
		},
		{
			name:       "returns 400 when invalid body",
			body:       `{"sku": "PS", "quantity": "one"}`,
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusBadRequest,
			wantStock:  2,
		},
		{
			name:       "returns 400 for unknown field",
			body:       `{"sku": "PS", "qty": 1}`,
			existing:   []inventory.Product{{SKU: "PS", Name: "Playstation", Stock: 2}},
			wantStatus: http.StatusBadRequest,
			wantStock:  2,
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
			req := httptest.NewRequest(http.MethodPost, "/reservations", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("POST /reservations status = %d, want %d; body: %q", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus != http.StatusCreated {
				contentType := rec.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("POST /reservations Content-Type = %q, want %q", contentType, "application/json")
				}
			}

			p, err := store.GetProduct("PS")
			if err != nil {
				t.Fatalf("GetProduct(%q): %v", "PS", err)
			}
			if p.Stock != tt.wantStock {
				t.Errorf("stock after POST /reservations = %d, want %d", p.Stock, tt.wantStock)
			}
		})
	}
}
