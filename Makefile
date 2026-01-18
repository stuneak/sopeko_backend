include .env
export

# ==================== Development ====================
migrateup:
	migrate -path db/sqlc/migration -database "$(DB_SOURCE)" -verbose up

migratedown:
	migrate -path db/sqlc/migration -database "$(DB_SOURCE)" -verbose down

new_migration:
	migrate create -ext sql -dir db/sqlc/migration -seq $(name)

sqlc:
	sqlc generate

server:
	go run main.go

test:
	go test -v -cover ./...

uplocal:
	docker-compose -f ./docker-compose.yml up -d --build

downlocal:
	docker-compose -f ./docker-compose.yml down


.PHONY: migrateup migratedown new_migration sqlc server test uplocal downlocal