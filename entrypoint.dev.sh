#!/bin/sh
set -e

echo "Running database migrations..."
migrate -path db/sqlc/migration -database "$DB_SOURCE" -verbose up

echo "Starting Air hot reload..."
exec air -c .air.toml
