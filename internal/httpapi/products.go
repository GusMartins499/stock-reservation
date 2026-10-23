package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

type productJSON struct {
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	Stock int    `json:"stock"`
}

func toProductResponse(p inventory.Product) productJSON {
	return productJSON{SKU: p.SKU, Name: p.Name, Stock: p.Stock}
}

func (api *API) createProduct(w http.ResponseWriter, r *http.Request) {
	var req productJSON
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p := inventory.Product{SKU: req.SKU, Name: req.Name, Stock: req.Stock}
	if err := api.store.AddProduct(p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, toProductResponse(p))
}

func (api *API) getProduct(w http.ResponseWriter, r *http.Request) {
	p, err := api.store.GetProduct(r.PathValue("sku"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, toProductResponse(p))
}
