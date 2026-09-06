.PHONY: verificar tidy build vet fmt fmt-check test prueba-rapida api lambda tf-fmt plan aplicar

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

# --- Despliegue -------------------------------------------------------------

LAMBDA_DIR := dist/lambda
ENV        ?= nheo
TF         := terraform -chdir=infra/envs/$(ENV)
VERSION    ?= $(shell git rev-parse HEAD)

# Los artefactos de Lambda. Un ejecutable por punto de entrada, y tiene que
# llamarse `bootstrap`: es lo que busca el runtime provided.al2023.
#
# arm64 porque cuesta menos por GB-segundo, y la instancia de base tambien es
# Graviton. CGO_ENABLED=0 porque el runtime no trae libc.
#
# El `touch` a una fecha fija no es manía: sin él, el zip cambia de hash en cada
# build aunque el código sea idéntico, y `terraform plan` propone actualizar las
# funciones en cada corrida. Con él, un plan sin cambios se ve sin cambios, que
# es lo que hace revisable la guarda de destrucción.
lambda:
	@rm -rf $(LAMBDA_DIR)
	@mkdir -p $(LAMBDA_DIR)/api $(LAMBDA_DIR)/migrate
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
		-o $(LAMBDA_DIR)/api/bootstrap ./cmd/lambda
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
		-o $(LAMBDA_DIR)/migrate/bootstrap ./cmd/lambda-migrate
	@touch -t 200001010000 $(LAMBDA_DIR)/api/bootstrap $(LAMBDA_DIR)/migrate/bootstrap
	@cd $(LAMBDA_DIR)/api     && zip -q -X ../api.zip bootstrap
	@cd $(LAMBDA_DIR)/migrate && zip -q -X ../migrate.zip bootstrap
	@ls -lh $(LAMBDA_DIR)/*.zip

tf-fmt:
	terraform fmt -recursive infra/

# `plan` y `aplicar` necesitan los zips: modules/go-lambda calcula el hash del
# fichero, asi que sin artefactos el plan ni siquiera evalua.
plan: lambda
	$(TF) plan -var app_version=$(VERSION)

aplicar: lambda
	$(TF) apply -var app_version=$(VERSION)
