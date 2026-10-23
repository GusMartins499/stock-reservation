package httpapi

import "github.com/GusMartins499/stock-reservation/internal/inventory"

type API struct {
	store *inventory.Store
}

func NewAPI(store *inventory.Store) *API {
	return &API{store: store}
}
