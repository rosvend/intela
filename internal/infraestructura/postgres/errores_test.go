package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rosvend/intela/internal/aplicacion"
)

// La razon de ser de esta tabla esta en internal/aplicacion/errores.go: "no
// encontrado" y "fallo la base de datos" no son lo mismo, y tratarlos igual
// tiene consecuencias caras. Un timeout de red confundido con "no hay fila"
// reclasifica un uso como ONI en silencio.
func TestTraducirError(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", Message: "duplicate key"}

	casos := []struct {
		nombre       string
		entrada      error
		nulo         bool
		noEncontrado bool
	}{
		{
			nombre:  "nil no es un error",
			entrada: nil,
			nulo:    true,
		},
		{
			nombre:       "sin filas es no encontrado",
			entrada:      pgx.ErrNoRows,
			noEncontrado: true,
		},
		{
			// Por esto la comprobacion es errors.Is y no ==: pgx.CollectOneRow
			// y algunas rutas de Row.Scan devuelven ErrNoRows ya envuelto.
			nombre:       "sin filas envuelto sigue siendo no encontrado",
			entrada:      fmt.Errorf("collect: %w", pgx.ErrNoRows),
			noEncontrado: true,
		},
		{
			// El caso caro. Un fallo transitorio NO puede pasar por "no hay fila".
			nombre:       "un timeout no es no encontrado",
			entrada:      context.DeadlineExceeded,
			noEncontrado: false,
		},
		{
			nombre:       "una violacion de constraint no es no encontrado",
			entrada:      pgErr,
			noEncontrado: false,
		},
		{
			nombre:       "cualquier otro error conserva su causa",
			entrada:      errors.New("la conexion se cayo"),
			noEncontrado: false,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := traducirError(c.entrada, "consulta %q", "x")

			if c.nulo {
				if got != nil {
					t.Fatalf("se esperaba nil, se obtuvo %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("se esperaba un error, se obtuvo nil")
			}
			if es := errors.Is(got, aplicacion.ErrNoEncontrado); es != c.noEncontrado {
				t.Fatalf("errors.Is(_, ErrNoEncontrado) = %v, se esperaba %v (error: %v)",
					es, c.noEncontrado, got)
			}
			// La causa original nunca se pierde: sin esto, diagnosticar un
			// fallo de red desde un log es imposible.
			if !c.noEncontrado && !errors.Is(got, c.entrada) {
				t.Fatalf("se perdio la causa original %v en %v", c.entrada, got)
			}
		})
	}

	// errors.As tiene que seguir recuperando el *pgconn.PgError con su codigo:
	// el adaptador de escritura de #18 distingue un 23505 de un fallo real.
	t.Run("errors.As recupera el PgError", func(t *testing.T) {
		got := traducirError(pgErr, "guardar reporte")
		var recuperado *pgconn.PgError
		if !errors.As(got, &recuperado) {
			t.Fatalf("errors.As no recupero el PgError de %v", got)
		}
		if recuperado.Code != "23505" {
			t.Fatalf("Code = %q, se esperaba \"23505\"", recuperado.Code)
		}
	})

	// Sin esta prueba, refactorizar a `return aplicacion.ErrNoEncontrado` pelado
	// pasa todos los casos de arriba y borra en silencio la unica pista de cual
	// de las seis consultas fallo.
	t.Run("el mensaje conserva el contexto", func(t *testing.T) {
		got := traducirError(pgx.ErrNoRows, "usuario por email %q", "ana@redes.co")
		if !strings.Contains(got.Error(), `usuario por email "ana@redes.co"`) {
			t.Fatalf("el mensaje no lleva el contexto formateado: %q", got.Error())
		}
	})
}
