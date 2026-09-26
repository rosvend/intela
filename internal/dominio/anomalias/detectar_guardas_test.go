package anomalias

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Este fichero recoge las guardas que el codigo YA cumplia y que ninguna
// prueba defendia: cada una murio al mutar a mano la linea que sostiene, y sin
// ellas esa mutacion dejaba el paquete entero en verde.

// ---------------------------------------------------------------------------
// El orden de Detectar es TOTAL

// TestElOrdenDeDetectarDesempataPorRefTitular fija la cuarta clave del orden.
//
// Con el fixture de `periodoConUnaDeCadaAnomalia` hay un solo
// `titular_sin_porcentaje`, asi que cambiar el desempate
// `strings.Compare(a.RefTitular, b.RefTitular)` por `return 0` dejaba el
// paquete en verde: no habia dos hallazgos que empataran en las tres primeras
// claves. Aqui hay tres.
//
// Importa por dos razones. El ADR 0005 exige que dos pasadas sobre el mismo
// dato devuelvan la MISMA lista, y `slices.SortFunc` NO es estable: sin la
// cuarta clave el orden entre empatados es el que deje el algoritmo, y puede
// cambiar entre versiones de Go sin que nada avise. Y es ademas la guarda que
// sostiene el prefijo de [Hallazgo.RefTitular]: si dos hallazgos del mismo tipo
// y la misma obra no tienen orden definido, cual de los dos se escribe primero
// -- y cual sobrevive a un ON CONFLICT DO NOTHING -- no lo fija nada.
func TestElOrdenDeDetectarDesempataPorRefTitular(t *testing.T) {
	t.Parallel()

	// Una obra con TRES coautores sin parte: los tres hallazgos comparten
	// tipo, ref_tipo y ref_id, y solo se distinguen por ref_titular.
	//
	// Los coautores llegan DESORDENADOS a proposito. Con la lista ya ordenada
	// los hallazgos salen del detector en el orden bueno por casualidad, el
	// SortFunc no tiene nada que mover y quitarle el cuarto desempate no se
	// nota: asi es como este mutante sobrevivia. Desordenados, el orden final
	// solo puede venir del SortFunc.
	p := Periodo{
		Periodo: "2025-01",
		Usos:    []Uso{{ID: "u1", ObraID: "o1", TipoObra: "serie", Modalidad: ModalidadTV}},
		Obras: []Obra{{
			ID:           "o1",
			CoautoresIPI: []string{"ipi-c", "ipi-a", "ipi-b"},
			Declarada:    true,
			Declaracion: repertorio.Declaracion{
				ObraID: "o1", Partes: []repertorio.Parte{parte("tit-z", "ipi-z", 100)},
			},
		}},
	}

	var titulares []string
	for _, h := range Detectar(p) {
		if h.Tipo == TipoTitularSinPorcentaje {
			titulares = append(titulares, h.RefTitular)
		}
	}

	quiero := []string{PrefijoIPI + "ipi-a", PrefijoIPI + "ipi-b", PrefijoIPI + "ipi-c"}
	if !slices.Equal(titulares, quiero) {
		t.Fatalf("orden de ref_titular = %v, se esperaba %v: sin el cuarto desempate el orden entre hallazgos "+
			"que empatan en (tipo, ref_tipo, ref_id) no esta definido y SortFunc no es estable", titulares, quiero)
	}

	// Y la propiedad de verdad: dos pasadas dan lo mismo.
	if segunda := refsDe(Detectar(p)); !slices.Equal(refsDe(Detectar(p)), segunda) {
		t.Fatal("dos pasadas sobre el mismo periodo devolvieron listas distintas (ADR 0005)")
	}
}

// ---------------------------------------------------------------------------
// Los dos espacios de nombres de ref_titular

// TestRefTitularDistingueIPIDeTitularID es la guarda de B5: los dos casos del
// detector 4 referencian espacios DISTINTOS y no se pueden pisar.
//
// El fallo que cierra es silencioso de principio a fin: la clave natural de
// `alertas` es UNIQUE (periodo, tipo, ref_tipo, ref_id, ref_titular) con
// ON CONFLICT DO NOTHING, asi que si un `titulares.id` coincide con un IPI --
// los dos son TEXT y nada lo impide -- los dos hallazgos colapsan en una fila y
// uno DESAPARECE sin que nada falle. Reproducido antes del arreglo: se mandaban
// 2 hallazgos distintos y la base reportaba 1 nueva.
func TestRefTitularDistingueIPIDeTitularID(t *testing.T) {
	t.Parallel()

	// El caso adversario: el titular_id de la parte sin IPI es EXACTAMENTE el
	// mismo string que el IPI del coautor sin parte.
	const colision = "00000042"

	obras := []Obra{{
		ID:           "o1",
		CoautoresIPI: []string{colision},
		Declarada:    true,
		Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
			parte(colision, "", 100),
		}},
	}}

	hallazgos := titularesSinPorcentaje(obras)
	if len(hallazgos) != 2 {
		t.Fatalf("hallazgos = %d, se esperaban 2 (el coautor sin parte y la parte sin IPI)", len(hallazgos))
	}

	claves := make(map[string]bool, 2)
	for _, h := range hallazgos {
		// La clave natural de `alertas`, tal cual la escribe la migracion.
		claves[h.Tipo+"|"+h.RefTipo+"|"+h.RefID+"|"+h.RefTitular] = true
	}
	if len(claves) != 2 {
		t.Fatalf("los dos hallazgos comparten clave natural (%v): uno de los dos se perdera "+
			"con el ON CONFLICT DO NOTHING, y cual sobrevive no lo fija nada", claves)
	}

	if hallazgos[0].RefTitular != PrefijoIPI+colision {
		t.Fatalf("el coautor sin parte referencia %q, se esperaba el IPI con prefijo %q",
			hallazgos[0].RefTitular, PrefijoIPI)
	}
	if hallazgos[1].RefTitular != PrefijoTitular+colision {
		t.Fatalf("la parte sin IPI referencia %q, se esperaba el titular_id con prefijo %q",
			hallazgos[1].RefTitular, PrefijoTitular)
	}
}

// ---------------------------------------------------------------------------
// El duplicado nombra la fila repetida, no la primera

// TestElDuplicadoNombraLaTerceraYNoLaPrimera prueba con TRES filas.
//
// Con dos, hacer que `vistas[clave]` recuerde la ULTIMA repeticion en vez de la
// primera deja el paquete en verde: con una sola repeticion las dos lecturas
// coinciden. Con tres se separan, y el godoc promete la primera -- "la alerta va
// sobre la fila REPETIDA, no sobre la primera: la primera es un registro
// legitimo".
//
// El detalle importa porque es lo que lee quien resuelve: si nombra la segunda
// repeticion como "la que ya venia", quien decida cual borrar se lleva por
// delante una fila que tambien era repetida y deja la original intacta creyendo
// lo contrario.
func TestElDuplicadoNombraLaTerceraYNoLaPrimera(t *testing.T) {
	t.Parallel()

	usos := []Uso{
		{ID: "u1", ReporteID: "r1", Fuente: "caracol", ClaveRegistro: "id_ficha=7|2025-01-02|20:00"},
		{ID: "u2", ReporteID: "r2", Fuente: "caracol", ClaveRegistro: "id_ficha=7|2025-01-02|20:00"},
		{ID: "u3", ReporteID: "r3", Fuente: "caracol", ClaveRegistro: "id_ficha=7|2025-01-02|20:00"},
	}

	hallazgos := duplicadosPorRegistro(usos)
	if got := refsDe(hallazgos); !slices.Equal(got, []string{
		"duplicado_registro|uso:u2", "duplicado_registro|uso:u3",
	}) {
		t.Fatalf("duplicadosPorRegistro = %v, se esperaban alertas sobre u2 y u3", got)
	}

	// Las DOS tienen que nombrar a u1, que es la primera. La tercera es la que
	// separa "recuerda la primera" de "recuerda la ultima".
	for _, h := range hallazgos {
		if !strings.Contains(h.Detalle, `ya venia en el uso "u1"`) {
			t.Fatalf("la alerta sobre %q no nombra a u1 como la primera: %s", h.RefID, h.Detalle)
		}
	}
}

// TestElDuplicadoComparaEntreEntregasDistintas fija lo que el godoc del
// detector 3 dice.
//
// La prueba que parecia defenderlo fijaba lo CONTRARIO sin saberlo: sus tres
// filas dejaban `ReporteID` sin fijar, o sea todas en la entrega "", asi que no
// habia ni una sola comparacion entre archivos distintos. Aqui las entregas son
// distintas de verdad.
//
// Y la otra mitad de la decision: la comparacion NO filtra por entrega. Dos
// filas de la MISMA entrega con la misma clave tambien se avisan. Que la
// ingesta ya rechace ese caso no es razon para no mirarlo -- `claveDe` compara
// celdas crudas y esta funcion una clave normalizada, asi que las dos puertas
// no dejan pasar lo mismo.
func TestElDuplicadoComparaEntreEntregasDistintas(t *testing.T) {
	t.Parallel()

	t.Run("entre entregas distintas: avisa", func(t *testing.T) {
		t.Parallel()
		got := refsDe(duplicadosPorRegistro([]Uso{
			{ID: "u1", ReporteID: "entrega-enero-semana1", Fuente: "caracol", ClaveRegistro: "k"},
			{ID: "u2", ReporteID: "entrega-enero-semana2", Fuente: "caracol", ClaveRegistro: "k"},
		}))
		if !slices.Equal(got, []string{"duplicado_registro|uso:u2"}) {
			t.Fatalf("= %v, se esperaba una alerta sobre u2", got)
		}
	})

	t.Run("dentro de la misma entrega: tambien avisa", func(t *testing.T) {
		t.Parallel()
		got := refsDe(duplicadosPorRegistro([]Uso{
			{ID: "u1", ReporteID: "misma", Fuente: "caracol", ClaveRegistro: "k"},
			{ID: "u2", ReporteID: "misma", Fuente: "caracol", ClaveRegistro: "k"},
		}))
		if !slices.Equal(got, []string{"duplicado_registro|uso:u2"}) {
			t.Fatalf("= %v: la comparacion no filtra por entrega a proposito", got)
		}
	})

	t.Run("otra fuente con la misma clave: no es duplicado", func(t *testing.T) {
		t.Parallel()
		// Los ids de fuente NO cruzan entre fuentes (ADR 0018): que el id=7 de
		// cine coincida con el id_ficha=7 de Caracol no significa nada.
		got := refsDe(duplicadosPorRegistro([]Uso{
			{ID: "u1", ReporteID: "r1", Fuente: "caracol", ClaveRegistro: "k"},
			{ID: "u2", ReporteID: "r2", Fuente: "cine", ClaveRegistro: "k"},
		}))
		if len(got) != 0 {
			t.Fatalf("= %v, se esperaba ninguna: la fuente entra en la clave", got)
		}
	})
}

// ---------------------------------------------------------------------------
// El punto ciego se cuenta

// TestSinClaveDeRegistroCuentaLasFilasNoCotejadas defiende que el silencio de
// `duplicadosPorRegistro` sobre las filas sin clave quede DICHO.
//
// No compararlas es correcto -- agruparlas bajo la cadena vacia bloquearia el
// periodo entero --, pero sin esta cifra el tablero muestra cero duplicados y
// nadie sabe sobre cuantas filas no se miro. El caso es real: `Hora` es
// `Requerida: false` en `MapaCaracol` y a la vez componente de su
// `ClaveRegistro`.
func TestSinClaveDeRegistroCuentaLasFilasNoCotejadas(t *testing.T) {
	t.Parallel()

	usos := []Uso{
		{ID: "u1", Fuente: "caracol", ClaveRegistro: "k1"},
		{ID: "u2", Fuente: "caracol", ClaveRegistro: ""},
		{ID: "u3", Fuente: "caracol", ClaveRegistro: ""},
		{ID: "u4", Fuente: "caracol", ClaveRegistro: "k2"},
	}

	if n := SinClaveDeRegistro(usos); n != 2 {
		t.Fatalf("SinClaveDeRegistro = %d, se esperaban 2", n)
	}
	// Y la propiedad que justifica contarlas: esas dos NO se comparan entre si
	// aunque las dos tengan la clave vacia.
	if got := refsDe(duplicadosPorRegistro(usos)); len(got) != 0 {
		t.Fatalf("las filas sin clave se compararon entre si: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Los detalles dicen la verdad

// TestElDetalleDeLaRetencionNarraElMotivoReal es la guarda de M6.
//
// `Completa()` falla por TRES cosas -- una parte sin IPI, una parte no
// positiva, y la suma que no da 100 -- y el detalle contaba siempre la tercera.
// Una obra cuyas partes suman 100 clavado pero a la que le falta un IPI salia
// como "suma 100% y no llega a 100", que ademas se contradice sola: mandaba a
// distribucion a perseguir un porcentaje mientras lo que falta es un
// identificador de persona.
//
// Asserta el TEXTO y no solo la referencia: la referencia era identica en los
// dos casos, que es exactamente por lo que el fallo pasaba en verde.
func TestElDetalleDeLaRetencionNarraElMotivoReal(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre  string
		obra    Obra
		incluye []string
		excluye []string
	}{
		{
			// El caso que mentia: la suma ES 100 y aun asi retiene.
			nombre: "suma 100 exactos pero una parte no trae IPI",
			obra: Obra{ID: "o1", Declarada: true, Declaracion: repertorio.Declaracion{
				ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60), parte("tit-b", "", 40),
				},
			}},
			incluye: []string{"no traen IPI", "tit-b", "a quien se le paga"},
			excluye: []string{"no llega a 100", "R-04 exige 100 exactos"},
		},
		{
			nombre: "la suma se queda corta de verdad",
			obra: Obra{ID: "o1", Declarada: true, Declaracion: repertorio.Declaracion{
				ObraID: "o1", Partes: []repertorio.Parte{parte("tit-a", "ipi-a", 60)},
			}},
			incluye: []string{"suma 60%", "R-04 exige 100 exactos"},
			excluye: []string{"no traen IPI"},
		},
		{
			// 150 y -50 suman 100: sin la comprobacion por parte, una
			// declaracion imposible pasaria por completa.
			nombre: "una parte no positiva",
			obra: Obra{ID: "o1", Declarada: true, Declaracion: repertorio.Declaracion{
				ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 150),
					{TitularID: "tit-b", IPI: "ipi-b", Porcentaje: decimal.NewFromInt(-50)},
				},
			}},
			incluye: []string{"no son positivas", "tit-b"},
			excluye: []string{"no traen IPI"},
		},
		{
			nombre: "version abierta y vacia",
			obra: Obra{ID: "o1", Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o1"}},
			incluye: []string{"abierta y no tiene ninguna parte declarada"},
			excluye: []string{"no traen IPI"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			hs := retencionPorDeclaracionIncompleta([]Obra{c.obra})
			if len(hs) != 1 {
				t.Fatalf("hallazgos = %d, se esperaba 1", len(hs))
			}
			for _, frase := range c.incluye {
				if !strings.Contains(hs[0].Detalle, frase) {
					t.Fatalf("el detalle no dice %q:\n%s", frase, hs[0].Detalle)
				}
			}
			for _, frase := range c.excluye {
				if strings.Contains(hs[0].Detalle, frase) {
					t.Fatalf("el detalle dice %q y no es el motivo real:\n%s", frase, hs[0].Detalle)
				}
			}
		})
	}
}

// TestLaObraSinDeclaracionNombraASusCoautores es la guarda de M9.
//
// El caso PEOR -- la obra sin ninguna declaracion -- era el unico que se
// quedaba sin un solo nombre: `titularesSinPorcentaje` calla ahi a proposito, y
// el detector 5 hablaba solo de la obra. La misma obra con una version abierta
// y VACIA emite N alertas con nombre y apellido, asi que el argumento de "no
// inundar el tablero" se desmontaba solo. `R-04` retiene el total en los dos
// casos y para desbloquearlo hay que llamar a alguien.
func TestLaObraSinDeclaracionNombraASusCoautores(t *testing.T) {
	t.Parallel()

	hs := retencionPorDeclaracionIncompleta([]Obra{{
		ID:           "o1",
		CoautoresIPI: []string{"ipi-a", "ipi-b"},
		Declarada:    false,
	}})
	if len(hs) != 1 {
		t.Fatalf("hallazgos = %d, se esperaba 1", len(hs))
	}
	for _, ipi := range []string{"ipi-a", "ipi-b"} {
		if !strings.Contains(hs[0].Detalle, ipi) {
			t.Fatalf("el detalle de la obra sin declaracion no nombra a %q:\n%s", ipi, hs[0].Detalle)
		}
	}

	// Sin coautores registrados no se inventa una frase vacia.
	sinCoautores := retencionPorDeclaracionIncompleta([]Obra{{ID: "o2", Declarada: false}})
	if strings.Contains(sinCoautores[0].Detalle, "coautor(es)") {
		t.Fatalf("una obra sin coautores no tendria que prometer nombres:\n%s", sinCoautores[0].Detalle)
	}
}

// TestElDetalleDelDuplicadoDeArchivoNoNiegaElPeriodo es la otra mitad de M6.
//
// La frase "no comparten fuente ni periodo" era TEXTO FIJO, y el periodo si
// puede coincidir -- de hecho coincide siempre que las dos patas caen en el mes
// evaluado. El propio detalle se desmentia dos lineas mas arriba, donde ya
// imprime el periodo de cada entrega.
func TestElDetalleDelDuplicadoDeArchivoNoNiegaElPeriodo(t *testing.T) {
	t.Parallel()

	// Las dos entregas del MISMO periodo, distinta fuente: justo lo que el
	// UNIQUE (sha256, fuente) deja pasar y lo que la frase negaba.
	hs := duplicadosPorHuella("2025-01", []Entrega{
		{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
		{ID: "r2", Fuente: "netflix", Periodo: "2025-01", SHA256: "aa"},
	})
	if len(hs) != 2 {
		t.Fatalf("hallazgos = %d, se esperaban 2 (una por cada entrega del periodo)", len(hs))
	}
	for _, h := range hs {
		if strings.Contains(h.Detalle, "no comparten fuente ni periodo") {
			t.Fatalf("el detalle niega que compartan periodo y lo comparten:\n%s", h.Detalle)
		}
		if !strings.Contains(h.Detalle, "el periodo no entra en esa clave") {
			t.Fatalf("el detalle no explica por que el UNIQUE no lo impide:\n%s", h.Detalle)
		}
	}
}

// TestElDetalleDeTipoObraNoAfirmaUnaParadaQueNoPasa es la guarda del texto de
// B3.
//
// El detalle decia "la corrida de TV se aborta" sobre CUALQUIER fila, incluida
// una de Netflix que no pasa por el motor de TV. Ahora nombra la modalidad de
// la fila y dice que el aborto ocurre "en cuanto esta fila entre en una
// corrida", que es lo unico cierto: hoy `MapaCaracol` tampoco mapea `canal_id`,
// asi que `UsosDeCanal` ni siquiera devuelve estas filas.
func TestElDetalleDeTipoObraNoAfirmaUnaParadaQueNoPasa(t *testing.T) {
	t.Parallel()

	hs := tipoObraSinMapear([]Uso{
		{ID: "u1", ObraID: "o1", Fuente: "caracol", Modalidad: ModalidadSuscripcion},
	})
	if len(hs) != 1 {
		t.Fatalf("hallazgos = %d, se esperaba 1", len(hs))
	}
	if !strings.Contains(hs[0].Detalle, "suscripcion") {
		t.Fatalf("el detalle no nombra la modalidad de la fila:\n%s", hs[0].Detalle)
	}
	if !strings.Contains(hs[0].Detalle, "en cuanto esta fila entre en una") {
		t.Fatalf("el detalle afirma una parada incondicional:\n%s", hs[0].Detalle)
	}
}

// ---------------------------------------------------------------------------
// ModalidadPonderaPorTipoObra

// TestModalidadPonderaPorTipoObra fija las siete modalidades del CHECK de
// `usos.modalidad` una por una, contra lo que hace el motor.
//
// Se lee del codigo de `reparto`: `ponderacionTipo` -- la unica funcion que
// mira TipoObra -- solo la llama `puntosTV`, alcanzable desde `case TV:` y
// desde `repartirSuscripcionOrden`, que sirve a `case Suscripcion, Hotel:`.
func TestModalidadPonderaPorTipoObra(t *testing.T) {
	t.Parallel()

	casos := map[string]bool{
		"tv":          true,
		"suscripcion": true,
		"hotel":       true,
		"cine":        false,
		"teatro":      false,
		"transporte":  false,
		"ott":         false,
		// Ante la duda se avisa: callar seria el lado silencioso.
		"":      true,
		"radio": true,
	}
	for modalidad, quiero := range casos {
		if got := ModalidadPonderaPorTipoObra(modalidad); got != quiero {
			t.Errorf("ModalidadPonderaPorTipoObra(%q) = %v, se esperaba %v", modalidad, got, quiero)
		}
	}
}
