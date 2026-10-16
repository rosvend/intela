package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
	"github.com/rosvend/intela/internal/dominio/anticipos"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
)

// Pruebas del adaptador de anticipos (#56) y de la migracion 00031 contra
// Postgres real. Lo que el doble de aplicacion EMULA -el indice de un solo
// vigente, la escritura condicional de Resolver y Descontar- aqui se prueba
// que la base lo hace cumplir de verdad.

// anticiposDe cablea el caso de uso como cmd/api, sobre el mismo *Store.
func anticiposDe(s *Store, r aplicacion.Reloj) aplicacion.Anticipos {
	return aplicacion.Anticipos{
		Repo:     s.Anticipos(),
		SMMLV:    s,
		IDs:      cripto.TokensAleatorios{},
		Reloj:    r,
		Bitacora: s,
		Unidad:   s,
	}
}

func solicitudPrueba(id, titular, monto string) anticipos.Anticipo {
	return anticipos.Anticipo{
		ID: id, TitularID: titular, Monto: pgDec(monto), Saldo: decimal.Zero,
		Estado: anticipos.EstadoSolicitado, Justificacion: "vivienda (RA 2.3.a)",
	}
}

var instanteAnticipos = time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)

func codigoPg(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code + "/" + pgErr.ConstraintName
	}
	return ""
}

// TestMigracionAnticiposDejaLasRestriccionesValidadas es evidencia ESTRUCTURAL
// de la 00031, no de comportamiento: que las restricciones existen con
// `convalidated = true` -una NOT VALID dejaria pasar las filas que ya
// estuvieran- y que el indice de R-32 es unico y parcial sobre los dos estados
// vigentes. Una prueba de comportamiento contra una tabla vacia no distingue
// una restriccion validada de una que no lo esta.
func TestMigracionAnticiposDejaLasRestriccionesValidadas(t *testing.T) {
	_, pool := sembrar(t)
	ctx := t.Context()

	for _, c := range []struct{ tabla, nombre string }{
		{"anticipos", "anticipo_saldo_acotado"},
		{"anticipos", "anticipo_saldo_solo_aprobado"},
		{"embargos", "embargo_vigencia_coherente"},
	} {
		var validada bool
		err := pool.QueryRow(ctx,
			`SELECT convalidated FROM pg_constraint
			  WHERE conname = $1 AND conrelid = $2::regclass`, c.nombre, c.tabla).Scan(&validada)
		if err != nil {
			t.Fatalf("restriccion %s.%s: %v", c.tabla, c.nombre, err)
		}
		if !validada {
			t.Fatalf("%s.%s existe NOT VALID: no se comprobo sobre las filas que ya habia", c.tabla, c.nombre)
		}
	}

	var unico bool
	var predicado string
	err := pool.QueryRow(ctx,
		`SELECT i.indisunique, pg_get_expr(i.indpred, i.indrelid)
		   FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		  WHERE c.relname = $1`, indiceUnVigente).Scan(&unico, &predicado)
	if err != nil {
		t.Fatalf("indice %s: %v", indiceUnVigente, err)
	}
	if !unico || predicado == "" {
		t.Fatalf("indice %s: unico=%v predicado=%q; tiene que ser UNIQUE y parcial", indiceUnVigente, unico, predicado)
	}
	for _, estado := range []string{"solicitado", "aprobado"} {
		if !strings.Contains(predicado, estado) {
			t.Fatalf("el predicado %q no cubre %q", predicado, estado)
		}
	}
}

// TestEsquemaDeAnticiposRechazaLoQueElDominioNoProduce: la base sostiene las
// invariantes aunque alguien escriba la fila sin pasar por el dominio.
func TestEsquemaDeAnticiposRechazaLoQueElDominioNoProduce(t *testing.T) {
	_, pool := sembrar(t)
	ctx := t.Context()
	insertar := func(id, titular, estado, monto, saldo string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO anticipos (id, titular_id, monto, saldo, estado) VALUES ($1, $2, $3, $4, $5)`,
			id, titular, pgDec(monto), pgDec(saldo), estado)
		return err
	}

	rechazos := []struct {
		nombre                       string
		estado, monto, saldo, codigo string
	}{
		{"una solicitud no debe nada todavia", "solicitado", "100", "100", "23514/anticipo_saldo_solo_aprobado"},
		{"un aprobado sin saldo es un descontado mal escrito", "aprobado", "100", "0", "23514/anticipo_saldo_solo_aprobado"},
		{"un descontado no conserva saldo", "descontado", "100", "1", "23514/anticipo_saldo_solo_aprobado"},
		{"un rechazado nunca se giro", "rechazado", "100", "1", "23514/anticipo_saldo_solo_aprobado"},
		{"el saldo no supera lo otorgado", "aprobado", "100", "100.01", "23514/anticipo_saldo_acotado"},
	}
	for i, c := range rechazos {
		err := insertar("ant-x"+string(rune('a'+i)), titularAna, c.estado, c.monto, c.saldo)
		if got := codigoPg(err); got != c.codigo {
			t.Fatalf("%s: err = %v (%s), se esperaba %s", c.nombre, err, got, c.codigo)
		}
	}

	// R-32 en la base: uno vigente por titular, en tramite o aprobado.
	if err := insertar("ant-1", titularAna, "aprobado", "100", "40"); err != nil {
		t.Fatalf("primer vigente: %v", err)
	}
	if got := codigoPg(insertar("ant-2", titularAna, "solicitado", "100", "0")); got != "23505/"+indiceUnVigente {
		t.Fatalf("segundo vigente de Ana: %s, se esperaba la violacion de %s", got, indiceUnVigente)
	}
	// Lo que ya no esta vigente no bloquea, y otro titular tampoco.
	if err := insertar("ant-3", titularAna, "descontado", "100", "0"); err != nil {
		t.Fatalf("un descontado junto al vigente: %v", err)
	}
	if err := insertar("ant-4", titularAna, "rechazado", "100", "0"); err != nil {
		t.Fatalf("un rechazado junto al vigente: %v", err)
	}
	if err := insertar("ant-5", titularBeto, "solicitado", "100", "0"); err != nil {
		t.Fatalf("el vigente de otro titular: %v", err)
	}
}

// TestRegistrarTraduceElSegundoVigente: la carrera de dos solicitudes que pasan
// las dos la comprobacion del caso de uso la decide el indice, y llega al
// nucleo como R-32 -no como un 500-. Un id repetido NO es R-32: es otra
// violacion de unicidad y sube como fallo.
func TestRegistrarTraduceElSegundoVigente(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	repo := s.Anticipos()

	if err := repo.Registrar(ctx, solicitudPrueba("ant-1", titularAna, "100"), instanteAnticipos); err != nil {
		t.Fatalf("Registrar: %v", err)
	}
	err := repo.Registrar(ctx, solicitudPrueba("ant-2", titularAna, "50"), instanteAnticipos)
	if !errors.Is(err, anticipos.ErrAnticipoVigente) {
		t.Fatalf("segunda solicitud: err = %v, se esperaba ErrAnticipoVigente", err)
	}

	err = repo.Registrar(ctx, solicitudPrueba("ant-1", titularBeto, "100"), instanteAnticipos)
	if err == nil || errors.Is(err, anticipos.ErrAnticipoVigente) {
		t.Fatalf("id repetido: err = %v; no es un anticipo vigente de Beto", err)
	}

	got, err := repo.PorID(ctx, "ant-1")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if got.Anticipo.Estado != anticipos.EstadoSolicitado || !got.Anticipo.Monto.Equal(pgDec("100")) ||
		got.Anticipo.Justificacion != "vivienda (RA 2.3.a)" || !got.Solicitado.Equal(instanteAnticipos) ||
		!got.Resuelto.IsZero() || got.ResueltoPor != "" {
		t.Fatalf("PorID = %+v", got)
	}
	if _, err := repo.PorID(ctx, "ant-nada"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("PorID inexistente: err = %v", err)
	}
}

// TestResolverSoloDesdeSolicitado: la segunda decision sobre la misma
// solicitud no pisa a la primera.
func TestResolverSoloDesdeSolicitado(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	repo := s.Anticipos()
	pedido := solicitudPrueba("ant-1", titularAna, "100")
	if err := repo.Registrar(ctx, pedido, instanteAnticipos); err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	aprobado := pedido
	aprobado.Estado, aprobado.Saldo, aprobado.Soporte = anticipos.EstadoAprobado, pgDec("100"), "acta 12"
	cuando := instanteAnticipos.Add(48 * time.Hour)
	if err := repo.Resolver(ctx, aprobado, cuando, usuarioAdmin); err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	rechazado := pedido
	rechazado.Estado, rechazado.MotivoRechazo = anticipos.EstadoRechazado, "tarde"
	if err := repo.Resolver(ctx, rechazado, cuando, usuarioAdmin); !errors.Is(err, anticipos.ErrEstadoInvalido) {
		t.Fatalf("segunda decision: err = %v, se esperaba ErrEstadoInvalido", err)
	}
	if err := repo.Resolver(ctx, solicitudPrueba("ant-nada", titularAna, "1"), cuando, usuarioAdmin); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("decision sobre un inexistente: err = %v", err)
	}

	got, err := repo.PorID(ctx, "ant-1")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if got.Anticipo.Estado != anticipos.EstadoAprobado || !got.Anticipo.Saldo.Equal(pgDec("100")) ||
		got.Anticipo.Soporte != "acta 12" || got.Anticipo.MotivoRechazo != "" ||
		got.ResueltoPor != usuarioAdmin || !got.Resuelto.Equal(cuando) {
		t.Fatalf("tras la decision = %+v; la segunda no puede haber escrito", got)
	}

	lista, err := repo.DeTitular(ctx, titularAna)
	if err != nil || len(lista) != 1 {
		t.Fatalf("DeTitular = %v, %v", lista, err)
	}
	if otros, err := repo.DeTitular(ctx, titularBeto); err != nil || len(otros) != 0 {
		t.Fatalf("DeTitular de Beto = %v, %v", otros, err)
	}
	if todos, err := repo.Listar(ctx); err != nil || len(todos) != 1 {
		t.Fatalf("Listar = %v, %v", todos, err)
	}
}

func sembrarAprobado(t *testing.T, s *Store, id, titular, monto string) {
	t.Helper()
	ctx := t.Context()
	repo := s.Anticipos()
	a := solicitudPrueba(id, titular, monto)
	if err := repo.Registrar(ctx, a, instanteAnticipos); err != nil {
		t.Fatalf("Registrar: %v", err)
	}
	a.Estado, a.Saldo, a.Soporte = anticipos.EstadoAprobado, a.Monto, "acta 12"
	if err := repo.Resolver(ctx, a, instanteAnticipos, usuarioAdmin); err != nil {
		t.Fatalf("Resolver: %v", err)
	}
}

// TestPorDescontarExigeUnidadYDescontarEsCondicional: el cerrojo de fila solo
// protege dentro de una transaccion, asi que fuera de una se niega; y
// Descontar no escribe sobre un saldo que otro ya movio.
func TestPorDescontarExigeUnidadYDescontarEsCondicional(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	repo := s.Anticipos()
	sembrarAprobado(t, s, "ant-1", titularAna, "1000")

	if _, _, err := repo.PorDescontar(ctx, titularAna); !errors.Is(err, errFueraDeUnidad) {
		t.Fatalf("PorDescontar sin unidad: err = %v, se esperaba errFueraDeUnidad", err)
	}

	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		a, hay, err := repo.PorDescontar(ctx, titularAna)
		if err != nil || !hay || a.ID != "ant-1" || !a.Saldo.Equal(pgDec("1000")) {
			t.Fatalf("PorDescontar = %+v, %v, %v", a, hay, err)
		}
		if _, hay, err := repo.PorDescontar(ctx, titularBeto); err != nil || hay {
			t.Fatalf("Beto no tiene anticipo: hay=%v err=%v", hay, err)
		}

		despues := a
		despues.Saldo = pgDec("400")
		// Un saldo anterior que no es el de la fila: alguien descontaria sobre
		// una lectura vieja.
		if err := repo.Descontar(ctx, despues, pgDec("999")); !errors.Is(err, anticipos.ErrEstadoInvalido) {
			t.Fatalf("Descontar con saldo viejo: err = %v, se esperaba ErrEstadoInvalido", err)
		}
		if err := repo.Descontar(ctx, despues, a.Saldo); err != nil {
			t.Fatalf("Descontar: %v", err)
		}
		saldado := despues
		saldado.Saldo, saldado.Estado = decimal.Zero, anticipos.EstadoDescontado
		if err := repo.Descontar(ctx, saldado, pgDec("400")); err != nil {
			t.Fatalf("Descontar hasta cero: %v", err)
		}
		if _, hay, err := repo.PorDescontar(ctx, titularAna); err != nil || hay {
			t.Fatalf("un descontado ya no se descuenta: hay=%v err=%v", hay, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unidad: %v", err)
	}
	got, err := repo.PorID(ctx, "ant-1")
	if err != nil || got.Anticipo.Estado != anticipos.EstadoDescontado || !got.Anticipo.Saldo.IsZero() {
		t.Fatalf("PorID = %+v, %v", got, err)
	}
}

// TestEmbargoVigenteMiraElDia: la vigencia es [reportado, levantado), y la
// pregunta es por un dia, no por hoy (RA 3.1.e, ADR 0027).
func TestEmbargoVigenteMiraElDia(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	ejecutar := ejecutorDePruebas(t, pool)
	ejecutar(`INSERT INTO embargos (titular_id, reportado, levantado, soporte)
	          VALUES ($1, '2026-03-01', '2026-06-01', 'oficio 1')`, titularAna)
	repo := s.Anticipos()

	dia := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return d.Add(15 * time.Hour)
	}
	for _, c := range []struct {
		dia    string
		quiero bool
	}{
		{"2026-02-28", false},
		{"2026-03-01", true},
		{"2026-05-31", true},
		{"2026-06-01", false},
	} {
		got, err := repo.EmbargoVigente(ctx, titularAna, dia(c.dia))
		if err != nil || got != c.quiero {
			t.Fatalf("EmbargoVigente(%s) = %v, %v; se esperaba %v", c.dia, got, err, c.quiero)
		}
	}
	if got, err := repo.EmbargoVigente(ctx, titularBeto, dia("2026-04-01")); err != nil || got {
		t.Fatalf("el embargo de Ana no es de Beto: %v, %v", got, err)
	}

	ejecutar(`INSERT INTO embargos (titular_id, reportado, soporte) VALUES ($1, '2026-09-01', 'oficio 2')`, titularAna)
	if got, err := repo.EmbargoVigente(ctx, titularAna, dia("2030-01-01")); err != nil || !got {
		t.Fatalf("un embargo sin levantar sigue vigente: %v, %v", got, err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO embargos (titular_id, reportado, levantado, soporte)
	                             VALUES ($1, '2026-03-01', '2026-03-01', 'oficio')`, titularAna); codigoPg(err) != "23514/embargo_vigencia_coherente" {
		t.Fatalf("un embargo levantado el dia que se reporto: err = %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO embargos (titular_id, reportado, soporte) VALUES ($1, '2026-03-01', ' ')`, titularAna); err == nil {
		t.Fatal("un embargo sin oficio de soporte no se admite")
	}
}

// TestClaseDeLeeElPadron es R-30: la clase sale de `titulares`.
func TestClaseDeLeeElPadron(t *testing.T) {
	s, _ := sembrar(t)
	repo := s.Anticipos()
	if c, err := repo.ClaseDe(t.Context(), titularAna); err != nil || c != afiliacion.ClaseSocio {
		t.Fatalf("Ana = %q, %v", c, err)
	}
	if c, err := repo.ClaseDe(t.Context(), titularBeto); err != nil || c != afiliacion.ClaseAdministrado {
		t.Fatalf("Beto = %q, %v", c, err)
	}
	if _, err := repo.ClaseDe(t.Context(), "tit-nadie"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("titular inexistente: err = %v", err)
	}
}

// TestDerechosLiquidadosSumaLasLineasDeLasOrdenesDeLaVentana es la base del 25%
// de RA 3.1.g (ADR 0027). Cada corrida prueba una exclusion distinta:
//
//   - prc-a: orden dentro de la ventana -> cuenta su LINEA (4000), no el neto
//     de la orden (3000, que ya lleva un anticipo descontado). La linea de
//     Beto en la misma corrida no es de Ana.
//   - prc-b: orden enviada justo hace dos anos -> fuera, la ventana es abierta
//     por abajo.
//   - prc-c: corrida con linea pero sin orden -> no se liquido, no cuenta.
//   - prc-d: orden enviada despues de hasta -> fuera.
func TestDerechosLiquidadosSumaLasLineasDeLasOrdenesDeLaVentana(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	ejecutar := ejecutorDePruebas(t, pool)

	sembrarCorridaEnVerificacion(t, pool, "prc-a", "bolsa-a", "2026-01", "4000")
	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
	          VALUES ('prc-a', $1, $2, 'IPI-00000002', 10, 300)`, obraCompleta, titularBeto)
	sembrarCorridaEnVerificacion(t, pool, "prc-b", "bolsa-b", "2026-02", "1000")
	sembrarCorridaEnVerificacion(t, pool, "prc-c", "bolsa-c", "2026-03", "500")
	sembrarCorridaEnVerificacion(t, pool, "prc-d", "bolsa-d", "2026-04", "700")

	orden := func(proceso, periodo, bruto, neto, enviada string) {
		ejecutar(`INSERT INTO ordenes_pago
		            (id, proceso_id, procesos, titular_id, periodo, circuito, bruto, neto, estado, enviada)
		          VALUES ('liq-'||$2||'-nacional-'||$3, $1, ARRAY[$1], $3, $2, 'nacional', $4, $5, 'enviada', $6)`,
			proceso, periodo, titularAna, pgDec(bruto), pgDec(neto), enviada)
	}
	orden("prc-a", "2026-01", "4000", "3000", "2025-01-10")
	ejecutar(`INSERT INTO ordenes_pago_deducciones (orden_id, concepto, monto)
	          VALUES ('liq-2026-01-nacional-'||$1, 'anticipo', 1000)`, titularAna)
	orden("prc-b", "2026-02", "1000", "1000", "2024-06-15")
	orden("prc-d", "2026-04", "700", "700", "2026-06-16")

	desde := instanteAnticipos.AddDate(-2, 0, 0)
	got, err := s.Anticipos().DerechosLiquidados(ctx, titularAna, desde, instanteAnticipos)
	if err != nil {
		t.Fatalf("DerechosLiquidados: %v", err)
	}
	if !got.Equal(pgDec("4000")) {
		t.Fatalf("derechos = %s, se esperaba solo la linea de prc-a (4000)", got)
	}
	if beto, err := s.Anticipos().DerechosLiquidados(ctx, titularBeto, desde, instanteAnticipos); err != nil || !beto.IsZero() {
		t.Fatalf("Beto no tiene orden propia: %s, %v", beto, err)
	}
}
