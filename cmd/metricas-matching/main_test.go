package main

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/rosvend/intela/internal/aplicacion"
)

func TestFechaDeResolucion(t *testing.T) {
	hoy := time.Date(2026, 9, 21, 15, 4, 5, 0, time.UTC)

	t.Run("sin FECHA es hoy", func(t *testing.T) {
		fecha, origen, err := fechaDeResolucion("", hoy)
		if err != nil {
			t.Fatalf("fechaDeResolucion: %v", err)
		}
		if !fecha.Equal(hoy) || origen != "hoy" {
			t.Fatalf("fecha=%s origen=%q, se esperaba hoy", fecha, origen)
		}
	})

	t.Run("con FECHA es ese dia, a medianoche UTC", func(t *testing.T) {
		fecha, origen, err := fechaDeResolucion("2024-06-01", hoy)
		if err != nil {
			t.Fatalf("fechaDeResolucion: %v", err)
		}
		if !fecha.Equal(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)) || origen != "FECHA" {
			t.Fatalf("fecha=%s origen=%q", fecha, origen)
		}
	})

	for _, malo := range []string{"junio", "2024-6-1", "01/06/2024", "2024-13-01", "2024-06"} {
		t.Run("rechaza "+malo, func(t *testing.T) {
			_, _, err := fechaDeResolucion(malo, hoy)
			if err == nil {
				t.Fatalf("FECHA=%q tenia que fallar", malo)
			}
			if !strings.Contains(err.Error(), "FECHA") || !strings.Contains(err.Error(), malo) {
				t.Fatalf("el error no nombra la variable y el valor: %v", err)
			}
		})
	}
}

// Del listado de vigentes solo interesan los dos parametros del escalon 3, en el
// orden en que se imprimen, aunque lleguen mezclados con veintitantos mas.
func TestSoloDelEscalon(t *testing.T) {
	desde := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	filas := []aplicacion.FilaParametro{
		{Clave: "deduccion.administrativa", Valor: "0.200000", VigenteDesde: desde},
		{Clave: aplicacion.ClaveUmbralBanda, Valor: "0.450000", VigenteDesde: desde},
		{Clave: "ott.wa", Valor: "0.500000", VigenteDesde: desde},
		{Clave: aplicacion.ClaveUmbralMatch, Valor: "0.600000", VigenteDesde: desde.AddDate(1, 0, 0)},
	}

	got := soloDelEscalon(filas)
	if len(got) != 2 || got[0].Clave != aplicacion.ClaveUmbralMatch || got[1].Clave != aplicacion.ClaveUmbralBanda {
		t.Fatalf("soloDelEscalon() = %+v, se esperaban umbral y luego banda", got)
	}
	if !got[0].VigenteDesde.Equal(desde.AddDate(1, 0, 0)) {
		t.Fatalf("la vigencia del umbral se perdio: %s", got[0].VigenteDesde)
	}

	if vacio := soloDelEscalon(nil); len(vacio) != 0 {
		t.Fatalf("sin filas no hay nada que imprimir: %+v", vacio)
	}
}

// El conjunto etiquetado tiene que ser ASCII salvo las tildes y signos que los
// casos ponen A PROPOSITO (tildes, `¡...!`). Una letra cirilica o griega que se
// parece a una latina -la "a" cirilica U+0430 que se colo en "errata"- es una
// trampa para editores: se ve igual y no es la misma letra (review de PR #146,
// S10).
//
// Se revisa el texto CRUDO del archivo y no el conjunto decodificado: la nota de
// cada caso, donde estaba la letra, no forma parte de la estructura que lee el
// comando.
func TestElConjuntoEtiquetadoNoTraeLetrasDeOtroAlfabeto(t *testing.T) {
	bruto, err := os.ReadFile("../../data/etiquetado/matching.json")
	if err != nil {
		t.Fatalf("leer el conjunto: %v", err)
	}

	for n, linea := range strings.Split(string(bruto), "\n") {
		for _, r := range linea {
			if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
				t.Errorf("linea %d trae la letra %q (U+%04X), que no es latina: %s",
					n+1, r, r, strings.TrimSpace(linea))
			}
		}
	}
}
