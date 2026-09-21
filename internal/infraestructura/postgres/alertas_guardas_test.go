package postgres

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
)

// Guardas que el codigo YA cumplia y que ninguna prueba defendia: cada una
// murio al mutar a mano la linea que sostiene.

// TestUnaResolucionRevertidaNoDejaAsientoHuerfano es el gemelo de
// [TestUnaPasadaRevertidaNoDejaAsientoHuerfano], para `Resolver`.
//
// Existe porque el arreglo del asiento de `Evaluar` estaba probado en las DOS
// direcciones y el de `Resolver` solo en una:
// [TestResolverAlertaYSuAsientoSonUnaSolaTransaccion] comprueba que un asiento
// que falla se lleva el UPDATE, pero no que una unidad revertida se lleve el
// asiento. Sustituir el `ctx` del `Asentar` de `Resolver` por
// `context.Background()` -- que es exactamente el bug que
// TestUnaPasadaRevertidaNoDejaAsientoHuerfano existe para cazar en `Evaluar` --
// pasaba entero.
//
// El modo de fallo es el peor de la bitacora: el asiento sobrevive al rollback
// y el libro afirma que alguien cerro una alerta que sigue abierta. `asientos`
// es append-only (ADR 0006), asi que esa linea no se corrige nunca.
func TestUnaResolucionRevertidaNoDejaAsientoHuerfano(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(alertas) == 0 {
		t.Fatal("no hay alertas que resolver")
	}
	id := alertas[0].ID

	// EnUnidad es reentrante: Resolver corre DENTRO de esta transaccion, asi
	// que abortarla tiene que llevarse el UPDATE y el asiento.
	abortar := errors.New("abortada a proposito")
	err = s.EnUnidad(t.Context(), func(ctx context.Context) error {
		if _, err := svc.Resolver(ctx, id, usuarioAdmin, "cerrada dentro de la unidad"); err != nil {
			return err
		}
		return abortar
	})
	if !errors.Is(err, abortar) {
		t.Fatalf("EnUnidad devolvio %v", err)
	}

	var resuelta bool
	if err := pool.QueryRow(t.Context(),
		`SELECT resuelta FROM alertas WHERE id = $1::uuid`, id).Scan(&resuelta); err != nil {
		t.Fatalf("releer la alerta: %v", err)
	}
	if resuelta {
		t.Fatal("la alerta sobrevivio resuelta a la reversion")
	}

	var asientos int
	if err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM asientos WHERE hecho = $1`,
		aplicacion.HechoAlertaResuelta).Scan(&asientos); err != nil {
		t.Fatalf("contar asientos: %v", err)
	}
	if asientos != 0 {
		t.Fatalf("sobrevivieron %d asientos de resolucion a la reversion: "+
			"el asiento de Resolver no comparte transaccion con el UPDATE", asientos)
	}
}

// TestEvaluarDevuelveLasCriticasAbiertasDelPeriodo asserta el campo que la
// compuerta de #34 va a leer, de punta a punta y contra Postgres.
//
// Nadie lo hacia: la prueba de este paquete llamaba a `CriticasAbiertas` por
// SEPARADO y la de httpapi usaba un doble con un 3 enlatado, asi que borrar el
// calculo entero de `Evaluar` -- dejando el campo en su cero -- dejaba todo en
// verde. Y un cero ahi no es un numero equivocado cualquiera: es el valor que
// DEJA PASAR la compuerta.
//
// El fixture tiene tres criticas (los dos duplicados y tipo_obra_sin_mapear) y
// tres que no lo son (oni y las dos de declaracion), asi que la cifra distingue
// "conto las criticas" de "conto todas".
func TestEvaluarDevuelveLasCriticasAbiertasDelPeriodo(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	resumen, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	// Cuantas de las detectadas bloquean, segun el dominio.
	quiero := 0
	for tipo, n := range resumen.PorTipo {
		if anomalias.EsCritica(tipo) {
			quiero += n
		}
	}
	if quiero == 0 {
		t.Fatal("el fixture no levanta ninguna anomalia critica: la prueba no distinguiria un cero de un acierto")
	}
	if resumen.CriticasAbiertas != quiero {
		t.Fatalf("CriticasAbiertas = %d, se esperaban %d.\n"+
			"Un cero aqui deja pasar la compuerta de #34 sin que nada falle.",
			resumen.CriticasAbiertas, quiero)
	}
	if resumen.CriticasAbiertas == resumen.Detectadas {
		t.Fatalf("CriticasAbiertas (%d) es igual a Detectadas (%d): se estan contando todas, no solo las que bloquean",
			resumen.CriticasAbiertas, resumen.Detectadas)
	}

	// Y baja al resolver una critica: es el predicado, no una foto fija.
	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	i := slices.IndexFunc(alertas, func(a aplicacion.Alerta) bool { return a.Critica })
	if i < 0 {
		t.Fatal("ninguna alerta llego marcada como critica")
	}
	if _, err := svc.Resolver(t.Context(), alertas[i].ID, usuarioAdmin, ""); err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	despues, err := svc.CriticasAbiertas(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("CriticasAbiertas: %v", err)
	}
	if despues != quiero-1 {
		t.Fatalf("tras resolver una critica quedan %d, se esperaban %d", despues, quiero-1)
	}
}

// TestElResumenCuentaLasFilasSinClaveDeRegistro es la guarda de la cifra del
// punto ciego (M7), de punta a punta.
//
// El fixture siembra filas con `hora` vacia: `ClaveDeRegistro` no las puede
// componer, asi que `duplicadosPorRegistro` no las compara con ninguna. Sin
// esta cifra, "cero duplicados" y "no se miro" se leen igual.
func TestElResumenCuentaLasFilasSinClaveDeRegistro(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()

	// Dos filas de Caracol SIN hora: su clave de registro no se puede componer
	// porque `MapaCaracol` mete Fecha y Hora en ClaveRegistro.
	insertarUsoDeAlertas(t, pool, "u-sinhora-1", repA, "alias", obraCompleta, "serie", "", "", "")
	insertarUsoDeAlertas(t, pool, "u-sinhora-2", repA, "alias", obraCompleta, "serie", "", "", "")

	svc := servicioDeAnomalias(s, instanteAlertas)
	resumen, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	if resumen.UsosSinCotejar < 2 {
		t.Fatalf("UsosSinCotejar = %d, se esperaban al menos 2: las filas sin clave componible "+
			"no se comparan con ninguna y el hueco tiene que quedar dicho", resumen.UsosSinCotejar)
	}

	// Y la propiedad que justifica contarlas: las dos filas sin clave NO se
	// marcan como duplicadas entre si, que bloquearia el periodo.
	for _, id := range []string{"u-sinhora-1", "u-sinhora-2"} {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM alertas WHERE tipo = $1 AND ref_id = $2`,
			anomalias.TipoDuplicadoRegistro, id).Scan(&n); err != nil {
			t.Fatalf("contar alertas de %q: %v", id, err)
		}
		if n != 0 {
			t.Fatalf("la fila sin clave %q salio marcada como duplicada", id)
		}
	}

	// La cifra queda ademas en el asiento, que es lo que se audita.
	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM asientos WHERE hecho = $1 ORDER BY id DESC LIMIT 1`,
		aplicacion.HechoAnomaliasEvaluadas).Scan(&payload); err != nil {
		t.Fatalf("leer el asiento: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"usos_sin_cotejar"`)) {
		t.Fatalf("el asiento de la pasada no registra usos_sin_cotejar: %s", payload)
	}
}

// TestListarAlertasNoDevuelveUnaListaTruncadaComoCompleta defiende el
// `filas.Err()` de [Store.ListarAlertas].
//
// Quitarlo no rompia nada: un fallo a mitad de stream solo sale por ahi, y sin
// la comprobacion una lista TRUNCADA se devuelve como lista completa. Sobre
// esta tabla eso se lee como "quedan menos anomalias de las que hay", que es la
// direccion en la que el error deja pasar el reparto.
//
// Se provoca cortando el contexto a mitad de stream. El instante exacto en que
// cae el corte no es determinista, asi que la prueba es de PROPIEDAD y se
// repite: en cada intento, o sale error, o sale la lista COMPLETA. Lo que no
// puede salir nunca es una lista corta con error nil, que es exactamente lo que
// devuelve la version sin `filas.Err()`.
func TestListarAlertasNoDevuelveUnaListaTruncadaComoCompleta(t *testing.T) {
	s, _ := sembrar(t)
	const sembradas = 400
	sembrarMuchasCriticas(t, s, periodoAlertas, sembradas)

	todas := aplicacion.FiltroAlertas{
		Periodo:    periodoAlertas,
		Paginacion: aplicacion.Paginacion{Limite: aplicacion.LimiteSinTope},
	}

	// Una lectura limpia primero, para saber que el sembrado esta entero y que
	// el fallo de abajo, si llega, es del corte y no del fixture.
	if base, err := s.ListarAlertas(t.Context(), todas); err != nil || len(base) != sembradas {
		t.Fatalf("lectura de control: %d alertas, err=%v", len(base), err)
	}

	// Timeouts crecientes: alguno cae con la consulta ya abierta y el stream a
	// medias, que es la ventana donde vive el fallo.
	for i := range 40 {
		plazo := time.Duration(i+1) * 100 * time.Microsecond
		ctx, cancelar := context.WithTimeout(t.Context(), plazo)
		alertas, err := s.ListarAlertas(ctx, todas)
		cancelar()

		if err != nil {
			continue // el corte se noto, que es lo correcto
		}
		if len(alertas) != sembradas {
			t.Fatalf("intento %d (plazo %v): ListarAlertas devolvio %d de %d alertas SIN error.\n"+
				"Una lista truncada por un fallo a mitad de stream esta pasando por lista completa: "+
				"sobre esta tabla eso se lee como \"quedan menos anomalias de las que hay\".",
				i+1, plazo, len(alertas), sembradas)
		}
	}
}

// TestElOrdenDeLaBandejaEsTotal defiende el desempate `, id` del ORDER BY.
//
// Quitarlo no rompia ninguna prueba, y el ADR 0005 lo exige: todas las alertas
// de una MISMA pasada llevan el mismo instante del Reloj -- a proposito -- asi
// que `detectada DESC` sola no las desempata y el listado cambia de orden entre
// lecturas. Con paginacion deja de ser cosmetico: dos paginas consecutivas
// pueden repetir una fila y saltarse otra.
//
// # Por que la lectura de control va SIN filtro de periodo
//
// Porque con `?periodo=` el indice `alertas_periodo (periodo, detectada DESC,
// id)` sirve la consulta ya ordenada por id, y el desempate del SQL resulta
// INDISTINGUIBLE de no tenerlo: medido, con filtro el orden sale por id igual
// aunque se quite `, id`, y sin filtro sale arbitrario. O sea que una prueba
// que solo mirara el camino del indice no defiende nada -- deja el ORDER BY
// apoyado en un indice que cualquiera puede cambiar o borrar en otra migracion,
// y el dia que eso pase la bandeja empieza a cambiar de orden sin que nada
// avise.
func TestElOrdenDeLaBandejaEsTotal(t *testing.T) {
	s, _ := sembrar(t)
	// Todas con el MISMO `detectada`, que es lo que hace una pasada real: es la
	// unica forma de que el desempate tenga algo que desempatar.
	sembrarMuchasCriticas(t, s, periodoAlertas, 300)

	leer := func(periodo string) []string {
		t.Helper()
		alertas, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
			Periodo:    periodo,
			Paginacion: aplicacion.Paginacion{Limite: aplicacion.LimiteSinTope},
		})
		if err != nil {
			t.Fatalf("ListarAlertas(%q): %v", periodo, err)
		}
		ids := make([]string, 0, len(alertas))
		for _, a := range alertas {
			ids = append(ids, a.ID)
		}
		return ids
	}

	// Sin filtro: el plan no puede apoyarse en el indice de periodo, asi que
	// aqui el orden lo tiene que poner el ORDER BY y nada mas.
	sinFiltro := leer("")
	if len(sinFiltro) != 300 {
		t.Fatalf("se leyeron %d de 300 alertas", len(sinFiltro))
	}
	ordenado := slices.Clone(sinFiltro)
	slices.Sort(ordenado)
	if !slices.Equal(sinFiltro, ordenado) {
		t.Fatalf("con `detectada` empatado el listado no sale ordenado por id.\n" +
			"Sin el desempate `, id` del ORDER BY, las alertas de una misma pasada no tienen orden definido " +
			"(ADR 0005) y dos paginas consecutivas pueden repetir una fila y saltarse otra.")
	}

	// Y el camino del indice tiene que dar lo mismo, lectura tras lectura.
	conFiltro := leer(periodoAlertas)
	for i := range 3 {
		if otra := leer(periodoAlertas); !slices.Equal(conFiltro, otra) {
			t.Fatalf("la lectura %d del periodo devolvio otro orden", i+2)
		}
	}
}
