package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/httpapi"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	ctx := context.Background()
	dsn := getenv("DATABASE_URL", "postgres://intela:intela@localhost:5432/intela?sslmode=disable")
	store, err := postgres.Conectar(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Cerrar()
	mig, err := os.ReadFile(getenv("MIGRATIONS", "migrations/00001_init.sql"))
	if err != nil {
		log.Fatal(err)
	}
	if err := store.Migrar(ctx, string(mig)); err != nil {
		log.Fatal("migrar: ", err)
	}
	hAdmin, _ := bcrypt.GenerateFromPassword([]byte("intela"), 10)
	hTit, _ := bcrypt.GenerateFromPassword([]byte("intela"), 10)
	hAud, _ := bcrypt.GenerateFromPassword([]byte("intela"), 10)
	if err := store.SembrarSiVacio(ctx, string(hAdmin), string(hTit), string(hAud)); err != nil {
		log.Fatal("seed: ", err)
	}
	svc := &aplicacion.Servicio{
		Repo: store,
		Reloj: postgres.RelojReal{},
		Obj: postgres.Disco{Dir: getenv("OBJECT_DIR", "/tmp/intela-objetos")},
		Notif: postgres.NotifLog{},
		Sim: postgres.SimLocal{S: store},
	}
	api := &httpapi.API{Svc: svc, Repo: store}
	addr := getenv("HTTP_ADDR", ":8080")
	srv := &http.Server{Addr: addr, Handler: api.Router(), ReadHeaderTimeout: 10 * time.Second}
	log.Println("api", addr)
	log.Fatal(srv.ListenAndServe())
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
