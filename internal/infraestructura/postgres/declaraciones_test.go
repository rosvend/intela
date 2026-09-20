package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// contadorDeConsultas cuenta sentencias contra la base. Es el instrumento con
// el que se mide el N+1 del item 1 SIN deducirlo del codigo: el defecto se
// midio asi -`log_statement = 'all'`, 14 sentencias para 12 versiones-, y la
// prueba tiene que medir lo mismo.
//
// Se engancha al pool en su construccion: pgx no deja poner un tracer despues,
// asi que el test monta su PROPIO pool sobre el mismo contenedor (misma DSN) en
// vez de tocar `testhelp`, que es ciclo de vida y nada mas.
type contadorDeConsultas struct {
	consultas int
}

var _ pgx.QueryTracer = (*contadorDeConsultas)(nil)

func (c *contadorDeConsultas) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.consultas++
	return ctx
}

func (c *contadorDeConsultas) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// storeInstrumentado devuelve un Store sobre un pool que cuenta consultas, en
// la MISMA base que ya sembro la prueba.
//
// El DSN entra como parametro y no se pide aqui: `testhelp.DSN` RESTAURA la
// plantilla en cada llamada -deja la base recien migrada y vacia-, asi que
// pedirlo despues de sembrar borraria los datos de la prueba. Medido: la
// primera version de esta prueba recibia 0 versiones por exactamente eso.
func storeInstrumentado(t *testing.T, dsn string, c *contadorDeConsultas) *Store {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("configurar el pool instrumentado: %v", err)
	}
	cfg.ConnConfig.Tracer = c
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("abrir el pool instrumentado: %v", err)
	}
	t.Cleanup(pool.Close)
	return Nuevo(pool)
}

// El item 1: el historial de una obra cuesta UN numero de consultas
// INDEPENDIENTE del numero de versiones. Con el bucle de antes eran 1 + N.
//
// Se mide con dos tamanos a proposito: un unico caso con N fijo pasaria igual
// si el numero de consultas resultara ser, por casualidad, el que se espera.
// Con 2 y con 9, "1" no puede ser un accidente de N.
func TestHistorialResuelveTodasLasVersionesEnUnaConsulta(t *testing.T) {
	for _, versiones := range []int{2, 9} {
		t.Run(fmt.Sprintf("%d versiones", versiones), func(t *testing.T) {
			// El DSN primero: `testhelp` restaura la plantilla en cada
			// llamada, y pedirlo despues de `sembrar` borraria lo sembrado.
			dsn := testhelp.DSN(t)
			s, _ := sembrar(t)
			t1 := time.Now().UTC().Truncate(time.Microsecond)
			for i := 0; i < versiones; i++ {
				// Cada version con DOS partes: si el LIMIT estuviera mal
				// puesto -sobre el resultado y no sobre las versiones-, una
				// version llegaria con una sola parte y la comprobacion de
				// abajo lo caza.
				if _, _, err := s.Guardar(t.Context(), partesDePrueba(t, int64(60-i), 40), t1.Add(time.Duration(i)*time.Hour), usuarioAdmin); err != nil {
					t.Fatalf("Guardar v%d: %v", i+1, err)
				}
			}

			c := &contadorDeConsultas{}
			historial, err := storeInstrumentado(t, dsn, c).Historial(t.Context(), obraSinDeclaracion, aplicacion.Paginacion{})
			if err != nil {
				t.Fatalf("Historial: %v", err)
			}
			if len(historial) != versiones {
				t.Fatalf("se esperaban %d versiones, llegaron %d", versiones, len(historial))
			}
			// El numero no depende de N: es la propiedad, no el valor.
			if c.consultas != 1 {
				t.Fatalf("consultas = %d para %d versiones, se esperaba 1", c.consultas, versiones)
			}
			// Y las partes de CADA version siguen siendo las suyas, no una
			// mezcla ni un recorte por el LIMIT.
			for i, vd := range historial {
				if vd.Version != i+1 {
					t.Fatalf("version %d en la posicion %d: el orden tiene que ser por version", vd.Version, i)
				}
				if len(vd.Declaracion.Partes) != 2 {
					t.Fatalf("la version %d llego con %d partes, se esperaban 2",
						vd.Version, len(vd.Declaracion.Partes))
				}
				if vd.Declaracion.Partes[0].Porcentaje.String() != fmt.Sprintf("%d", 60-i) {
					t.Fatalf("la version %d no lleva su porcentaje: %+v",
						vd.Version, vd.Declaracion.Partes)
				}
			}
		})
	}
}

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
	// Truncado: Guardar trunca a microsegundos antes de escribir, y el
	// vigenteDesde que devuelve refleja eso.
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	version, vigenteDesde, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), ahora, usuarioAdmin)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, se esperaba 1", version)
	}
	if !vigenteDesde.Equal(ahora) {
		t.Fatalf("vigenteDesde = %s, se esperaba %s: sin version previa no hay ajuste que hacer", vigenteDesde, ahora)
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

	v1, _, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), t1, usuarioAdmin)
	if err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}
	v2, _, err := s.Guardar(t.Context(), partesDePrueba(t, 70, 30), t2, usuarioAdmin)
	if err != nil {
		t.Fatalf("Guardar v2: %v", err)
	}
	if v1 != 1 || v2 != 2 {
		t.Fatalf("versiones = %d, %d; se esperaba 1, 2", v1, v2)
	}

	historial, err := s.Historial(t.Context(), obraSinDeclaracion, aplicacion.Paginacion{})
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

// El caso del item 3: una obra con versiones y NINGUNA abierta.
//
// No es hipotetico: una edicion cierra la ultima version y la siguiente no
// llega a abrirse -o el historial se importa asi-, y el `SELECT ... WHERE
// vigente_hasta IS NULL` no devuelve filas. Con el `version = 1` de antes, la
// edicion intentaba abrir la version 1 otra vez y moria con una violacion de
// la clave primaria (obra_id, version): un 500 que no dice nada de lo que
// pasa. El consecutivo sale del historial, no de las versiones abiertas.
func TestGuardarConTodasLasVersionesCerradasAbreLaSiguiente(t *testing.T) {
	s, pool := sembrar(t)
	t1 := time.Now().UTC().Truncate(time.Microsecond)
	t2 := t1.Add(time.Hour)

	if _, _, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), t1, usuarioAdmin); err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}
	if _, _, err := s.Guardar(t.Context(), partesDePrueba(t, 70, 30), t2, usuarioAdmin); err != nil {
		t.Fatalf("Guardar v2: %v", err)
	}

	// Se cierra la v2 a mano y no se abre la 3: es el estado del defecto. No
	// hay forma de llegar a el por la API, y por eso hay que fabricarlo.
	if _, err := pool.Exec(t.Context(),
		`UPDATE declaracion_versiones SET vigente_hasta = $2 WHERE obra_id = $1 AND version = 2`,
		obraSinDeclaracion, t2.Add(time.Hour)); err != nil {
		t.Fatalf("cerrar la version 2: %v", err)
	}

	v3, _, err := s.Guardar(t.Context(), partesDePrueba(t, 50, 50), t2.Add(2*time.Hour), usuarioAdmin)
	if err != nil {
		t.Fatalf("Guardar con todas las versiones cerradas: %v", err)
	}
	if v3 != 3 {
		t.Fatalf("version = %d, se esperaba 3 (MAX(version) + 1), no 1", v3)
	}

	historial, err := s.Historial(t.Context(), obraSinDeclaracion, aplicacion.Paginacion{})
	if err != nil {
		t.Fatalf("Historial: %v", err)
	}
	if len(historial) != 3 {
		t.Fatalf("se esperaban 3 versiones, llegaron %d", len(historial))
	}
	// La 3 es la unica abierta, y las dos anteriores siguen cerradas: el
	// historial es append-only y esta edicion no lo reescribio.
	for _, vd := range historial {
		if vd.Version == 3 && vd.VigenteHasta != nil {
			t.Fatalf("la version 3 tiene que quedar abierta: %+v", vd)
		}
		if vd.Version != 3 && vd.VigenteHasta == nil {
			t.Fatalf("la version %d tendria que seguir cerrada: %+v", vd.Version, vd)
		}
	}
	// Y sin huecos: el consecutivo que se abre es el que sigue al mayor.
	if historial[2].Declaracion.Partes[0].Porcentaje.String() != "50" {
		t.Fatalf("la version 3 no lleva la declaracion nueva: %+v", historial[2].Declaracion.Partes)
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

	if _, _, err := s.Guardar(t.Context(), partesDePrueba(t, 60, 40), t1, usuarioAdmin); err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}
	if _, _, err := s.Guardar(t.Context(), partesDePrueba(t, 70, 30), t2, usuarioAdmin); err != nil {
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

	_, _, err = s.Guardar(t.Context(), d, time.Now().UTC(), usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// ---------------------------------------------------------------------------
// VigentesDeObras: la lectura que sirve el catalogo.

// La pagina entera del catalogo se resuelve con UNA llamada: las tres obras
// declaradas salen con su version abierta y sus partes, y las que no tienen
// declaracion NO salen -la ausencia es el dato, no un hueco que rellenar con
// ceros, porque una declaracion vacia daria el mismo Estado() (`incompleta`)
// que una obra sin declarar (R-04)-.
func TestVigentesDeObrasLeeLaVersionAbiertaDeLaPagina(t *testing.T) {
	s, _ := sembrar(t)

	vigentes, err := s.VigentesDeObras(t.Context(), []string{
		obraCompleta, obraIncompleta, obraSinDeclaracion, obraSinIPI, "obra-que-no-existe",
	})
	if err != nil {
		t.Fatalf("VigentesDeObras: %v", err)
	}

	if len(vigentes) != 3 {
		t.Fatalf("se esperaban 3 obras declaradas, llegaron %d: %v", len(vigentes), vigentes)
	}
	if _, hay := vigentes[obraSinDeclaracion]; hay {
		t.Fatal("una obra sin declaracion no puede aparecer en el mapa")
	}
	if _, hay := vigentes["obra-que-no-existe"]; hay {
		t.Fatal("un id que no existe en el catalogo no puede aparecer en el mapa")
	}

	completa := vigentes[obraCompleta]
	if completa.Version != 1 || completa.VigenteHasta != nil {
		t.Fatalf("obra completa = version %d, vigente_hasta %v; se esperaba la 1 abierta",
			completa.Version, completa.VigenteHasta)
	}
	if len(completa.Declaracion.Partes) != 2 || !completa.Declaracion.Completa() {
		t.Fatalf("obra completa = %+v, se esperaba 60+40", completa.Declaracion.Partes)
	}
	if completa.Declaracion.ObraID != obraCompleta {
		t.Fatalf("ObraID = %q, se esperaba %q", completa.Declaracion.ObraID, obraCompleta)
	}

	incompleta := vigentes[obraIncompleta]
	if len(incompleta.Declaracion.Partes) != 1 ||
		!incompleta.Declaracion.Partes[0].Porcentaje.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("obra incompleta = %+v, se esperaba una parte de 60", incompleta.Declaracion.Partes)
	}
	if incompleta.Declaracion.Completa() {
		t.Fatal("60 no suma 100: la declaracion es incompleta (R-04)")
	}

	// Suma 100 con una parte sin IPI: las dos partes llegan enteras, porque
	// quien decide el estado es el dominio, no una suma en SQL.
	sinIPI := vigentes[obraSinIPI]
	if len(sinIPI.Declaracion.Partes) != 2 {
		t.Fatalf("obra sin IPI = %+v, se esperaban sus dos partes", sinIPI.Declaracion.Partes)
	}
	if sinIPI.Declaracion.Completa() {
		t.Fatal("una parte sin IPI deja la declaracion incompleta aunque sume 100")
	}
}

// Editar cierra la version anterior y abre una nueva: el catalogo tiene que
// ver la ABIERTA, no la ultima que se escribio en el historial ni una mezcla
// de las dos.
func TestVigentesDeObrasSoloVeLaVersionAbierta(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	t1 := time.Now().UTC().Truncate(time.Microsecond)

	if _, _, err := s.Guardar(ctx, partesDePrueba(t, 60, 40), t1, usuarioAdmin); err != nil {
		t.Fatalf("Guardar v1: %v", err)
	}
	if _, _, err := s.Guardar(ctx, partesDePrueba(t, 25, 0), t1.Add(time.Hour), usuarioAdmin); err != nil {
		t.Fatalf("Guardar v2: %v", err)
	}

	vigentes, err := s.VigentesDeObras(ctx, []string{obraSinDeclaracion})
	if err != nil {
		t.Fatalf("VigentesDeObras: %v", err)
	}
	vd, hay := vigentes[obraSinDeclaracion]
	if !hay {
		t.Fatal("la obra se declaro dos veces y no aparece en el mapa")
	}
	if vd.Version != 2 {
		t.Fatalf("version = %d, se esperaba la 2 (la abierta)", vd.Version)
	}
	if len(vd.Declaracion.Partes) != 1 ||
		!vd.Declaracion.Partes[0].Porcentaje.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("partes = %+v, se esperaba la parte de la version 2 y no la de la 1",
			vd.Declaracion.Partes)
	}
}

// Una version abierta SIN partes sigue siendo una declaracion: se reporta la
// version y cero partes, en vez de desaparecer del mapa. Con un JOIN interno
// desapareceria, y el catalogo leeria "esta obra no tiene declaracion", que es
// justo la confusion que `version_vigente` existe para evitar.
func TestVigentesDeObrasReportaUnaVersionSinPartesComoDeclaracion(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO declaracion_versiones (obra_id, version, vigente_desde) VALUES ($1, 1, now())`,
		obraSinDeclaracion); err != nil {
		t.Fatalf("abrir una version vacia: %v", err)
	}

	vigentes, err := s.VigentesDeObras(ctx, []string{obraSinDeclaracion})
	if err != nil {
		t.Fatalf("VigentesDeObras: %v", err)
	}
	vd, hay := vigentes[obraSinDeclaracion]
	if !hay {
		t.Fatal("la version abierta existe y la obra no puede salir como si no tuviera declaracion")
	}
	if vd.Version != 1 {
		t.Fatalf("version = %d, se esperaba 1", vd.Version)
	}
	if len(vd.Declaracion.Partes) != 0 {
		t.Fatalf("partes = %+v, se esperaba ninguna", vd.Declaracion.Partes)
	}
}

// Una lista vacia no consulta: es el caso de una pagina del catalogo sin
// resultados, y no puede costar un viaje a la base.
func TestVigentesDeObrasSinIDsNoConsulta(t *testing.T) {
	s, _ := sembrar(t)

	vigentes, err := s.VigentesDeObras(t.Context(), nil)
	if err != nil {
		t.Fatalf("VigentesDeObras: %v", err)
	}
	if len(vigentes) != 0 {
		t.Fatalf("mapa = %v, se esperaba vacio", vigentes)
	}
}

// Un titular_id que no esta en el padron es un dato malo del CUERPO, no una
// obra ausente: tiene centinela propio para que el adaptador HTTP lo pueda
// separar del 404 de la obra.
func TestGuardarTitularInexistenteEsErrTitularInexistente(t *testing.T) {
	s, _ := sembrar(t)

	d, err := repertorio.NuevaDeclaracion(obraSinDeclaracion, []repertorio.Parte{
		{TitularID: "titular-que-no-existe", IPI: "IPI-00000099", Porcentaje: decimal.NewFromInt(100)},
	})
	if err != nil {
		t.Fatalf("construir la declaracion: %v", err)
	}

	_, _, err = s.Guardar(t.Context(), d, time.Now().UTC(), usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrTitularInexistente) {
		t.Fatalf("se esperaba ErrTitularInexistente, se obtuvo %v", err)
	}

	// La FK revienta DENTRO de la misma transaccion que abrio la version: no
	// puede quedar una version huerfana ni un asiento sin las partes que
	// describe (mismo patron que TestGuardarRevierteLaVersionSiElAsientoFalla).
	historial, err := s.Historial(t.Context(), obraSinDeclaracion, aplicacion.Paginacion{})
	if err != nil {
		t.Fatalf("Historial: %v", err)
	}
	if len(historial) != 0 {
		t.Fatalf("la version quedo huerfana: %+v", historial)
	}
	asientos, err := s.De(t.Context(), "obra", obraSinDeclaracion)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("no se esperaba ningun asiento: %+v", asientos)
	}
}

// Ultima linea de defensa: el EXCLUDE de la migracion 00008 rechaza un
// solape aunque alguien lo intente por fuera de Guardar.
func TestElEsquemaRechazaVigenciasQueSeSolapan(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	t1 := time.Now().UTC()
	if _, _, err := s.Guardar(ctx, partesDePrueba(t, 100, 0), t1, usuarioAdmin); err != nil {
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

// El asiento se escribe en la MISMA transaccion que la version (ADR 0006):
// Guardar deja exactamente un asiento por escritura, con el payload que
// describe esa version.
func TestGuardarAsientaElHechoEnLaMismaTransaccion(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	version, _, err := s.Guardar(ctx, partesDePrueba(t, 60, 40), ahora, usuarioAdmin)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	asientos, err := s.De(ctx, "obra", obraSinDeclaracion)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento, hubo %d", len(asientos))
	}
	a := asientos[0]
	if a.Hecho != "declaracion.guardada" || a.ActorID != usuarioAdmin || !a.Cuando.Equal(ahora) {
		t.Fatalf("asiento = %+v", a)
	}
	var payload struct {
		Version int    `json:"version"`
		Estado  string `json:"estado"`
	}
	if err := json.Unmarshal(a.Payload, &payload); err != nil {
		t.Fatalf("payload no es JSON: %v", err)
	}
	if payload.Version != version {
		t.Fatalf("payload.Version = %d, se esperaba %d", payload.Version, version)
	}
}

// El caso que motivo el arreglo: si el asiento falla, la version tiene que
// quedar SIN escribir, no huerfana. actor_id referencia a usuarios(id); un
// actor que no existe hace fallar el INSERT en `asientos` sin tocar ninguna
// otra tabla -es la unica forma de forzar ese fallo especifico sin doblar la
// base-, y el rollback de EnTransaccion tiene que deshacer TODO lo anterior
// en la misma llamada: la version abierta y sus partes.
func TestGuardarRevierteLaVersionSiElAsientoFalla(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	_, _, err := s.Guardar(ctx, partesDePrueba(t, 60, 40), time.Now().UTC(), "actor-que-no-existe")
	if err == nil {
		t.Fatal("se esperaba que el asiento fallara por el actor inexistente")
	}

	historial, err := s.Historial(ctx, obraSinDeclaracion, aplicacion.Paginacion{})
	if err != nil {
		t.Fatalf("Historial: %v", err)
	}
	if len(historial) != 0 {
		t.Fatalf("la version quedo huerfana: %+v", historial)
	}
	asientos, err := s.De(ctx, "obra", obraSinDeclaracion)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("no se esperaba ningun asiento: %+v", asientos)
	}
}

// Dos ediciones SECUENCIALES (la segunda ya con la primera confirmada) que
// piden un instante que no queda estrictamente despues del vigente_desde de
// la version anterior -mismo instante exacto, o uno posterior en Go pero que
// cae en el MISMO microsegundo una vez truncado- no pueden dejar
// vigente_hasta == vigente_desde: el CHECK declaracion_vigencia_coherente
// rechazaria una edicion valida. Guardar tiene que empujar la segunda hacia
// adelante en los dos casos.
//
// El caso "delta sub-microsegundo" es el que Roy reprodujo contra Postgres
// real: comparar en la resolucion de nanosegundo de Go antes de truncar deja
// pasar un ahora que ES posterior en Go (300ns > 0) pero que trunca al MISMO
// microsegundo que desdeAnterior, y el fallo sale como un 500 opaco en vez de
// una edicion aceptada. Este subtest esta pensado para fallar contra un
// Guardar que trunque solo en la comparacion sin truncar primero -o que no
// truncase en absoluto- y pasar con el ajuste de arriba en la funcion.
func TestGuardarEmpujaVigenteDesdeSiCoincideConLaAnterior(t *testing.T) {
	casos := []struct {
		nombre string
		delta  time.Duration
	}{
		{nombre: "mismo instante exacto", delta: 0},
		{nombre: "delta sub-microsegundo", delta: 300 * time.Nanosecond},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, _ := sembrar(t)
			ctx := t.Context()
			t1 := time.Now().UTC().Truncate(time.Microsecond)
			t2 := t1.Add(c.delta)

			v1, vd1, err := s.Guardar(ctx, partesDePrueba(t, 60, 40), t1, usuarioAdmin)
			if err != nil {
				t.Fatalf("Guardar v1: %v", err)
			}
			_, vd2, err := s.Guardar(ctx, partesDePrueba(t, 70, 30), t2, usuarioAdmin)
			if err != nil {
				t.Fatalf("Guardar v2 con delta %s sobre v1: %v", c.delta, err)
			}

			historial, err := s.Historial(ctx, obraSinDeclaracion, aplicacion.Paginacion{})
			if err != nil {
				t.Fatalf("Historial: %v", err)
			}
			if len(historial) != 2 {
				t.Fatalf("se esperaban 2 versiones, llegaron %d", len(historial))
			}
			if historial[0].VigenteHasta == nil || !historial[0].VigenteHasta.After(vd1) {
				t.Fatalf("version %d cerro en %+v, se esperaba un instante posterior a %s", v1, historial[0].VigenteHasta, vd1)
			}
			if !historial[1].VigenteDesde.Equal(*historial[0].VigenteHasta) {
				t.Fatalf("version 2 abre en %s, version 1 cerro en %s: tienen que coincidir",
					historial[1].VigenteDesde, *historial[0].VigenteHasta)
			}
			if !vd2.Equal(*historial[0].VigenteHasta) {
				t.Fatalf("Guardar devolvio vigenteDesde = %s, pero la base tiene %s", vd2, *historial[0].VigenteHasta)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// R-01 de punta a punta: el caso de uso montado sobre la base real.

// El tercer caso que pide el criterio S8 del plan. Los otros dos -el del caso de
// uso y el del handler- corren sobre dobles, y los dos dan por hecho lo mismo:
// que el puerto honra `FiltroTitulares.IDs` y devuelve las filas de verdad,
// personas naturales incluidas, para que el veredicto salga de
// `PuedeRecibirReparto()`. Aqui no hay doble: [aplicacion.Declaraciones] se
// monta sobre el `*Store` real, con una sociedad sembrada en el padron de
// verdad, asi que lo que se prueba es la costura entera -la consulta acotada por
// ids, la entidad del dominio decidiendo, y la escritura que no ocurre-.
//
// # La asercion que solo se puede hacer aqui
//
// Que el rechazo no deja rastro. Se lee de la BASE y no del valor devuelto: un
// rechazo que hubiera cerrado la version abierta -o que hubiera dejado una
// version 2 a medias- devolveria el mismo centinela, y [Store.Guardar] cierra la
// version anterior y abre la nueva dentro de una sola transaccion, asi que lo
// unico que dice lo que paso es lo que quedo escrito. Por eso el test deja
// primero una version ABIERTA y valida: sin ella, "no se cerro nada" seria
// indistinguible de "no habia nada que cerrar".
func TestGuardarSplitsRechazaR01SinCerrarLaVersionAbierta(t *testing.T) {
	casos := []struct {
		nombre         string
		juridica       string
		nombreEnPadron string
		ipiDeLaParte   string
	}{
		// La primera lleva IPI en el padron y en la parte: lo que la descalifica
		// es `persona_natural`, no la falta de IPI.
		{"una sociedad con IPI", titularProductora, "Productora del Caribe S.A.S.", "IPI-00000077"},
		// Y esta no lo lleva en el padron, porque el esquema solo obliga al IPI
		// a las personas naturales (`titular_natural_tiene_ipi`): el rechazo no
		// puede depender de que la fila traiga el dato. La PARTE si lleva uno,
		// porque [repertorio.NuevaDeclaracion] lo exige en toda parte; sin el,
		// lo que rebotaria seria la validacion pura y no `R-01`.
		{"una sociedad sin IPI", titularCadena, "Cadena del Norte S.A.", "IPI-00000088"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, _ := padronCompleto(t)
			ctx := t.Context()
			// Truncado a microsegundos: es la resolucion de TIMESTAMPTZ y
			// Store.Guardar trunca antes de escribir (mismo motivo que
			// TestGuardarPrimeraVersion).
			ahora := time.Now().UTC().Truncate(time.Microsecond)
			decl := aplicacion.Declaraciones{Gestion: s, Padron: s, Reloj: reloj.Fijo{Instante: ahora}}

			// Las dos lecturas con las que se comprueba que no se escribio nada.
			// Son de la BASE -`declaracion_versiones` y la bitacora de la obra-
			// y no del valor que devuelve la llamada.
			historialDeLaObra := func() []aplicacion.VersionDeclaracion {
				t.Helper()
				historial, err := s.Historial(ctx, obraSinDeclaracion, aplicacion.Paginacion{})
				if err != nil {
					t.Fatalf("Historial: %v", err)
				}
				return historial
			}
			asientosDeLaObra := func() []aplicacion.Asiento {
				t.Helper()
				asientos, err := s.De(ctx, "obra", obraSinDeclaracion)
				if err != nil {
					t.Fatalf("De: %v", err)
				}
				return asientos
			}

			// 1. Una v1 valida y ABIERTA: dos personas naturales al 60/40. No es
			// decoracion, es lo que hace posible la asercion que vale -que el
			// rechazo no cierra una version que ya estaba abierta-.
			partesV1 := []repertorio.Parte{
				{TitularID: titularAna, IPI: "IPI-00000001", Porcentaje: decimal.NewFromInt(60)},
				{TitularID: titularBeto, IPI: "IPI-00000002", Porcentaje: decimal.NewFromInt(40)},
			}
			v1, err := decl.GuardarSplits(ctx, obraSinDeclaracion, partesV1, usuarioAdmin)
			if err != nil {
				t.Fatalf("guardar la v1 con dos personas naturales: %v", err)
			}
			if v1.Version != 1 {
				t.Fatalf("version = %d, se esperaba 1: obraSinDeclaracion no tiene ninguna version sembrada", v1.Version)
			}

			historialV1 := historialDeLaObra()
			asientosV1 := asientosDeLaObra()
			if len(historialV1) != 1 || len(asientosV1) != 1 {
				t.Fatalf("la v1 dejo %d versiones y %d asientos, se esperaba 1 y 1",
					len(historialV1), len(asientosV1))
			}

			// 2. El intento que tiene que rebotar, con la sociedad MEZCLADA con
			// una persona natural. Mezclarla es la mitad que importa: el caso de
			// uso pide los ids que la declaracion nombra, y lo que vuelve son
			// esas dos filas -la de Ana incluida-, asi que el rechazo solo puede
			// salir de preguntarle `PuedeRecibirReparto()` a la fila de la
			// sociedad. Con una declaracion donde todo lo que vuelve esta
			// excluido, el test pasaria igual con un caso de uso que rechazara
			// por "algo de la lista no es persona natural".
			partesMezcladas := []repertorio.Parte{
				{TitularID: c.juridica, IPI: c.ipiDeLaParte, Porcentaje: decimal.NewFromInt(50)},
				{TitularID: titularAna, IPI: "IPI-00000001", Porcentaje: decimal.NewFromInt(50)},
			}
			_, err = decl.GuardarSplits(ctx, obraSinDeclaracion, partesMezcladas, usuarioAdmin)
			if !errors.Is(err, aplicacion.ErrTitularNoEsPersonaNatural) {
				t.Fatalf("se esperaba ErrTitularNoEsPersonaNatural, se obtuvo %v", err)
			}
			// El error nombra la fila del padron que lo provoco: si el veredicto
			// hubiera salido de la parte de Ana -que si puede recibir reparto-,
			// el nombre seria el suyo.
			if !strings.Contains(err.Error(), c.nombreEnPadron) {
				t.Fatalf("el error no nombra la fila del padron que lo provoco: %v", err)
			}

			// 3. Y contra la base, las cuatro aserciones de que no se escribio
			// nada.
			//
			// (a) El historial sigue teniendo UNA version. Historial lee
			// `declaracion_versiones`, que es la tabla donde Guardar abre la
			// version nueva: una segunda fila aqui es el intento fallido.
			historial := historialDeLaObra()
			if len(historial) != len(historialV1) {
				t.Fatalf("el historial paso de %d versiones a %d: el intento fallido escribio",
					len(historialV1), len(historial))
			}

			// (b) Sigue siendo la 1 y sigue ABIERTA: `vigente_hasta` nulo es
			// exactamente "nadie la cerro". Es la mitad que un rechazo tardio
			// -posterior a Guardar- habria roto: la v1 quedaria cerrada, la v2
			// no llegaria a existir y la obra se quedaria sin declaracion
			// vigente.
			if historial[0].Version != 1 || historial[0].VigenteHasta != nil {
				t.Fatalf("version vigente = %d con vigente_hasta %v, se esperaba la 1 abierta",
					historial[0].Version, historial[0].VigenteHasta)
			}

			// (c) Y sus partes siguen siendo las de la v1 -60/40, las dos
			// personas naturales-, ni una mezcla con las del intento ni una
			// version nueva a medias. Salen de `declaraciones` filtrada por esa
			// version, que es la tabla donde se escriben.
			partes := historial[0].Declaracion.Partes
			if len(partes) != 2 {
				t.Fatalf("la v1 abierta tiene %d partes, se esperaban 2: %+v", len(partes), partes)
			}
			if partes[0].TitularID != titularAna || !partes[0].Porcentaje.Equal(decimal.NewFromInt(60)) {
				t.Fatalf("primera parte de la v1 = %+v, se esperaba %s al 60", partes[0], titularAna)
			}
			if partes[1].TitularID != titularBeto || !partes[1].Porcentaje.Equal(decimal.NewFromInt(40)) {
				t.Fatalf("segunda parte de la v1 = %+v, se esperaba %s al 40", partes[1], titularBeto)
			}

			// (d) La bitacora de la obra no gano un asiento. El asiento lo
			// escribe Guardar en la MISMA transaccion que la version, asi que
			// uno de mas seria la prueba de que la escritura llego a empezar.
			asientos := asientosDeLaObra()
			if len(asientos) != len(asientosV1) {
				t.Fatalf("la bitacora de la obra paso de %d asientos a %d",
					len(asientosV1), len(asientos))
			}
			if asientos[0].Hecho != "declaracion.guardada" {
				t.Fatalf("el asiento que queda es de %q, se esperaba el de la v1 (`declaracion.guardada`)",
					asientos[0].Hecho)
			}
		})
	}
}

// El historial ya no se corta con un error a partir de N versiones: se pagina, y
// la pagina va de la version MAS RECIENTE hacia atras -la abierta es la ultima y
// es la que las pantallas necesitan-, en orden ascendente dentro de la pagina.
// Con limite 2 sobre 5 versiones, ninguna queda inalcanzable.
func TestHistorialPaginaDesdeLaVersionMasReciente(t *testing.T) {
	s, _ := sembrar(t)
	t1 := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		// Dos partes por version: el LIMIT va sobre las versiones, no sobre las
		// partes, y una version recortada a una parte lo delataria.
		if _, _, err := s.Guardar(t.Context(), partesDePrueba(t, int64(60-i), 40), t1.Add(time.Duration(i)*time.Hour), usuarioAdmin); err != nil {
			t.Fatalf("Guardar v%d: %v", i+1, err)
		}
	}

	versionesDe := func(pag aplicacion.Paginacion) []int {
		t.Helper()
		historial, err := s.Historial(t.Context(), obraSinDeclaracion, pag)
		if err != nil {
			t.Fatalf("Historial(%+v): %v", pag, err)
		}
		var vs []int
		for _, v := range historial {
			if len(v.Declaracion.Partes) != 2 {
				t.Fatalf("la version %d trae %d partes, se esperaban 2", v.Version, len(v.Declaracion.Partes))
			}
			vs = append(vs, v.Version)
		}
		return vs
	}

	casos := []struct {
		nombre string
		pag    aplicacion.Paginacion
		quiero []int
	}{
		{"primera pagina: las mas recientes", aplicacion.Paginacion{Limite: 2}, []int{4, 5}},
		{"segunda pagina", aplicacion.Paginacion{Limite: 2, Desplazamiento: 2}, []int{2, 3}},
		{"ultima pagina: la mas vieja", aplicacion.Paginacion{Limite: 2, Desplazamiento: 4}, []int{1}},
		{"mas alla del final", aplicacion.Paginacion{Limite: 2, Desplazamiento: 10}, nil},
		{"sin tope", aplicacion.Paginacion{Limite: aplicacion.LimiteSinTope}, []int{1, 2, 3, 4, 5}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := versionesDe(c.pag); !slices.Equal(got, c.quiero) {
				t.Fatalf("versiones = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}
