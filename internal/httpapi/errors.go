package httpapi

import (
	"errors"
	"net/http"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type errorJSON struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inventory.ErrProductNotFound):
		writeJSON(w, http.StatusNotFound, errorJSON{Error: inventory.ErrProductNotFound.Error()})
	case errors.Is(err, inventory.ErrProductAlreadyExists):
		writeJSON(w, http.StatusConflict, errorJSON{Error: inventory.ErrProductAlreadyExists.Error()})
	case errors.Is(err, inventory.ErrInsufficientStock):
		writeJSON(w, http.StatusConflict, errorJSON{Error: inventory.ErrInsufficientStock.Error()})
	case errors.Is(err, inventory.ErrReservationNotFound):
		writeJSON(w, http.StatusNotFound, errorJSON{Error: inventory.ErrReservationNotFound.Error()})
	case errors.Is(err, inventory.ErrReservationNotPending):
		writeJSON(w, http.StatusConflict, errorJSON{Error: inventory.ErrReservationNotPending.Error()})
	case errors.Is(err, inventory.ErrReservationExpired):
		writeJSON(w, http.StatusConflict, errorJSON{Error: inventory.ErrReservationExpired.Error()})
	case errors.Is(err, inventory.ErrInvalidQuantity):
		writeJSON(w, http.StatusUnprocessableEntity, errorJSON{Error: inventory.ErrInvalidQuantity.Error()})
	case errors.Is(err, inventory.ErrInvalidSKU):
		writeJSON(w, http.StatusBadRequest, errorJSON{Error: inventory.ErrInvalidSKU.Error()})
	case errors.Is(err, inventory.ErrInvalidIdempotencyKey):
		writeJSON(w, http.StatusBadRequest, errorJSON{Error: inventory.ErrInvalidIdempotencyKey.Error()})
	case errors.Is(err, inventory.ErrIdempotencyKeyReused):
		writeJSON(w, http.StatusUnprocessableEntity, errorJSON{Error: inventory.ErrIdempotencyKeyReused.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorJSON{Error: "internal server error"})
	}
}
