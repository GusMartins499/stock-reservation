package httpapi

import (
	"context"
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

	reservation, replayed, err := api.store.ReserveOnce(r.Context(), inventory.ReservationRequest{
		Key:      idempotencyKeyFrom(r.Context()),
		SKU:      req.SKU,
		Quantity: req.Quantity,
		TTL:      api.reservationTTL,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	if replayed {
		w.Header().Set(idempotentReplayedHeader, "true")
	}
	writeJSON(w, http.StatusCreated, toReservationResponse(reservation))
}

func (api *API) confirmReservation(w http.ResponseWriter, r *http.Request) {
	api.transitionReservation(w, r, api.store.ConfirmReservation)
}

func (api *API) cancelReservation(w http.ResponseWriter, r *http.Request) {
	api.transitionReservation(w, r, api.store.CancelReservation)
}

func (api *API) transitionReservation(
	w http.ResponseWriter,
	r *http.Request,
	transition func(context.Context, string) (inventory.Reservation, error),
) {
	reservation, err := transition(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toReservationResponse(reservation))
}
