.PHONY: verificar tidy build vet fmt fmt-check test api

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
test:
	go test ./...

api:
	go run ./cmd/api
