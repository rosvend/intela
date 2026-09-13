package repertorio

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestCompletaSoloConCienEIPI(t *testing.T) {
	d := Declaracion{Partes: []Parte{{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(60)}, {TitularID: "b", IPI: "2", Porcentaje: decimal.NewFromInt(40)}}}
	if !d.Completa() {
		t.Fatal("deberia ser completa")
	}
	d.Partes[1].Porcentaje = decimal.NewFromInt(30)
	if d.Completa() {
		t.Fatal("40+30 no es 100")
	}
}

// Regresion: la suma cuadraba pero las partes eran imposibles. 150 y -50 dan
// 100, y sin validar cada parte la declaracion pasaba por "completa".
func TestPartesNegativasNoSonCompletas(t *testing.T) {
	d := Declaracion{Partes: []Parte{
		{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(150)},
		{TitularID: "b", IPI: "2", Porcentaje: decimal.NewFromInt(-50)},
	}}
	if d.Completa() {
		t.Fatal("150 y -50 suman 100 pero ninguna declaracion valida tiene una parte negativa")
	}
	if got := d.Estado(); got != "incompleta" {
		t.Fatalf("Estado() = %q, se esperaba \"incompleta\"", got)
	}

	// Una parte en cero tampoco: un titular al 0% no es titular.
	d.Partes = []Parte{
		{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(100)},
		{TitularID: "b", IPI: "2", Porcentaje: decimal.Zero},
	}
	if d.Completa() {
		t.Fatal("una parte al 0% no puede contar como declarada")
	}
}

func TestNuevaDeclaracion(t *testing.T) {
	cien := func(id string) []Parte {
		return []Parte{{TitularID: id, IPI: "IPI-" + id, Porcentaje: decimal.NewFromInt(100)}}
	}

	casos := []struct {
		nombre    string
		obraID    string
		partes    []Parte
		quiereErr bool
	}{
		{
			nombre: "suma exacta 100 es valida",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(60)},
				{TitularID: "b", IPI: "2", Porcentaje: decimal.NewFromInt(40)},
			},
		},
		{
			nombre: "suma menor a 100 es valida -- es la declaracion_incompleta de R-04, no un rechazo",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(60)},
			},
		},
		{
			nombre:    "falta el id de la obra",
			obraID:    "",
			partes:    cien("a"),
			quiereErr: true,
		},
		{
			nombre:    "sin partes",
			obraID:    "obra-1",
			partes:    nil,
			quiereErr: true,
		},
		{
			nombre: "suma mayor a 100 se rechaza -- no tiene lectura de negocio",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(60)},
				{TitularID: "b", IPI: "2", Porcentaje: decimal.NewFromInt(60)},
			},
			quiereErr: true,
		},
		{
			nombre: "parte sin IPI se rechaza",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "", Porcentaje: decimal.NewFromInt(100)},
			},
			quiereErr: true,
		},
		{
			nombre: "porcentaje en cero se rechaza",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.Zero},
			},
			quiereErr: true,
		},
		{
			nombre: "porcentaje negativo se rechaza",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(-10)},
			},
			quiereErr: true,
		},
		{
			nombre: "titular duplicado se rechaza",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(50)},
				{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(50)},
			},
			quiereErr: true,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevaDeclaracion(c.obraID, c.partes)
			if c.quiereErr {
				if !errors.Is(err, ErrDeclaracionInvalida) {
					t.Fatalf("err = %v, se esperaba ErrDeclaracionInvalida", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, no se esperaba ninguno", err)
			}
		})
	}
}
