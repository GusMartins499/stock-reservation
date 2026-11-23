package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type reservationRequest struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type reservationJSON struct {
	ID        string    `json:"id"`
	SKU       string    `json:"sku"`
	Quantity  int       `json:"quantity"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

func toReservationResponse(r inventory.Reservation) reservationJSON {
	return reservationJSON{
		ID:        r.ID,
		SKU:       r.SKU,
		Quantity:  r.Quantity,
		Status:    string(r.Status),
		ExpiresAt: r.ExpiresAt.UTC(),
	}
}

func (api *API) createReservation(w http.ResponseWriter, r *http.Request) {
	var req reservationRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorJSON{Error: "invalid JSON body"})
		return
	}

	reservation, err := api.store.Reserve(r.Context(), req.SKU, req.Quantity, api.reservationTTL)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toReservationResponse(reservation))
}
