package reparto_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Golden de la corrida Canal Z (RD 9.1.1). ADR 0005: la CI reejecuta
// corridas de referencia y falla si el resultado cambia entre versiones.
func TestReparto_GoldenCanalZ(t *testing.T) {
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica", CanalID: "z",
			DuracionMin: d("70"), Emisiones: 1, Rating: d("4.5")},
		{ObraID: "y", Modalidad: reparto.TV, TipoObra: "serie", CanalID: "z",
			DuracionMin: d("48"), Emisiones: 10, Rating: d("9")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	r, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)

	type linea struct {
		ObraID  string `json:"obra_id"`
		Puntos  string `json:"puntos"`
		Importe string `json:"importe"`
	}
	type golden struct {
		Neto       string  `json:"neto"`
		ValorPunto string  `json:"valor_punto"`
		Residuo    string  `json:"residuo"`
		Obras      []linea `json:"obras"`
	}
	got := golden{
		Neto:       r.Neto.StringFixed(2),
		ValorPunto: r.ValorPunto.String(),
		Residuo:    r.Residuo.StringFixed(2),
	}
	for _, o := range r.Obras {
		got.Obras = append(got.Obras, linea{
			ObraID:  o.ObraID,
			Puntos:  o.Puntos.String(),
			Importe: o.Importe.StringFixed(2),
		})
	}

	path := filepath.Join("testdata", "canal_z.json")
	wantRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	gotRaw, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wantPretty, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(gotRaw) != string(wantPretty) {
		t.Fatalf("golden divergio:\n got=%s\nwant=%s", gotRaw, wantPretty)
	}
}
