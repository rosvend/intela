package postgres

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

func TestAsentarYLeerDe(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	cuando := time.Now().UTC().Truncate(time.Microsecond)
	a := aplicacion.Asiento{
		Hecho:   "declaracion.guardada",
		RefTipo: "obra",
		RefID:   obraSinDeclaracion,
		ActorID: usuarioAdmin,
		Payload: []byte(`{"version":1,"estado":"completa"}`),
		Cuando:  cuando,
	}
	if err := s.Asentar(ctx, a); err != nil {
		t.Fatalf("Asentar: %v", err)
	}

	asientos, err := s.De(ctx, "obra", obraSinDeclaracion)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento, llegaron %d", len(asientos))
	}
	got := asientos[0]
	if got.ID == "" {
		t.Fatal("el id lo asigna la base y no puede llegar vacio")
	}
	if got.Hecho != a.Hecho || got.RefTipo != a.RefTipo || got.RefID != a.RefID || got.ActorID != a.ActorID {
		t.Fatalf("asiento mal escaneado: %+v", got)
	}
	// JSONB no promete bytes identicos -- Postgres reordena las claves y
	// normaliza los espacios al guardar-, asi que la comparacion es
	// semantica, no de string.
	var gotJSON, quieroJSON any
	if err := json.Unmarshal(got.Payload, &gotJSON); err != nil {
		t.Fatalf("el payload leido no es JSON: %v", err)
	}
	if err := json.Unmarshal(a.Payload, &quieroJSON); err != nil {
		t.Fatalf("el payload de prueba no es JSON: %v", err)
	}
	if !reflect.DeepEqual(gotJSON, quieroJSON) {
		t.Fatalf("payload = %s, se esperaba %s", got.Payload, a.Payload)
	}
	if !got.Cuando.Equal(cuando) {
		t.Fatalf("Cuando = %s, se esperaba %s", got.Cuando, cuando)
	}
}

// El actor es opcional -una corrida automatica no tiene un usuario detras-.
// NULLIF vacia a NULL en la escritura y COALESCE lo trae de vuelta como "".
func TestAsentarSinActor(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho:   "matching.cascada",
		RefTipo: "uso",
		RefID:   "uso-1",
		ActorID: "",
		Payload: []byte(`{}`),
		Cuando:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Asentar sin actor: %v", err)
	}

	asientos, err := s.De(ctx, "uso", "uso-1")
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 || asientos[0].ActorID != "" {
		t.Fatalf("asientos = %+v", asientos)
	}
}

// De devuelve del mas antiguo al mas nuevo: es el orden en que ocurrieron
// los hechos.
func TestDeOrdenaPorCuando(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	t1 := time.Now().UTC()
	t2 := t1.Add(time.Hour)

	if err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho: "segundo", RefTipo: "obra", RefID: obraSinDeclaracion, Cuando: t2, Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("Asentar segundo: %v", err)
	}
	if err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho: "primero", RefTipo: "obra", RefID: obraSinDeclaracion, Cuando: t1, Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("Asentar primero: %v", err)
	}

	asientos, err := s.De(ctx, "obra", obraSinDeclaracion)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 2 || asientos[0].Hecho != "primero" || asientos[1].Hecho != "segundo" {
		t.Fatalf("orden = %+v", asientos)
	}
}

func TestAsientoPorID(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho: "declaracion.guardada", RefTipo: "obra", RefID: obraSinDeclaracion,
		Cuando: time.Now().UTC(), Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("Asentar: %v", err)
	}
	lista, err := s.De(ctx, "obra", obraSinDeclaracion)
	if err != nil || len(lista) != 1 {
		t.Fatalf("De: %v / %+v", err, lista)
	}

	got, err := s.AsientoPorID(ctx, lista[0].ID)
	if err != nil {
		t.Fatalf("AsientoPorID: %v", err)
	}
	if got.ID != lista[0].ID {
		t.Fatalf("ID = %q, se esperaba %q", got.ID, lista[0].ID)
	}
}

func TestAsientoPorIDInexistente(t *testing.T) {
	s, _ := sembrar(t)

	// Un UUID valido que no existe en la tabla: prueba el camino de
	// ErrNoEncontrado sin depender de una violacion de formato.
	_, err := s.AsientoPorID(t.Context(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// La bitacora es append-only por trigger (ADR 0006, migracion 00001): esta
// prueba lo comprueba contra el esquema, no solo contra el codigo Go que hoy
// no tiene Actualizar ni Borrar.
func TestLaBitacoraRechazaUpdateYDelete(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	if err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho: "declaracion.guardada", RefTipo: "obra", RefID: obraSinDeclaracion,
		Cuando: time.Now().UTC(), Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("Asentar: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE asientos SET hecho = 'otro' WHERE ref_id = $1`, obraSinDeclaracion); err == nil {
		t.Fatal("se esperaba que el trigger append-only rechazara el UPDATE")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM asientos WHERE ref_id = $1`, obraSinDeclaracion); err == nil {
		t.Fatal("se esperaba que el trigger append-only rechazara el DELETE")
	}
}
