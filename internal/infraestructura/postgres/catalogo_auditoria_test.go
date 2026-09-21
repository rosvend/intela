package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// El asiento del ABM del catalogo, contra PostgreSQL de verdad (issue #91).
//
// # Por que estas pruebas viven en este paquete y no en `aplicacion`
//
// Porque lo que hay que probar no es que [aplicacion.Catalogo] LLAME a
// Asentar -eso ya lo comprueban los dobles de aplicacion/catalogo_test.go-
// sino que la obra y su asiento comparten commit y ROLLBACK. Un doble no
// puede probar eso: la transaccion la abre el adaptador, y el unico fallo que
// importa -- el asiento que revienta despues de una escritura confirmada -- solo
// existe si hay una base que confirmar. El ADR 0010 lo pide por su nombre, y
// el harness de testhelp es el mismo que usa el resto del paquete.
//
// El fallo del asiento se fuerza con un actor inexistente: `asientos.actor_id`
// referencia `usuarios(id)`, asi que el INSERT en `asientos` falla SIN tocar
// ninguna otra tabla. Es la misma via que usa
// TestGuardarRevierteLaVersionSiElAsientoFalla en declaraciones_test.go.

const (
	actorInexistente = "usr-que-no-existe"
	obraNueva        = "obra-recien-nacida"
)

// instanteDelAsiento es el `cuando` del PRIMER asiento de cada prueba.
// Truncado a microsegundo porque TIMESTAMPTZ no guarda mas: sin truncar, el
// valor leido nunca seria igual al escrito.
var instanteDelAsiento = time.Date(2026, 3, 14, 15, 9, 26, 535_000, time.UTC)

// pasoDelReloj es lo que avanza el reloj entre un asiento y el siguiente.
const pasoDelReloj = time.Second

// relojEnPasos devuelve instantes distintos y PREDECIBLES: el primero es
// instanteDelAsiento y cada llamada suma pasoDelReloj.
//
// No es reloj.Fijo, y la diferencia importa. [Store.De] ordena por
// `cuando, id`, y el id es un gen_random_uuid(): dos asientos escritos en el
// MISMO instante quedan en un orden estable entre consultas -que es lo que el
// ADR 0005 exige- pero no necesariamente cronologico. Un reloj congelado
// pondria las tres correcciones de una obra en el mismo microsegundo y la
// historia saldria barajada, que no es lo que le pasa a nadie en produccion:
// alli el puerto es reloj.Sistema y dos peticiones HTTP no caen en el mismo
// microsegundo. Este doble reproduce eso sin renunciar a ser determinista.
type relojEnPasos struct{ llamadas int }

func (r *relojEnPasos) Ahora() time.Time {
	ahora := instanteDelAsiento.Add(time.Duration(r.llamadas) * pasoDelReloj)
	r.llamadas++
	return ahora
}

var _ aplicacion.Reloj = (*relojEnPasos)(nil)

// catalogoConBitacora cablea el caso de uso como lo hace cmd/api: el mismo
// *Store satisface los cuatro puertos, y el nucleo sigue viendo cuatro.
func catalogoConBitacora(s *Store) aplicacion.Catalogo {
	return aplicacion.Catalogo{
		Obras:         s.CatalogoObras(),
		Declaraciones: s,
		Bitacora:      s,
		Unidad:        s,
		Reloj:         &relojEnPasos{},
	}
}

// metadatosDePrueba es el bloque que se da de alta y luego se corrige.
func metadatosDePrueba(ajustar ...func(*repertorio.Metadatos)) repertorio.Metadatos {
	m := repertorio.Metadatos{
		Titulo: "Senoritas de Uribe",
		Genero: "Comedia",
		Anio:   1997,
		Tipo:   repertorio.TipoSerie,
		IDA:    "IDA-7",
		Coautores: []repertorio.Coautor{
			{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: repertorio.RolGuionista},
		},
	}
	for _, f := range ajustar {
		f(&m)
	}
	return m
}

func asientoObra(t *testing.T, a aplicacion.Asiento) aplicacion.AsientoObra {
	t.Helper()

	var p aplicacion.AsientoObra
	if err := json.Unmarshal(a.Payload, &p); err != nil {
		t.Fatalf("el payload del asiento %q no es JSON: %v", a.Hecho, err)
	}
	return p
}

// hechos es el atajo para comparar una historia entera de un vistazo.
func hechos(asientos []aplicacion.Asiento) []string {
	out := make([]string, 0, len(asientos))
	for _, a := range asientos {
		out = append(out, a.Hecho)
	}
	return out
}

// ---------------------------------------------------------------------------
// Alta

// Criterio 1: el alta asienta, y en la misma transaccion.
func TestRegistrarObraAsientaElAlta(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if _, err := catalogoConBitacora(s).
		RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}

	// La obra esta, con sus coautores: la lectura la reconstruye por el mismo
	// constructor del dominio, asi que esto tambien prueba que no quedo a
	// medias dentro de la unidad.
	if _, err := s.CatalogoObras().PorID(ctx, obraNueva); err != nil {
		t.Fatalf("PorID despues del alta: %v", err)
	}

	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento, hubo %d", len(asientos))
	}
	a := asientos[0]
	if a.Hecho != aplicacion.HechoObraRegistrada {
		t.Fatalf("hecho = %q, se esperaba %q", a.Hecho, aplicacion.HechoObraRegistrada)
	}
	if a.ActorID != usuarioAdmin {
		t.Fatalf("actor = %q, se esperaba %q", a.ActorID, usuarioAdmin)
	}
	if !a.Cuando.Equal(instanteDelAsiento) {
		t.Fatalf("cuando = %s, se esperaba %s", a.Cuando, instanteDelAsiento)
	}
	// El id lo pone la base (gen_random_uuid): un asiento sin identificador no
	// se puede referenciar desde el asiento que lo corrija.
	if a.ID == "" {
		t.Fatal("el asiento volvio sin identificador")
	}

	p := asientoObra(t, a)
	if p.Antes != nil {
		t.Fatalf("un alta no tiene estado anterior: %+v", p.Antes)
	}
	if p.Despues.Titulo != "Senoritas de Uribe" || p.Despues.Anio != 1997 {
		t.Fatalf("payload.despues = %+v", p.Despues)
	}
	if len(p.Despues.Coautores) != 1 || p.Despues.Coautores[0].IPI != "IPI-00000001" {
		t.Fatalf("payload.despues.coautores = %+v", p.Despues.Coautores)
	}
}

// Criterio 2, que es el que justifica la unidad de trabajo entera: si el
// asiento falla, la obra NO queda escrita. Sin la unidad, Registrar habria
// confirmado su propia transaccion y este PorID encontraria una obra que nadie
// puede explicar.
func TestRegistrarObraRevierteLaObraSiElAsientoFalla(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	_, err := catalogoConBitacora(s).
		RegistrarObra(ctx, obraNueva, metadatosDePrueba(), actorInexistente)
	if err == nil {
		t.Fatal("se esperaba que el asiento fallara por el actor inexistente")
	}

	if _, err := s.CatalogoObras().PorID(ctx, obraNueva); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("la obra quedo huerfana: PorID devolvio %v", err)
	}
	// Y tampoco quedaron los coautores, que es la otra mitad de la escritura.
	var coautores int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM obra_coautores WHERE obra_id = $1`, obraNueva).
		Scan(&coautores); err != nil {
		t.Fatalf("contar coautores: %v", err)
	}
	if coautores != 0 {
		t.Fatalf("quedaron %d coautores de una obra que no existe", coautores)
	}
	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("no se esperaba ningun asiento: %+v", asientos)
	}
}

// ---------------------------------------------------------------------------
// Correccion

// Criterio 3: del asiento de una correccion se reconstruye QUE cambio. El
// bloque se reemplaza entero, asi que el estado anterior no queda en ninguna
// tabla -- `obras` se sobreescribe y `obra_coautores` se borra --: o esta en el
// payload o no esta en ningun sitio.
func TestActualizarMetadatosObraAsientaQueCambio(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	cat := catalogoConBitacora(s)

	if _, err := cat.RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}

	corregidos := metadatosDePrueba(func(m *repertorio.Metadatos) {
		m.Titulo = "Senoritas de Uribe (restaurada)"
		m.Anio = 1998
		m.Coautores = append(m.Coautores, repertorio.Coautor{
			Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista,
		})
	})
	if _, err := cat.ActualizarMetadatosObra(ctx, obraNueva, corregidos, usuarioAdmin); err != nil {
		t.Fatalf("ActualizarMetadatosObra: %v", err)
	}

	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 2 {
		t.Fatalf("se esperaban 2 asientos (alta y correccion), hubo %d: %v",
			len(asientos), hechos(asientos))
	}

	a := asientos[1]
	if a.Hecho != aplicacion.HechoObraCorregida {
		t.Fatalf("hecho = %q, se esperaba %q", a.Hecho, aplicacion.HechoObraCorregida)
	}

	p := asientoObra(t, a)
	if p.Antes == nil {
		t.Fatal("una correccion sin estado anterior no explica nada")
	}
	// El "antes" es lo que la base tenia, no lo que llego en la peticion: es
	// lo que hace reconstruible el estado que esta correccion sustituyo.
	if p.Antes.Titulo != "Senoritas de Uribe" || p.Antes.Anio != 1997 {
		t.Fatalf("payload.antes = %+v", *p.Antes)
	}
	if len(p.Antes.Coautores) != 1 {
		t.Fatalf("payload.antes.coautores = %+v", p.Antes.Coautores)
	}
	if p.Despues.Titulo != "Senoritas de Uribe (restaurada)" || p.Despues.Anio != 1998 {
		t.Fatalf("payload.despues = %+v", p.Despues)
	}
	if len(p.Despues.Coautores) != 2 {
		t.Fatalf("payload.despues.coautores = %+v", p.Despues.Coautores)
	}
	quiero := []string{"titulo", "anio", "coautores"}
	if !slices.Equal(p.Cambios, quiero) {
		t.Fatalf("payload.cambios = %v, se esperaba %v", p.Cambios, quiero)
	}
}

// Criterio 2 por el otro lado: si el asiento de la correccion falla, la obra
// se queda como estaba. Es el caso peor de los dos, porque aqui el rollback
// tiene que reponer coautores que el UPDATE ya habia borrado.
func TestActualizarMetadatosObraRevierteLosMetadatosSiElAsientoFalla(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	cat := catalogoConBitacora(s)

	if _, err := cat.RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}

	corregidos := metadatosDePrueba(func(m *repertorio.Metadatos) {
		m.Titulo = "Titulo que no debe quedar"
		m.Coautores = []repertorio.Coautor{
			{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista},
		}
	})
	if _, err := cat.ActualizarMetadatosObra(ctx, obraNueva, corregidos, actorInexistente); err == nil {
		t.Fatal("se esperaba que el asiento fallara por el actor inexistente")
	}

	obra, err := s.CatalogoObras().PorID(ctx, obraNueva)
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	m := obra.Metadatos()
	if m.Titulo != "Senoritas de Uribe" {
		t.Fatalf("titulo = %q: la correccion se confirmo pese a que el asiento fallo", m.Titulo)
	}
	if len(m.Coautores) != 1 || m.Coautores[0].IPI != "IPI-00000001" {
		t.Fatalf("coautores = %+v: el rollback no repuso los que el UPDATE borro", m.Coautores)
	}

	// Sigue estando SOLO el asiento del alta.
	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if quiero := []string{aplicacion.HechoObraRegistrada}; !slices.Equal(hechos(asientos), quiero) {
		t.Fatalf("historia = %v, se esperaba %v", hechos(asientos), quiero)
	}
}

// Una obra que no esta en el catalogo no se da de alta por la puerta de atras
// ni deja asiento: un PATCH que insertara convertiria un id mal escrito en una
// obra fantasma, y contra el catalogo resuelve todo el matching.
func TestActualizarMetadatosObraInexistenteNoAsienta(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	_, err := catalogoConBitacora(s).
		ActualizarMetadatosObra(ctx, "obra-fantasma", metadatosDePrueba(), usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}

	asientos, err := s.De(ctx, aplicacion.RefObra, "obra-fantasma")
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("no se esperaba ningun asiento: %+v", asientos)
	}
}

// ---------------------------------------------------------------------------
// La lectura

// Criterio 4: De(ctx, "obra", id) devuelve la historia de una obra del
// catalogo, del hecho mas antiguo al mas nuevo.
//
// La declaracion de la obra comparte ref_tipo y ref_id a proposito: un auditor
// que pregunta por una obra quiere su historia ENTERA -- alta, correcciones y
// declaraciones -- en el orden en que ocurrio, no tres consultas que luego
// tenga que entrelazar a mano.
func TestDeDevuelveLaHistoriaCompletaDeUnaObra(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	cat := catalogoConBitacora(s)

	if _, err := cat.RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}
	primera := metadatosDePrueba(func(m *repertorio.Metadatos) { m.Genero = "Drama" })
	if _, err := cat.ActualizarMetadatosObra(ctx, obraNueva, primera, usuarioAdmin); err != nil {
		t.Fatalf("primera correccion: %v", err)
	}
	segunda := metadatosDePrueba(func(m *repertorio.Metadatos) {
		m.Genero = "Drama"
		m.IMDB = "tt7654321"
	})
	if _, err := cat.ActualizarMetadatosObra(ctx, obraNueva, segunda, usuarioAdmin); err != nil {
		t.Fatalf("segunda correccion: %v", err)
	}

	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	quiero := []string{
		aplicacion.HechoObraRegistrada,
		aplicacion.HechoObraCorregida,
		aplicacion.HechoObraCorregida,
	}
	if !slices.Equal(hechos(asientos), quiero) {
		t.Fatalf("historia = %v, se esperaba %v", hechos(asientos), quiero)
	}

	// La cadena es reconstruible: el "despues" de cada asiento es el "antes"
	// del siguiente. Es la propiedad de la que cuelga poder explicar por que
	// una obra es como es hoy sin mirar ninguna otra tabla.
	for i := 1; i < len(asientos); i++ {
		previo, actual := asientoObra(t, asientos[i-1]), asientoObra(t, asientos[i])
		if actual.Antes == nil {
			t.Fatalf("el asiento %d no trae estado anterior", i)
		}
		if previo.Despues.Titulo != actual.Antes.Titulo ||
			previo.Despues.Genero != actual.Antes.Genero ||
			previo.Despues.IMDB != actual.Antes.IMDB {
			t.Fatalf("la cadena se rompe en el asiento %d: despues=%+v antes=%+v",
				i, previo.Despues, *actual.Antes)
		}
	}

	// Y por el caso de uso, que es por donde entra un lector de verdad.
	porElCasoDeUso, err := cat.HistorialObra(ctx, obraNueva)
	if err != nil {
		t.Fatalf("HistorialObra: %v", err)
	}
	if !slices.Equal(hechos(porElCasoDeUso), quiero) {
		t.Fatalf("HistorialObra = %v, se esperaba %v", hechos(porElCasoDeUso), quiero)
	}
}

// Una obra sin asientos devuelve una lista vacia y ningun error: un conjunto
// vacio NO es ErrNoEncontrado (ver doc.go).
func TestHistorialDeUnaObraSinAsientosEsVacio(t *testing.T) {
	s, _ := sembrar(t)

	asientos, err := catalogoConBitacora(s).HistorialObra(t.Context(), obraCompleta)
	if err != nil {
		t.Fatalf("HistorialObra: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("se esperaba ninguno, hubo %d", len(asientos))
	}
}

// ---------------------------------------------------------------------------
// La unidad de trabajo

// EnUnidad es reentrante: una unidad abierta dentro de otra es la MISMA. Sin
// esto, la de dentro pediria una segunda conexion al pool y esperaria un
// cerrojo que solo suelta la de fuera al confirmar -- un interbloqueo, que con
// pool_max_conns=2 se ve como una prueba colgada y no como un error.
func TestEnUnidadEsReentrante(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	cat := catalogoConBitacora(s)

	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		_, err := cat.RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin)
		return err
	})
	if err != nil {
		t.Fatalf("EnUnidad anidada: %v", err)
	}
	if _, err := s.CatalogoObras().PorID(ctx, obraNueva); err != nil {
		t.Fatalf("PorID: %v", err)
	}
}

// Y lo que la reentrancia no puede costar: el error de dentro sigue
// revirtiendo la unidad de fuera entera.
func TestEnUnidadRevierteLaAnidadaConLaDeFuera(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	cat := catalogoConBitacora(s)
	fallo := errors.New("el caso de uso de fuera se arrepintio")

	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		if _, err := cat.RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin); err != nil {
			return err
		}
		return fallo
	})
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error de fuera, se obtuvo %v", err)
	}

	if _, err := s.CatalogoObras().PorID(ctx, obraNueva); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("la obra sobrevivio al rollback de la unidad de fuera: %v", err)
	}
	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("el asiento sobrevivio al rollback: %+v", asientos)
	}
}

// ---------------------------------------------------------------------------
// El cerrojo de fila (bloqueante 4)

// bitacoraConPausa envuelve la bitacora real y se detiene justo antes de
// escribir el asiento. Para entonces Actualizar ya corrio dentro de la MISMA
// transaccion: el cerrojo de fila sigue tomado y sin confirmar. Es el punto
// exacto en el que el test de concurrencia de abajo necesita congelar a T1
// para forzar la ventana que el bloqueante 4 describe.
type bitacoraConPausa struct {
	*Store
	listo  chan struct{}
	seguir chan struct{}
}

func (b *bitacoraConPausa) Asentar(ctx context.Context, a aplicacion.Asiento) error {
	close(b.listo)
	select {
	case <-b.seguir:
	case <-ctx.Done():
		// La pausa mira el contexto porque el que la suelta es el test, y un
		// test puede morirse antes de soltarla: un t.Fatal a mitad de camino
		// -el de esperarBloqueoPorUpdate, sin ir mas lejos- dejaria a T1
		// dormida para siempre CON su transaccion abierta y su conexion del
		// pool tomada. El paquete entonces no termina, y lo que CI reporta
		// diez minutos despues es un "test timed out" en vez del fallo que de
		// verdad ocurrio. Con esta rama, cancelar el contexto del test
		// (t.Context) basta para que T1 se despierte, revierta y devuelva la
		// conexion.
		return ctx.Err()
	}
	return b.Store.Asentar(ctx, a)
}

// esperarBloqueoPorUpdate espera, consultando pg_stat_activity y no un sleep a
// ciegas, a que otra sesion este de verdad bloqueada en un
// "... FROM obras ... FOR UPDATE": es la prueba, contra el estado real del
// servidor, de que [Store.Bloquear] de T2 quedo esperando el cerrojo de T1 y
// no que le gano la carrera a un release prematuro.
func esperarBloqueoPorUpdate(t *testing.T, vigia *pgx.Conn, ctx context.Context) {
	t.Helper()
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		var bloqueada bool
		err := vigia.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM pg_stat_activity
			    WHERE wait_event_type = 'Lock' AND query ILIKE '%FROM obras%FOR UPDATE%'
			 )`).Scan(&bloqueada)
		if err != nil {
			t.Fatalf("consultar pg_stat_activity: %v", err)
		}
		if bloqueada {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("T2 nunca quedo bloqueada esperando el cerrojo de fila de T1")
}

// Criterio del bloqueante 4: dos PATCH concurrentes sobre la MISMA obra no
// pueden dejar un asiento cuyo "antes" es un estado que esa transaccion no
// sustituyo.
//
// No basta lanzar dos goroutines y confiar en el scheduler: sin control, casi
// siempre corren en secuencia y la prueba pasaria igual con o sin el cerrojo
// -que es exactamente por que ninguna prueba de #91 detectaba esto-. Aqui se
// fuerza la ventana exacta: T1 se pausa justo despues de escribir (Actualizar
// ya corrio, la fila esta bloqueada y sin confirmar) y ANTES de asentar; T2
// arranca en ese punto, y el test espera -consultando el servidor, no con un
// sleep a ciegas- a que la consulta FOR UPDATE de T2 quede realmente
// bloqueada detras del cerrojo de T1 antes de soltarlo. Eso reproduce la
// carrera siempre, no algunas veces.
//
// Sin [Store.Bloquear] antes de PorID, T2 leeria el estado ORIGINAL mientras
// T1 sigue sin confirmar -su propio UPDATE la bloquearia igual, pero DESPUES
// de que su PorID ya hubiera leido de mas-, y asentaria antes=original en vez
// de antes=Version-T1. Ese es el escenario que esta prueba distingue.
func TestActualizarMetadatosObraConcurrenteAsientaLaCadenaCompleta(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	if _, err := catalogoConBitacora(s).
		RegistrarObra(ctx, obraNueva, metadatosDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}

	// Conexion APARTE del pool de 2 que usan T1 y T2 (ver testhelp.Pool): si la
	// consulta de vigilancia pidiera del mismo pool, competiria por la unica
	// conexion libre y podria quedarse esperando detras de las dos
	// transacciones que esta vigilando -un interbloqueo del propio test-.
	vigia, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		t.Fatalf("abrir conexion de vigilancia: %v", err)
	}
	defer func() { _ = vigia.Close(ctx) }()

	listoT1 := make(chan struct{})
	liberarT1 := make(chan struct{})
	fin1 := make(chan error, 1)
	fin2 := make(chan error, 1)

	// El cierre ordenado, valga el camino que valga. t.Fatal es un Goexit, asi
	// que estos defer corren tambien cuando la prueba se rinde a medias: se
	// suelta a T1 -sin arriesgar un segundo close, de ahi el OnceFunc- y se
	// espera a que las dos goroutines terminen ANTES de que el cleanup de
	// sembrar cierre el pool. Sin esto, un fallo de esta prueba se reporta
	// como el timeout de 10 minutos del paquete y no como lo que fue.
	var enVuelo sync.WaitGroup
	liberarUnaVez := sync.OnceFunc(func() { close(liberarT1) })
	defer enVuelo.Wait()
	defer liberarUnaVez()

	cat1 := aplicacion.Catalogo{
		Obras:         s.CatalogoObras(),
		Declaraciones: s,
		Bitacora:      &bitacoraConPausa{Store: s, listo: listoT1, seguir: liberarT1},
		Unidad:        s,
		Reloj:         &relojEnPasos{},
	}
	enVuelo.Add(1)
	go func() {
		defer enVuelo.Done()
		_, err := cat1.ActualizarMetadatosObra(ctx, obraNueva,
			metadatosDePrueba(func(m *repertorio.Metadatos) { m.Titulo = "Version-T1" }), usuarioAdmin)
		fin1 <- err
	}()

	select {
	case <-listoT1:
	case <-time.After(10 * time.Second):
		t.Fatal("T1 nunca llego al punto de pausa (Actualizar corrido, asiento pendiente)")
	}

	cat2 := catalogoConBitacora(s)
	enVuelo.Add(1)
	go func() {
		defer enVuelo.Done()
		_, err := cat2.ActualizarMetadatosObra(ctx, obraNueva,
			metadatosDePrueba(func(m *repertorio.Metadatos) { m.Titulo = "Version-T2" }), usuarioAdmin)
		fin2 <- err
	}()
	esperarBloqueoPorUpdate(t, vigia, ctx)

	liberarUnaVez()
	if err := <-fin1; err != nil {
		t.Fatalf("ActualizarMetadatosObra de T1: %v", err)
	}
	if err := <-fin2; err != nil {
		t.Fatalf("ActualizarMetadatosObra de T2: %v", err)
	}

	asientos, err := s.De(ctx, aplicacion.RefObra, obraNueva)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	// Se identifican por CONTENIDO y no por posicion: T1 y T2 usan relojes
	// independientes, asi que el orden de `cuando` no es lo que esta prueba
	// quiere comprobar -el invariante vale sin importar cual de los dos
	// escriba primero.
	var correccionT1, correccionT2 *aplicacion.Asiento
	for i := range asientos {
		if asientos[i].Hecho != aplicacion.HechoObraCorregida {
			continue
		}
		switch asientoObra(t, asientos[i]).Despues.Titulo {
		case "Version-T1":
			correccionT1 = &asientos[i]
		case "Version-T2":
			correccionT2 = &asientos[i]
		}
	}
	if len(asientos) != 3 || correccionT1 == nil || correccionT2 == nil {
		t.Fatalf("se esperaban 3 asientos (alta y las dos correcciones), historia = %v", hechos(asientos))
	}

	p1, p2 := asientoObra(t, *correccionT1), asientoObra(t, *correccionT2)
	if p1.Antes == nil || p1.Antes.Titulo != "Senoritas de Uribe" {
		t.Fatalf("T1.antes = %+v, se esperaba el alta original", p1.Antes)
	}
	// El invariante que el bloqueante 4 protege: T2 tiene que ver el COMMIT de
	// T1, no el estado de antes de que T1 empezara. Sin el cerrojo, esto sale
	// "Senoritas de Uribe" -el mismo que T1.antes- y el tramo real
	// (alta -> Version-T1 -> Version-T2) queda invisible para siempre, porque
	// el estado anterior no sobrevive en ninguna otra tabla.
	if p2.Antes == nil || p2.Antes.Titulo != "Version-T1" {
		t.Fatalf("T2.antes = %+v, se esperaba %q (el despues de T1)", p2.Antes, "Version-T1")
	}

	final, err := s.CatalogoObras().PorID(ctx, obraNueva)
	if err != nil {
		t.Fatalf("PorID final: %v", err)
	}
	if final.Metadatos().Titulo != "Version-T2" {
		t.Fatalf("titulo final = %q, se esperaba %q", final.Metadatos().Titulo, "Version-T2")
	}
}
