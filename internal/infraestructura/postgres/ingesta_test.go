package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// Fixtures de ingesta. Aparte de semilla_test.go por la misma razon por la que
// aquellas no viven en testhelp: quien copie estas no quiere las de repertorio.
//
// Los sha son de 64 caracteres hexadecimales porque el esquema lo comprueba
// (`sha256 ~ '^[0-9a-f]{64}$'`); un "abc" no pasa.
const (
	shaParrilla = "1111111111111111111111111111111111111111111111111111111111111111"
	shaOtro     = "2222222222222222222222222222222222222222222222222222222222222222"

	reporteEnero   = "rep-caracol-enero"
	reporteFebrero = "rep-caracol-febrero"
)

// sembrarReportes deja dos reportes de periodos distintos y devuelve el Store.
func sembrarReportes(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	pool := testhelp.Pool(t)
	s := &Store{pool: pool}
	ctx := t.Context()

	if err := s.GuardarReporte(ctx, reporteEnero, "caracol", "2026-01",
		shaParrilla, "reportes/"+shaParrilla, 128); err != nil {
		t.Fatalf("sembrar reporte de enero: %v", err)
	}
	if err := s.GuardarReporte(ctx, reporteFebrero, "caracol", "2026-02",
		shaOtro, "reportes/"+shaOtro, 256); err != nil {
		t.Fatalf("sembrar reporte de febrero: %v", err)
	}
	return s, pool
}

func usoPendiente(id, reporteID, titulo string) aplicacion.UsoPersistido {
	return aplicacion.UsoPersistido{
		ID:        id,
		ReporteID: reporteID,
		Fuente:    "caracol",
		Titulo:    titulo,
		Modalidad: reparto.TV,
		Escalon:   "pendiente",
		ONI:       true,
		Emisiones: 3,
	}
}

// ---------------------------------------------------------------------------
// reportes
// ---------------------------------------------------------------------------

func TestGuardarReporteDejaLaFilaCompleta(t *testing.T) {
	s, pool := sembrarReportes(t)

	var (
		fuente, periodo, sha, clave string
		nbytes                      int
	)
	err := pool.QueryRow(t.Context(),
		`SELECT fuente, periodo, sha256, clave_objeto, nbytes FROM reportes WHERE id = $1`,
		reporteEnero).Scan(&fuente, &periodo, &sha, &clave, &nbytes)
	if err != nil {
		t.Fatalf("leer el reporte: %v", err)
	}
	if fuente != "caracol" || periodo != "2026-01" {
		t.Fatalf("procedencia mal guardada: %q %q", fuente, periodo)
	}
	if sha != shaParrilla {
		t.Fatalf("sha256 = %q, se esperaba %q", sha, shaParrilla)
	}
	if clave != "reportes/"+shaParrilla || nbytes != 128 {
		t.Fatalf("clave %q, nbytes %d", clave, nbytes)
	}
	_ = s
}

// La deteccion de duplicado es la UNICA razon de ser del UNIQUE (sha256,
// fuente), y el adaptador tiene que traducirla a vocabulario del nucleo: un
// 23505 crudo llegaria al handler como 500 en vez de como 409.
func TestGuardarReporteTraduceElDuplicadoDeHuella(t *testing.T) {
	s, _ := sembrarReportes(t)

	err := s.GuardarReporte(t.Context(), "rep-otro-id", "caracol", "2026-03",
		shaParrilla, "reportes/"+shaParrilla, 128)
	if !errors.Is(err, aplicacion.ErrReporteDuplicado) {
		t.Fatalf("se esperaba ErrReporteDuplicado, se obtuvo %v", err)
	}
	// El mensaje dice cual fue: un centinela pelado no sirve para depurar.
	if err != nil && !strings.Contains(err.Error(), "caracol") {
		t.Fatalf("el error no nombra la fuente: %v", err)
	}
	_ = s
}

// El esquema permite a proposito que dos fuentes entreguen los mismos bytes:
// el UNIQUE es sobre el PAR. Deduplicar solo por contenido perderia la segunda
// entrega, que es una entrega real de otro usuario.
func TestGuardarReporteAdmiteLaMismaHuellaDeOtraFuente(t *testing.T) {
	s, _ := sembrarReportes(t)

	if err := s.GuardarReporte(t.Context(), "rep-netflix", "netflix", "2026-01",
		shaParrilla, "reportes/"+shaParrilla, 128); err != nil {
		t.Fatalf("otra fuente con los mismos bytes es una entrega valida: %v", err)
	}
}

// ---------------------------------------------------------------------------
// usos
// ---------------------------------------------------------------------------

// El invariante numero uno del sistema, comprobado contra el catalogo de la
// base y no contra la migracion: si alguien anade una columna de importe a
// `usos`, sumar dinero por fila pasa a ser expresable en SQL, que es
// exactamente lo que el reglamento no permite.
func TestLaTablaDeUsosNoTieneNingunaColumnaDeImporte(t *testing.T) {
	_, pool := sembrarReportes(t)

	filas, err := pool.Query(t.Context(),
		`SELECT column_name FROM information_schema.columns
		  WHERE table_name IN ('usos','usos_rechazados') ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatalf("leer el catalogo: %v", err)
	}
	defer filas.Close()

	prohibidas := []string{"importe", "monto", "valor", "dinero", "bruto", "neto", "pago", "tarifa"}
	for filas.Next() {
		var col string
		if err := filas.Scan(&col); err != nil {
			t.Fatalf("escanear: %v", err)
		}
		for _, p := range prohibidas {
			if strings.Contains(col, p) {
				t.Fatalf("columna %q: un reporte de uso PONDERA la bolsa, no la aporta", col)
			}
		}
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("recorrer el catalogo: %v", err)
	}
}

func TestGuardarUsosPersisteLaFormaCanonica(t *testing.T) {
	s, _ := sembrarReportes(t)
	ctx := t.Context()

	u := usoPendiente("uso-1", reporteEnero, "La Casa de las Dos Palmas")
	u.IDsFuente = "ID_Ficha=1234"
	u.TipoObra = "serie"
	u.DuracionMin = decimal.RequireFromString("52.5000")
	u.Rating = decimal.RequireFromString("3.250000")
	u.Vistas = decimal.RequireFromString("1200.00")

	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{u}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	leido, err := s.UsoPorID(ctx, "uso-1")
	if err != nil {
		t.Fatalf("UsoPorID: %v", err)
	}
	if leido.Titulo != u.Titulo || leido.Fuente != "caracol" || leido.ReporteID != reporteEnero {
		t.Fatalf("identidad mal escaneada: %+v", leido)
	}
	if leido.Modalidad != reparto.TV {
		t.Fatalf("Modalidad = %q, se esperaba %q", leido.Modalidad, reparto.TV)
	}
	if leido.Escalon != "pendiente" || !leido.ONI || leido.ObraID != "" {
		t.Fatalf("una fila recien ingerida no esta identificada: %+v", leido)
	}
	if leido.Emisiones != 3 {
		t.Fatalf("Emisiones = %d, se esperaba 3", leido.Emisiones)
	}
	// Equal y no ==: NUMERIC vuelve con la escala de la columna y decimal
	// compara exponentes en ==.
	if !leido.DuracionMin.Equal(u.DuracionMin) || !leido.Rating.Equal(u.Rating) {
		t.Fatalf("las medidas no sobrevivieron el viaje: %+v", leido)
	}
	if !leido.Vistas.Equal(u.Vistas) {
		t.Fatalf("Vistas = %s, se esperaba %s", leido.Vistas, u.Vistas)
	}
}

// El caso que da nombre al issue: dos filas buenas y una malformada dejan dos
// usos canonicos y un rechazo. Las tres se persisten; solo dos se leen.
func TestGuardarUsosSeparaElLoteEnCanonicoYRechazado(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	mala := usoPendiente("uso-mala", reporteEnero, "Radio Novela")
	mala.Modalidad = "radio"
	mala.RechazoMotivo = `modalidad "radio" fuera de tv|cine|ott|hotel`

	err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{
		usoPendiente("uso-1", reporteEnero, "La Casa"),
		mala,
		usoPendiente("uso-2", reporteEnero, "Cronica"),
	})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	var canonicos, rechazos int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos`).Scan(&canonicos); err != nil {
		t.Fatalf("contar usos: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos_rechazados`).Scan(&rechazos); err != nil {
		t.Fatalf("contar rechazos: %v", err)
	}
	if canonicos != 2 || rechazos != 1 {
		t.Fatalf("usos = %d, rechazos = %d; se esperaba 2 y 1", canonicos, rechazos)
	}

	// El motivo queda guardado, no solo el hecho de que se rechazo.
	var motivo, titulo string
	if err := pool.QueryRow(ctx,
		`SELECT motivo, titulo FROM usos_rechazados WHERE id = $1`, "uso-mala").
		Scan(&motivo, &titulo); err != nil {
		t.Fatalf("leer el rechazo: %v", err)
	}
	if !strings.Contains(motivo, "modalidad") {
		t.Fatalf("el motivo no nombra el campo: %q", motivo)
	}
	if titulo != "Radio Novela" {
		t.Fatalf("la fila rechazada perdio su titulo: %q", titulo)
	}

	// Y no se cuela por ninguna de las dos lecturas canonicas.
	pendientes, err := s.UsosSinResolver(ctx)
	if err != nil {
		t.Fatalf("UsosSinResolver: %v", err)
	}
	delPeriodo, err := s.UsosDePeriodo(ctx, "2026-01")
	if err != nil {
		t.Fatalf("UsosDePeriodo: %v", err)
	}
	for _, lista := range [][]aplicacion.UsoPersistido{pendientes, delPeriodo} {
		if len(lista) != 2 {
			t.Fatalf("se esperaban 2 filas, llegaron %d", len(lista))
		}
		for _, u := range lista {
			if u.ID == "uso-mala" {
				t.Fatal("una fila rechazada no puede aparecer en una lectura canonica")
			}
		}
	}

	// Un id rechazado tampoco se resuelve por UsoPorID: existe en el log, no
	// como uso.
	if _, err := s.UsoPorID(ctx, "uso-mala"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Un lote es un hecho: o entra entero o no entra. Si la mitad se guardara, la
// entrega quedaria con un recuento que no cuadra con el archivo y nadie sabria
// que mitad falta.
//
// La fila mala apunta a una obra que no existe, que es una violacion de clave
// foranea: no la detecta la validacion estructural del caso de uso, asi que
// llega viva hasta el INSERT.
func TestGuardarUsosEsAtomico(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	rota := usoPendiente("uso-rota", reporteEnero, "Apunta a la nada")
	rota.ONI = false
	rota.ObraID = "obra-que-no-existe"
	rota.Escalon = "alias"

	err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{
		usoPendiente("uso-1", reporteEnero, "La Casa"),
		rota,
	})
	if err == nil {
		t.Fatal("se esperaba error: la fila apunta a una obra inexistente")
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos`).Scan(&n); err != nil {
		t.Fatalf("contar usos: %v", err)
	}
	if n != 0 {
		t.Fatalf("el lote se guardo a medias: %d filas", n)
	}
}

func TestGuardarUsosSinFilasNoEsError(t *testing.T) {
	s, _ := sembrarReportes(t)

	if err := s.GuardarUsos(t.Context(), nil); err != nil {
		t.Fatalf("un lote vacio no es un error: %v", err)
	}
}

// "Sin resolver" es escalon pendiente, no ONI: la cola manual es otro puerto
// (RepositorioONI) y otra pregunta. Una fila ya resuelta por alias no vuelve a
// la cascada.
func TestUsosSinResolverSoloDevuelveLasPendientes(t *testing.T) {
	s, _ := sembrarReportes(t)
	ctx := t.Context()

	// Hace falta una obra de verdad: usos.obra_id es clave foranea.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, tipo) VALUES ('obra-1', 'La Casa', 'serie')`); err != nil {
		t.Fatalf("sembrar obra: %v", err)
	}

	resuelta := usoPendiente("uso-resuelta", reporteEnero, "La Casa")
	resuelta.Escalon = "alias"
	resuelta.ONI = false
	resuelta.ObraID = "obra-1"
	resuelta.Evidencia = "alias caracol/ID_Ficha=1234"

	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{
		usoPendiente("uso-pendiente", reporteEnero, "Sin identificar"),
		resuelta,
	}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	pendientes, err := s.UsosSinResolver(ctx)
	if err != nil {
		t.Fatalf("UsosSinResolver: %v", err)
	}
	if len(pendientes) != 1 || pendientes[0].ID != "uso-pendiente" {
		t.Fatalf("se esperaba solo uso-pendiente, llego %+v", pendientes)
	}

	// La fila resuelta si se lee por id, con su obra y su evidencia: es la
	// pregunta 3 del ADR 0006, COMO se reconocio.
	leida, err := s.UsoPorID(ctx, "uso-resuelta")
	if err != nil {
		t.Fatalf("UsoPorID: %v", err)
	}
	if leida.ObraID != "obra-1" || leida.ONI || leida.Evidencia == "" {
		t.Fatalf("la resolucion no sobrevivio: %+v", leida)
	}
}

// El periodo no esta en `usos`: vive en el reporte del que salio la fila
// (ADR 0004, el periodo es dato). Filtrar por el es cruzar las dos tablas.
func TestUsosDePeriodoFiltraPorElPeriodoDelReporte(t *testing.T) {
	s, _ := sembrarReportes(t)
	ctx := t.Context()

	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{
		usoPendiente("uso-ene-b", reporteEnero, "Enero B"),
		usoPendiente("uso-ene-a", reporteEnero, "Enero A"),
		usoPendiente("uso-feb", reporteFebrero, "Febrero"),
	}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	enero, err := s.UsosDePeriodo(ctx, "2026-01")
	if err != nil {
		t.Fatalf("UsosDePeriodo: %v", err)
	}
	if len(enero) != 2 {
		t.Fatalf("se esperaban 2 filas de enero, llegaron %d", len(enero))
	}
	// ADR 0005: una lista sin orden explicito no es reproducible.
	if enero[0].ID != "uso-ene-a" || enero[1].ID != "uso-ene-b" {
		t.Fatalf("las filas no vienen ordenadas por id: %q, %q", enero[0].ID, enero[1].ID)
	}

	vacio, err := s.UsosDePeriodo(ctx, "2030-12")
	if err != nil {
		t.Fatalf("un periodo sin filas no es un error: %v", err)
	}
	if len(vacio) != 0 {
		t.Fatalf("se esperaba lista vacia, llegaron %d", len(vacio))
	}
}

func TestUsoPorIDInexistente(t *testing.T) {
	s, _ := sembrarReportes(t)

	if _, err := s.UsoPorID(t.Context(), "uso-que-no-existe"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestUsosSinResolverSinFilasNoEsError(t *testing.T) {
	s, _ := sembrarReportes(t)

	usos, err := s.UsosSinResolver(t.Context())
	if err != nil {
		t.Fatalf("una tabla vacia no es un error: %v", err)
	}
	if len(usos) != 0 {
		t.Fatalf("se esperaba lista vacia, llegaron %d", len(usos))
	}
}

// ---------------------------------------------------------------------------
// El caso de uso completo, contra PostgreSQL y contra el disco de verdad
// ---------------------------------------------------------------------------

// Lo que pide el plan de pruebas del issue, de punta a punta: subir un fixture
// deja fila y objeto; resubirlo da duplicado y NO toca el objeto; y un lote con
// dos filas buenas y una malformada deja dos usos y un rechazo.
func TestIngestaDePuntaAPunta(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	almacen := objetos.Disco{Dir: t.TempDir()}
	ingesta := aplicacion.Ingesta{Reportes: s, Almacen: almacen}

	// Una parrilla de las de verdad en miniatura: la granularidad es la
	// emision, no la obra.
	fixture := []byte("Titulo,ID_Ficha,Fecha,Duracion\n" +
		"La Casa de las Dos Palmas,1234,20260115,52\n" +
		"La Casa de las Dos Palmas,1234,20260116,52\n")

	rep, err := ingesta.GuardarReporte(ctx, "caracol-real", "2026-01", fixture)
	if err != nil {
		t.Fatalf("GuardarReporte: %v", err)
	}

	guardado, err := almacen.Obtener(ctx, rep.ClaveObjeto)
	if err != nil {
		t.Fatalf("la evidencia no llego a la boveda: %v", err)
	}
	if string(guardado) != string(fixture) {
		t.Fatal("los bytes de la boveda no son los que se subieron")
	}

	var enBase string
	if err := pool.QueryRow(ctx, `SELECT sha256 FROM reportes WHERE id = $1`, rep.ID).
		Scan(&enBase); err != nil {
		t.Fatalf("leer el reporte: %v", err)
	}
	if enBase != rep.SHA256 {
		t.Fatalf("la huella de la base (%q) no es la del acuse (%q)", enBase, rep.SHA256)
	}

	// Resubida: duplicado, y el objeto sin cambios (ADR 0006).
	if _, err := ingesta.GuardarReporte(ctx, "caracol-real", "2026-01", fixture); !errors.Is(err, aplicacion.ErrReporteDuplicado) {
		t.Fatalf("se esperaba ErrReporteDuplicado, se obtuvo %v", err)
	}
	trasResubida, err := almacen.Obtener(ctx, rep.ClaveObjeto)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if string(trasResubida) != string(fixture) {
		t.Fatal("la resubida modifico la evidencia")
	}

	// El lote: dos buenas y una malformada.
	mala := usoPendiente("", "", "Radio Novela")
	mala.Modalidad = "radio"

	rechazados, err := ingesta.GuardarUsos(ctx, rep.ID, []aplicacion.UsoPersistido{
		usoPendiente("", "", "La Casa de las Dos Palmas"),
		mala,
		usoPendiente("", "", "Cronica de una Muerte"),
	})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 1 {
		t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
	}

	var canonicos, rechazos int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos WHERE reporte_id = $1`, rep.ID).
		Scan(&canonicos); err != nil {
		t.Fatalf("contar usos: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos_rechazados WHERE reporte_id = $1`, rep.ID).
		Scan(&rechazos); err != nil {
		t.Fatalf("contar rechazos: %v", err)
	}
	if canonicos != 2 || rechazos != 1 {
		t.Fatalf("usos = %d, rechazos = %d; se esperaba 2 y 1", canonicos, rechazos)
	}
}

// Borrar un reporte se lleva sus filas por delante, las canonicas y las
// rechazadas: el log de rechazos cuelga de la misma evidencia, no es un
// almacen paralelo con vida propia.
func TestBorrarUnReporteArrastraSusRechazos(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	mala := usoPendiente("uso-mala", reporteEnero, "Radio Novela")
	mala.RechazoMotivo = "modalidad desconocida"

	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{mala}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM reportes WHERE id = $1`, reporteEnero); err != nil {
		t.Fatalf("borrar el reporte: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos_rechazados`).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Fatalf("quedaron %d rechazos huerfanos", n)
	}
}

// Contexto cancelado: el adaptador tiene que subir el error, no devolver una
// lista vacia como si la tabla estuviera vacia.
func TestUsosSinResolverConContextoCancelado(t *testing.T) {
	s, _ := sembrarReportes(t)

	ctx, cancelar := context.WithCancel(t.Context())
	cancelar()

	if _, err := s.UsosSinResolver(ctx); err == nil {
		t.Fatal("se esperaba error con el contexto cancelado")
	}
}
