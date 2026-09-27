package reparto

import (
	"testing"

	"github.com/shopspring/decimal"
)

// El orden "excluir despues" no es API de produccion: solo se ejercita aqui
// via export_test.go para documentar que no es equivalente a excluir antes.
func TestRepartirSuscripcion_ExclusionDespuesNoEsEquivalente(t *testing.T) {
	snap := Snapshot{
		GrupoPrivadosPct:   decimal.RequireFromString("50"),
		GrupoRegionalesPct: decimal.RequireFromString("20"),
		GrupoPremiumPct:    decimal.RequireFromString("10"),
		GrupoLideresPct:    decimal.RequireFromString("10"),
		GrupoEstandarPct:   decimal.RequireFromString("10"),
		PondUnitario:       decimal.RequireFromString("2.8"),
	}
	usos := []Uso{
		{
			ObraID: "buena", Modalidad: Suscripcion, TipoObra: "unitario",
			Grupo: GrupoPrivadosNacionales, DuracionMin: decimal.RequireFromString("1"),
			Emisiones: 1, Rating: decimal.RequireFromString("1"),
		},
		{
			ObraID: "fuera", Modalidad: Suscripcion, TipoObra: "unitario",
			Grupo: GrupoPrivadosNacionales, DuracionMin: decimal.RequireFromString("1"),
			Emisiones: 1, Rating: decimal.RequireFromString("1"),
			FueraDeRepertorio: true,
		},
	}
	neto := decimal.RequireFromString("1000")

	antes, _, _, _, _, err := repartirSuscripcion(neto, usos, snap)
	if err != nil {
		t.Fatal(err)
	}
	despues, _, _, noDist, partes, err := RepartirSuscripcionDespuesExclusion(neto, usos, snap)
	if err != nil {
		t.Fatal(err)
	}

	importe := func(oo []LineaObra, id string) decimal.Decimal {
		for _, o := range oo {
			if o.ObraID == id {
				return o.Importe
			}
		}
		return decimal.Zero
	}
	if !importe(antes, "buena").Equal(decimal.RequireFromString("500.00")) {
		t.Fatalf("antes buena=%s", importe(antes, "buena"))
	}
	if importe(despues, "buena").GreaterThanOrEqual(importe(antes, "buena")) {
		t.Fatalf("despues deberia dar menos a buena: antes=%s despues=%s",
			importe(antes, "buena"), importe(despues, "buena"))
	}
	if noDist.IsZero() {
		t.Fatal("despues: la parte de fuera debio ir a NoDistribuido")
	}
	found := false
	for _, p := range partes {
		if p.Motivo == MotivoExclusionR27 && p.ObraID == "fuera" {
			found = true
		}
	}
	if !found {
		t.Fatalf("faltaba parte exclusion_r27: %+v", partes)
	}
}
