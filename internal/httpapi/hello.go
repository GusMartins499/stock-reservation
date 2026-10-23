package httpapi

import (
	"fmt"
	"net/http"
)

func (api *API) hello(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, "hello golang")
}
