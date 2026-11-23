package postgres

import "context"

func (s *Store) ExpireDue(ctx context.Context, limit int) (int, error) {
	var expired int
	err := s.pool.QueryRow(ctx, `
		WITH due AS (
			SELECT id FROM reservations
			WHERE status = 'pending' AND expires_at <= now()
			ORDER BY expires_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		), expired AS (
			UPDATE reservations r SET status = 'expired'
			FROM due
			WHERE r.id = due.id
			RETURNING r.sku, r.quantity
		), totals AS (
			SELECT sku, sum(quantity)::integer AS quantity FROM expired GROUP BY sku
		), locked AS (
			SELECT p.sku, t.quantity
			FROM products p
			JOIN totals t ON t.sku = p.sku
			ORDER BY p.sku
			FOR UPDATE OF p
		), restocked AS (
			UPDATE products p SET stock = p.stock + l.quantity
			FROM locked l
			WHERE p.sku = l.sku
		)
		SELECT count(*) FROM expired`,
		limit,
	).Scan(&expired)
	return expired, err
}
