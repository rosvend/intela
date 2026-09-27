package aplicacion

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// parametrosFalsos registra con que se le llamo. Es lo que hace comprobable
// que la fecha del PERIODO llega intacta al puerto y que el instante de
// Vigentes sale del reloj y no de time.Now().
type parametrosFalsos struct {
	fechaRecibida time.Time
	ahoraRecibido time.Time
	idRecibido    string
	congelaciones int

	id    string
	snap  reparto.Snapshot
	filas []FilaParametro
	err   error
}

func (p *parametrosFalsos) SnapshotEnFecha(_ context.Context, fechaPeriodo time.Time) (string, reparto.Snapshot, error) {
	p.fechaRecibida = fechaPeriodo
	p.congelaciones++
	if p.err != nil {
		return "", reparto.Snapshot{}, p.err
	}
	return p.id, p.snap, nil
}

func (p *parametrosFalsos) SnapshotPorID(_ context.Context, id string) (reparto.Snapshot, error) {
	p.idRecibido = id
	if p.err != nil {
		return reparto.Snapshot{}, p.err
	}
	return p.snap, nil
}

func (p *parametrosFalsos) Vigentes(_ context.Context, ahora time.Time) ([]FilaParametro, error) {
	p.ahoraRecibido = ahora
	if p.err != nil {
		return nil, p.err
	}
	return p.filas, nil
}

var instanteParametros = time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC)

func servicioParametros(fuente *parametrosFalsos) Parametros {
	return Parametros{Fuente: fuente, Reloj: relojFijo{instante: instanteParametros}}
}

// El instante de la lectura de administracion sale del puerto Reloj (ADR
// 0002). Con un time.Now() dentro del adaptador, la lista dependeria de la
// hora del servidor de base de datos.
func TestVigentesPreguntaPorElInstanteDelReloj(t *testing.T) {
	fuente := &parametrosFalsos{
		filas: []FilaParametro{{
			Clave: "deduccion.administrativa", Valor: "0.200000",
			OrganoAprobador: "Asamblea General", Reglamento: "RD 14.5.1",
		}},
	}

	filas, err := servicioParametros(fuente).Vigentes(t.Context())
	if err != nil {
		t.Fatalf("Vigentes: %v", err)
	}
	if !fuente.ahoraRecibido.Equal(instanteParametros) {
		t.Errorf("el puerto recibio %v, se esperaba el instante del reloj %v",
			fuente.ahoraRecibido, instanteParametros)
	}
	if len(filas) != 1 || filas[0].OrganoAprobador == "" {
		t.Fatalf("la lista tiene que traer la procedencia: %+v", filas)
	}
}

func TestVigentesEnvuelveElErrorDelPuerto(t *testing.T) {
	fallo := errors.New("la base no responde")
	_, err := servicioParametros(&parametrosFalsos{err: fallo}).Vigentes(t.Context())
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error del puerto, dio: %v", err)
	}
}

// La fecha es la del periodo y llega intacta: repartir en marzo el periodo de
// enero tiene que resolver contra enero.
func TestCongelarResuelveContraLaFechaDelPeriodoYNoContraElReloj(t *testing.T) {
	fuente := &parametrosFalsos{
		id:   "snp-" + strings.Repeat("a", 64),
		snap: reparto.Snapshot{Reglamento: "RD 9.1.1", AdminPct: decimal.RequireFromString("0.20")},
	}
	periodo := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	id, snap, err := servicioParametros(fuente).Congelar(t.Context(), periodo)
	if err != nil {
		t.Fatalf("Congelar: %v", err)
	}
	if !fuente.fechaRecibida.Equal(periodo) {
		t.Errorf("el puerto recibio %v, se esperaba la fecha del periodo %v",
			fuente.fechaRecibida, periodo)
	}
	if id != fuente.id || snap.Reglamento != "RD 9.1.1" {
		t.Errorf("Congelar = (%q, %+v), se esperaba lo que devolvio el puerto", id, snap)
	}
}

// Sin esta guarda, una fecha sin rellenar resuelve contra el ano 1 y el fallo
// que sale es "faltan todas las clausulas", que manda a cargar parametros a
// quien lo que tiene es un campo vacio.
func TestCongelarSinFechaDelPeriodoNoLlegaAlPuerto(t *testing.T) {
	fuente := &parametrosFalsos{}

	_, _, err := servicioParametros(fuente).Congelar(t.Context(), time.Time{})
	if err == nil {
		t.Fatal("una fecha de periodo vacia tiene que dar error")
	}
	if !strings.Contains(err.Error(), "fecha del periodo") {
		t.Errorf("el error tiene que nombrar lo que falta, dio: %v", err)
	}
	if fuente.congelaciones != 0 {
		t.Errorf("se llamo al puerto %d veces, se esperaba 0", fuente.congelaciones)
	}
}

// El error de parametro ausente sube SIN envolver en un texto que lo tape:
// quien llama lo distingue con errors.Is y saca las claves con errors.As.
func TestCongelarNoTapaElParametroAusente(t *testing.T) {
	fuente := &parametrosFalsos{
		err: &ErrorParametroAusente{
			Fecha:  time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			Claves: []string{"ott.wa", "ott.wb"},
		},
	}

	_, _, err := servicioParametros(fuente).Congelar(t.Context(), time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrParametroAusente) {
		t.Fatalf("se esperaba ErrParametroAusente, dio: %v", err)
	}
	var ausente *ErrorParametroAusente
	if !errors.As(err, &ausente) {
		t.Fatalf("el error tiene que seguir siendo el tipado, dio: %v", err)
	}
	if !slices.Equal(ausente.Claves, []string{"ott.wa", "ott.wb"}) {
		t.Errorf("Claves = %v, se esperaban las dos", ausente.Claves)
	}
	// El mensaje nombra la fecha y las clausulas: es lo que se acciona.
	if !strings.Contains(err.Error(), "2026-01-31") || !strings.Contains(err.Error(), "ott.wb") {
		t.Errorf("mensaje = %q, se esperaba la fecha y las clausulas", err.Error())
	}
}

// Recalcular lee el snapshot del proceso, nunca vuelve a resolver la fecha.
func TestCongeladoLeePorIDYNoResuelveNada(t *testing.T) {
	fuente := &parametrosFalsos{
		snap: reparto.Snapshot{Reglamento: "RD 9.1.1"},
	}
	id := "snp-" + strings.Repeat("b", 64)

	snap, err := servicioParametros(fuente).Congelado(t.Context(), id)
	if err != nil {
		t.Fatalf("Congelado: %v", err)
	}
	if fuente.idRecibido != id {
		t.Errorf("el puerto recibio %q, se esperaba %q", fuente.idRecibido, id)
	}
	if fuente.congelaciones != 0 {
		t.Errorf("Congelado resolvio la fecha %d veces, no puede resolverla ninguna",
			fuente.congelaciones)
	}
	if snap.Reglamento != "RD 9.1.1" {
		t.Errorf("Reglamento = %q, se esperaba el del snapshot congelado", snap.Reglamento)
	}
}

// Un snapshot corrupto no se degrada a "no encontrado": "no existe" invita a
// volver a resolver la fecha, y eso produciria una cifra distinta de la que se
// pago.
func TestCongeladoDejaSubirElSnapshotCorrupto(t *testing.T) {
	fuente := &parametrosFalsos{err: ErrSnapshotCorrupto}

	_, err := servicioParametros(fuente).Congelado(t.Context(), "snp-"+strings.Repeat("c", 64))
	if !errors.Is(err, ErrSnapshotCorrupto) {
		t.Fatalf("se esperaba ErrSnapshotCorrupto, dio: %v", err)
	}
	if errors.Is(err, ErrNoEncontrado) {
		t.Errorf("un snapshot corrupto no es uno ausente: %v", err)
	}
}

// El puerto lo satisface el adaptador de PostgreSQL; esta asercion es sobre el
// doble, y existe para que un cambio de firma del puerto rompa aqui -- en la
// prueba del caso de uso -- y no solo al compilar cmd/api.
var _ ParametrosNormativos = (*parametrosFalsos)(nil)
