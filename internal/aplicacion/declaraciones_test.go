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
	guardadas             int
	declRecibida          repertorio.Declaracion
	ahoraRecibida         time.Time
	actorIDRecibido       string
	versionADevolver      int
	vigenteDesdeADevolver time.Time
	historial             []VersionDeclaracion
	vigente               VersionDeclaracion
	err                   error
}

// Guardar devuelve, por defecto, el mismo ahora que recibe -asi el doble
// falso se comporta como el caso comun, sin vigenteDesdeADevolver de por
// medio, en las pruebas que no estan verificando el ajuste del puerto-.
func (g *gestionFalsa) Guardar(_ context.Context, d repertorio.Declaracion, ahora time.Time, actorID string) (int, time.Time, error) {
	g.guardadas++
	g.declRecibida = d
	g.ahoraRecibida = ahora
	g.actorIDRecibido = actorID
	if g.err != nil {
		return 0, time.Time{}, g.err
	}
	version := g.versionADevolver
	if version == 0 {
		version = 1
	}
	vigenteDesde := ahora
	if !g.vigenteDesdeADevolver.IsZero() {
		vigenteDesde = g.vigenteDesdeADevolver
	}
	return version, vigenteDesde, nil
}

func (g *gestionFalsa) Historial(_ context.Context, _ string) ([]VersionDeclaracion, error) {
	return g.historial, g.err
}

func (g *gestionFalsa) VigenteEn(_ context.Context, _ string, _ time.Time) (VersionDeclaracion, error) {
	return g.vigente, g.err
}

func partesValidas() []repertorio.Parte {
	return []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(40)},
	}
}

// GuardarSplits delega TODO el trabajo de guardar y asentar en un unico
// metodo de puerto (ver el comentario de GestionDeclaraciones en puertos.go):
// esta prueba comprueba que le llegan los datos correctos, no que orqueste
// una escritura y un asiento por separado -eso ya no existe, y es a proposito.
func TestGuardarSplitsValidaYDelegaEnElPuerto(t *testing.T) {
	gestion := &gestionFalsa{versionADevolver: 1}
	momento := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{instante: momento}}

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
	if gestion.actorIDRecibido != "usr-admin" {
		t.Fatalf("actor = %q", gestion.actorIDRecibido)
	}
}

// GuardarSplits tiene que devolver el vigente_desde que el PUERTO dice que
// escribio, no el instante de Reloj.Ahora() que le mando: el puerto puede
// ajustarlo (ver postgres.Store.Guardar) para que no choque con la version
// que cierra, y devolver el valor local en vez del real es precisamente el
// bloqueante que esta prueba existe para cazar.
func TestGuardarSplitsDevuelveElVigenteDesdeDelPuertoNoElDelReloj(t *testing.T) {
	momento := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ajustado := momento.Add(time.Microsecond)
	gestion := &gestionFalsa{versionADevolver: 2, vigenteDesdeADevolver: ajustado}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{instante: momento}}

	vd, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err != nil {
		t.Fatalf("GuardarSplits: %v", err)
	}
	if !vd.VigenteDesde.Equal(ajustado) {
		t.Fatalf("VigenteDesde = %s, se esperaba el valor ajustado del puerto %s", vd.VigenteDesde, ajustado)
	}
}

// La validacion del dominio corre antes de tocar el puerto: unas partes que
// suman mas de 100 no llegan ni a intentarse.
func TestGuardarSplitsInvalidoNoTocaElPuerto(t *testing.T) {
	gestion := &gestionFalsa{}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

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
}

// Si Guardar falla -incluido un fallo al asentar, que ahora vive DENTRO de
// esa misma llamada (ver [Store.Guardar] en postgres/declaraciones.go)-,
// GuardarSplits solo tiene que propagar el error: ya no hay una segunda
// escritura de la que deshacerse en este nivel. La prueba de que no queda una
// version huerfana es de integracion, contra Postgres real:
// TestGuardarRevierteLaVersionSiElAsientoFalla.
func TestGuardarSplitsPropagaElErrorDelPuerto(t *testing.T) {
	gestion := &gestionFalsa{err: errors.New("version y asiento fallaron juntos")}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err == nil {
		t.Fatal("se esperaba un error cuando el puerto falla")
	}
}

func TestGuardarSplitsPropagaNoEncontrado(t *testing.T) {
	gestion := &gestionFalsa{err: ErrNoEncontrado}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-inexistente", partesValidas(), "usr-admin")
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestHistorialPasaAlPuerto(t *testing.T) {
	quiero := []VersionDeclaracion{{Version: 1}, {Version: 2}}
	gestion := &gestionFalsa{historial: quiero}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

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
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

	got, err := d.VigenteEn(t.Context(), "obra-1", momento)
	if err != nil {
		t.Fatalf("VigenteEn: %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("version = %d", got.Version)
	}
}
