package reparto

import (
	"testing"

	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/shopspring/decimal"
)

func snapDemo() Snapshot {
	return Snapshot{
		AdminPct: decimal.Zero, SocialPct: decimal.Zero, ReservaPct: decimal.Zero,
		PondCine: decimal.NewFromFloat(5), PondUnitario: decimal.NewFromFloat(2.8),
		PondSerie: decimal.NewFromFloat(1.3), PondSketch: decimal.NewFromFloat(0.8),
		Wa: decimal.NewFromFloat(0.5), Wb: decimal.NewFromFloat(0.3), Wc: decimal.NewFromFloat(0.2),
		Reglamento: "RD-IX-seed",
	}
}

func TestCanalZReglamento(t *testing.T) {
	bolsa := Bolsa{UsuarioID: "canal-z", Periodo: "2025", Circuito: "nacional", Bruto: decimal.NewFromInt(1_000_000)}
	usos := []Uso{
		{ObraID: "pelicula-x", Modalidad: TV, TipoObra: "cinematografica", DuracionMin: decimal.NewFromInt(70), Emisiones: 1, Rating: decimal.NewFromFloat(4.5)},
		{ObraID: "serie-y", Modalidad: TV, TipoObra: "serie", DuracionMin: decimal.NewFromInt(48), Emisiones: 10, Rating: decimal.NewFromFloat(9)},
	}
	decs := map[string]repertorio.Declaracion{
		"pelicula-x": {ObraID: "pelicula-x", Partes: []repertorio.Parte{{TitularID: "ana", IPI: "I-1", Porcentaje: decimal.NewFromInt(100)}}},
		"serie-y":    {ObraID: "serie-y", Partes: []repertorio.Parte{{TitularID: "bruno", IPI: "I-2", Porcentaje: decimal.NewFromInt(100)}}},
	}
	r := Repartir(bolsa, usos, decs, snapDemo())
	if !r.Obras[0].Puntos.Equal(decimal.NewFromInt(1575)) {
		t.Fatalf("puntos pelicula %s", r.Obras[0].Puntos)
	}
	if !r.Obras[1].Puntos.Equal(decimal.NewFromInt(5616)) {
		t.Fatalf("puntos serie %s", r.Obras[1].Puntos)
	}
	suma := r.Obras[0].Importe.Add(r.Obras[1].Importe)
	if !suma.Equal(decimal.NewFromInt(1_000_000)) {
		t.Fatalf("residuo no cerrado: %s", suma)
	}
}

func TestDeclaracionIncompletaRetieneTotal(t *testing.T) {
	bolsa := Bolsa{Circuito: "nacional", Bruto: decimal.NewFromInt(1000)}
	usos := []Uso{{ObraID: "z", Modalidad: Cine, Taquilla: decimal.NewFromInt(10)}}
	decs := map[string]repertorio.Declaracion{
		"z": {ObraID: "z", Partes: []repertorio.Parte{{TitularID: "ana", IPI: "I-1", Porcentaje: decimal.NewFromInt(40)}}},
	}
	r := Repartir(bolsa, usos, decs, snapDemo())
	if !r.Obras[0].Retenida || len(r.Titulares) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestReproducible(t *testing.T) {
	bolsa := Bolsa{Circuito: "nacional", Bruto: decimal.NewFromInt(999)}
	usos := []Uso{
		{ObraID: "a", Modalidad: Cine, Taquilla: decimal.NewFromInt(1)},
		{ObraID: "b", Modalidad: Cine, Taquilla: decimal.NewFromInt(2)},
	}
	decs := map[string]repertorio.Declaracion{
		"a": {Partes: []repertorio.Parte{{TitularID: "t", IPI: "1", Porcentaje: decimal.NewFromInt(100)}}},
		"b": {Partes: []repertorio.Parte{{TitularID: "t", IPI: "1", Porcentaje: decimal.NewFromInt(100)}}},
	}
	r1 := Repartir(bolsa, usos, decs, snapDemo())
	r2 := Repartir(bolsa, usos, decs, snapDemo())
	if !r1.Obras[0].Importe.Equal(r2.Obras[0].Importe) || !r1.Obras[1].Importe.Equal(r2.Obras[1].Importe) {
		t.Fatal("no reproducible")
	}
}
