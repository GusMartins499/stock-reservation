package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/GusMartins499/stock-reservation/internal/inventory"
)

const (
	idempotencyKeyHeader     = "Idempotency-Key"
	idempotentReplayedHeader = "Idempotent-Replayed"
	maxIdempotencyKeyLength  = 255
)

type idempotencyKeyContextKey struct{}

func requireIdempotencyKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(idempotencyKeyHeader)
		if strings.TrimSpace(key) == "" || len(key) > maxIdempotencyKeyLength {
			writeError(w, inventory.ErrInvalidIdempotencyKey)
			return
		}
		ctx := context.WithValue(r.Context(), idempotencyKeyContextKey{}, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func idempotencyKeyFrom(ctx context.Context) string {
	key, _ := ctx.Value(idempotencyKeyContextKey{}).(string)
	return key
}
