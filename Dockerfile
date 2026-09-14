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
#
# ./cmd/seed NO esta en la lista, y no es un olvido: se compila en su propia
# etapa, mas abajo. Ver la etapa `seed`.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/ ./cmd/api ./cmd/worker ./cmd/scheduler ./cmd/migrate

# La raiz de la boveda de reportes crudos, creada AQUI para poder copiarla con
# su dueno en las imagenes finales.
#
# Docker inicializa un volumen con nombre a partir de lo que la imagen tenga en
# el punto de montaje. Sin este directorio, el volumen `objetos` nacia
# root:root 0755 y el proceso, que corre como nonroot (uid 65532, distroless),
# no podia escribir: `mkdir /objetos/reportes: permission denied`, y habia que
# hacerle un chown a mano al volumen. Con el directorio en la imagen y el dueno
# correcto, un volumen recien creado lo hereda.
RUN mkdir -p /out/objetos

# El seed se compila aparte para que su binario no comparta sistema de ficheros
# con los binarios del servicio: la imagen de la API no puede contenerlo ni por
# un COPY de mas.
FROM build AS build-seed
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out-seed/ ./cmd/seed

# Imagen del sembrador. Se pide explicitamente con `target: seed`, que es lo
# que hace el servicio `seed` de docker-compose.yml.
#
# Esta separada porque `/app/seed` con SEED_RESET=true borra 21 tablas
# -titulares, obras, declaraciones, bolsas, usuarios...- y reescribe `usuarios`
# con una clave publicada en docs/ARRANQUE.md. Un binario asi no viaja en la
# misma imagen que atiende produccion: la unica forma de ejecutarlo tiene que
# ser desplegar a proposito una imagen distinta. La guarda del propio comando
# (ver semilla.vaciar) es la segunda mitad, no la primera.
FROM gcr.io/distroless/static-debian12:nonroot AS seed
WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/objetos /objetos
COPY --from=build-seed /out-seed/seed /app/seed
USER nonroot
ENTRYPOINT ["/app/seed"]

# Etapa FINAL, y por eso la imagen por defecto: un `docker build` sin --target
# construye esta. Es la que publica CI y la que despliega el CD.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=build /out/api       /app/api
COPY --from=build /out/worker    /app/worker
COPY --from=build /out/scheduler /app/scheduler
# Las migraciones van EMBEBIDAS en este binario (migrations/embed.go), asi que
# no hay que copiar el directorio ni acertar con la variable MIGRATIONS.
COPY --from=build /out/migrate   /app/migrate
COPY --from=build --chown=nonroot:nonroot /out/objetos /objetos
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
