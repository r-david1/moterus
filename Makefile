.PHONY: build test run migrate-up migrate-down docker-up docker-down vet tidy

APP_NAME := auth-service
# Rol dueño de la base — solo para DDL (migraciones). No lo use el proceso api.
DATABASE_URL ?= postgres://auth_service:auth_service_dev_password@localhost:5432/auth_service?sslmode=disable
# Rol de login con privilegios acotados (migración 000003) — el que usa el proceso api en runtime.
DATABASE_URL_APLICACION ?= postgres://rol_login_identidad:identidad_app_dev_password@localhost:5432/auth_service?sslmode=disable
# Redis para el limitador de tasa de Confianza (ADR 0018). Sin definir, el
# proceso api monta EvaluadorConfianzaNoOp (WARN de arranque, sin límites reales).
REDIS_URL ?= redis://localhost:6379/0

build:
	go build -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

# test-integracion corre los mismos tests de `go test ./...` pero con las
# variables de entorno que destraban los tests de test/integracion (se
# saltan limpiamente sin ellas, ver test/integracion/entorno_test.go).
# Requiere `make docker-up` + `make migrate-up` antes.
test-integracion:
	DATABASE_URL=$(DATABASE_URL) DATABASE_URL_APLICACION=$(DATABASE_URL_APLICACION) REDIS_URL=$(REDIS_URL) go test ./... -v

# carga corre los escenarios de test/carga contra un servidor ya
# levantado (make run, en otra terminal). Requiere k6 instalado
# (https://k6.io/docs/get-started/installation/); no se instala desde
# este Makefile a propósito (no es una dependencia de build/test normal).
carga:
	k6 run test/carga/rate_limiting_test.js

vet:
	go vet ./...

tidy:
	go mod tidy

run:
	DATABASE_URL_APLICACION=$(DATABASE_URL_APLICACION) REDIS_URL=$(REDIS_URL) go run ./cmd/api

migrate-up:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrador up

migrate-down:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrador down

docker-up:
	docker compose -f deployments/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker-compose.yml down
