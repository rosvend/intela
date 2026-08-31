.PHONY: verificar tidy build vet fmt fmt-check test prueba-rapida api seed

# La puerta que pide la revision de #6: lo mismo que corre CI, en local.
verificar: tidy build vet fmt-check test

tidy:
	go mod tidy

build:
	go build ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# `gofmt -l` no falla por si solo: lista y devuelve 0. Aqui tiene que fallar.
fmt-check:
	@test -z "$$(gofmt -l .)" || { echo "Sin formatear:"; gofmt -l .; exit 1; }

# Todo el arbol, no solo el dominio: `aplicacion`, `httpapi` y `postgres`
# no los ejecutaba nada.
#
# Desde que hay adaptadores, esto incluye las pruebas de integracion y por
# tanto NECESITA un demonio de Docker: el ADR 0010 pide probar los adaptadores
# contra PostgreSQL de verdad. Es lo mismo que corre CI, que es de lo que
# `verificar` es la puerta.
test:
	go test ./...

# El bucle rapido: solo unidad, sin Docker. NO es la puerta -las pruebas que
# se salta son justamente las que prueban que el SQL funciona.
prueba-rapida:
	go test -short ./...

api:
	go run ./cmd/api

seed:
	go run ./cmd/seed
