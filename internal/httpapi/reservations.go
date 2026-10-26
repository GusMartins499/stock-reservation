package httpapi

import (
	"encoding/json"
	"net/http"
)

type reservationRequest struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

func (api *API) createReservation(w http.ResponseWriter, r *http.Request) {
	var req reservationRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorJSON{Error: "invalid JSON body"})
		return
	}

	if err := api.store.Reserve(req.SKU, req.Quantity); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
