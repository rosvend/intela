package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	_, pool := sembrarReportes(t)

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

	// Hace falta una obra de verdad: usos.obra_id es clave foranea. `genero` y
	// `anio` los anadio 00002 como NOT NULL sin DEFAULT, asi que hay que darlos.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, genero, anio, tipo)
		 VALUES ('obra-1', 'La Casa', 'Telenovela', 1994, 'serie')`); err != nil {
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

	rechazados, err := ingesta.GuardarUsos(ctx, rep, []aplicacion.UsoPersistido{
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

// La forma que produce un adaptador de formato (#25): SOLO los campos que
// EXISTEN en el archivo del cliente.
//
// La regresion vive aqui y no solo en `aplicacion` porque el defecto se veia
// en la COLUMNA, no en la estructura: `emisiones` a cero y `fuente` en blanco,
// ya escritos en `usos`. Contra un doble en memoria se comprueba lo que el caso
// de uso devuelve; contra PostgreSQL, lo que queda guardado, que es lo que van
// a leer #26 y el reparto.
//
// Los DEFAULT del esquema (`oni DEFAULT TRUE`, `emisiones DEFAULT 1`) no
// intervienen: insertarUso manda los tres valores siempre, asi que el unico
// sitio donde pueden ponerse es el caso de uso.
func TestIngestaEstampaLosDefaultsDelEsquemaEnLaTabla(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	ingesta := aplicacion.Ingesta{Reportes: s, Almacen: objetos.Disco{Dir: t.TempDir()}}

	rep, err := ingesta.GuardarReporte(ctx, "caracol-crudo", "2026-01",
		[]byte("Titulo,ID_Ficha,Duracion\nLa Casa de las Dos Palmas,1234,52\n"))
	if err != nil {
		t.Fatalf("GuardarReporte: %v", err)
	}

	// Ni Fuente, ni Escalon, ni ONI, ni Emisiones: ninguno es columna de una
	// parrilla, asi que un adaptador que mapee lo que hay los deja en el valor
	// cero de Go.
	recien := aplicacion.UsoPersistido{
		Titulo:      "La Casa de las Dos Palmas",
		Modalidad:   reparto.TV,
		IDsFuente:   "ID_Ficha=1234",
		TipoObra:    "serie",
		DuracionMin: decimal.NewFromInt(52),
	}

	rechazados, err := ingesta.GuardarUsos(ctx, rep, []aplicacion.UsoPersistido{recien})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 0 {
		t.Fatalf("una fila recien parseada no puede salir rechazada: %q", rechazados[0].RechazoMotivo)
	}

	var (
		fuente, escalon string
		oni             bool
		emisiones       int64
		obraID          *string
	)
	err = pool.QueryRow(ctx,
		`SELECT fuente, escalon, oni, emisiones, obra_id FROM usos WHERE reporte_id = $1`,
		rep.ID).Scan(&fuente, &escalon, &oni, &emisiones, &obraID)
	if err != nil {
		t.Fatalf("leer la fila canonica: %v", err)
	}

	if fuente != rep.Fuente {
		t.Errorf("fuente = %q, se esperaba %q: Alias() indexa por fuente y la cadena vacia no casa nunca",
			fuente, rep.Fuente)
	}
	if escalon != "pendiente" {
		t.Errorf("escalon = %q, se esperaba \"pendiente\"", escalon)
	}
	if !oni {
		t.Error("oni = false en una fila sin obra: a la salida de ingesta nadie ha identificado nada")
	}
	if emisiones != 1 {
		t.Errorf("emisiones = %d, se esperaba 1: las emisiones multiplican (RD 9.1.1), "+
			"asi que un cero deja la fila aportando cero puntos sin ningun sintoma", emisiones)
	}
	if obraID != nil {
		t.Errorf("obra_id = %q, se esperaba NULL", *obraID)
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

// H5 contra la base de verdad: una fila que llega ya identificada acaba en
// `usos_rechazados` con su motivo, y NO en `usos`.
//
// Se comprueba en la TABLA y no solo en lo que devuelve el caso de uso porque
// lo que hay que demostrar es estructural: que `usos` no puede recibir por esta
// via una fila con `obra_id` o con un escalon que la cascada nunca decidio. Es
// lo que hace que `usos.evidencia` y `usos.puntaje` -la pregunta 3 del ADR
// 0006- no puedan mentir.
//
// La fila buena que la acompana sigue entrando: el lote no se pierde entero.
func TestIngestaRechazaEnLaTablaLaFilaQueLlegaYaIdentificada(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	ingesta := aplicacion.Ingesta{Reportes: s, Almacen: objetos.Disco{Dir: t.TempDir()}}

	rep, err := ingesta.GuardarReporte(ctx, "caracol-manipulado", "2026-01",
		[]byte("Titulo,ID_Ficha\nLa Casa de las Dos Palmas,1234\n"))
	if err != nil {
		t.Fatalf("GuardarReporte: %v", err)
	}

	// La que trae obra_id: un obra_id que no existe en `obras` es ademas una
	// violacion de clave foranea que reventaria el INSERT del lote ENTERO.
	conObra := usoPendiente("", "", "Viene Con Obra")
	conObra.ObraID = "obra-que-no-existe"
	conObra.ONI = false

	// La que trae un escalon adelantado: describe una decision de la cascada
	// que nunca ocurrio.
	conEscalon := usoPendiente("", "", "Viene Resuelta Por Alias")
	conEscalon.Escalon = "alias"
	conEscalon.Evidencia = "alias inventado por el adaptador"

	// La que trae SOLO evidencia, y es la que se colaba: sin obra_id y con el
	// escalon vacio, ninguna de las otras dos reglas la ve, el relleno la deja
	// en "pendiente" y su evidencia se escribia VERBATIM en la columna. Queda
	// una fila que dice como se reconocio una obra que nadie reconocio, y es
	// indistinguible de una que si (pregunta 3 del ADR 0006).
	conEvidencia := usoPendiente("", "", "Dice Como Se Reconocio")
	conEvidencia.Escalon = ""
	conEvidencia.Evidencia = "alias caracol/ID_Ficha=1234"

	rechazados, err := ingesta.GuardarUsos(ctx, rep, []aplicacion.UsoPersistido{
		conObra,
		conEscalon,
		conEvidencia,
		usoPendiente("", "", "La Casa de las Dos Palmas"),
	})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 3 {
		t.Fatalf("se esperaban 3 rechazos, llegaron %d", len(rechazados))
	}

	var canonicos, rechazos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos WHERE reporte_id = $1`, rep.ID).Scan(&canonicos); err != nil {
		t.Fatalf("contar usos: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos_rechazados WHERE reporte_id = $1`, rep.ID).Scan(&rechazos); err != nil {
		t.Fatalf("contar rechazos: %v", err)
	}
	if canonicos != 1 || rechazos != 3 {
		t.Fatalf("usos = %d, rechazos = %d; se esperaba 1 y 3", canonicos, rechazos)
	}

	// Y en `usos` no quedo NINGUNA fila con obra_id, con escalon adelantado ni
	// con evidencia: las tres formas de decir "esto ya se identifico".
	var intrusas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos
		  WHERE reporte_id = $1
		    AND (obra_id IS NOT NULL OR escalon <> 'pendiente' OR evidencia <> '')`,
		rep.ID).Scan(&intrusas); err != nil {
		t.Fatalf("buscar filas ya identificadas: %v", err)
	}
	if intrusas != 0 {
		t.Fatalf("%d filas identificadas se colaron en usos: la cascada es el unico camino", intrusas)
	}

	// El motivo tiene que decir QUE campo la aparto: es lo que sirve para
	// volver a pedirle al cliente exactamente eso (ADR 0016).
	//
	// Se compara el motivo ENTERO y fila a fila, no "que en algun sitio aparezca
	// la palabra obra_id". Con la version por substring esta prueba pasaba
	// aunque se borrara la regla de obra_id: la fila caia en la comprobacion de
	// coherencia oni/obra, que la rechazaba igual pero con el motivo contrario
	// -"marcada como identificada y sin obra_id", de una fila que traia
	// obra_id-, y el substring casaba con las dos. Los recuentos de arriba
	// tampoco cambiaban. La prueba solo demostraba "acaba en el log", nunca
	// "acaba en el log por lo que decimos".
	//
	// GuardarUsos deriva el id de la POSICION en el lote, asi que cada motivo se
	// mira contra la fila exacta que lo produjo.
	motivos := map[string]string{}
	filas, err := pool.Query(ctx,
		`SELECT id, motivo FROM usos_rechazados WHERE reporte_id = $1`, rep.ID)
	if err != nil {
		t.Fatalf("leer motivos: %v", err)
	}
	defer filas.Close()

	for filas.Next() {
		var id, m string
		if err := filas.Scan(&id, &m); err != nil {
			t.Fatalf("escanear motivo: %v", err)
		}
		motivos[id] = m
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("recorrer motivos: %v", err)
	}

	esperados := map[string]string{
		rep.ID + "-0": "obra_id en la ingesta: identificar es trabajo de la cascada (ADR 0007)",
		rep.ID + "-1": `escalon "alias" en la ingesta: solo sale "pendiente" de aqui`,
		rep.ID + "-2": "evidencia en la ingesta: como se reconocio lo escribe la cascada (ADR 0007)",
	}
	for id, quiero := range esperados {
		if motivos[id] != quiero {
			t.Errorf("motivo de %q = %q, se esperaba %q", id, motivos[id], quiero)
		}
	}
}

// El borde de H5 contra la base de verdad: un obra_id de solo blancos es "sin
// obra", y no cuesta ni un motivo falso ni el lote.
//
// # Por que esta prueba tiene que vivir contra PostgreSQL
//
// El defecto tenia DOS sintomas y el segundo solo existe aqui:
//
//  1. En Go, la fila se rechazaba con el motivo de H5 -"obra_id en la
//     ingesta"- acusando de traer una obra a una fila que no traia ninguna.
//     Eso se ve con un doble en memoria.
//  2. En cuanto se arregla SOLO en Go, la fila pasa como vacia y llega al
//     INSERT con el blanco intacto. El NULLIF del INSERT compara con la cadena
//     vacia LITERAL, asi que no lo anula, y el CHECK uso_resuelto_tiene_obra
//     la rechaza con un 23514 DENTRO de la transaccion del lote. No se pierde
//     esa fila: se pierden TODAS. Ese sintoma no existe contra un doble -no
//     hay CHECK que violar- y es el mas caro de los dos: el reporte ya quedo
//     escrito, asi que reintentar el mismo archivo choca con
//     ErrReporteDuplicado.
//
// El lote lleva las tres clases de fila a la vez -normal, con blanco, y con
// obra de verdad- porque lo que hay que demostrar es que conviven: la del
// blanco entra como cualquier otra, la de la obra se aparta con su motivo
// verdadero, y ninguna de las dos se lleva por delante a las buenas.
func TestIngestaNoPierdeElLotePorUnObraIDEnBlanco(t *testing.T) {
	const motivoH5 = "obra_id en la ingesta: identificar es trabajo de la cascada (ADR 0007)"

	// El NBSP (U+00A0) no es rebuscado: es con lo que Excel rellena las celdas
	// que se ven vacias, y es el que separa un recorte hecho a mano de
	// strings.TrimSpace, cuya definicion de blanco es unicode.IsSpace.
	blancos := map[string]string{
		"espacio":          " ",
		"tabulador":        "\t",
		"salto de linea":   "\n",
		"nbsp":             " ",
		"varios mezclados": " \t\r\n  ",
	}

	for nombre, blanco := range blancos {
		t.Run(nombre, func(t *testing.T) {
			s, pool := sembrarReportes(t)
			ctx := t.Context()

			ingesta := aplicacion.Ingesta{Reportes: s, Almacen: objetos.Disco{Dir: t.TempDir()}}

			rep, err := ingesta.GuardarReporte(ctx, "caracol-blancos", "2026-01",
				[]byte("Titulo,ID_Ficha\nLa Casa de las Dos Palmas,1234\n"))
			if err != nil {
				t.Fatalf("GuardarReporte: %v", err)
			}

			// La del blanco: no tiene obra ninguna, solo la celda rellena. ONI a
			// false, que es el valor cero de Go y lo que deja un adaptador de
			// formato (#25) -`oni` no es columna de ninguna parrilla-. Con el
			// true de usoPendiente, la asercion de mas abajo pasaria sin que la
			// guarda que estampa ONI llegara a ejecutarse.
			conBlanco := usoPendiente("", "", "Blanco En Obra")
			conBlanco.ObraID = blanco
			conBlanco.ONI = false

			// La que SI trae obra, con blancos alrededor: sigue siendo H5. El
			// obra_id no existe en `obras`, que es lo que la haria reventar por
			// clave foranea si se colara hasta `usos`.
			conObra := usoPendiente("", "", "Viene Con Obra")
			conObra.ObraID = blanco + "obra-que-no-existe" + blanco
			conObra.ONI = false

			rechazados, err := ingesta.GuardarUsos(ctx, rep, []aplicacion.UsoPersistido{
				usoPendiente("", "", "Buena Uno"),
				conBlanco,
				conObra,
				usoPendiente("", "", "Buena Dos"),
			})
			// El primer sintoma que se ve si el arreglo esta a medias: el lote
			// entero cae con 23514 y aqui llega un error.
			if err != nil {
				t.Fatalf("un blanco en una fila no puede tumbar el lote: %v", err)
			}
			if len(rechazados) != 1 || rechazados[0].Titulo != "Viene Con Obra" {
				t.Fatalf("se esperaba 1 rechazo, el de la obra de verdad: %+v", rechazados)
			}
			if rechazados[0].RechazoMotivo != motivoH5 {
				t.Fatalf("motivo = %q, se esperaba %q", rechazados[0].RechazoMotivo, motivoH5)
			}

			// Tres canonicas -las dos buenas y la del blanco- y un rechazo. Con
			// el defecto original salen 2 y 2: la del blanco se iba al log con el
			// motivo falso. Con el arreglo a medias salen 0 y 0: se pierde todo.
			var canonicos, rechazos int
			if err := pool.QueryRow(ctx,
				`SELECT count(*) FROM usos WHERE reporte_id = $1`, rep.ID).Scan(&canonicos); err != nil {
				t.Fatalf("contar usos: %v", err)
			}
			if err := pool.QueryRow(ctx,
				`SELECT count(*) FROM usos_rechazados WHERE reporte_id = $1`, rep.ID).Scan(&rechazos); err != nil {
				t.Fatalf("contar rechazos: %v", err)
			}
			if canonicos != 3 || rechazos != 1 {
				t.Fatalf("usos = %d, rechazos = %d; se esperaba 3 y 1", canonicos, rechazos)
			}

			// Y la del blanco quedo en la COLUMNA como NULL, no como un blanco:
			// es lo que dice el CHECK y lo que la cascada (ADR 0007) va a buscar.
			// `obra_id = ''` no puede existir -es referencia a obras(id)-, pero
			// un blanco no vacio si pasaria la clave foranea el dia que exista una
			// obra con ese id, y entonces la fila mentiria en silencio.
			var (
				obraID  *string
				oni     bool
				escalon string
			)
			err = pool.QueryRow(ctx,
				`SELECT obra_id, oni, escalon FROM usos WHERE id = $1`, rep.ID+"-1").
				Scan(&obraID, &oni, &escalon)
			if err != nil {
				t.Fatalf("leer la fila del blanco: %v", err)
			}
			if obraID != nil {
				t.Errorf("obra_id = %q, se esperaba NULL: un blanco es sin obra", *obraID)
			}
			if !oni || escalon != "pendiente" {
				t.Errorf("oni = %v, escalon = %q: la fila del blanco tiene que salir sin identificar",
					oni, escalon)
			}
		})
	}
}

// H4b contra la base de verdad: un objeto desgarrado bajo la clave NO puede
// acabar certificado por una fila de `reportes`.
//
// La regresion vive aqui y no solo en `aplicacion` porque el dano se veia en la
// TABLA: una fila de acuse declarando un SHA-256 que el objeto real no tiene.
// Eso es evidencia falsa (ADR 0006) y ademas no se recupera sola -la resubida
// choca con el UNIQUE (sha256, fuente) y sale ErrReporteDuplicado-, asi que
// contra un doble en memoria no se ve lo que de verdad importa: que la fila no
// llegue a existir.
//
// El objeto truncado se planta a mano, que es como queda tras un fallo de
// escritura, una copia restaurada a medias o un kill -9.
func TestGuardarReporteNoCertificaUnObjetoTruncadoDeLaBoveda(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	dir := t.TempDir()
	almacen := objetos.Disco{Dir: dir}
	ingesta := aplicacion.Ingesta{Reportes: s, Almacen: almacen}

	fixture := []byte("Titulo,ID_Ficha,Fecha,Duracion\n" +
		"La Casa de las Dos Palmas,1234,20260115,52\n")

	// La clave que le va a tocar a esta entrega, ocupada por un resto truncado.
	sha := sha256.Sum256(fixture)
	clave := "reportes/" + hex.EncodeToString(sha[:])
	if err := almacen.Poner(ctx, clave, fixture[:20]); err != nil {
		t.Fatalf("plantar el objeto desgarrado: %v", err)
	}

	_, err := ingesta.GuardarReporte(ctx, "caracol-desgarrado", "2026-01", fixture)
	if !errors.Is(err, aplicacion.ErrEvidenciaCorrupta) {
		t.Fatalf("se esperaba ErrEvidenciaCorrupta, se obtuvo %v", err)
	}

	// Lo que cierra el agujero: en la tabla no quedo acuse ninguno.
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM reportes WHERE fuente = $1`, "caracol-desgarrado").
		Scan(&n); err != nil {
		t.Fatalf("contar reportes: %v", err)
	}
	if n != 0 {
		t.Fatalf("quedo %d acuse certificando una huella que el objeto no tiene", n)
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

// Un blanco en UNA celda de UNA fila no puede costar la entrega, y estos tres
// campos lo costaban.
//
// Vive aqui y no solo en `aplicacion` porque el dano de estos casos SOLO existe
// contra la base: la restriccion que revienta no la tiene ningun doble en
// memoria, y lo que se pierde no es la fila del blanco sino TODAS las del lote
// -la escritura es UNA transaccion a proposito-. Y el reporte ya quedo escrito
// antes, asi que reintentar el mismo archivo choca con ErrReporteDuplicado: la
// entrega no se recupera sin cirugia en la base.
//
// Cada subprueba lleva el blanco acompanado de filas buenas, porque lo que hay
// que demostrar es justamente que conviven.
func TestIngestaNoPierdeElLotePorUnBlancoEnUnaFila(t *testing.T) {
	// El NBSP (U+00A0) es con lo que Excel rellena las celdas que se ven
	// vacias, y es el que separa un recorte hecho a mano de strings.TrimSpace,
	// cuya definicion de blanco es unicode.IsSpace.
	const nbsp = "\u00a0"

	blancosDeCelda := map[string]string{
		"espacios":       "   ",
		"tabulador":      "\t",
		"salto de linea": "\n",
		"nbsp":           nbsp,
		"mezclados":      " \t" + nbsp + "\r\n ",
	}

	// rechazo_motivo es el campo con DOS sintomas, y no el mismo para todos los
	// blancos. Un motivo no vacio hace que el adaptador rutee la fila al log de
	// rechazos sin validarla, y alli espera CHECK (btrim(motivo) <> ''):
	//
	//   - Con espacios, `btrim` los quita, el CHECK falla y salta un 23514
	//     DENTRO de la transaccion del lote. Se pierden TODAS las filas.
	//   - Con un tabulador, un salto de linea o el NBSP, NO salta: `btrim` sin
	//     segundo argumento quita SOLO espacios, asi que el motivo sobrevive al
	//     CHECK. El sintoma es entonces silencioso y peor de encontrar: una
	//     fila buena queda en `usos_rechazados` con un motivo en blanco, o sea
	//     fuera de `usos`, o sea sin ponderar la bolsa. Nadie recibe un error,
	//     y el archivo aparece completo en el acuse.
	//
	// Los dos se arreglan con lo mismo -recortar el motivo una vez, arriba- y
	// los dos se comprueban aqui: `usos` = 3 y `usos_rechazados` = 0.
	t.Run("motivo en blanco", func(t *testing.T) {
		for nombre, blanco := range blancosDeCelda {
			t.Run(nombre, func(t *testing.T) {
				s, pool := sembrarReportes(t)
				ctx := t.Context()
				ingesta := aplicacion.Ingesta{Reportes: s, Almacen: objetos.Disco{Dir: t.TempDir()}}

				rep, err := ingesta.GuardarReporte(ctx, "caracol-motivo", "2026-01",
					[]byte("Titulo,ID_Ficha\nLa Casa de las Dos Palmas,1234\n"))
				if err != nil {
					t.Fatalf("GuardarReporte: %v", err)
				}

				conBlanco := usoPendiente("", "", "Buena Con Motivo En Blanco")
				conBlanco.RechazoMotivo = blanco

				rechazados, err := ingesta.GuardarUsos(ctx, rep, []aplicacion.UsoPersistido{
					usoPendiente("", "", "Buena Uno"),
					conBlanco,
					usoPendiente("", "", "Buena Dos"),
				})
				if err != nil {
					t.Fatalf("un motivo en blanco no puede tumbar el lote: %v", err)
				}
				if len(rechazados) != 0 {
					t.Fatalf("ninguna de las tres es rechazable, y una fila en "+
						"`usos_rechazados` no pondera: motivo %q",
						rechazados[0].RechazoMotivo)
				}
				contar(t, ctx, pool, rep.ID, 3, 0)
			})
		}
	})

	// 23505: la escapatoria `if u.ID == ""` de GuardarUsos se salta con un
	// blanco, y las dos filas llegan al INSERT con el mismo id literal. La
	// clave primaria no tiene btrim que la salve: cualquier blanco vale.
	t.Run("id en blanco", func(t *testing.T) {
		for nombre, blanco := range blancosDeCelda {
			t.Run(nombre, func(t *testing.T) {
				s, pool := sembrarReportes(t)
				ctx := t.Context()
				ingesta := aplicacion.Ingesta{Reportes: s, Almacen: objetos.Disco{Dir: t.TempDir()}}

				rep, err := ingesta.GuardarReporte(ctx, "caracol-id", "2026-01",
					[]byte("Titulo,ID_Ficha\nLa Casa de las Dos Palmas,1234\n"))
				if err != nil {
					t.Fatalf("GuardarReporte: %v", err)
				}

				primera := usoPendiente(blanco, "", "Buena Uno")
				segunda := usoPendiente(blanco, "", "Buena Dos")

				if _, err := ingesta.GuardarUsos(ctx, rep,
					[]aplicacion.UsoPersistido{primera, segunda}); err != nil {
					t.Fatalf("dos ids en blanco no pueden tumbar el lote: %v", err)
				}
				contar(t, ctx, pool, rep.ID, 2, 0)

				// Y los ids que quedan son los derivados, o sea la entrega y la
				// LINEA de las que salio cada fila (ADR 0006).
				for id, titulo := range map[string]string{
					rep.ID + "-0": "Buena Uno",
					rep.ID + "-1": "Buena Dos",
				} {
					var leido string
					if err := pool.QueryRow(ctx, `SELECT titulo FROM usos WHERE id = $1`, id).
						Scan(&leido); err != nil {
						t.Fatalf("el id derivado %q no esta en la tabla: %v", id, err)
					}
					if leido != titulo {
						t.Errorf("la fila %q es %q, se esperaba %q", id, leido, titulo)
					}
				}
			})
		}
	})

	// Este no revienta la base: sale del caso de uso con un motivo FALSO
	// -`escalon " " en la ingesta`, cuando lo que llego era una celda vacia- y
	// la fila buena acaba en el log de rechazos, o sea sin ponderar. La forma
	// se ve contra la COLUMNA: `escalon` tiene que quedar en 'pendiente', que
	// es lo que la cascada (ADR 0007) busca con UsosSinResolver.
	t.Run("escalon en blanco", func(t *testing.T) {
		for nombre, blanco := range blancosDeCelda {
			t.Run(nombre, func(t *testing.T) {
				s, pool := sembrarReportes(t)
				ctx := t.Context()
				ingesta := aplicacion.Ingesta{Reportes: s, Almacen: objetos.Disco{Dir: t.TempDir()}}

				rep, err := ingesta.GuardarReporte(ctx, "caracol-escalon", "2026-01",
					[]byte("Titulo,ID_Ficha\nLa Casa de las Dos Palmas,1234\n"))
				if err != nil {
					t.Fatalf("GuardarReporte: %v", err)
				}

				conBlanco := usoPendiente("", "", "Buena Con Escalon En Blanco")
				conBlanco.Escalon = blanco

				rechazados, err := ingesta.GuardarUsos(ctx, rep, []aplicacion.UsoPersistido{
					usoPendiente("", "", "Buena Uno"),
					conBlanco,
				})
				if err != nil {
					t.Fatalf("GuardarUsos: %v", err)
				}
				if len(rechazados) != 0 {
					t.Fatalf("una celda vacia no es un escalon adelantado: motivo %q",
						rechazados[0].RechazoMotivo)
				}
				contar(t, ctx, pool, rep.ID, 2, 0)

				var escalon string
				if err := pool.QueryRow(ctx,
					`SELECT escalon FROM usos WHERE id = $1`, rep.ID+"-1").Scan(&escalon); err != nil {
					t.Fatalf("leer la fila del blanco: %v", err)
				}
				if escalon != "pendiente" {
					t.Errorf("escalon = %q, se esperaba \"pendiente\": "+
						"UsosSinResolver filtra por ese valor", escalon)
				}
			})
		}
	})
}

// contar comprueba de una vez las dos mitades de un lote: las filas canonicas
// de `usos` y las del log de rechazos. Es la asercion que distingue "se aparto
// una fila" de "se perdio la entrega", y se repite en cada prueba de lote.
func contar(t *testing.T, ctx context.Context, pool *pgxpool.Pool, reporteID string, canonicos, rechazos int) {
	t.Helper()

	var hayCanonicos, hayRechazos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos WHERE reporte_id = $1`, reporteID).Scan(&hayCanonicos); err != nil {
		t.Fatalf("contar usos: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos_rechazados WHERE reporte_id = $1`, reporteID).Scan(&hayRechazos); err != nil {
		t.Fatalf("contar rechazos: %v", err)
	}
	if hayCanonicos != canonicos || hayRechazos != rechazos {
		t.Fatalf("usos = %d, rechazos = %d; se esperaba %d y %d",
			hayCanonicos, hayRechazos, canonicos, rechazos)
	}
}
