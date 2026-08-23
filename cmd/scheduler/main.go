package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

func main() {
	ctx := context.Background()
	store, err := postgres.Conectar(ctx, getenv("DATABASE_URL", "postgres://intela:intela@localhost:5432/intela?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Cerrar()
	svc := &aplicacion.Servicio{Repo: store, Reloj: postgres.RelojReal{}}
	log.Println("scheduler")
	for {
		if err := svc.AbrirDesdeCalendario(ctx); err != nil {
			log.Println("calendario", err)
		}
		time.Sleep(30 * time.Second)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
