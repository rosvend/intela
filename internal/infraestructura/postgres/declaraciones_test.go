package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

func partesDePrueba(t *testing.T, split1, split2 int64) repertorio.Declaracion {
	t.Helper()
	partes := []repertorio.Parte{
		{TitularID: titularAna, IPI: "IPI-00000001", Porcentaje: decimal.NewFromInt(split1)},
	}
	if split2 != 0 {
		partes = append(partes, repertorio.Parte{
			TitularID: titularBeto, IPI: "IPI-00000002", Porcentaje: decimal.NewFromInt(split2),
		})
	}
	d, err := repertorio.NuevaDeclaracion(obraSinDeclaracion, partes)
	if err != nil {
		t.Fatalf("construir la declaracion de prueba: %v", err)
	}
	return d
}

func TestGuardarPrimeraVersion(t *testing.T) {
	s, _ := sembrar(t)
	ahora := time.Now().UTC()

	version, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), ahora)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, se esperaba 1", version)
	}

	vd, err := s.VigenteEn(t.Context(), obraSinDeclaracion, ahora)
	if err != nil {
		t.Fatalf("VigenteEn: %v", err)
	}
	if vd.Version != 1 || vd.VigenteHasta != nil {
		t.Fatalf("version vigente = %+v, se esperaba version 1 abierta", vd)
	}
	if !vd.Declaracion.Completa() {
		t.Fatalf("60+40 tiene que ser completa: %+v", vd.Declaracion.Partes)
	}
}

// El caso central del issue: editar CIERRA la version anterior -no la borra,
// no la pisa- y abre una nueva. Las dos quedan consultables.
func TestEditarAbreNuevaVersionYConservaLaAnterior(t *testing.T) {
	s, _ := sembrar(t)
	// Truncado a microsegundos: TIMESTAMPTZ no guarda mas resolucion que esa,
	// y comparar el valor leido contra un time.Time con nanosegundos de mas
	// falla la comparacion por una precision que la base nunca prometio.
	t1 := time.Now().UTC().Truncate(time.Microsecond)
	t2 := t1.Add(time.Hour)

	v1, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), t1)
	if err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}
	v2, err := s.Guardar(t.Context(), partesDePrueba(t, 70, 30), t2)
	if err != nil {
		t.Fatalf("Guardar v2: %v", err)
	}
	if v1 != 1 || v2 != 2 {
		t.Fatalf("versiones = %d, %d; se esperaba 1, 2", v1, v2)
	}

	historial, err := s.Historial(t.Context(), obraSinDeclaracion)
	if err != nil {
		t.Fatalf("Historial: %v", err)
	}
	if len(historial) != 2 {
		t.Fatalf("se esperaban 2 versiones, llegaron %d", len(historial))
	}

	// La primera quedo cerrada exactamente en t2, la segunda sigue abierta.
	if historial[0].Version != 1 || historial[0].VigenteHasta == nil || !historial[0].VigenteHasta.Equal(t2) {
		t.Fatalf("version 1 = %+v, se esperaba cerrada en %s", historial[0], t2)
	}
	if historial[1].Version != 2 || historial[1].VigenteHasta != nil {
		t.Fatalf("version 2 = %+v, se esperaba abierta", historial[1])
	}

	// Las partes de cada version son las que se guardaron EN ESA version, no
	// una mezcla: es la propiedad que protege partesDeObra/todasLasPartes en
	// repertorio.go al filtrar por vigente_hasta IS NULL.
	if !historial[0].Declaracion.Partes[0].Porcentaje.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("version 1 = %+v, se esperaba 60 en la primera parte", historial[0].Declaracion.Partes)
	}
	if !historial[1].Declaracion.Partes[0].Porcentaje.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("version 2 = %+v, se esperaba 70 en la primera parte", historial[1].Declaracion.Partes)
	}
}

// La pregunta que un reproceso necesita responder: que version regia ANTES
// del corte y cual DESPUES. Objetivo 5: un reparto de un periodo pasado usa
// el split vigente entonces, no el de hoy.
func TestVigenteEnResuelveAntesYDespuesDelCorte(t *testing.T) {
	s, _ := sembrar(t)
	// Truncado a microsegundos: TIMESTAMPTZ no guarda mas resolucion que esa,
	// y comparar el valor leido contra un time.Time con nanosegundos de mas
	// falla la comparacion por una precision que la base nunca prometio.
	t1 := time.Now().UTC().Truncate(time.Microsecond)
	t2 := t1.Add(time.Hour)

	if _, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), t1); err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}
	if _, err := s.Guardar(t.Context(), partesDePrueba(t, 70, 30), t2); err != nil {
		t.Fatalf("Guardar v2: %v", err)
	}

	antes, err := s.VigenteEn(t.Context(), obraSinDeclaracion, t1.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("VigenteEn antes del corte: %v", err)
	}
	if antes.Version != 1 {
		t.Fatalf("version antes del corte = %d, se esperaba 1", antes.Version)
	}

	despues, err := s.VigenteEn(t.Context(), obraSinDeclaracion, t2.Add(time.Minute))
	if err != nil {
		t.Fatalf("VigenteEn despues del corte: %v", err)
	}
	if despues.Version != 2 {
		t.Fatalf("version despues del corte = %d, se esperaba 2", despues.Version)
	}

	// Exactamente en t2 ya rige la nueva: el corte es [vigente_desde, vigente_hasta).
	enElCorte, err := s.VigenteEn(t.Context(), obraSinDeclaracion, t2)
	if err != nil {
		t.Fatalf("VigenteEn en el corte: %v", err)
	}
	if enElCorte.Version != 2 {
		t.Fatalf("version en el corte = %d, se esperaba 2 (el intervalo es medio-abierto)", enElCorte.Version)
	}
}

func TestVigenteEnSinDeclaracionEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)

	_, err := s.VigenteEn(t.Context(), obraSinDeclaracion, time.Now().UTC())
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestGuardarObraInexistenteEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)

	d, err := repertorio.NuevaDeclaracion("obra-que-no-existe", []repertorio.Parte{
		{TitularID: titularAna, IPI: "IPI-00000001", Porcentaje: decimal.NewFromInt(100)},
	})
	if err != nil {
		t.Fatalf("construir la declaracion: %v", err)
	}

	_, err = s.Guardar(t.Context(), d, time.Now().UTC())
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Ultima linea de defensa: el EXCLUDE de la migracion 00007 rechaza un
// solape aunque alguien lo intente por fuera de Guardar.
func TestElEsquemaRechazaVigenciasQueSeSolapan(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	t1 := time.Now().UTC()
	if _, err := s.Guardar(ctx, partesDePrueba(t, 100, 0), t1); err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}

	// La version 1 sigue abierta (vigente_hasta IS NULL). Insertar una segunda
	// version tambien abierta para la misma obra se solapa con la primera
	// desde vigente_desde en adelante.
	_, err := pool.Exec(ctx,
		`INSERT INTO declaracion_versiones (obra_id, version, vigente_desde) VALUES ($1, 2, $2)`,
		obraSinDeclaracion, t1.Add(time.Minute))
	if err == nil {
		t.Fatal("se esperaba que el EXCLUDE rechazara el solape")
	}
}
