package afiliacion

import (
	"errors"
	"testing"
)

func solicitudValida() Afiliado {
	return Afiliado{
		ID:                 "afil-1",
		Nombre:             "Ana Escritora",
		Email:              "ana@redes.co",
		DocumentoIdentidad: "12345678",
		IPI:                "IPI-00000001",
		Subtipo:            Socio,
		Estado:             EstadoPendiente,
		PersonaNatural:     true,
		ClaveRUT:           "afiliaciones/afil-1/rut",
		ClaveCertBancaria:  "afiliaciones/afil-1/banco",
	}
}

func TestValidarSolicitudAceptaUnaCompleta(t *testing.T) {
	t.Parallel()
	if err := solicitudValida().ValidarSolicitud(); err != nil {
		t.Fatalf("ValidarSolicitud: %v", err)
	}
}

func TestValidarSolicitudRechazaCamposVacios(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		mutar  func(*Afiliado)
		quiero error
	}{
		{
			nombre: "sin nombre",
			mutar:  func(a *Afiliado) { a.Nombre = "  " },
			quiero: ErrNombreObligatorio,
		},
		{
			nombre: "sin correo",
			mutar:  func(a *Afiliado) { a.Email = "" },
			quiero: ErrEmailInvalido,
		},
		{
			nombre: "correo sin dominio",
			mutar:  func(a *Afiliado) { a.Email = "ana@" },
			quiero: ErrEmailInvalido,
		},
		{
			nombre: "sin documento",
			mutar:  func(a *Afiliado) { a.DocumentoIdentidad = "" },
			quiero: ErrDocumentoObligatorio,
		},
		{
			nombre: "subtipo desconocido",
			mutar:  func(a *Afiliado) { a.Subtipo = "honorario" },
			quiero: ErrSubtipoInvalido,
		},
		{
			nombre: "sin RUT",
			mutar:  func(a *Afiliado) { a.ClaveRUT = "" },
			quiero: ErrDocumentosPago,
		},
		{
			nombre: "sin certificacion bancaria",
			mutar:  func(a *Afiliado) { a.ClaveCertBancaria = "" },
			quiero: ErrDocumentosPago,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			a := solicitudValida()
			c.mutar(&a)
			if err := a.ValidarSolicitud(); !errors.Is(err, c.quiero) {
				t.Fatalf("se esperaba %v, se obtuvo %v", c.quiero, err)
			}
		})
	}
}

// R-28: declarar que se pertenece a otra SGC sin adjuntar la renuncia
// bloquea la solicitud. El booleano a solas no es evidencia.
func TestExclusividadSinRenunciaBloquea(t *testing.T) {
	t.Parallel()
	a := solicitudValida()
	a.PerteneceOtraSGC = true

	if err := a.ValidarSolicitud(); !errors.Is(err, ErrExclusividad) {
		t.Fatalf("se esperaba ErrExclusividad, se obtuvo %v", err)
	}
}

func TestExclusividadConRenunciaPasa(t *testing.T) {
	t.Parallel()
	a := solicitudValida()
	a.PerteneceOtraSGC = true
	a.ClaveRenuncia = "afiliaciones/afil-1/renuncia"

	if err := a.ValidarSolicitud(); err != nil {
		t.Fatalf("con la renuncia adjunta tiene que pasar: %v", err)
	}
}

func TestIPIEsOpcionalAlSolicitar(t *testing.T) {
	t.Parallel()
	a := solicitudValida()
	a.IPI = ""
	if err := a.ValidarSolicitud(); err != nil {
		t.Fatalf("el IPI es opcional en el alta: %v", err)
	}
}

func TestAdmitirPasaAPendienteDeAdmision(t *testing.T) {
	t.Parallel()
	got, err := solicitudValida().Admitir("tit-1")
	if err != nil {
		t.Fatalf("Admitir: %v", err)
	}
	if got.Estado != EstadoAdmitido {
		t.Fatalf("Estado = %q, se esperaba %q", got.Estado, EstadoAdmitido)
	}
	if got.TitularID != "tit-1" {
		t.Fatalf("TitularID = %q", got.TitularID)
	}
}

func TestAdmitirExigeIPIParaPersonaNatural(t *testing.T) {
	t.Parallel()
	a := solicitudValida()
	a.IPI = ""
	if _, err := a.Admitir("tit-1"); !errors.Is(err, ErrIPIObligatorio) {
		t.Fatalf("se esperaba ErrIPIObligatorio, se obtuvo %v", err)
	}
}

func TestAdmitirNoExigeIPIParaQuienNoEsPersonaNatural(t *testing.T) {
	t.Parallel()
	a := solicitudValida()
	a.PersonaNatural = false
	a.IPI = ""
	got, err := a.Admitir("tit-1")
	if err != nil {
		t.Fatalf("Admitir: %v", err)
	}
	if got.Estado != EstadoAdmitido {
		t.Fatalf("Estado = %q", got.Estado)
	}
}

func TestNoSeAdmiteDosVeces(t *testing.T) {
	t.Parallel()
	a, err := solicitudValida().Admitir("tit-1")
	if err != nil {
		t.Fatalf("Admitir: %v", err)
	}
	if _, err := a.Admitir("tit-2"); !errors.Is(err, ErrEstadoInvalido) {
		t.Fatalf("se esperaba ErrEstadoInvalido, se obtuvo %v", err)
	}
}

func TestRechazarSoloDesdePendiente(t *testing.T) {
	t.Parallel()
	got, err := solicitudValida().Rechazar()
	if err != nil {
		t.Fatalf("Rechazar: %v", err)
	}
	if got.Estado != EstadoRechazado {
		t.Fatalf("Estado = %q", got.Estado)
	}
	if _, err := got.Rechazar(); !errors.Is(err, ErrEstadoInvalido) {
		t.Fatalf("se esperaba ErrEstadoInvalido, se obtuvo %v", err)
	}
	if _, err := got.Admitir("tit-1"); !errors.Is(err, ErrEstadoInvalido) {
		t.Fatalf("una rechazada no se admite: %v", err)
	}
}

// R-30: el subtipo gobierna el anticipo. Un administrado admitido no lo pide.
func TestElegibleAnticipoSoloSocioAdmitido(t *testing.T) {
	t.Parallel()

	socioPendiente := solicitudValida()
	if socioPendiente.ElegibleAnticipo() {
		t.Fatal("un socio pendiente todavia no cobra ni pide anticipo")
	}

	socio, err := socioPendiente.Admitir("tit-1")
	if err != nil {
		t.Fatalf("Admitir: %v", err)
	}
	if !socio.ElegibleAnticipo() {
		t.Fatal("un socio admitido tiene que poder pedir anticipo")
	}

	administrado := solicitudValida()
	administrado.Subtipo = TitularAdministrado
	admitido, err := administrado.Admitir("tit-2")
	if err != nil {
		t.Fatalf("Admitir: %v", err)
	}
	if admitido.ElegibleAnticipo() {
		t.Fatal("un titular administrado no pide anticipo, aunque este admitido")
	}
}
