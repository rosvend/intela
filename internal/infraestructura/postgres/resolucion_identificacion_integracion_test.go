package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/infraestructura/triage"
)

// Pruebas de integracion de la resolucion manual (#175): el caso de uso con el
// Store REAL en los cuatro puertos. Lo que se comprueba aqui y no en
// aplicacion/resolucion_identificacion_test.go es lo que un doble no tiene: los
// CHECK del esquema, el FOR UPDATE, el cerrojo de aviso y el rollback de verdad.

// revisorDePrueba es el actor de todo este fichero.
const revisorDePrueba = "usr-revisor"

// resolucionDePrueba cablea el caso de uso como lo hacen cmd/api y cmd/lambda.
func resolucionDePrueba(s *Store, reloj aplicacion.Reloj) aplicacion.ResolucionIdentificacion {
	return aplicacion.ResolucionIdentificacion{
		Repo: s, Bitacora: s, Unidad: s, Reloj: reloj,
		Ejemplos: s, Rankeador: triage.Heuristico{},
	}
}

// relojQuieto es un Reloj que no avanza: el instante entra por el puerto (ADR
// 0002) y aqui interesa que sea comparable, no que sea el de verdad.
type relojQuieto struct{ instante time.Time }

func (r relojQuieto) Ahora() time.Time { return r.instante }

// sembrarResolucion deja un caso ONI con dos candidatos sobre el catalogo de
// sembrarIdentificacion, mas el usuario revisor que firma.
//
// u-1 ya trae el par (caracol, id_ficha, 871732) en ids_fuente, que es lo que
// hace observable el aprendizaje del alias.
func sembrarResolucion(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 VALUES ($1, 'revisor@redes.test', 'Ana Perez', 'administrador', $2)`,
		revisorDePrueba, hashBcrypt); err != nil {
		t.Fatalf("sembrar el revisor: %v", err)
	}

	// u-1 y u-2 son el MISMO programa en dos emisiones: las dos con el par
	// canonico. u-3 no trae identificadores. Las tres pasan a ONI con bandeja.
	if _, err := pool.Exec(ctx,
		`UPDATE usos SET escalon = 'oni', evidencia = 'banda ambigua', canal_id = 'caracol'
		  WHERE id IN ('u-1', 'u-2', 'u-3')`); err != nil {
		t.Fatalf("pasar a ONI: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO candidatos_match (uso_id, obra_id, puntaje, orden, titulo_consultado) VALUES
		 ('u-1', $1, 0.52941, 0, 'titulo emitido'),
		 ('u-1', $2, 0.41000, 1, 'titulo original'),
		 ('u-2', $1, 0.52941, 0, 'titulo emitido'),
		 ('u-3', $1, 0.52941, 0, 'titulo emitido')`, obraIda, obraImdb); err != nil {
		t.Fatalf("sembrar la bandeja: %v", err)
	}
	return s, pool
}

// El instante fijo de estas pruebas, para poder comparar `resuelto_en`.
var instanteResolucion = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

func solicitudResolucion(usoID, decision, obraID, nota string) aplicacion.SolicitudResolucion {
	return aplicacion.SolicitudResolucion{UsoID: usoID, Decision: decision, ObraID: obraID, Nota: nota}
}

// leerResolucion trae lo que la resolucion escribe y no esta en filaUso.
//
// resueltoPor es cadena y no puntero para que la struct sea comparable con ==:
// con punteros, dos lecturas de la MISMA fila salen distintas y una prueba de
// "no cambio nada" fallaria siempre.
type resolucionEscrita struct {
	escalon     string
	obraIDNulo  bool
	obraID      string
	oni         bool
	resueltoPor string
	resueltoEn  *time.Time
	nota        string
	tipoObra    string
}

func leerResolucion(t *testing.T, pool *pgxpool.Pool, id string) resolucionEscrita {
	t.Helper()
	var r resolucionEscrita
	err := pool.QueryRow(t.Context(),
		`SELECT escalon, obra_id IS NULL, COALESCE(obra_id, ''), oni,
		        COALESCE(resuelto_por, ''), resuelto_en, nota_resolucion, tipo_obra
		   FROM usos WHERE id = $1`, id).
		Scan(&r.escalon, &r.obraIDNulo, &r.obraID, &r.oni,
			&r.resueltoPor, &r.resueltoEn, &r.nota, &r.tipoObra)
	if err != nil {
		t.Fatalf("leer la resolucion de %q: %v", id, err)
	}
	return r
}

// igualResolucion compara dos lecturas de la misma fila, instante incluido.
func igualResolucion(a, b resolucionEscrita) bool {
	if a.escalon != b.escalon || a.obraIDNulo != b.obraIDNulo || a.obraID != b.obraID ||
		a.oni != b.oni || a.resueltoPor != b.resueltoPor || a.nota != b.nota ||
		a.tipoObra != b.tipoObra {
		return false
	}
	switch {
	case a.resueltoEn == nil && b.resueltoEn == nil:
		return true
	case a.resueltoEn == nil || b.resueltoEn == nil:
		return false
	default:
		return a.resueltoEn.Equal(*b.resueltoEn)
	}
}

// ---------------------------------------------------------------------------
// Asignar

func TestResolverIntegracionAsignaAUnaCandidata(t *testing.T) {
	s, pool := sembrarResolucion(t)
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	caso, err := r.Resolver(t.Context(),
		solicitudResolucion("u-1", "asignar", obraIda, "  coincide la ficha tecnica  "),
		revisorDePrueba, "Ana Perez")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	f := leerResolucion(t, pool, "u-1")
	if f.escalon != identificacion.EscalonManual || f.obraID != obraIda || f.oni {
		t.Fatalf("la fila quedo %+v", f)
	}
	if f.resueltoPor != revisorDePrueba ||
		f.resueltoEn == nil || !f.resueltoEn.Equal(instanteResolucion) {
		t.Fatalf("la fila no quedo firmada: %+v", f)
	}
	// La nota se guarda RECORTADA: el caso de uso es el unico que la normaliza.
	if f.nota != "coincide la ficha tecnica" {
		t.Fatalf("nota = %q", f.nota)
	}
	// tipo_obra vacio + obra: el tipo sale del catalogo, el mismo CASE que
	// GuardarMatch (#165/#169).
	if f.tipoObra != "serie" {
		t.Fatalf("tipo_obra = %q, se esperaba el del catalogo (serie)", f.tipoObra)
	}

	// La bandeja NO se borra: es la evidencia de lo que el motor propuso.
	candidatos, err := s.CandidatosDeUso(t.Context(), "u-1")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(candidatos) != 2 || candidatos[0].ObraID != obraIda {
		t.Fatalf("la bandeja se perdio: %+v", candidatos)
	}

	// El alias se aprende a nombre de quien decidio.
	var quien *string
	if err := pool.QueryRow(t.Context(),
		`SELECT quien FROM alias_obra WHERE fuente = 'caracol' AND tipo_id = 'id_ficha' AND valor = '871732'`).
		Scan(&quien); err != nil {
		t.Fatalf("leer el alias: %v", err)
	}
	if quien == nil || *quien != revisorDePrueba {
		t.Fatalf("quien = %v, se esperaba %q", quien, revisorDePrueba)
	}

	// La respuesta ya viene resuelta.
	if caso.Estado != aplicacion.EstadoCasoAsignado || caso.Nota != "coincide la ficha tecnica" {
		t.Fatalf("caso = %+v", caso)
	}

	// Y el asiento lo devuelve GET /auditoria/obra/{id}: la asignacion
	// referencia la OBRA (D8).
	asientos, err := s.De(t.Context(), "obra", obraIda)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 || asientos[0].Hecho != aplicacion.HechoIdentificacionAsignada {
		t.Fatalf("asientos = %+v", asientos)
	}

	historial, err := aplicacion.Auditoria{Bitacora: s}.HistorialDeObra(t.Context(), obraIda)
	if err != nil {
		t.Fatalf("HistorialDeObra: %v", err)
	}
	var visto bool
	for _, a := range historial {
		if a.Hecho == aplicacion.HechoIdentificacionAsignada {
			visto = true
		}
	}
	if !visto {
		t.Fatalf("el historial de la obra no trae la asignacion: %+v", historial)
	}
}

func TestResolverIntegracionAsignaAUnaObraBuscada(t *testing.T) {
	s, pool := sembrarResolucion(t)
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	// obraVacia esta en el catalogo y NO era candidata de u-3 (que solo tiene
	// a obraIda en su bandeja).
	if _, err := r.Resolver(t.Context(),
		solicitudResolucion("u-3", "asignar", obraVacia, "la busque a mano"),
		revisorDePrueba, "Ana Perez"); err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	f := leerResolucion(t, pool, "u-3")
	if f.escalon != identificacion.EscalonManual || f.obraID != obraVacia || f.oni {
		t.Fatalf("la fila quedo %+v", f)
	}

	asientos, err := s.De(t.Context(), "obra", obraVacia)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("asientos = %+v", asientos)
	}
	// El payload se lee decodificado y no con un Contains sobre el crudo: la
	// columna es jsonb y Postgres lo reformatea (claves ordenadas, espacio
	// despues de los dos puntos).
	var payload map[string]any
	if err := json.Unmarshal(asientos[0].Payload, &payload); err != nil {
		t.Fatalf("payload del asiento: %v", err)
	}
	if payload["candidata"] != false {
		t.Fatalf("el asiento no dice que la obra no era candidata: %v", payload)
	}
	if _, hay := payload["puntaje"]; hay {
		t.Fatalf("una obra buscada no tiene puntaje que heredar: %v", payload)
	}
}

// ---------------------------------------------------------------------------
// Descartar

func TestResolverIntegracionDescarta(t *testing.T) {
	s, pool := sembrarResolucion(t)
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	caso, err := r.Resolver(t.Context(),
		solicitudResolucion("u-1", "descartar", "", "no es un uso del repertorio"),
		revisorDePrueba, "Ana Perez")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	f := leerResolucion(t, pool, "u-1")
	if f.escalon != identificacion.EscalonDescartado || !f.obraIDNulo || f.oni {
		t.Fatalf("la fila quedo %+v: un descartado no tiene obra y no es ONI", f)
	}
	if f.resueltoPor == "" || f.resueltoEn == nil || f.nota == "" {
		t.Fatalf("un descarte va firmado y con nota: %+v", f)
	}
	if caso.Estado != aplicacion.EstadoCasoDescartado || caso.ObraAsignada != nil {
		t.Fatalf("caso = %+v", caso)
	}

	// Fuera del listado publico ONI (R-18) y fuera del conteo de pendientes.
	var enPublico int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM oni_publico WHERE id = 'u-1'`).Scan(&enPublico); err != nil {
		t.Fatalf("oni_publico: %v", err)
	}
	if enPublico != 0 {
		t.Error("un descartado no puede salir en el listado publico ONI")
	}

	pag, err := s.ListarCasosIdentificacion(t.Context(), aplicacion.ConsultaCasos{
		Escalones: []string{identificacion.EscalonONI, identificacion.EscalonManual, identificacion.EscalonDescartado},
	})
	if err != nil {
		t.Fatalf("ListarCasosIdentificacion: %v", err)
	}
	if pag.Pendientes != 2 {
		t.Fatalf("pendientes = %d, se esperaban 2: u-1 ya no esta pendiente", pag.Pendientes)
	}
	var descartados int
	for _, c := range pag.Casos {
		if c.Escalon == identificacion.EscalonDescartado {
			descartados++
		}
	}
	if descartados != 1 {
		t.Fatalf("el descartado no aparece con su escalon: %+v", idsDeCasos(pag))
	}

	// No pondera: UsosDeCanal solo devuelve filas con obra, y el resumen lo
	// cuenta por su motivo.
	filas, resumen, err := s.UsosDeCanal(t.Context(), "2024", "caracol", 2024)
	if err != nil {
		t.Fatalf("UsosDeCanal: %v", err)
	}
	for _, f := range filas {
		if f.Uso.ID == "u-1" {
			t.Fatalf("un descartado no puede llegar al motor de reparto: %+v", f.Uso)
		}
	}
	if resumen.Descartados != 1 {
		t.Fatalf("resumen = %+v, se esperaba un descartado contado", resumen)
	}
}

// ---------------------------------------------------------------------------
// Concurrencia

// Dos personas resuelven el MISMO caso a la vez, una asignando y la otra
// descartando. Exactamente una gana, la otra recibe ErrCasoYaResuelto, y queda
// UN solo asiento: el perdedor no puede dejar rastro de una decision que no
// tomo.
func TestResolverIntegracionDosResolucionesConcurrentesSoloUnaGana(t *testing.T) {
	s, pool := sembrarResolucion(t)
	otro := storeAparte(t, s)

	type salida struct {
		err error
	}
	largada := make(chan struct{})
	salidas := make([]salida, 2)

	var wg sync.WaitGroup
	servicios := []aplicacion.ResolucionIdentificacion{
		resolucionDePrueba(s, relojQuieto{instanteResolucion}),
		resolucionDePrueba(otro, relojQuieto{instanteResolucion}),
	}
	solicitudes := []aplicacion.SolicitudResolucion{
		solicitudResolucion("u-1", "asignar", obraIda, "coincide la ficha"),
		solicitudResolucion("u-1", "descartar", "", "no es del repertorio"),
	}
	for i := range servicios {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-largada
			_, err := servicios[i].Resolver(context.Background(), solicitudes[i], revisorDePrueba, "Ana Perez")
			salidas[i] = salida{err: err}
		}(i)
	}
	close(largada)
	wg.Wait()

	ganadoras, perdedoras := 0, 0
	for _, s := range salidas {
		switch {
		case s.err == nil:
			ganadoras++
		case errors.Is(s.err, identificacion.ErrCasoYaResuelto):
			perdedoras++
		default:
			t.Fatalf("error inesperado de una resolucion concurrente: %v", s.err)
		}
	}
	if ganadoras != 1 || perdedoras != 1 {
		t.Fatalf("ganadoras = %d, perdedoras = %d; se esperaba exactamente una de cada", ganadoras, perdedoras)
	}

	// Un solo asiento: el que perdio no asento nada.
	var asientos int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM asientos WHERE hecho IN ($1, $2)`,
		aplicacion.HechoIdentificacionAsignada, aplicacion.HechoIdentificacionDescartada).Scan(&asientos); err != nil {
		t.Fatalf("contar asientos: %v", err)
	}
	if asientos != 1 {
		t.Fatalf("asientos = %d, se esperaba exactamente uno", asientos)
	}

	// Y la fila quedo en UN escalon resuelto, coherente con el asiento.
	f := leerResolucion(t, pool, "u-1")
	if f.escalon != identificacion.EscalonManual && f.escalon != identificacion.EscalonDescartado {
		t.Fatalf("la fila quedo en %q", f.escalon)
	}
}

// ---------------------------------------------------------------------------
// El asiento es parte de "hecho" (ADR 0006), contra la base real

// El asiento falla (bitacoraQueFalla, de procesos_integracion_test.go): el
// repositorio, la unidad y las lecturas siguen siendo el Store real, y lo unico
// doblado es la escritura de la bitacora. Asi el rollback que se comprueba es el
// de Postgres de verdad.
func TestResolverIntegracionSiElAsientoFallaNoQuedaNadaEscrito(t *testing.T) {
	s, pool := sembrarResolucion(t)
	r := aplicacion.ResolucionIdentificacion{
		Repo: s, Unidad: s, Reloj: relojQuieto{instanteResolucion},
		Bitacora:  bitacoraQueFalla{s},
		Ejemplos:  s,
		Rankeador: triage.Heuristico{},
	}

	_, err := r.Resolver(t.Context(),
		solicitudResolucion("u-1", "asignar", obraIda, "coincide la ficha"),
		revisorDePrueba, "Ana Perez")
	if err == nil {
		t.Fatal("un asiento que falla tiene que hacer fallar el caso de uso")
	}

	// El rollback es de verdad: la fila sigue en ONI, sin nota y sin firma.
	f := leerResolucion(t, pool, "u-1")
	if f.escalon != identificacion.EscalonONI || !f.obraIDNulo || !f.oni ||
		f.nota != "" || f.resueltoPor != "" || f.resueltoEn != nil {
		t.Fatalf("la fila no se revirtio: %+v", f)
	}
	// Y el alias tampoco quedo: se aprendio dentro de la unidad que fallo.
	var alias int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM alias_obra WHERE valor = '871732'`).Scan(&alias); err != nil {
		t.Fatalf("contar alias: %v", err)
	}
	if alias != 0 {
		t.Fatalf("el alias sobrevivio al rollback: %d filas", alias)
	}
	var ejemplos int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM ejemplos_resolucion WHERE uso_id = 'u-1'`).Scan(&ejemplos); err != nil {
		t.Fatalf("contar ejemplos: %v", err)
	}
	if ejemplos != 0 {
		t.Fatalf("el ejemplo etiquetado sobrevivio al rollback: %d filas", ejemplos)
	}
}

// ---------------------------------------------------------------------------
// Cerrojo de periodo (D3)

// Resolver un caso de un periodo que YA esta en reparto espera al cerrojo de
// aviso del periodo, el MISMO que usan la evaluacion de anomalias, la ingesta y
// la valorizacion (#171). Sin eso, la resolucion se colaria entre la compuerta
// y el calculo.
func TestResolverIntegracionEsperaElCerrojoDelPeriodo(t *testing.T) {
	s, _ := sembrarResolucion(t)
	// Pool aparte: el de testhelp es de UNA conexion y la retiene la
	// transaccion de fuera. Con el mismo Store, la resolucion esperaria una
	// conexion libre y no el cerrojo, y la prueba pasaria por el motivo
	// equivocado.
	otro := storeAparte(t, s)
	r := resolucionDePrueba(otro, relojQuieto{instanteResolucion})

	termino := make(chan error, 1)
	err := s.EnUnidad(t.Context(), func(ctx context.Context) error {
		if err := s.BloquearAlertasDePeriodo(ctx, "2024"); err != nil {
			return err
		}
		go func() {
			_, err := r.Resolver(context.Background(),
				solicitudResolucion("u-1", "descartar", "", "no es del repertorio"),
				revisorDePrueba, "Ana Perez")
			termino <- err
		}()

		// Con el cerrojo tomado por esta transaccion, la resolucion no puede
		// haber terminado todavia.
		esperarCerrojoDeAvisoEnEspera(t, s)
		select {
		case err := <-termino:
			t.Fatalf("la resolucion no espero el cerrojo del periodo: %v", err)
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EnUnidad: %v", err)
	}

	select {
	case err := <-termino:
		if err != nil {
			t.Fatalf("la resolucion fallo al soltarse el cerrojo: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("la resolucion no termino despues de soltarse el cerrojo")
	}
}

// BloquearPeriodoDeUsos exige una unidad abierta: sin ella, el
// pg_advisory_xact_lock se soltaria al acabar la sentencia y el cerrojo no
// cerraria nada.
func TestBloquearPeriodoDeUsosFueraDeUnidadFalla(t *testing.T) {
	s, _ := sembrarResolucion(t)

	err := s.BloquearPeriodoDeUsos(t.Context(), "2024")
	if !errors.Is(err, errFueraDeUnidad) {
		t.Fatalf("err = %v, se esperaba errFueraDeUnidad", err)
	}
}

// ---------------------------------------------------------------------------
// El alias resuelve el reporte siguiente en el escalon 1 (ADR 0007)

func TestResolverIntegracionElAliasAprendidoResuelveElReporteSiguiente(t *testing.T) {
	s, pool := sembrarResolucion(t)
	ctx := t.Context()
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	if _, err := r.Resolver(ctx,
		solicitudResolucion("u-1", "asignar", obraIda, "coincide la ficha"),
		revisorDePrueba, "Ana Perez"); err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	// Llega un reporte NUEVO, de otro periodo, con el mismo id_ficha.
	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ('rep-2', 'caracol', '2024-12', repeat('d', 64), 'reportes/rep-2.csv', 64)`); err != nil {
		t.Fatalf("sembrar la segunda entrega: %v", err)
	}
	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-nuevo", ReporteID: "rep-2", Fuente: "caracol",
		Titulo: "La Casa de las Dos Palmas", IDsFuente: "id_ficha=871732",
	})

	cascada := cascadaDePrueba(t, s, pool)
	if _, err := cascada.ResolverUsos(ctx, "2024-12"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	f := leerUso(t, pool, "u-nuevo")
	if f.escalon != identificacion.EscalonAlias || f.obraID != obraIda || f.oni {
		t.Fatalf("la fila nueva quedo %+v: el alias tenia que resolverla en el escalon 1", f)
	}
}

// La cascada no pisa una decision humana: 'manual' ni 'descartado' son
// reprocesables, y la corrida los deja intactos.
func TestResolverUsosNoPisaUnaResolucionManualNiUnDescarte(t *testing.T) {
	s, pool := sembrarResolucion(t)
	ctx := t.Context()
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	if _, err := r.Resolver(ctx,
		solicitudResolucion("u-1", "asignar", obraIda, "coincide la ficha"),
		revisorDePrueba, "Ana Perez"); err != nil {
		t.Fatalf("asignar u-1: %v", err)
	}
	if _, err := r.Resolver(ctx,
		solicitudResolucion("u-2", "descartar", "", "no es del repertorio"),
		revisorDePrueba, "Ana Perez"); err != nil {
		t.Fatalf("descartar u-2: %v", err)
	}

	antesUno, antesDos := leerResolucion(t, pool, "u-1"), leerResolucion(t, pool, "u-2")

	cascada := cascadaDePrueba(t, s, pool)
	if _, err := cascada.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	if despues := leerResolucion(t, pool, "u-1"); !igualResolucion(despues, antesUno) {
		t.Fatalf("la cascada piso la asignacion: antes %+v, ahora %+v", antesUno, despues)
	}
	if despues := leerResolucion(t, pool, "u-2"); !igualResolucion(despues, antesDos) {
		t.Fatalf("la cascada piso el descarte: antes %+v, ahora %+v", antesDos, despues)
	}
}

// ---------------------------------------------------------------------------
// Casos que el nucleo rechaza y que no deben tocar la fila

func TestResolverIntegracionRechazaUnaObraFueraDelCatalogo(t *testing.T) {
	s, pool := sembrarResolucion(t)
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})
	antes := leerResolucion(t, pool, "u-1")

	_, err := r.Resolver(t.Context(),
		solicitudResolucion("u-1", "asignar", "obra-que-no-existe", "coincide la ficha"),
		revisorDePrueba, "Ana Perez")
	if !errors.Is(err, aplicacion.ErrObraInexistente) {
		t.Fatalf("err = %v, se esperaba ErrObraInexistente", err)
	}
	if despues := leerResolucion(t, pool, "u-1"); !igualResolucion(despues, antes) {
		t.Fatalf("nada puede cambiar con una obra inexistente: %+v", despues)
	}
}

// El par canonico ya apunta a otra obra: el alias no se pisa y la asignacion no
// se escribe (D6).
func TestResolverIntegracionAliasEnConflicto(t *testing.T) {
	s, pool := sembrarResolucion(t)
	ctx := t.Context()
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraImdb, ""); err != nil {
		t.Fatalf("sembrar el alias: %v", err)
	}
	antes := leerResolucion(t, pool, "u-1")

	_, err := r.Resolver(ctx,
		solicitudResolucion("u-1", "asignar", obraIda, "coincide la ficha"),
		revisorDePrueba, "Ana Perez")
	if !errors.Is(err, aplicacion.ErrAliasEnConflicto) {
		t.Fatalf("err = %v, se esperaba ErrAliasEnConflicto", err)
	}
	if despues := leerResolucion(t, pool, "u-1"); !igualResolucion(despues, antes) {
		t.Fatalf("un conflicto de alias no escribe nada: %+v", despues)
	}

	// Y el alias original sigue donde estaba.
	actual, err := s.Alias(ctx, "caracol", "id_ficha", "871732")
	if err != nil {
		t.Fatalf("Alias: %v", err)
	}
	if actual != obraImdb {
		t.Fatalf("el alias se piso: %q", actual)
	}
}

func TestResolverIntegracionCasoDesconocidoEsNoEncontrado(t *testing.T) {
	s, _ := sembrarResolucion(t)
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	_, err := r.Resolver(t.Context(),
		solicitudResolucion("uso-que-no-existe", "descartar", "", "no es del repertorio"),
		revisorDePrueba, "Ana Perez")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

// El mensaje de ErrCasoNoPendiente nombra el escalon actual: quien lo recibe
// tiene que poder decir "esto ya lo resolvio la cascada" y no un 409 mudo.
func TestResolverIntegracionCasoNoPendienteNombraElEscalon(t *testing.T) {
	s, _ := sembrarResolucion(t)
	ctx := t.Context()
	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})

	if err := s.GuardarMatch(ctx, "u-3", "oni", identificacion.Resultado{
		ObraID: obraImdb, Escalon: identificacion.EscalonDifuso,
		Puntaje: decimal.RequireFromString("0.90"), Evidencia: "difuso",
	}); err != nil {
		t.Fatalf("GuardarMatch: %v", err)
	}

	_, err := r.Resolver(ctx,
		solicitudResolucion("u-3", "descartar", "", "no es del repertorio"),
		revisorDePrueba, "Ana Perez")
	if !errors.Is(err, identificacion.ErrCasoNoPendiente) {
		t.Fatalf("err = %v, se esperaba ErrCasoNoPendiente", err)
	}
	if !strings.Contains(err.Error(), identificacion.EscalonDifuso) {
		t.Fatalf("el mensaje no nombra el escalon actual: %v", err)
	}
}
