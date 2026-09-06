package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func mudo() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Las ordenes destructivas se rechazan ANTES de conectar.
//
// Esta funcion vive dentro de la VPC y su nombre esta publicado en los outputs
// de Terraform. Quien tenga lambda:InvokeFunction puede llamarla; sin esta
// comprobacion, `{"orden":"reset"}` tira el esquema entero sin pasar por
// Terraform y sin que la guarda de destruccion vea nada.
//
// Que no haya base levantada en esta prueba ES la prueba: si el rechazo llegara
// tarde, Aplicar intentaria conectar y el error hablaria de la conexion, no de
// la orden.
func TestOrdenesDestructivasRechazadas(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	for _, orden := range []string{"reset", "down", "down-to", "redo", "create", "fix"} {
		t.Run(orden, func(t *testing.T) {
			_, err := atender(mudo())(t.Context(), peticion{Orden: orden})
			if err == nil {
				t.Fatalf("la orden %q tiene que rechazarse", orden)
			}
			if !strings.Contains(err.Error(), "no permitida") {
				t.Errorf("tiene que rechazarse por la lista, no por otra cosa: %v", err)
			}
		})
	}
}

// Las permitidas pasan el filtro y llegan a intentar conectar. El DSN apunta a
// un puerto donde no hay nadie, asi que el error esperado es de conexion: eso
// demuestra que la orden se acepto y que el fallo esta mas abajo.
func TestOrdenesPermitidasPasanElFiltro(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	for _, orden := range ordenesPermitidas {
		t.Run(orden, func(t *testing.T) {
			_, err := atender(mudo())(t.Context(), peticion{Orden: orden})
			if err != nil && strings.Contains(err.Error(), "no permitida") {
				t.Fatalf("la orden %q esta en la lista y aun asi se rechazo: %v", orden, err)
			}
		})
	}
}

// Terraform manda {"orden":"up"}, pero una invocacion sin cuerpo tiene que
// seguir aplicando el esquema y no rechazarse por vacia.
func TestOrdenVaciaUsaLaPorDefecto(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	_, err := atender(mudo())(t.Context(), peticion{Orden: ""})
	if err != nil && strings.Contains(err.Error(), "no permitida") {
		t.Fatalf("la orden por defecto tiene que estar permitida: %v", err)
	}
}
