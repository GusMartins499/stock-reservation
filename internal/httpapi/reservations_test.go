package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

const testTTL = 10 * time.Minute

func TestAPI_CreateReservation(t *testing.T) {
	expiresAt := time.Date(2026, 11, 27, 12, 10, 0, 0, time.UTC)
	pending := inventory.Reservation{
		ID:        "6f1c2b9e-3d4a-4b5c-8e7f-1a2b3c4d5e6f",
		SKU:       "PS",
		Quantity:  1,
		Status:    inventory.StatusPending,
		ExpiresAt: expiresAt,
	}

	tests := []struct {
		name            string
		body            string
		reservation     inventory.Reservation
		storeErr        error
		wantStatus      int
		wantReservation reservationJSON
		wantStoreCalled bool
	}{
		{
			name:        "returns 201 and the pending reservation",
			body:        `{"sku": "PS", "quantity": 1}`,
			reservation: pending,
			wantStatus:  http.StatusCreated,
			wantReservation: reservationJSON{
				ID:        pending.ID,
				SKU:       "PS",
				Quantity:  1,
				Status:    "pending",
				ExpiresAt: expiresAt,
			},
			wantStoreCalled: true,
		},
		{
			name:            "returns 409 when insufficient stock",
			body:            `{"sku": "PS", "quantity": 1}`,
			storeErr:        inventory.ErrInsufficientStock,
			wantStatus:      http.StatusConflict,
			wantStoreCalled: true,
		},
		{
			name:            "returns 422 when invalid quantity",
			body:            `{"sku": "PS", "quantity": 1}`,
			storeErr:        inventory.ErrInvalidQuantity,
			wantStatus:      http.StatusUnprocessableEntity,
			wantStoreCalled: true,
		},
		{
			name:            "returns 404 for unknown SKU",
			body:            `{"sku": "PS", "quantity": 1}`,
			storeErr:        inventory.ErrProductNotFound,
			wantStatus:      http.StatusNotFound,
			wantStoreCalled: true,
		},
		{
			name:       "returns 400 when invalid body",
			body:       `{"sku": "PS", "quantity": "one"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns 400 for unknown field",
			body:       `{"sku": "PS", "qty": 1}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{reservation: tt.reservation, err: tt.storeErr}
			api := NewAPI(store, testTTL)
			req := httptest.NewRequest(http.MethodPost, "/reservations", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("POST /reservations status = %d, want %d; body: %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("POST /reservations Content-Type = %q, want %q", contentType, "application/json")
			}
			if tt.wantStoreCalled {
				if store.reservedSKU != "PS" || store.reservedQty != 1 || store.reservedTTL != testTTL {
					t.Errorf("Reserve() got (%q, %d, %v), want (%q, %d, %v)", store.reservedSKU, store.reservedQty, store.reservedTTL, "PS", 1, testTTL)
				}
			} else if store.reservedSKU != "" {
				t.Errorf("Reserve() called with %q, want no call", store.reservedSKU)
			}
			if tt.wantStatus == http.StatusCreated {
				var got reservationJSON
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode body %q: %v", rec.Body.String(), err)
				}
				if got != tt.wantReservation {
					t.Errorf("POST /reservations body = %+v, want %+v", got, tt.wantReservation)
				}
			}
		})
	}
}

func TestAPI_ReservationTransitions(t *testing.T) {
	const id = "6f1c2b9e-3d4a-4b5c-8e7f-1a2b3c4d5e6f"
	expiresAt := time.Date(2026, 11, 27, 12, 10, 0, 0, time.UTC)

	actions := []struct {
		name     string
		status   inventory.ReservationStatus
		calledID func(*fakeStore) string
	}{
		{name: "confirm", status: inventory.StatusConfirmed, calledID: func(f *fakeStore) string { return f.confirmedID }},
		{name: "cancel", status: inventory.StatusCancelled, calledID: func(f *fakeStore) string { return f.cancelledID }},
	}

	tests := []struct {
		name       string
		storeErr   error
		wantStatus int
	}{
		{name: "returns 200 and the reservation", wantStatus: http.StatusOK},
		{name: "returns 404 for unknown reservation", storeErr: inventory.ErrReservationNotFound, wantStatus: http.StatusNotFound},
		{name: "returns 409 when not pending", storeErr: inventory.ErrReservationNotPending, wantStatus: http.StatusConflict},
		{name: "returns 409 when expired", storeErr: inventory.ErrReservationExpired, wantStatus: http.StatusConflict},
	}

	for _, action := range actions {
		for _, tt := range tests {
			t.Run(action.name+"/"+tt.name, func(t *testing.T) {
				store := &fakeStore{
					reservation: inventory.Reservation{ID: id, SKU: "PS", Quantity: 2, Status: action.status, ExpiresAt: expiresAt},
					err:         tt.storeErr,
				}
				api := NewAPI(store, testTTL)
				endpoint := "/reservations/" + id + "/" + action.name
				req := httptest.NewRequest(http.MethodPost, endpoint, nil)
				rec := httptest.NewRecorder()
				api.Routes().ServeHTTP(rec, req)

				if rec.Code != tt.wantStatus {
					t.Errorf("POST %s status = %d, want %d; body: %q", endpoint, rec.Code, tt.wantStatus, rec.Body.String())
				}
				if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
					t.Errorf("POST %s Content-Type = %q, want %q", endpoint, contentType, "application/json")
				}
				if got := action.calledID(store); got != id {
					t.Errorf("store called with id %q, want %q", got, id)
				}
				if tt.wantStatus == http.StatusOK {
					var got reservationJSON
					if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
						t.Fatalf("decode body %q: %v", rec.Body.String(), err)
					}
					want := reservationJSON{ID: id, SKU: "PS", Quantity: 2, Status: string(action.status), ExpiresAt: expiresAt}
					if got != want {
						t.Errorf("POST %s body = %+v, want %+v", endpoint, got, want)
					}
				}
			})
		}
	}
}
