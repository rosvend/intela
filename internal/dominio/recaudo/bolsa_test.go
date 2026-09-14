package recaudo

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func dec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal %q: %v", s, err)
	}
	return d
}

func TestNuevaBolsaAceptaLoQueCabeEnLaColumna(t *testing.T) {
	casos := []struct {
		nombre   string
		usuario  string
		periodo  string
		circuito Circuito
		bruto    string
	}{
		{"periodo de ano y mes", "caracol", "2025-01", Nacional, "1000000.00"},
		{"periodo de ano suelto", "caracol", "2025", Nacional, "1000000.00"},
		{"internacional", "dago-films", "2025-01", Internacional, "200000.00"},
		{"diciembre", "caracol", "2025-12", Nacional, "1000000.00"},
		{"enero", "caracol", "2025-01", Nacional, "1000000.00"},
		// El importe mas grande que cabe en NUMERIC(18,2).
		{"bruto en el limite de la columna", "caracol", "2025-01", Nacional, "9999999999999999.99"},
		{"sin decimales", "netflix", "2025-01", Nacional, "500000"},
		{"un solo decimal", "netflix", "2025-01", Nacional, "500000.5"},
		// Cero es un recaudo valido: un usuario puede no haber pagado nada en
		// el periodo y eso es un hecho, no un dato mal formado.
		{"bruto cero", "netflix", "2025-01", Nacional, "0"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			b, err := NuevaBolsa(c.usuario, c.periodo, c.circuito, dec(t, c.bruto))
			if err != nil {
				t.Fatalf("NuevaBolsa: %v", err)
			}
			if b.UsuarioID != c.usuario || b.Periodo != c.periodo || b.Circuito != c.circuito {
				t.Fatalf("bolsa = %+v, no coincide con la entrada", b)
			}
			if !b.Bruto.Equal(dec(t, c.bruto)) {
				t.Fatalf("bruto = %s, se esperaba %s", b.Bruto, c.bruto)
			}
		})
	}
}

func TestNuevaBolsaRecorta(t *testing.T) {
	b, err := NuevaBolsa("  caracol  ", " 2025-01 ", Nacional, dec(t, "1000000.00"))
	if err != nil {
		t.Fatalf("NuevaBolsa: %v", err)
	}
	if b.UsuarioID != "caracol" || b.Periodo != "2025-01" {
		t.Fatalf("bolsa = %+v, se esperaba sin espacios alrededor", b)
	}
}

func TestNuevaBolsaRechaza(t *testing.T) {
	casos := []struct {
		nombre   string
		usuario  string
		periodo  string
		circuito Circuito
		bruto    string
	}{
		{"sin usuario", "", "2025-01", Nacional, "1000000.00"},
		{"usuario en blanco", "   ", "2025-01", Nacional, "1000000.00"},
		{"sin periodo", "caracol", "", Nacional, "1000000.00"},
		{"periodo con formato libre", "caracol", "enero de 2025", Nacional, "1000000.00"},
		{"periodo con mes de un digito", "caracol", "2025-1", Nacional, "1000000.00"},
		{"periodo con dia", "caracol", "2025-01-15", Nacional, "1000000.00"},
		// Un mes que no existe no es un periodo. Sin esto se puede guardar
		// dinero bajo `2025-13`, que no cuadra con ningun corte del
		// reglamento y que ningun reparto sabe cerrar.
		{"mes cero", "caracol", "2025-00", Nacional, "1000000.00"},
		{"mes trece", "caracol", "2025-13", Nacional, "1000000.00"},
		{"mes noventa y nueve", "caracol", "2025-99", Nacional, "1000000.00"},
		// RD 10.3 / R-35: los circuitos son dos y estan cerrados. Un tercero
		// inventado -"regional", por ejemplo- produciria una bolsa que ningun
		// reparto sabe tratar.
		{"circuito inventado", "caracol", "2025-01", Circuito("regional"), "1000000.00"},
		{"circuito vacio", "caracol", "2025-01", Circuito(""), "1000000.00"},
		{"bruto negativo", "caracol", "2025-01", Nacional, "-1"},
		// La columna es NUMERIC(18,2): un tercer decimal lo redondearia
		// Postgres al escribir, y el dominio habria validado una cifra que la
		// base guarda distinta.
		{"bruto con tres decimales", "caracol", "2025-01", Nacional, "100.555"},
		// NUMERIC(18,2) son 16 digitos enteros. Un importe de 17 pasaba la
		// comprobacion de escala y lo rechazaba Postgres al insertar, con un
		// error de restriccion en vez de "este importe no cabe".
		{"bruto de 17 digitos enteros", "caracol", "2025-01", Nacional, "10000000000000000.00"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevaBolsa(c.usuario, c.periodo, c.circuito, dec(t, c.bruto))
			if !errors.Is(err, ErrBolsaInvalida) {
				t.Fatalf("err = %v, se esperaba ErrBolsaInvalida", err)
			}
		})
	}
}

func TestCircuitosSonLosDosDelReglamento(t *testing.T) {
	// El orden importa: es el mismo del CHECK de la columna `bolsas.circuito`,
	// y el mensaje de error lo cita tal cual.
	quiere := []Circuito{Nacional, Internacional}
	tiene := Circuitos()
	if len(tiene) != len(quiere) {
		t.Fatalf("Circuitos() = %v, se esperaban %v", tiene, quiere)
	}
	for i := range quiere {
		if tiene[i] != quiere[i] {
			t.Fatalf("Circuitos()[%d] = %q, se esperaba %q", i, tiene[i], quiere[i])
		}
	}
}
