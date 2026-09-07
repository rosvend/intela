package semilla

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

func TestCargarSiembraElJuegoCompleto(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()

	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	decls, err := store.Declaraciones(ctx)
	if err != nil {
		t.Fatalf("Declaraciones: %v", err)
	}
	if got := decls[ObraCine].Estado(); got != "completa" {
		t.Fatalf("obra cine Estado() = %q, se esperaba completa", got)
	}
	if got := decls[ObraSerie].Estado(); got != "incompleta" {
		t.Fatalf("obra serie Estado() = %q, se esperaba incompleta", got)
	}
	if got := decls[ObraUnitario].Estado(); got != "completa" {
		t.Fatalf("obra unitario Estado() = %q, se esperaba completa", got)
	}
	if n := len(decls[ObraUnitario].Partes); n != 3 {
		t.Fatalf("obra unitario: %d coautores, se esperaban 3", n)
	}

	usos, err := store.UsosDePeriodo(ctx, Periodo)
	if err != nil {
		t.Fatalf("UsosDePeriodo: %v", err)
	}
	modalidad := map[reparto.Modalidad]int{}
	for _, u := range usos {
		modalidad[u.Modalidad]++
		if u.ONI {
			t.Fatalf("uso %s quedo en ONI", u.ID)
		}
	}
	for _, m := range []reparto.Modalidad{reparto.TV, reparto.Cine, reparto.OTT} {
		if modalidad[m] == 0 {
			t.Fatalf("no hay usos de %s en el periodo", m)
		}
	}

	// Las dos mitades de la consulta con la que un auditor separa lo aprobado
	// de lo inventado. Lo publicado son las cuatro ponderaciones de RD 9.1.1
	// y los dos coeficientes de duracion (80% artistica, 48 min/hora). Las
	// deducciones, la reserva y el umbral de matching son techos del
	// reglamento o decisiones de ingenieria, y ninguna Asamblea las resolvio
	// (ADR 0004).
	var nSinteticos, nPublicados int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE reglamento =  $1),
		        COUNT(*) FILTER (WHERE reglamento <> $1)
		   FROM parametros`, ReglamentoSintetico,
	).Scan(&nSinteticos, &nPublicados); err != nil {
		t.Fatalf("contar parametros por procedencia: %v", err)
	}
	if nSinteticos != 7 {
		t.Fatalf("parametros con %s: %d, se esperaban 7", ReglamentoSintetico, nSinteticos)
	}
	if nPublicados != 6 {
		t.Fatalf("parametros presentados como aprobados: %d, se esperaban 6 (ponderacion.* y duracion.* de RD 9.1.1)", nPublicados)
	}

	var nBolsas int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bolsas`).Scan(&nBolsas); err != nil {
		t.Fatalf("contar bolsas: %v", err)
	}
	if nBolsas != 4 {
		t.Fatalf("bolsas = %d, se esperaban 4", nBolsas)
	}
}

func TestCargarEsIdempotenteSinReset(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	cargar := func() error {
		return Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio())
	}
	if err := cargar(); err != nil {
		t.Fatalf("primera carga: %v", err)
	}
	if err := cargar(); err != nil {
		t.Fatalf("segunda carga sin reset: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM obras`).Scan(&n); err != nil {
		t.Fatalf("contar obras: %v", err)
	}
	if n != 4 {
		t.Fatalf("obras = %d despues de recargar, se esperaban 4 (no duplicar)", n)
	}
}

func TestCargarConResetReescribe(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	// UN solo almacen para las dos cargas. Con disco(t) dos veces, cada carga
	// escribia en un t.TempDir() distinto -TempDir devuelve un directorio nuevo
	// en cada llamada- y el reset nunca se ejercitaba contra una boveda que YA
	// tuviera los objetos, que es el caso real: el volumen `objetos` de
	// docker-compose es el mismo entre corridas, y ahi la reescritura pasa por
	// el camino de ErrObjetoYaExiste.
	almacen := disco(t)
	if err := Cargar(ctx, store, almacen, hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("carga inicial: %v", err)
	}
	if err := Cargar(ctx, store, almacen, hasher(), clavesPrueba(), true, silencio()); err != nil {
		t.Fatalf("carga con reset: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM obras`).Scan(&n); err != nil {
		t.Fatalf("contar obras: %v", err)
	}
	if n != 4 {
		t.Fatalf("obras = %d tras reset, se esperaban 4", n)
	}
}

func TestCargarResetRechazaSiHayAsientos(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("carga inicial: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO asientos (hecho, ref_tipo, ref_id) VALUES ('prueba', 'obra', $1)`,
		ObraCine); err != nil {
		t.Fatalf("insertar asiento: %v", err)
	}

	err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), true, silencio())
	if !errors.Is(err, ErrBitacoraNoVacia) {
		t.Fatalf("se esperaba ErrBitacoraNoVacia, se obtuvo %v", err)
	}
}

// TestCargarDejaElCatalogoLegible cruza las dos mitades que nadie cruzaba: el
// seed ESCRIBE el catalogo y el caso de uso lo LEE.
//
// Sin el cruce, el seed pasaba verde escribiendo `obras` sin `obra_coautores`
// y GET /obras devolvia 500 en las cuatro obras -"una obra del catalogo
// necesita al menos un coautor con IPI"-, porque la lectura reconstruye la
// entidad por el mismo constructor que la crea. Las pruebas de este paquete
// solo miraban que el dataset cargara, y las de postgres/catalogo_test.go solo
// el adaptador con sus propios fixtures.
func TestCargarDejaElCatalogoLegible(t *testing.T) {
	store, _ := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	catalogo := aplicacion.Catalogo{Obras: store}

	obras, err := catalogo.BuscarObras(ctx, aplicacion.FiltroObras{})
	if err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	esperadas := []string{ObraCine, ObraSerie, ObraSketch, ObraUnitario} // ORDER BY id
	if len(obras) != len(esperadas) {
		t.Fatalf("BuscarObras devolvio %d obras, se esperaban %d", len(obras), len(esperadas))
	}
	for i, id := range esperadas {
		if obras[i].ID() != id {
			t.Fatalf("obra %d: %q, se esperaba %q", i, obras[i].ID(), id)
		}
	}

	// Cada obra tambien por su propia ruta, que es el GET /obras/{id}.
	for _, id := range esperadas {
		obra, err := catalogo.ObraPorID(ctx, id)
		if err != nil {
			t.Fatalf("ObraPorID(%q): %v", id, err)
		}
		coautores := obra.Coautores()
		if len(coautores) == 0 {
			t.Fatalf("obra %q sin coautores: el dominio la rechaza al leerla", id)
		}
		for _, c := range coautores {
			if c.IPI == "" {
				t.Fatalf("obra %q: coautor %q sin IPI", id, c.Nombre)
			}
		}
	}

	// El filtro por IPI resuelve contra obra_coautores: es la comprobacion de
	// que las filas estan ahi y no solo de que la lectura no revienta.
	deCarla, err := catalogo.BuscarObras(ctx, aplicacion.FiltroObras{IPI: "IPI-00000003"})
	if err != nil {
		t.Fatalf("BuscarObras por IPI: %v", err)
	}
	if len(deCarla) != 1 || deCarla[0].ID() != ObraUnitario {
		t.Fatalf("obras de IPI-00000003 = %v, se esperaba solo %s", ids(deCarla), ObraUnitario)
	}
}

func ids(obras []repertorio.Obra) []string {
	out := make([]string, len(obras))
	for i, o := range obras {
		out[i] = o.ID()
	}
	return out
}

// TestResetRechazaObrasAjenas es la guarda que faltaba: vaciar solo mira si la
// bitacora esta vacia, y el estado real de REDES -catalogo y padron cargados,
// ningun reparto asentado todavia- pasa esa comprobacion.
func TestResetRechazaObrasAjenas(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("carga inicial: %v", err)
	}
	// Una obra del catalogo real, que el dataset no conoce.
	if _, err := pool.Exec(ctx, `
		INSERT INTO obras (id, titulo, genero, anio, tipo)
		VALUES ('obra-real-redes', 'Cafe con aroma de mujer', 'Telenovela', 1994, 'telenovela')`,
	); err != nil {
		t.Fatalf("insertar obra real: %v", err)
	}

	err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), true, silencio())
	if !errors.Is(err, ErrDatosNoSinteticos) {
		t.Fatalf("se esperaba ErrDatosNoSinteticos, se obtuvo %v", err)
	}

	var quedan int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM obras WHERE id = 'obra-real-redes'`).Scan(&quedan); err != nil {
		t.Fatalf("contar la obra real: %v", err)
	}
	if quedan != 1 {
		t.Fatal("el reset borro la obra que no era del dataset")
	}
}

// TestResetRechazaTitularesAjenos: el padron IPI es la otra mitad de lo que
// REDES ya tiene cargado, y `usuarios` se reescribe con una clave publicada en
// docs/ARRANQUE.md.
func TestResetRechazaTitularesAjenos(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `
		INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
		VALUES ('tit-real', 'Titular Real', 'IPI-99999999', true, 'socio')`,
	); err != nil {
		t.Fatalf("insertar titular real: %v", err)
	}

	err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), true, silencio())
	if !errors.Is(err, ErrDatosNoSinteticos) {
		t.Fatalf("se esperaba ErrDatosNoSinteticos, se obtuvo %v", err)
	}
}

// TestCargarDetectaLaIdentificacionAMedias: 4 de 6 usos identificados es el
// residuo exacto que deja una caida dentro de identificar. La recarga decia
// "dataset ya sembrado" y salia con 0.
func TestCargarDetectaLaIdentificacionAMedias(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	almacen := disco(t)
	if err := Cargar(ctx, store, almacen, hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("carga inicial: %v", err)
	}

	// Devolver dos usos a "pendiente", como si identificar hubiera muerto a
	// mitad del lote. El CHECK uso_resuelto_tiene_obra obliga a mover oni y
	// obra_id juntos, que es justo lo que el escalon 'pendiente' significa.
	etiqueta, err := pool.Exec(ctx, `
		UPDATE usos
		   SET obra_id = NULL, oni = true, escalon = 'pendiente',
		       evidencia = '', puntaje = 0
		 WHERE id IN (SELECT id FROM usos ORDER BY id LIMIT 2)`)
	if err != nil {
		t.Fatalf("desidentificar dos usos: %v", err)
	}
	if etiqueta.RowsAffected() != 2 {
		t.Fatalf("se desidentificaron %d usos, se esperaban 2", etiqueta.RowsAffected())
	}

	err = Cargar(ctx, store, almacen, hasher(), clavesPrueba(), false, silencio())
	if err == nil {
		t.Fatal("una base a medias se reporto como completa: Cargar salio sin error")
	}
	if !strings.Contains(err.Error(), "SEED_RESET") {
		t.Fatalf("el error no dice como salir del atasco: %v", err)
	}
}

// TestIdentificarEsAtomico: identificar corria N Exec sueltos fuera de
// transaccion, deshaciendo para el mismo lote la atomicidad que GuardarUsos
// acababa de dar. Un fallo a mitad dejaba las filas anteriores identificadas.
func TestIdentificarEsAtomico(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	d := Construir()

	if err := registrarObras(ctx, store, d.Obras); err != nil {
		t.Fatalf("registrarObras: %v", err)
	}
	ingesta := aplicacion.Ingesta{Reportes: store, Almacen: disco(t)}
	r := d.Reportes[0]
	rep, err := ingesta.GuardarReporte(ctx, r.Fuente, r.Periodo, r.Bytes)
	if err != nil {
		t.Fatalf("GuardarReporte: %v", err)
	}
	if _, err := ingesta.GuardarUsos(ctx, rep, usosCrudos(r.Usos)); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	// Un uso de mas: su id derivado no existe en `usos`, asi que identificar
	// falla DESPUES de haber actualizado los anteriores.
	conSobrante := append(append([]aplicacion.UsoPersistido{}, r.Usos...), r.Usos[0])
	if err := identificar(ctx, store, rep.ID, conSobrante); err == nil {
		t.Fatal("identificar acepto un uso que no estaba en la tabla")
	}

	var identificados int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM usos WHERE reporte_id = $1 AND obra_id IS NOT NULL`,
		rep.ID).Scan(&identificados); err != nil {
		t.Fatalf("contar identificados: %v", err)
	}
	if identificados != 0 {
		t.Fatalf("%d usos quedaron identificados tras un fallo a mitad: no hubo rollback", identificados)
	}
}

// TestCargarSiembraElAliasDeCadaUso comprueba que el escalon "alias" no sea
// una etiqueta huerfana.
//
// Los seis usos salian con escalon='alias' y `alias_obra` VACIA: un auditor que
// siguiera el escalon no encontraba nada. La consulta de abajo es exactamente
// ese camino -de la fila de uso a la fila de alias por (fuente, valor)- y con
// la tabla vacia no casa ninguna.
func TestCargarSiembraElAliasDeCadaUso(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	var huerfanos int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM usos u
		 WHERE u.escalon = 'alias'
		   AND NOT EXISTS (
		         SELECT 1 FROM alias_obra a
		          WHERE a.fuente  = u.fuente
		            AND a.valor   = u.ids_fuente
		            AND a.obra_id = u.obra_id)`).Scan(&huerfanos); err != nil {
		t.Fatalf("cruzar usos con alias_obra: %v", err)
	}
	if huerfanos != 0 {
		t.Fatalf("%d usos con escalon 'alias' sin fila en alias_obra que los explique", huerfanos)
	}

	var nAlias int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM alias_obra`).Scan(&nAlias); err != nil {
		t.Fatalf("contar alias: %v", err)
	}
	if nAlias == 0 {
		t.Fatal("alias_obra vacia: el escalon 'alias' seria mentira")
	}
}

// TestHashearRechazaClavesRepetidas: dos roles con la misma clave dan dos
// hashes bcrypt distintos -bcrypt lleva sal-, asi que ni la base ni el hasher
// lo notan. Una sola persona firmaria las dos mitades del control de doble
// firma del `RD 13.5`, que es la propiedad que este seed presume.
func TestHashearRechazaClavesRepetidas(t *testing.T) {
	c := clavesPrueba()
	c.Contabilidad = c.Distribucion

	if _, err := hashear(hasher(), c); !errors.Is(err, ErrClaveRepetida) {
		t.Fatalf("se esperaba ErrClaveRepetida, se obtuvo %v", err)
	}
	if _, err := hashear(hasher(), clavesPrueba()); err != nil {
		t.Fatalf("claves distintas: %v", err)
	}
}
func abrir(t *testing.T) (*postgres.Store, *pgxpool.Pool) {
	t.Helper()
	pool := testhelp.Pool(t)
	return postgres.Nuevo(pool), pool
}

func disco(t *testing.T) aplicacion.AlmacenObjetos {
	t.Helper()
	return objetos.Disco{Dir: t.TempDir()}
}

func hasher() aplicacion.Hasher {
	return cripto.Bcrypt{Coste: bcrypt.MinCost}
}

func clavesPrueba() Claves {
	return Claves{
		Admin:        "admin-local",
		Distribucion: "distribucion-local",
		Contabilidad: "contabilidad-local",
		Auditor:      "auditor-local",
		Titular:      "ana-local",
	}
}

func silencio() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
