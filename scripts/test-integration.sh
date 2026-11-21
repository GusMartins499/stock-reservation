#!/bin/sh
set -eu

DB_NAME="stock_reservation_test_$$"
PSQL="docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -q -U stock"

drop_db() {
	$PSQL -d postgres -c "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE)"
}
trap drop_db EXIT

$PSQL -d postgres -c "CREATE DATABASE $DB_NAME"
$PSQL -d "$DB_NAME" < db/schema.sql

TEST_DATABASE_URL="postgres://stock:stock@localhost:5434/$DB_NAME" \
	go test -race -count=1 ./internal/postgres/ "$@"
