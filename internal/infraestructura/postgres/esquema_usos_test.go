package postgres

import (
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// TestLosCheckDeUsosSostienenElVocabularioDeRD9 comprueba la ULTIMA defensa
// del esquema canonico: los CHECK de `modalidad` y `tipo_obra` que anadio la
// migracion 00011.
//
// Escribe saltandose validarUso a proposito. El caso de uso ya rechaza lo que
// no es del reglamento, y con esa puerta cerrada nadie ejercita el CHECK, que
// es lo unico que queda en pie si manana se escribe en `usos` por otro camino.
//
// Un solo Restore para todas las subpruebas: cada una usa ids distintos y el
// contenedor es compartido por el binario entero.
func TestLosCheckDeUsosSostienenElVocabularioDeRD9(t *testing.T) {
	s, _ := sembrarReportes(t)

	insertar := func(id, modalidad, tipoObra string) error {
		_, err := s.pool.Exec(t.Context(),
			`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad, tipo_obra)
			 VALUES ($1, $2, 'caracol', 'Una obra', $3, $4)`,
			id, reporteEnero, modalidad, tipoObra)
		return err
	}

	// Las cuatro originales mas teatro (RD 9.3), transporte (RD 9.4) y
	// suscripcion (RD 9.5). Hotel sigue porque RD 9.6 remite a 9.5, no porque
	// tenga formula propia.
	t.Run("acepta las siete modalidades del reglamento", func(t *testing.T) {
		modalidades := reparto.Modalidades()
		if len(modalidades) != 7 {
			t.Fatalf("Modalidades() = %d, el reglamento define 7", len(modalidades))
		}
		for _, m := range modalidades {
			if err := insertar("uso-mod-"+string(m), string(m), ""); err != nil {
				t.Errorf("la base rechaza la modalidad %q del reglamento: %v", m, err)
			}
		}
	})

	t.Run("rechaza una octava modalidad", func(t *testing.T) {
		err := insertar("uso-radio", "radio", "")
		if err == nil {
			t.Fatal("la base acepto una modalidad que el reglamento no define")
		}
		if !strings.Contains(err.Error(), "usos_modalidad_check") {
			t.Errorf("el rechazo no viene del CHECK de modalidad: %v", err)
		}
	})

	// La tabla de ponderacion de RD 9.1.1 esta indexada por estas categorias.
	// La cadena vacia es legal: es el "todavia sin mapear" de P-05, y falla
	// ruidosamente en el motor, no aqui.
	t.Run("acepta las categorias de la ponderacion", func(t *testing.T) {
		for _, tipo := range []string{"", "cinematografica", "unitario", "serie", "telenovela", "sketches"} {
			if err := insertar("uso-tipo-"+tipo, "tv", tipo); err != nil {
				t.Errorf("la base rechaza el tipo %q de RD 9.1.1: %v", tipo, err)
			}
		}
	})

	// "magazine" es un SubGenero real de la parrilla de Caracol, y no es una
	// categoria del reglamento. Antes del CHECK sumaba cero puntos en
	// silencio, que es una obra sin pagar.
	t.Run("rechaza un genero de parrilla", func(t *testing.T) {
		err := insertar("uso-magazine", "tv", "magazine")
		if err == nil {
			t.Fatal("la base acepto un tipo_obra que la ponderacion de RD 9.1.1 no conoce")
		}
		if !strings.Contains(err.Error(), "usos_tipo_obra_check") {
			t.Errorf("el rechazo no viene del CHECK de tipo_obra: %v", err)
		}
	})

	// espectadores (RD 9.2) y exhibiciones (RD 9.4) con la misma forma no
	// negativa que el resto de las medidas.
	for _, columna := range []string{"espectadores", "exhibiciones"} {
		t.Run(columna+" no admite negativos", func(t *testing.T) {
			_, err := s.pool.Exec(t.Context(),
				`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad, `+columna+`)
				 VALUES ($1, $2, 'caracol', 'Una obra', 'tv', -1)`,
				"uso-neg-"+columna, reporteEnero)
			if err == nil {
				t.Fatalf("la base acepto %s = -1", columna)
			}
			if !strings.Contains(err.Error(), columna) {
				t.Errorf("el rechazo no menciona la columna: %v", err)
			}
		})
	}
}
