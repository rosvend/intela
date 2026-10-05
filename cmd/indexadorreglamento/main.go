// Command indexadorreglamento embebe docs/reglamentos/**/*.md por numeral y reemplaza el indice de buscar_reglamento (#67).
//
//	DATABASE_URL=... EMBEDDINGS_PROVEEDOR=falso|bedrock go run ./cmd/indexadorreglamento [-dir docs/reglamentos]
//
// Se corre a mano (make indexar-reglamento) cuando cambian los .md; nunca al arrancar cmd/api.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/embeddings"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/reglamentos"
)

func main() {
	log := config.Logger("indexadorreglamento")
	if err := ejecutar(log); err != nil {
		log.Error("indexacion fallida", slog.Any("error", err))
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	dir := flag.String("dir", "docs/reglamentos", "carpeta con los reglamentos en Markdown")
	flag.Parse()

	ctx, cancelar := context.WithTimeout(context.Background(), config.Duracion("INDEXADOR_TIMEOUT", 10*time.Minute))
	defer cancelar()

	e, err := embeddings.Elegir(ctx, config.Cadena("EMBEDDINGS_PROVEEDOR", ""), config.Cadena("EMBEDDINGS_MODELO", ""))
	if err != nil {
		return err
	}
	if e.Motor == nil {
		return errors.New("EMBEDDINGS_PROVEEDOR vacio: sin motor no hay indice")
	}
	secciones, err := reglamentos.Leer(os.DirFS(*dir))
	if err != nil {
		return err
	}
	store, err := postgres.Abrir(ctx, config.Cadena("DATABASE_URL", ""))
	if err != nil {
		return err
	}
	defer store.CerrarPool()

	n, err := aplicacion.IndexarReglamento{Motor: e.Motor, Almacen: store}.Indexar(ctx, secciones)
	if err != nil {
		return err
	}
	log.Info("reglamento indexado", slog.Int("secciones", n), slog.String("proveedor", e.Proveedor))
	return nil
}
