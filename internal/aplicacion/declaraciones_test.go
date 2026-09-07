package aplicacion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// gestionFalsa cuenta cuantas veces le tocaron la base. Es lo que hace
// comprobable que la validacion del dominio corre ANTES y no despues, igual
// que catalogoFalso.
type gestionFalsa struct {
	guardadas        int
	declRecibida     repertorio.Declaracion
	ahoraRecibida    time.Time
	versionADevolver int
	historial        []VersionDeclaracion
	vigente          VersionDeclaracion
	err              error
}

func (g *gestionFalsa) Guardar(_ context.Context, d repertorio.Declaracion, ahora time.Time) (int, error) {
	g.guardadas++
	g.declRecibida = d
	g.ahoraRecibida = ahora
	if g.err != nil {
		return 0, g.err
	}
	if g.versionADevolver == 0 {
		return 1, nil
	}
	return g.versionADevolver, nil
}

func (g *gestionFalsa) Historial(_ context.Context, _ string) ([]VersionDeclaracion, error) {
	return g.historial, g.err
}

func (g *gestionFalsa) VigenteEn(_ context.Context, _ string, _ time.Time) (VersionDeclaracion, error) {
	return g.vigente, g.err
}

// bitacoraFalsa cuenta cuantos asientos se pidieron y puede simular el fallo
// de escribirlos.
type bitacoraFalsa struct {
	asentados     int
	asientoUltimo Asiento
	errAsentar    error
}

func (b *bitacoraFalsa) Asentar(_ context.Context, a Asiento) error {
	b.asentados++
	b.asientoUltimo = a
	return b.errAsentar
}

func (b *bitacoraFalsa) De(_ context.Context, _, _ string) ([]Asiento, error) { return nil, nil }
func (b *bitacoraFalsa) AsientoPorID(_ context.Context, _ string) (Asiento, error) {
	return Asiento{}, nil
}

func partesValidas() []repertorio.Parte {
	return []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(40)},
	}
}

func TestGuardarSplitsValidaGuardaYAsienta(t *testing.T) {
	gestion := &gestionFalsa{versionADevolver: 1}
	bitacora := &bitacoraFalsa{}
	momento := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	d := Declaraciones{Gestion: gestion, Bitacora: bitacora, Reloj: relojFijo{instante: momento}}

	vd, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err != nil {
		t.Fatalf("GuardarSplits: %v", err)
	}
	if vd.Version != 1 || !vd.VigenteDesde.Equal(momento) {
		t.Fatalf("version = %+v", vd)
	}
	if gestion.guardadas != 1 {
		t.Fatalf("se esperaba 1 escritura, hubo %d", gestion.guardadas)
	}
	if gestion.declRecibida.ObraID != "obra-1" {
		t.Fatalf("al puerto le llego otra obra: %q", gestion.declRecibida.ObraID)
	}
	if bitacora.asentados != 1 {
		t.Fatalf("se esperaba 1 asiento, hubo %d", bitacora.asentados)
	}
	if bitacora.asientoUltimo.RefTipo != "obra" || bitacora.asientoUltimo.RefID != "obra-1" {
		t.Fatalf("asiento mal referenciado: %+v", bitacora.asientoUltimo)
	}
	if bitacora.asientoUltimo.ActorID != "usr-admin" {
		t.Fatalf("actor = %q", bitacora.asientoUltimo.ActorID)
	}
}

// La validacion del dominio corre antes de tocar el puerto: unas partes que
// suman mas de 100 no llegan ni a intentarse.
func TestGuardarSplitsInvalidoNoTocaElPuerto(t *testing.T) {
	gestion := &gestionFalsa{}
	bitacora := &bitacoraFalsa{}
	d := Declaraciones{Gestion: gestion, Bitacora: bitacora, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(60)},
	}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, repertorio.ErrDeclaracionInvalida) {
		t.Fatalf("se esperaba ErrDeclaracionInvalida, se obtuvo %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se intento guardar una declaracion que el dominio rechaza")
	}
	if bitacora.asentados != 0 {
		t.Fatal("se asento un hecho que nunca ocurrio")
	}
}

// El asiento es parte de la definicion de hecho (ADR 0006): si Asentar falla,
// GuardarSplits devuelve error aunque la escritura ya se hizo.
func TestGuardarSplitsPropagaElFalloDelAsiento(t *testing.T) {
	gestion := &gestionFalsa{versionADevolver: 1}
	bitacora := &bitacoraFalsa{errAsentar: errors.New("bitacora caida")}
	d := Declaraciones{Gestion: gestion, Bitacora: bitacora, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err == nil {
		t.Fatal("se esperaba un error cuando el asiento falla")
	}
	if gestion.guardadas != 1 {
		t.Fatalf("la escritura ya se habia hecho: se esperaba 1, hubo %d", gestion.guardadas)
	}
}

func TestGuardarSplitsPropagaNoEncontrado(t *testing.T) {
	gestion := &gestionFalsa{err: ErrNoEncontrado}
	d := Declaraciones{Gestion: gestion, Bitacora: &bitacoraFalsa{}, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-inexistente", partesValidas(), "usr-admin")
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestHistorialPasaAlPuerto(t *testing.T) {
	quiero := []VersionDeclaracion{{Version: 1}, {Version: 2}}
	gestion := &gestionFalsa{historial: quiero}
	d := Declaraciones{Gestion: gestion, Bitacora: &bitacoraFalsa{}, Reloj: relojFijo{}}

	got, err := d.Historial(t.Context(), "obra-1")
	if err != nil {
		t.Fatalf("Historial: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("historial = %+v", got)
	}
}

func TestVigenteEnPasaElMomento(t *testing.T) {
	momento := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	quiero := VersionDeclaracion{Version: 3}
	gestion := &gestionFalsa{vigente: quiero}
	d := Declaraciones{Gestion: gestion, Bitacora: &bitacoraFalsa{}, Reloj: relojFijo{}}

	got, err := d.VigenteEn(t.Context(), "obra-1", momento)
	if err != nil {
		t.Fatalf("VigenteEn: %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("version = %d", got.Version)
	}
}
