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
	svc := &aplicacion.Servicio{Repo: store, Reloj: postgres.RelojReal{}, Obj: postgres.Disco{Dir: getenv("OBJECT_DIR", "/tmp/intela-objetos")}, Notif: postgres.NotifLog{}, Sim: postgres.SimLocal{S: store}}
	log.Println("worker")
	for {
		if err := svc.ProcesarCola(ctx); err != nil {
			log.Println("cola", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
