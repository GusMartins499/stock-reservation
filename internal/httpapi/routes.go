package httpapi

import "net/http"

func (api *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", api.hello)
	mux.HandleFunc("POST /products", api.createProduct)
	mux.HandleFunc("GET /products/{sku}", api.getProduct)
	mux.HandleFunc("POST /reservations", api.createReservation)
	mux.HandleFunc("POST /reservations/{id}/confirm", api.confirmReservation)
	mux.HandleFunc("POST /reservations/{id}/cancel", api.cancelReservation)

	return mux
}
