package anticipos

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// smmlvPrueba es un SMMLV sintetico, no el decretado: 5 SMMLV = 6.500.000.
const smmlvPrueba = "1300000"

// situacionSocio es un socio sin embargos ni anticipos previos, con 4.000.000
// de derechos liquidados en la ventana: el 25% (1.000.000) es menor que 5
// SMMLV (6.500.000), asi que el tope es 1.000.000.
func situacionSocio() Situacion {
	return Situacion{
		Clase:    afiliacion.ClaseSocio,
		Derechos: dec("4000000"),
		SMMLV:    dec(smmlvPrueba),
	}
}

func solicitudDe(monto string) Solicitud {
	return Solicitud{
		ID:            "ant-1",
		TitularID:     "tit-ana",
		Monto:         dec(monto),
		Justificacion: "tratamiento medico no cubierto por la EPS (RA 2.3.b)",
	}
}

func previo(id string, estado Estado, saldo string) Anticipo {
	return Anticipo{
		ID: id, TitularID: "tit-ana", Monto: dec("500000"), Saldo: dec(saldo), Estado: estado,
		Justificacion: "anterior", Soporte: "acta 1",
	}
}

// TestSolicitar recorre cada regla de elegibilidad por separado (R-30, R-31,
// R-32) sobre la misma situacion base, de modo que un caso solo puede fallar
// por la regla que nombra.
func TestSolicitar(t *testing.T) {
	casos := []struct {
		nombre    string
		solicitud Solicitud
		ajustar   func(*Situacion)
		quiero    error
	}{
		{nombre: "un socio dentro del tope solicita", solicitud: solicitudDe("1000000")},

		// R-30 / RA 2.2.
		{
			nombre: "un titular administrado no puede pedir anticipo", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.Clase = afiliacion.ClaseAdministrado },
			quiero:  ErrNoEsSocio,
		},
		{
			nombre: "una clase vacia no es socio", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.Clase = "" },
			quiero:  ErrNoEsSocio,
		},

		// R-31 / RA 3.1.g: el 25% manda cuando es menor.
		{nombre: "el 25 por ciento exacto cabe", solicitud: solicitudDe("1000000")},
		{nombre: "un centavo sobre el 25 por ciento no cabe", solicitud: solicitudDe("1000000.01"), quiero: ErrSuperaTope},

		// R-31 / RA 3.1.h: con derechos altos manda el tope de 5 SMMLV.
		{
			nombre: "5 smmlv exactos caben aunque el 25 por ciento sea mayor", solicitud: solicitudDe("6500000"),
			ajustar: func(s *Situacion) { s.Derechos = dec("40000000") },
		},
		{
			nombre: "un centavo sobre 5 smmlv no cabe aunque el 25 por ciento si", solicitud: solicitudDe("6500000.01"),
			ajustar: func(s *Situacion) { s.Derechos = dec("40000000") },
			quiero:  ErrSuperaTope,
		},
		{
			nombre: "sin derechos en la ventana no hay cupo", solicitud: solicitudDe("1"),
			ajustar: func(s *Situacion) { s.Derechos = decimal.Zero },
			quiero:  ErrSuperaTope,
		},

		// R-32 / RA 3.1.c, 3.1.d, 3.1.e.
		{
			nombre: "con una solicitud en tramite no se pide otra", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.Anticipos = []Anticipo{previo("ant-0", EstadoSolicitado, "0")} },
			quiero:  ErrAnticipoVigente,
		},
		{
			nombre: "con un anticipo aprobado con saldo no se pide otro", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.Anticipos = []Anticipo{previo("ant-0", EstadoAprobado, "1")} },
			quiero:  ErrSaldoPendiente,
		},
		{
			nombre: "con embargo reportado no se pide anticipo", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.EmbargoReportado = true },
			quiero:  ErrEmbargoReportado,
		},
		{
			nombre: "un anticipo ya descontado no impide pedir otro", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.Anticipos = []Anticipo{previo("ant-0", EstadoDescontado, "0")} },
		},
		{
			nombre: "un anticipo rechazado no impide pedir otro", solicitud: solicitudDe("1000"),
			ajustar: func(s *Situacion) { s.Anticipos = []Anticipo{previo("ant-0", EstadoRechazado, "0")} },
		},

		// La solicitud misma.
		{nombre: "monto cero", solicitud: solicitudDe("0"), quiero: ErrSolicitudInvalida},
		{nombre: "monto negativo", solicitud: solicitudDe("-10"), quiero: ErrSolicitudInvalida},
		{nombre: "monto con tres decimales", solicitud: solicitudDe("100.005"), quiero: ErrSolicitudInvalida},
		{
			nombre: "sin justificacion (RA 3.2.1.d y e)", quiero: ErrSolicitudInvalida,
			solicitud: Solicitud{ID: "ant-1", TitularID: "tit-ana", Monto: dec("1000"), Justificacion: "  "},
		},
		{
			nombre: "sin titular", quiero: ErrSolicitudInvalida,
			solicitud: Solicitud{ID: "ant-1", Monto: dec("1000"), Justificacion: "x"},
		},
		{
			nombre: "sin id", quiero: ErrSolicitudInvalida,
			solicitud: Solicitud{TitularID: "tit-ana", Monto: dec("1000"), Justificacion: "x"},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := situacionSocio()
			if c.ajustar != nil {
				c.ajustar(&s)
			}
			a, err := Solicitar(c.solicitud, s)
			if c.quiero != nil {
				if !errors.Is(err, c.quiero) {
					t.Fatalf("err = %v, se esperaba %v", err, c.quiero)
				}
				return
			}
			if err != nil {
				t.Fatalf("Solicitar: %v", err)
			}
			if a.Estado != EstadoSolicitado {
				t.Errorf("estado = %q, se esperaba solicitado", a.Estado)
			}
			if !a.Monto.Equal(c.solicitud.Monto) {
				t.Errorf("monto = %s, se esperaba %s", a.Monto, c.solicitud.Monto)
			}
			// Pedir no es recibir: el saldo nace en la aprobacion (RA 3.1.b).
			if !a.Saldo.IsZero() {
				t.Errorf("saldo = %s, una solicitud no debe nada todavia", a.Saldo)
			}
			if a.TitularID != c.solicitud.TitularID || a.ID != c.solicitud.ID {
				t.Errorf("identidad = %q/%q", a.ID, a.TitularID)
			}
		})
	}
}

// TestSolicitarElMensajeNombraElTope es lo que lee el socio cuando se pasa: sin
// la cifra, "supera el tope" no le dice cuanto puede pedir.
func TestSolicitarElMensajeNombraElTope(t *testing.T) {
	_, err := Solicitar(solicitudDe("2000000"), situacionSocio())
	if !errors.Is(err, ErrSuperaTope) {
		t.Fatalf("err = %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "1000000.00") {
		t.Fatalf("el mensaje %q no nombra el tope 1000000.00", got)
	}
}

func solicitado() Anticipo {
	a, err := Solicitar(solicitudDe("1000000"), situacionSocio())
	if err != nil {
		panic(err)
	}
	return a
}

// TestAprobar comprueba que aprobar vuelve a mirar las reglas en la fecha de la
// aprobacion: entre pedir y aprobar puede llegar un embargo, perder el socio su
// calidad o moverse la ventana del cupo. Lo que se otorga es lo que cuenta
// (RA 3.1.e: "no podran hacer desembolsos").
func TestAprobar(t *testing.T) {
	casos := []struct {
		nombre  string
		desde   func() Anticipo
		soporte string
		ajustar func(*Situacion)
		quiero  error
	}{
		{nombre: "se aprueba con el soporte del acta", desde: solicitado, soporte: "acta 12, resolucion 34"},
		{
			nombre: "la propia solicitud en tramite no se bloquea a si misma", desde: solicitado, soporte: "acta 12",
			ajustar: func(s *Situacion) { s.Anticipos = []Anticipo{solicitado()} },
		},
		{nombre: "sin soporte no se aprueba (RA 3.2.5)", desde: solicitado, soporte: " ", quiero: ErrResolucionIncompleta},
		{
			nombre: "un embargo reportado despues de pedir bloquea el desembolso", desde: solicitado, soporte: "acta 12",
			ajustar: func(s *Situacion) { s.EmbargoReportado = true },
			quiero:  ErrEmbargoReportado,
		},
		{
			nombre: "quien dejo de ser socio no recibe el anticipo", desde: solicitado, soporte: "acta 12",
			ajustar: func(s *Situacion) { s.Clase = afiliacion.ClaseAdministrado },
			quiero:  ErrNoEsSocio,
		},
		{
			nombre: "si el cupo bajo por debajo del monto no se otorga", desde: solicitado, soporte: "acta 12",
			ajustar: func(s *Situacion) { s.Derechos = dec("3999999.96") },
			quiero:  ErrSuperaTope,
		},
		{
			nombre: "otro anticipo con saldo bloquea la aprobacion", desde: solicitado, soporte: "acta 12",
			ajustar: func(s *Situacion) { s.Anticipos = []Anticipo{previo("ant-0", EstadoAprobado, "10")} },
			quiero:  ErrSaldoPendiente,
		},
		{
			nombre: "un aprobado no se vuelve a aprobar", soporte: "acta 12", quiero: ErrEstadoInvalido,
			desde: func() Anticipo { return aprobado(t, "1000000") },
		},
		{
			nombre: "un rechazado no se aprueba", soporte: "acta 12", quiero: ErrEstadoInvalido,
			desde: func() Anticipo {
				r, err := solicitado().Rechazar("sin soportes")
				if err != nil {
					t.Fatal(err)
				}
				return r
			},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := situacionSocio()
			if c.ajustar != nil {
				c.ajustar(&s)
			}
			a, err := c.desde().Aprobar(c.soporte, s)
			if c.quiero != nil {
				if !errors.Is(err, c.quiero) {
					t.Fatalf("err = %v, se esperaba %v", err, c.quiero)
				}
				return
			}
			if err != nil {
				t.Fatalf("Aprobar: %v", err)
			}
			if a.Estado != EstadoAprobado {
				t.Errorf("estado = %q", a.Estado)
			}
			// Lo que queda por cubrir es TODO lo otorgado (RA 2.1).
			if !a.Saldo.Equal(a.Monto) {
				t.Errorf("saldo = %s, se esperaba el monto %s", a.Saldo, a.Monto)
			}
			if a.Soporte != c.soporte {
				t.Errorf("soporte = %q", a.Soporte)
			}
		})
	}
}

func TestRechazar(t *testing.T) {
	r, err := solicitado().Rechazar("  no demuestra la circunstancia de RA 2.3  ")
	if err != nil {
		t.Fatalf("Rechazar: %v", err)
	}
	if r.Estado != EstadoRechazado || !r.Saldo.IsZero() {
		t.Fatalf("rechazado = %+v", r)
	}
	if r.MotivoRechazo != "no demuestra la circunstancia de RA 2.3" {
		t.Fatalf("motivo = %q, se guarda recortado", r.MotivoRechazo)
	}

	// RA 3.2.3: "En caso de negacion, debera justificarlo".
	if _, err := solicitado().Rechazar(" "); !errors.Is(err, ErrResolucionIncompleta) {
		t.Fatalf("rechazar sin motivo: err = %v", err)
	}
	if _, err := aprobado(t, "1000").Rechazar("tarde"); !errors.Is(err, ErrEstadoInvalido) {
		t.Fatalf("rechazar un aprobado: err = %v", err)
	}
}

func aprobado(t *testing.T, monto string) Anticipo {
	t.Helper()
	s := situacionSocio()
	s.Derechos = dec("40000000")
	a, err := Solicitar(solicitudDe(monto), s)
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}
	a, err = a.Aprobar("acta 12", s)
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	return a
}

// TestRecuperarAtraviesaVariasLiquidaciones es R-33: el anticipo se cubre con
// los derechos de las liquidaciones que siguen, una tras otra, hasta saldarse; y
// a partir de ahi la liquidacion vuelve a pagarse entera.
func TestRecuperarAtraviesaVariasLiquidaciones(t *testing.T) {
	a := aprobado(t, "1000")
	pasos := []struct {
		disponible, descuento, saldo string
		estado                       Estado
	}{
		{"300", "300", "700", EstadoAprobado},
		{"500", "500", "200", EstadoAprobado},
		{"400", "200", "0", EstadoDescontado},
		{"600", "0", "0", EstadoDescontado},
	}
	total := decimal.Zero
	for i, p := range pasos {
		var descuento decimal.Decimal
		descuento, a = a.Recuperar(dec(p.disponible))
		if !descuento.Equal(dec(p.descuento)) {
			t.Fatalf("liquidacion %d: descuento = %s, se esperaba %s", i+1, descuento, p.descuento)
		}
		if !a.Saldo.Equal(dec(p.saldo)) {
			t.Fatalf("liquidacion %d: saldo = %s, se esperaba %s", i+1, a.Saldo, p.saldo)
		}
		if a.Estado != p.estado {
			t.Fatalf("liquidacion %d: estado = %q, se esperaba %q", i+1, a.Estado, p.estado)
		}
		total = total.Add(descuento)
	}
	// Ni un centavo de mas ni de menos: se recupera exactamente lo otorgado.
	if !total.Equal(a.Monto) {
		t.Fatalf("se recupero %s de un anticipo de %s", total, a.Monto)
	}
}

func TestRecuperar(t *testing.T) {
	casos := []struct {
		nombre     string
		anticipo   func() Anticipo
		disponible string
		descuento  string
		saldo      string
	}{
		{"descuenta todo lo disponible si el saldo es mayor", func() Anticipo { return aprobado(t, "1000") }, "250.55", "250.55", "749.45"},
		{"no descuenta mas que el saldo", func() Anticipo { return aprobado(t, "1000") }, "5000", "1000", "0"},
		{"lo disponible exacto salda", func() Anticipo { return aprobado(t, "1000") }, "1000", "1000", "0"},
		{"sin disponible no descuenta", func() Anticipo { return aprobado(t, "1000") }, "0", "0", "1000"},
		{"un disponible negativo no suma saldo", func() Anticipo { return aprobado(t, "1000") }, "-50", "0", "1000"},
		{"una solicitud sin aprobar no se descuenta", solicitado, "500", "0", "0"},
		{"un rechazado no se descuenta", func() Anticipo {
			r, _ := solicitado().Rechazar("x")
			return r
		}, "500", "0", "0"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			antes := c.anticipo()
			descuento, despues := antes.Recuperar(dec(c.disponible))
			if !descuento.Equal(dec(c.descuento)) {
				t.Errorf("descuento = %s, se esperaba %s", descuento, c.descuento)
			}
			if !despues.Saldo.Equal(dec(c.saldo)) {
				t.Errorf("saldo = %s, se esperaba %s", despues.Saldo, c.saldo)
			}
			if despues.Saldo.IsNegative() {
				t.Errorf("saldo negativo: %s", despues.Saldo)
			}
			// La identidad del libro: lo que se desconto sale del saldo, ni mas
			// ni menos.
			if !antes.Saldo.Sub(descuento).Equal(despues.Saldo) {
				t.Errorf("saldo antes %s - descuento %s != saldo despues %s", antes.Saldo, descuento, despues.Saldo)
			}
		})
	}
}
