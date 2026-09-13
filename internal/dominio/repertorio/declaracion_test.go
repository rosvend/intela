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

// El caso mas grave de precision: dos partes que suman un poco MENOS de 100
// -declaracion_incompleta valida, R-04, nada se reparte- pero que redondean
// cada una por separado a un valor que SI suma exactamente 100 en
// NUMERIC(8,4). Sin el chequeo de precision, esto pasaba NuevaDeclaracion
// como incompleta (49.99996 + 50.00003 = 99.99999) y llegaba a guardarse tal
// cual: Postgres redondea 49.99996 a 50.0000 (el quinto decimal es 6) y
// 50.00003 a 50.0000 (el quinto decimal es 3), y una obra que el dominio
// nunca valido como completa queda leyendose "completa" para el motor de
// reparto -exactamente lo que R-04 prohibe: repartir sobre una declaracion
// que no llego a sumar 100 tal como se declaro.
func TestNuevaDeclaracionRechazaPrecisionQuePodriaRedondearAIncompletaOCompleta(t *testing.T) {
	_, err := NuevaDeclaracion("obra-1", []Parte{
		{TitularID: "a", IPI: "1", Porcentaje: decimal.RequireFromString("49.99996")},
		{TitularID: "b", IPI: "2", Porcentaje: decimal.RequireFromString("50.00003")},
	})
	if !errors.Is(err, ErrDeclaracionInvalida) {
		t.Fatalf("err = %v, se esperaba ErrDeclaracionInvalida: estas partes solo son 'incompletas' hasta que"+
			" Postgres las redondea, y para entonces ya se guardaron", err)
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
		{
			// Roy, revisando la #106: un porcentaje positivo por debajo de la
			// mitad del ultimo decimal pasaba esta validacion (es > 0 en la
			// precision arbitraria de Go) y NUMERIC(8,4) lo redondeaba a 0.0000
			// al escribir, reventando el CHECK de la tabla con un error
			// generico en vez de ErrDeclaracionInvalida.
			nombre: "porcentaje que redondearia a cero en NUMERIC(8,4) se rechaza",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.RequireFromString("0.00004")},
			},
			quiereErr: true,
		},
		{
			nombre: "porcentaje con mas de 4 decimales se rechaza aunque la suma de exacto 100",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.RequireFromString("33.33335")},
				{TitularID: "b", IPI: "2", Porcentaje: decimal.RequireFromString("66.66665")},
			},
			quiereErr: true,
		},
		{
			nombre: "exactamente 4 decimales es valido -- no es una falsa alarma de precision",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.RequireFromString("33.3333")},
			},
		},
		{
			nombre: "ceros de mas alla del cuarto decimal no cuentan como precision de mas",
			obraID: "obra-1",
			partes: []Parte{
				{TitularID: "a", IPI: "1", Porcentaje: decimal.RequireFromString("60.000000")},
			},
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
