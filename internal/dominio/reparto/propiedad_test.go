package reparto_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// tiposObraTV son los unicos tipo_obra que ponderacionTipo reconoce para TV
// (RD 9.1.1); un tipo fuera de esta lista es un error tipado, no el punto que
// esta propiedad quiere ejercitar.
var tiposObraTV = []string{"cinematografica", "unitario", "serie", "telenovela", "sketches"}

// partesPorcentaje compone k partes enteras positivas que suman exactamente
// 100, via cortes distintos en [1,99] (k<=3, asi que el sorteo de cortes
// distintos termina casi siempre en la primera vuelta).
func partesPorcentaje(t *rapid.T, k int) []int {
	if k == 1 {
		return []int{100}
	}
	cortes := make(map[int]bool, k-1)
	for len(cortes) < k-1 {
		cortes[rapid.IntRange(1, 99).Draw(t, "corte")] = true
	}
	ordenados := make([]int, 0, k-1)
	for c := range cortes {
		ordenados = append(ordenados, c)
	}
	sort.Ints(ordenados)
	partes := make([]int, 0, k)
	prev := 0
	for _, c := range ordenados {
		partes = append(partes, c-prev)
		prev = c
	}
	return append(partes, 100-prev)
}

// TestReparto_PropiedadCierreYReproducibilidad cubre los dos criterios de
// aceptacion de la issue #33 que ningun test por ejemplo prueba: que lo
// repartido, retenido, deducido, reservado y no distribuido cierra siempre
// exacto contra el bruto de la bolsa (sin perder centavos), y que dos
// corridas con las mismas entradas producen un Resultado byte-identico
// (ADR 0005).
//
// Se limita a la modalidad TV: ya tiene golden (RD 9.1.1) y ejercita el
// pipeline completo (deducciones R-06/R-07, puntos, declaraciones, retencion
// R-04, residuo). Generar tambien las demas modalidades no probaria una
// propiedad distinta, solo repetiria el mismo cierre con otra formula de
// puntos que ya tiene su propio test por ejemplo.
func TestReparto_PropiedadCierreYReproducibilidad(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		snap := snapBase()
		snap.AdminPct = decimal.NewFromInt(int64(rapid.IntRange(1, 200).Draw(t, "adminPct"))).Div(decimal.NewFromInt(10))
		snap.SocialPct = decimal.NewFromInt(int64(rapid.IntRange(1, 100).Draw(t, "socialPct"))).Div(decimal.NewFromInt(10))
		snap.ReservaPct = decimal.NewFromInt(int64(rapid.IntRange(1, 50).Draw(t, "reservaPct"))).Div(decimal.NewFromInt(10))

		circuito := recaudo.Nacional
		if rapid.Bool().Draw(t, "internacional") {
			circuito = recaudo.Internacional
		}
		centavos := rapid.Int64Range(100, 100_000_000).Draw(t, "brutoCentavos")
		bruto := decimal.NewFromInt(centavos).Div(decimal.NewFromInt(100))
		b, err := recaudo.NuevaBolsa("u1", "2025", circuito, bruto)
		if err != nil {
			t.Fatalf("bolsa invalida: %v", err)
		}

		numObras := rapid.IntRange(1, 6).Draw(t, "numObras")
		usos := make([]reparto.Uso, 0, numObras)
		var decls []repertorio.Declaracion
		for i := 0; i < numObras; i++ {
			obraID := fmt.Sprintf("obra-%d", i)
			tipo := rapid.SampledFrom(tiposObraTV).Draw(t, "tipoObra")
			duracion := rapid.IntRange(1, 500).Draw(t, "duracionMin")
			ratingDecimo := rapid.IntRange(1, 200).Draw(t, "ratingDecimo")
			emisiones := rapid.IntRange(1, 50).Draw(t, "emisiones")
			usos = append(usos, reparto.Uso{
				ObraID:      obraID,
				Modalidad:   reparto.TV,
				TipoObra:    tipo,
				DuracionMin: decimal.NewFromInt(int64(duracion)),
				Rating:      decimal.NewFromInt(int64(ratingDecimo)).Div(decimal.NewFromInt(10)),
				Emisiones:   int64(emisiones),
			})

			if rapid.Bool().Draw(t, "declaracionCompleta") {
				k := rapid.IntRange(1, 3).Draw(t, "numPartes")
				partes := make([]repertorio.Parte, 0, k)
				for j, pct := range partesPorcentaje(t, k) {
					partes = append(partes, repertorio.Parte{
						TitularID:  fmt.Sprintf("t-%d-%d", i, j),
						IPI:        fmt.Sprintf("IPI-%d-%d", i, j),
						Porcentaje: decimal.NewFromInt(int64(pct)),
					})
				}
				decl, err := repertorio.NuevaDeclaracion(obraID, partes)
				if err != nil {
					t.Fatalf("declaracion invalida: %v", err)
				}
				decls = append(decls, decl)
			}
			// Sin `if`: la obra queda sin declaracion, y aplicarDeclaraciones la
			// retiene por R-04 -- el otro camino que esta propiedad cruza.
		}

		r1, err := reparto.Reparto(b, usos, snap, decls, reparto.Opciones{})
		if err != nil {
			t.Fatalf("reparto: %v", err)
		}

		sumaTitulares := decimal.Zero
		for _, tt := range r1.Titulares {
			sumaTitulares = sumaTitulares.Add(tt.Importe)
		}
		cierre := r1.Admin.Add(r1.Social).Add(r1.Reserva).Add(r1.Retenido).
			Add(sumaTitulares).Add(r1.Residuo).Add(r1.NoDistribuido)
		if !cierre.Equal(bruto) {
			t.Fatalf("cierre: admin(%s)+social(%s)+reserva(%s)+retenido(%s)+titulares(%s)+residuo(%s)+noDist(%s) = %s != bruto %s",
				r1.Admin, r1.Social, r1.Reserva, r1.Retenido, sumaTitulares, r1.Residuo, r1.NoDistribuido, cierre, bruto)
		}

		r2, err := reparto.Reparto(b, usos, snap, decls, reparto.Opciones{})
		if err != nil {
			t.Fatalf("segunda corrida: %v", err)
		}
		j1, err := json.Marshal(r1)
		if err != nil {
			t.Fatalf("marshal r1: %v", err)
		}
		j2, err := json.Marshal(r2)
		if err != nil {
			t.Fatalf("marshal r2: %v", err)
		}
		if !bytes.Equal(j1, j2) {
			t.Fatalf("dos corridas con las mismas entradas no son byte-identicas:\n%s\n!=\n%s", j1, j2)
		}
	})
}
