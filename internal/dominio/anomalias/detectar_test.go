package anomalias

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// refsDe reduce los hallazgos a "tipo|ref_tipo:ref_id[#ref_titular]", que es
// lo que se compara en la mayoria de los casos: el detalle es prosa y atarlo
// palabra por palabra haria fallar la prueba al corregir una frase.
func refsDe(hs []Hallazgo) []string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		ref := h.Tipo + "|" + h.RefTipo + ":" + h.RefID
		if h.RefTitular != "" {
			ref += "#" + h.RefTitular
		}
		out = append(out, ref)
	}
	return out
}

func parte(titularID, ipi string, pct int64) repertorio.Parte {
	return repertorio.Parte{TitularID: titularID, IPI: ipi, Porcentaje: decimal.NewFromInt(pct)}
}

// ---------------------------------------------------------------------------
// 1. ONI

func TestDeteccionONI(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		usos   []Uso
		quiero []string
	}{
		{
			nombre: "el escalon oni levanta alerta",
			usos:   []Uso{{ID: "u1", Escalon: identificacion.EscalonONI, Titulo: "Programa X"}},
			quiero: []string{"oni|uso:u1"},
		},
		{
			// El caso que mas caro sale equivocarse: `usos.oni` es DEFAULT
			// TRUE, asi que una fila recien ingerida esta pendiente Y con la
			// bandera puesta. Filtrar por la bandera marcaria como ONI una
			// entrega entera que nadie ha intentado identificar todavia.
			nombre: "pendiente NO es oni aunque la bandera oni este puesta",
			usos:   []Uso{{ID: "u1", Escalon: identificacion.EscalonPendiente}},
			quiero: nil,
		},
		{
			// R-27 / RD 9.5, migracion 00007: fuera de repertorio no es lo
			// mismo que no identificada.
			nombre: "excluido NO es oni",
			usos:   []Uso{{ID: "u1", Escalon: identificacion.EscalonExcluido}},
			quiero: nil,
		},
		{
			nombre: "los escalones que resolvieron no levantan nada",
			usos: []Uso{
				{ID: "u1", Escalon: identificacion.EscalonAlias, ObraID: "o1"},
				{ID: "u2", Escalon: identificacion.EscalonIDGlobal, ObraID: "o1"},
				{ID: "u3", Escalon: identificacion.EscalonDifuso, ObraID: "o2"},
				{ID: "u4", Escalon: identificacion.EscalonManual, ObraID: "o2"},
			},
			quiero: nil,
		},
		{
			nombre: "sin usos no hay hallazgos",
			usos:   nil,
			quiero: nil,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if got := refsDe(deteccionONI(c.usos)); !slices.Equal(got, c.quiero) {
				t.Fatalf("deteccionONI = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

func TestDeteccionONINombraLaEntregaYElTitulo(t *testing.T) {
	t.Parallel()

	hs := deteccionONI([]Uso{{
		ID: "u1", ReporteID: "rep-1", Fuente: "caracol",
		Titulo: "Noticiero de la Noche", Escalon: identificacion.EscalonONI,
	}})
	if len(hs) != 1 {
		t.Fatalf("se esperaba un hallazgo, hubo %d", len(hs))
	}
	// Quien persigue la alerta tiene que poder pedirle al cliente la linea
	// exacta sin abrir la base.
	for _, quiero := range []string{"Noticiero de la Noche", "caracol", "rep-1"} {
		if !strings.Contains(hs[0].Detalle, quiero) {
			t.Fatalf("el detalle %q no nombra %q", hs[0].Detalle, quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Duplicado por huella

func TestDuplicadosPorHuella(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		periodo  string
		entregas []Entrega
		quiero   []string
	}{
		{
			nombre:  "misma huella en dos fuentes del mismo periodo: las dos",
			periodo: "2025-01",
			entregas: []Entrega{
				{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
				{ID: "r2", Fuente: "netflix", Periodo: "2025-01", SHA256: "aa"},
			},
			quiero: []string{"duplicado_archivo|reporte:r1", "duplicado_archivo|reporte:r2"},
		},
		{
			// El periodo NO esta en el UNIQUE (sha256, fuente), asi que la
			// otra pata puede vivir en otro mes. Solo se alerta sobre la del
			// periodo evaluado.
			nombre:  "misma huella cruzando periodos: solo la del periodo evaluado",
			periodo: "2025-02",
			entregas: []Entrega{
				{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
				{ID: "r2", Fuente: "netflix", Periodo: "2025-02", SHA256: "aa"},
			},
			quiero: []string{"duplicado_archivo|reporte:r2"},
		},
		{
			nombre:  "huellas distintas no colisionan",
			periodo: "2025-01",
			entregas: []Entrega{
				{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
				{ID: "r2", Fuente: "netflix", Periodo: "2025-01", SHA256: "bb"},
			},
			quiero: nil,
		},
		{
			nombre:  "una sola entrega nunca colisiona consigo misma",
			periodo: "2025-01",
			entregas: []Entrega{
				{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
			},
			quiero: nil,
		},
		{
			// Sin esta guarda, dos entregas con la huella vacia -- que el
			// CHECK del esquema no permite, pero un doble de prueba si --
			// saldrian como duplicadas entre si.
			nombre:  "la huella vacia no agrupa",
			periodo: "2025-01",
			entregas: []Entrega{
				{ID: "r1", Fuente: "caracol", Periodo: "2025-01"},
				{ID: "r2", Fuente: "netflix", Periodo: "2025-01"},
			},
			quiero: nil,
		},
		{
			nombre:  "la colision entera fuera del periodo no levanta nada",
			periodo: "2025-03",
			entregas: []Entrega{
				{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
				{ID: "r2", Fuente: "netflix", Periodo: "2025-02", SHA256: "aa"},
			},
			quiero: nil,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			got := refsDe(duplicadosPorHuella(c.periodo, c.entregas))
			if !slices.Equal(got, c.quiero) {
				t.Fatalf("duplicadosPorHuella = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

func TestDuplicadoPorHuellaNombraLaOtraEntrega(t *testing.T) {
	t.Parallel()

	hs := duplicadosPorHuella("2025-02", []Entrega{
		{ID: "r-viejo", Fuente: "caracol", Periodo: "2025-01", SHA256: "abc"},
		{ID: "r-nuevo", Fuente: "netflix", Periodo: "2025-02", SHA256: "abc"},
	})
	if len(hs) != 1 {
		t.Fatalf("se esperaba un hallazgo, hubo %d", len(hs))
	}
	// Sin nombrar la otra pata, quien resuelve no puede decidir cual de las
	// dos entregas se queda sin consultar la base.
	for _, quiero := range []string{"r-viejo", "2025-01", "caracol", "abc"} {
		if !strings.Contains(hs[0].Detalle, quiero) {
			t.Fatalf("el detalle %q no nombra %q", hs[0].Detalle, quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Duplicado por registro

func TestDuplicadosPorRegistro(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		usos   []Uso
		quiero []string
	}{
		{
			nombre: "misma clave en dos entregas del mismo periodo",
			usos: []Uso{
				{ID: "u1", ReporteID: "r1", Fuente: "caracol", ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=20:00:00"},
				{ID: "u2", ReporteID: "r2", Fuente: "caracol", ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=20:00:00"},
			},
			quiero: []string{"duplicado_registro|uso:u2"},
		},
		{
			// La granularidad de Caracol es la EMISION: 29 ID_Ficha distintos
			// en 59 filas del archivo real. Con `id_ficha` sola como clave,
			// estas dos emisiones legitimas del mismo programa serian un
			// duplicado inventado.
			nombre: "dos emisiones del mismo programa a distinta hora NO son duplicado",
			usos: []Uso{
				{ID: "u1", Fuente: "caracol", ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=20:00:00"},
				{ID: "u2", Fuente: "caracol", ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=22:00:00"},
			},
			quiero: nil,
		},
		{
			// Los ids de fuente NO cruzan entre fuentes (ADR 0018). Que el
			// `id=7` de cine coincida con el `id_ficha=7` de Caracol no
			// significa nada.
			nombre: "la misma clave en fuentes distintas NO es duplicado",
			usos: []Uso{
				{ID: "u1", Fuente: "caracol", ClaveRegistro: "id=7"},
				{ID: "u2", Fuente: "cine", ClaveRegistro: "id=7"},
			},
			quiero: nil,
		},
		{
			// El falso positivo mas caro posible: agrupar bajo la cadena
			// vacia marcaria como duplicadas entre si TODAS las filas de las
			// que no se sabe nada, y bloquearia el periodo entero.
			nombre: "las claves vacias no se comparan entre si",
			usos: []Uso{
				{ID: "u1", Fuente: "caracol"},
				{ID: "u2", Fuente: "caracol"},
				{ID: "u3", Fuente: "caracol"},
			},
			quiero: nil,
		},
		{
			nombre: "tres repeticiones levantan dos alertas, no tres",
			usos: []Uso{
				{ID: "u1", Fuente: "netflix", ClaveRegistro: "netflix_id=9"},
				{ID: "u2", Fuente: "netflix", ClaveRegistro: "netflix_id=9"},
				{ID: "u3", Fuente: "netflix", ClaveRegistro: "netflix_id=9"},
			},
			// La primera es el registro legitimo: borrarla tambien perderia
			// el hecho.
			quiero: []string{"duplicado_registro|uso:u2", "duplicado_registro|uso:u3"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if got := refsDe(duplicadosPorRegistro(c.usos)); !slices.Equal(got, c.quiero) {
				t.Fatalf("duplicadosPorRegistro = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

func TestDuplicadoPorRegistroNombraLaFilaQueLlegoPrimero(t *testing.T) {
	t.Parallel()

	hs := duplicadosPorRegistro([]Uso{
		{ID: "u1", ReporteID: "r1", Fuente: "caracol", ClaveRegistro: "id_ficha=7"},
		{ID: "u2", ReporteID: "r2", Fuente: "caracol", ClaveRegistro: "id_ficha=7"},
	})
	if len(hs) != 1 {
		t.Fatalf("se esperaba un hallazgo, hubo %d", len(hs))
	}
	for _, quiero := range []string{"u1", "r1", "r2", "id_ficha=7"} {
		if !strings.Contains(hs[0].Detalle, quiero) {
			t.Fatalf("el detalle %q no nombra %q", hs[0].Detalle, quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Titulares sin porcentaje

func TestTitularesSinPorcentaje(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		obras  []Obra
		quiero []string
	}{
		{
			// El caso real del dataset: dos coautores en el catalogo, una
			// sola parte declarada (docs/dominio/fixtures.md).
			nombre: "coautor del catalogo sin parte declarada",
			obras: []Obra{{
				ID:           "o1",
				CoautoresIPI: []string{"ipi-a", "ipi-b"},
				Declarada:    true,
				Declaracion: repertorio.Declaracion{
					ObraID: "o1", Partes: []repertorio.Parte{parte("tit-a", "ipi-a", 60)},
				},
			}},
			quiero: []string{"titular_sin_porcentaje|obra:o1#ipi:ipi-b"},
		},
		{
			nombre: "declaracion completa y todos los coautores declarados: nada",
			obras: []Obra{{
				ID:           "o1",
				CoautoresIPI: []string{"ipi-a", "ipi-b"},
				Declarada:    true,
				Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60), parte("tit-b", "ipi-b", 40),
				}},
			}},
			quiero: nil,
		},
		{
			// `declaraciones.ipi` es TEXT NOT NULL sin CHECK de no-vacio: el
			// esquema lo admite y el porcentaje no dice a quien se paga.
			nombre: "parte con IPI vacio",
			obras: []Obra{{
				ID:           "o1",
				CoautoresIPI: []string{"ipi-a"},
				Declarada:    true,
				Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60), parte("tit-x", "", 40),
				}},
			}},
			quiero: []string{"titular_sin_porcentaje|obra:o1#titular:tit-x"},
		},
		{
			// El esquema lo permite y VigentesDeObras usa LEFT JOIN a
			// proposito para no confundirlo con la ausencia de version.
			nombre: "version abierta y VACIA: uno por coautor",
			obras: []Obra{{
				ID:           "o1",
				CoautoresIPI: []string{"ipi-a", "ipi-b"},
				Declarada:    true,
				Declaracion:  repertorio.Declaracion{ObraID: "o1"},
			}},
			quiero: []string{
				"titular_sin_porcentaje|obra:o1#ipi:ipi-a",
				"titular_sin_porcentaje|obra:o1#ipi:ipi-b",
			},
		},
		{
			// La frontera con el detector 5: sin ninguna version no hay nada
			// que decir de cada coautor por separado, y emitir N avisos que
			// dicen lo mismo inundaria el tablero. Lo cubre entero
			// retencionPorDeclaracionIncompleta con UNA alerta.
			nombre: "obra SIN declaracion: este detector calla",
			obras: []Obra{{
				ID:           "o1",
				CoautoresIPI: []string{"ipi-a", "ipi-b"},
				Declarada:    false,
			}},
			quiero: nil,
		},
		{
			nombre: "un IPI de coautor vacio no se persigue",
			obras: []Obra{{
				ID:           "o1",
				CoautoresIPI: []string{""},
				Declarada:    true,
				Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 100),
				}},
			}},
			quiero: nil,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if got := refsDe(titularesSinPorcentaje(c.obras)); !slices.Equal(got, c.quiero) {
				t.Fatalf("titularesSinPorcentaje = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. Retencion por declaracion incompleta

func TestRetencionPorDeclaracionIncompleta(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		obras  []Obra
		quiero []string
	}{
		{
			nombre: "suma 100 en dos partes: no retiene",
			obras: []Obra{{
				ID: "o1", Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60), parte("tit-b", "ipi-b", 40),
				}},
			}},
			quiero: nil,
		},
		{
			// R-04 / RD 13.1.3: se retiene el TOTAL, no se reparte el 60.
			nombre: "suma 60: retiene",
			obras: []Obra{{
				ID: "o1", Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60),
				}},
			}},
			quiero: []string{"reserva_declaracion_incompleta|obra:o1"},
		},
		{
			// El caso que el dataset NO tiene (fixtures.md lo dice por
			// escrito) y que R-04 cubre igual.
			nombre: "obra sin ninguna declaracion: retiene",
			obras:  []Obra{{ID: "o1", Declarada: false}},
			quiero: []string{"reserva_declaracion_incompleta|obra:o1"},
		},
		{
			nombre: "version abierta y vacia: retiene",
			obras: []Obra{{
				ID: "o1", Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o1"},
			}},
			quiero: []string{"reserva_declaracion_incompleta|obra:o1"},
		},
		{
			// El criterio es Declaracion.Completa(), que tambien mira el IPI:
			// una suma de 100 con una parte sin IPI NO esta completa.
			nombre: "suma 100 con una parte sin IPI: retiene igual",
			obras: []Obra{{
				ID: "o1", Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60), parte("tit-b", "", 40),
				}},
			}},
			quiero: []string{"reserva_declaracion_incompleta|obra:o1"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			got := refsDe(retencionPorDeclaracionIncompleta(c.obras))
			if !slices.Equal(got, c.quiero) {
				t.Fatalf("retencionPorDeclaracionIncompleta = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

func TestRetencionDistingueSinDeclararDeIncompleta(t *testing.T) {
	t.Parallel()

	sin := retencionPorDeclaracionIncompleta([]Obra{{ID: "o1"}})
	incompleta := retencionPorDeclaracionIncompleta([]Obra{{
		ID: "o2", Declarada: true,
		Declaracion: repertorio.Declaracion{ObraID: "o2", Partes: []repertorio.Parte{
			parte("tit-a", "ipi-a", 60),
		}},
	}})

	// Las dos retienen, pero el arreglo es distinto -- declarar desde cero
	// contra completar lo declarado -- y el detalle tiene que decir cual.
	if !strings.Contains(sin[0].Detalle, "no tiene ninguna Declaracion") {
		t.Fatalf("la obra sin declarar dice %q", sin[0].Detalle)
	}
	if !strings.Contains(incompleta[0].Detalle, "60") {
		t.Fatalf("la obra incompleta no nombra la suma: %q", incompleta[0].Detalle)
	}
}

// ---------------------------------------------------------------------------
// 6. tipo_obra sin mapear

func TestTipoObraSinMapear(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		usos   []Uso
		quiero []string
	}{
		{
			nombre: "identificada y sin tipo_obra",
			usos:   []Uso{{ID: "u1", ObraID: "o1"}},
			quiero: []string{"tipo_obra_sin_mapear|uso:u1"},
		},
		{
			nombre: "identificada y con una de las cuatro categorias",
			usos: []Uso{
				{ID: "u1", ObraID: "o1", TipoObra: "cinematografica"},
				{ID: "u2", ObraID: "o1", TipoObra: "unitario"},
				{ID: "u3", ObraID: "o2", TipoObra: "serie"},
				{ID: "u4", ObraID: "o2", TipoObra: "telenovela"},
				{ID: "u5", ObraID: "o3", TipoObra: "sketches"},
			},
			quiero: nil,
		},
		{
			// Sin obra no llega al motor (`UsosDeCanal` filtra por
			// obra_id IS NOT NULL) y su problema ya tiene otra alerta: sin
			// este filtro, cada fila ONI de Caracol levantaria dos avisos
			// por el mismo hecho.
			nombre: "sin obra identificada: calla",
			usos: []Uso{
				{ID: "u1", Escalon: identificacion.EscalonONI},
				{ID: "u2", Escalon: identificacion.EscalonPendiente},
				{ID: "u3", Escalon: identificacion.EscalonExcluido},
			},
			quiero: nil,
		},
		{
			// Las tres modalidades cuyo motor SI lee tipo_obra:
			// `ponderacionTipo` solo la llama `puntosTV`, y a `puntosTV` se
			// llega por `case TV:` y por `repartirSuscripcionOrden`, que
			// sirve a `case Suscripcion, Hotel:`.
			nombre: "modalidades que ponderan por tipo_obra: avisa",
			usos: []Uso{
				{ID: "u1", ObraID: "o1", Modalidad: ModalidadTV},
				{ID: "u2", ObraID: "o2", Modalidad: ModalidadSuscripcion},
				{ID: "u3", ObraID: "o3", Modalidad: ModalidadHotel},
			},
			quiero: []string{
				"tipo_obra_sin_mapear|uso:u1",
				"tipo_obra_sin_mapear|uso:u2",
				"tipo_obra_sin_mapear|uso:u3",
			},
		},
		{
			// El caso que hacia dano: `MapaNetflix` no declara CampoTipoObra,
			// asi que TODA fila de Netflix llega con el tipo vacio. Como
			// EsCritica marca este tipo como bloqueante, un periodo de OTT
			// levantaba N criticas y repartia perfectamente -- y la compuerta
			// de #34, que lee CriticasAbiertas, no habria abierto NUNCA por un
			// campo que la corrida de OTT no lee. `puntosOTT`,
			// `puntosCineTeatro` y `puntosTransporte` no miran TipoObra en
			// ninguna linea.
			nombre: "modalidades que NO leen tipo_obra: calla",
			usos: []Uso{
				{ID: "u1", ObraID: "o1", Modalidad: "ott"},
				{ID: "u2", ObraID: "o2", Modalidad: "cine"},
				{ID: "u3", ObraID: "o3", Modalidad: "teatro"},
				{ID: "u4", ObraID: "o4", Modalidad: "transporte"},
			},
			quiero: nil,
		},
		{
			// Ante la duda se AVISA. El vocabulario lo cierran
			// `reparto.ParseModalidad` y el CHECK de `usos.modalidad`, asi
			// que llegar aqui con otra cosa ya es un fallo en otro sitio:
			// callar dejaria pasar en silencio la fila de la que menos se
			// sabe.
			nombre: "modalidad desconocida: avisa, no calla",
			usos:   []Uso{{ID: "u1", ObraID: "o1", Modalidad: "radio"}},
			quiero: []string{"tipo_obra_sin_mapear|uso:u1"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if got := refsDe(tipoObraSinMapear(c.usos)); !slices.Equal(got, c.quiero) {
				t.Fatalf("tipoObraSinMapear = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Detectar

// periodoConUnaDeCadaAnomalia arma el caso del criterio de aceptacion: un
// periodo con una anomalia de cada tipo.
func periodoConUnaDeCadaAnomalia() Periodo {
	return Periodo{
		Periodo: "2025-01",
		Usos: []Uso{
			// 1. ONI.
			{ID: "u-oni", ReporteID: "r1", Fuente: "caracol", Titulo: "Sin casar",
				Escalon: identificacion.EscalonONI},
			// 3. Duplicado de registro: u-dup2 repite a u-dup1.
			{ID: "u-dup1", ReporteID: "r1", Fuente: "caracol", ObraID: "o-ok",
				Escalon: identificacion.EscalonAlias, TipoObra: "serie",
				ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=20:00:00"},
			{ID: "u-dup2", ReporteID: "r2", Fuente: "caracol", ObraID: "o-ok",
				Escalon: identificacion.EscalonAlias, TipoObra: "serie",
				ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=20:00:00"},
			// 6. tipo_obra sin mapear.
			{ID: "u-sintipo", ReporteID: "r1", Fuente: "caracol", ObraID: "o-ok",
				Escalon:       identificacion.EscalonAlias,
				ClaveRegistro: "id_ficha=8|fecha=2025-01-03|hora=21:00:00"},
			// Las dos obras con problema de declaracion entran por sus usos.
			{ID: "u-incompleta", ReporteID: "r1", Fuente: "caracol", ObraID: "o-incompleta",
				Escalon: identificacion.EscalonAlias, TipoObra: "unitario",
				ClaveRegistro: "id_ficha=9|fecha=2025-01-04|hora=19:00:00"},
		},
		// 2. Duplicado de archivo: r1 y r3 comparten bytes sin compartir fuente.
		Entregas: []Entrega{
			{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
			{ID: "r2", Fuente: "caracol", Periodo: "2025-01", SHA256: "bb"},
			{ID: "r3", Fuente: "netflix", Periodo: "2024-12", SHA256: "aa"},
		},
		Obras: []Obra{
			{ID: "o-incompleta", CoautoresIPI: []string{"ipi-a", "ipi-b"}, Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o-incompleta", Partes: []repertorio.Parte{
					parte("tit-a", "ipi-a", 60),
				}}},
			{ID: "o-ok", CoautoresIPI: []string{"ipi-c"}, Declarada: true,
				Declaracion: repertorio.Declaracion{ObraID: "o-ok", Partes: []repertorio.Parte{
					parte("tit-c", "ipi-c", 100),
				}}},
		},
	}
}

func TestDetectarLevantaUnaDeCadaTipo(t *testing.T) {
	t.Parallel()

	got := refsDe(Detectar(periodoConUnaDeCadaAnomalia()))
	quiero := []string{
		"duplicado_archivo|reporte:r1",
		"duplicado_registro|uso:u-dup2",
		"oni|uso:u-oni",
		"reserva_declaracion_incompleta|obra:o-incompleta",
		"tipo_obra_sin_mapear|uso:u-sintipo",
		"titular_sin_porcentaje|obra:o-incompleta#ipi:ipi-b",
	}
	if !slices.Equal(got, quiero) {
		t.Fatalf("Detectar = %v,\nse esperaba %v", got, quiero)
	}
}

// El orden no es el de deteccion sino (tipo, ref_tipo, ref_id, ref_titular).
// ADR 0005: dos pasadas sobre el mismo dato tienen que devolver lo mismo, en
// el mismo orden, aunque un detector cambie de sitio.
func TestDetectarDevuelveUnOrdenEstable(t *testing.T) {
	t.Parallel()

	p := periodoConUnaDeCadaAnomalia()
	primera := refsDe(Detectar(p))
	segunda := refsDe(Detectar(p))
	if !slices.Equal(primera, segunda) {
		t.Fatalf("dos pasadas dan %v y %v", primera, segunda)
	}
	if !slices.IsSorted(primera) {
		t.Fatalf("la salida no esta ordenada: %v", primera)
	}
}

func TestDetectarSobreUnPeriodoLimpioNoDevuelveNada(t *testing.T) {
	t.Parallel()

	limpio := Periodo{
		Periodo: "2025-01",
		Usos: []Uso{{
			ID: "u1", ReporteID: "r1", Fuente: "caracol", ObraID: "o1",
			Escalon: identificacion.EscalonAlias, TipoObra: "serie",
			ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=20:00:00",
		}},
		Entregas: []Entrega{{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"}},
		Obras: []Obra{{
			ID: "o1", CoautoresIPI: []string{"ipi-a"}, Declarada: true,
			Declaracion: repertorio.Declaracion{ObraID: "o1", Partes: []repertorio.Parte{
				parte("tit-a", "ipi-a", 100),
			}},
		}},
	}
	if hs := Detectar(limpio); len(hs) != 0 {
		t.Fatalf("un periodo limpio devolvio %v", refsDe(hs))
	}
}

// Detectar nunca devuelve nil: el caso de uso lo recorre y lo cuenta, y un nil
// que se cuela como "sin anomalias" es indistinguible de una lista vacia solo
// hasta que alguien lo serializa a JSON.
func TestDetectarDevuelveListaVaciaYNoNil(t *testing.T) {
	t.Parallel()

	if hs := Detectar(Periodo{Periodo: "2025-01"}); hs == nil {
		t.Fatal("Detectar devolvio nil en vez de una lista vacia")
	}
}

// ---------------------------------------------------------------------------
// EsCritica y el vocabulario

func TestEsCritica(t *testing.T) {
	t.Parallel()

	casos := map[string]bool{
		// Falsean las cifras del periodo: una emision contada dos veces infla
		// los puntos de su obra y desinfla los de todas las demas del canal;
		// una fila sin categoria aborta la corrida de TV.
		TipoDuplicadoArchivo:  true,
		TipoDuplicadoRegistro: true,
		TipoTipoObraSinMapear: true,
		// Estados validos del modelo: ONI es una etapa del ADR 0007 y la
		// retencion de R-04 retiene ESA obra sin parar el periodo.
		TipoONI:                          false,
		TipoReservaDeclaracionIncompleta: false,
		TipoTitularSinPorcentaje:         false,
		// Un tipo que no existe no bloquea nada.
		"inventado": false,
	}
	for tipo, quiero := range casos {
		if got := EsCritica(tipo); got != quiero {
			t.Fatalf("EsCritica(%q) = %v, se esperaba %v", tipo, got, quiero)
		}
	}
}

// Los cinco primeros strings los fijo el tablero de la #104 antes de que este
// paquete existiera (`web/src/reparto/tipos.ts`). Cambiar una grafia deja el
// tablero pintando la etiqueta cruda en vez de "Duplicado de archivo", y eso
// no lo ve ninguna prueba de Go: por eso la lista se fija aqui literal.
func TestLosTiposSonLosDelContratoDelTablero(t *testing.T) {
	t.Parallel()

	quiero := []string{
		"oni",
		"duplicado_archivo",
		"duplicado_registro",
		"titular_sin_porcentaje",
		"reserva_declaracion_incompleta",
		"tipo_obra_sin_mapear",
	}
	if got := Tipos(); !slices.Equal(got, quiero) {
		t.Fatalf("Tipos() = %v, se esperaba %v", got, quiero)
	}
	for _, tipo := range quiero {
		if !EsTipo(tipo) {
			t.Fatalf("EsTipo(%q) = false", tipo)
		}
	}
	if EsTipo("onni") {
		t.Fatal("EsTipo acepto una grafia que no esta en la lista")
	}
}
