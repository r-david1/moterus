.PHONY: build test run migrate-up migrate-down docker-up docker-down vet tidy

APP_NAME := auth-service
# Rol dueño de la base — solo para DDL (migraciones). No lo use el proceso api.
DATABASE_URL ?= postgres://auth_service:auth_service_dev_password@localhost:5432/auth_service?sslmode=disable
# Rol de login con privilegios acotados (migración 000003) — el que usa el proceso api en runtime.
DATABASE_URL_APLICACION ?= postgres://rol_login_identidad:identidad_app_dev_password@localhost:5432/auth_service?sslmode=disable

build:
	go build -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

run:
	DATABASE_URL_APLICACION=$(DATABASE_URL_APLICACION) go run ./cmd/api

migrate-up:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrador up

migrate-down:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrador down

docker-up:
	docker compose -f deployments/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker-compose.yml down
