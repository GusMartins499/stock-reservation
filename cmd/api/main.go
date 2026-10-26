package main

import (
	"log"
	"net/http"

	"github.com/GusMartins499/stock-reservation/internal/httpapi"
	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

func main() {
	store := inventory.NewStore()
	api := httpapi.NewAPI(store)
	log.Fatal(http.ListenAndServe(":8080", api.Routes()))
}
