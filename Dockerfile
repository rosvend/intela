FROM golang:1.24-bookworm AS build
WORKDIR /src

# Capa de dependencias aparte: cambiar codigo no vuelve a descargar modulos.
# go.sum va incluido y `go mod download` ya no lleva `|| true`, que se tragaba
# precisamente el fallo de go.sum ausente.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -trimpath quita las rutas de compilacion del binario, y -ldflags="-s -w"
# los simbolos de depuracion: builds reproducibles y binarios mas pequenos.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/ ./cmd/api ./cmd/worker ./cmd/scheduler ./cmd/migrate ./cmd/seed

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api       /app/api
COPY --from=build /out/worker    /app/worker
COPY --from=build /out/scheduler /app/scheduler
# Las migraciones van EMBEBIDAS en este binario (migrations/embed.go), asi que
# no hay que copiar el directorio ni acertar con la variable MIGRATIONS.
COPY --from=build /out/migrate   /app/migrate
COPY --from=build /out/seed      /app/seed
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
