package aplicacion

import (
	"fmt"
	"slices"
	"strings"
)

// Contrato de usos.ids_fuente (ADR 0018).
//
// La columna es TEXT y la escriben y la leen modulos distintos: la ingesta
// (adaptadores de formato y seed) escribe, la cascada de identificacion lee.
// Este fichero es el unico sitio donde se decide que va dentro: una linea
// "clave=valor" por identificador, con la clave tomada de la lista cerrada de
// abajo. Quien escriba ids_fuente usa EscribirIDsFuente; quien lo lea usa
// LeerIDsFuente. Ninguno de los dos lados escribe strings de clave a mano.

// Claves de ids_fuente. Minuscula fija y propia: no son el encabezado del
// archivo del cliente (que puede cambiar de grafia entre entregas), son el
// vocabulario con el que se aprende en alias_obra.tipo_id. Cambiar una rompe
// todos los alias ya aprendidos con ella.
const (
	// Locales: identificadores del sistema que emitio el reporte. Solo sirven
	// como alias contra el catalogo, nunca para cruzar fuentes
	// (docs/dominio/identificadores.md).
	ClaveIDFicha    = "id_ficha"    // Caracol: programa u obra
	ClaveShowID     = "show_id"     // Netflix: show
	ClaveSeriesID   = "series_id"   // Netflix: temporada
	ClaveNetflixID  = "netflix_id"  // Netflix: episodio
	ClaveIDPelicula = "id_pelicula" // cine: pelicula (sintetico hasta que el cliente entregue el formato)

	// Globales: el escalon 2 de la cascada.
	ClaveIDA  = "ida"
	ClaveEIDR = "eidr"
	ClaveIMDB = "imdb"
)

// clavesIDsFuente es la lista cerrada. Una clave que no este aqui no forma
// parte del contrato: EscribirIDsFuente la rechaza y LeerIDsFuente la ignora.
var clavesIDsFuente = []string{
	ClaveIDFicha, ClaveShowID, ClaveSeriesID, ClaveNetflixID, ClaveIDPelicula,
	ClaveIDA, ClaveEIDR, ClaveIMDB,
}

// EsClaveIDsFuente dice si la clave esta en la lista cerrada. Lo usan los
// escritores (adaptadores de ingesta, seed) al validar un mapa, para no
// repetir la lista fuera de este fichero.
func EsClaveIDsFuente(clave string) bool {
	return slices.Contains(clavesIDsFuente, clave)
}

// IDFuente es un identificador de una fila de reporte.
type IDFuente struct {
	Clave string
	Valor string
}

// ---------------------------------------------------------------------------
// La clave logica de un REGISTRO (ADR 0018, punto 5 llevado a la fila)
// ---------------------------------------------------------------------------

// Componentes de la clave de registro que NO son un identificador de fuente,
// sino campos canonicos de `usos`. No colisionan con [clavesIDsFuente]: ni
// `fecha` ni `hora` estan en esa lista, y [EsClaveIDsFuente] sigue diciendo
// que no.
const (
	ComponenteFecha = "fecha"
	ComponenteHora  = "hora"
)

// clavesDeRegistro dice, por fuente, QUE identifica un registro suyo.
//
// # Por que vive aqui y no en el adaptador de ingesta
//
// Porque esta escrito con el vocabulario del ADR 0018 -- `id_ficha`,
// `netflix_id`, `id_pelicula` --, y ese vocabulario se decide en ESTE fichero
// y en ningun otro. `ingesta.Mapa.ClaveRegistro` dice lo mismo en el otro
// idioma: nombres de COLUMNA del archivo del cliente (`ID_Ficha`, `Fecha`),
// que es lo unico que sirve mientras la fila todavia es una fila de un .xlsx.
// En cuanto la fila esta persistida, esas columnas ya no existen: lo que queda
// es `usos.ids_fuente` mas los campos canonicos, y una segunda lista de
// columnas de archivo seria justo la deriva que el ADR 0018 existe para
// impedir.
//
// Que las dos listas digan lo mismo no se sostiene en un comentario:
// `TestClaveDeRegistroCuadraConLosMapasDeIngesta` traduce cada
// `Mapa.ClaveRegistro` a este vocabulario y exige que coincida componente a
// componente. Si alguien anade una columna a la clave de Caracol y no la anade
// aqui, esa prueba falla.
//
// # Por que Caracol lleva fecha y hora
//
// Porque su granularidad es la EMISION, no la obra: 29 `ID_Ficha` distintos en
// 59 filas del archivo real, con titulos emitidos hasta cuatro veces. Con
// `id_ficha` sola, 30 emisiones legitimas se marcarian como registros
// duplicados. Es la misma razon por la que `MapaCaracol` lleva las dos
// columnas en su ClaveRegistro.
//
// # Por que Netflix va por episodio y no por show
//
// `netflix_id` identifica el EPISODIO y es unico en las 49 filas del archivo
// real; `show_id` se repite. Para aprender un alias manda `show_id` (ADR 0018,
// punto 5), porque el alias cubre el show entero; para decidir si dos filas son
// el mismo hecho manda el episodio. Son dos preguntas distintas sobre la misma
// fila y por eso tienen dos claves distintas.
//
// Una fuente que no este en este mapa NO tiene clave de registro, y eso no es
// un fallo: significa que todavia no se sabe que identifica un registro suyo.
// Ver [ClaveDeRegistro].
var clavesDeRegistro = map[string][]string{
	// Los nombres son los de `ingesta.Fuente*`, que es lo que se estampa en
	// `usos.fuente`. Van como literales porque `internal/aplicacion` no puede
	// importar `internal/infraestructura` (ADR 0002); la prueba de arriba es
	// la que impide que se separen.
	"caracol": {ClaveIDFicha, ComponenteFecha, ComponenteHora},
	"netflix": {ClaveNetflixID},
	// Provisional: dos exhibiciones legitimas de la misma pelicula saldrian como duplicado (P-21).
	"cine": {ClaveIDPelicula},
	// rcn y expreso-bolivariano sin clave a proposito: no hay formato del cliente (P-21).
}

// ComponentesDeClaveDeRegistro devuelve los componentes ordenados de la clave
// logica de una fuente, o nil si esa fuente no tiene ninguna declarada.
//
// Devuelve una copia: el mapa de arriba es estado de paquete y quien lo reciba
// no debe poder reordenarlo.
func ComponentesDeClaveDeRegistro(fuente string) []string {
	return slices.Clone(clavesDeRegistro[fuente])
}

// ClaveDeRegistro compone la clave logica de una fila persistida, o devuelve
// la cadena vacia si no se puede componer.
//
// Es lo que distingue "dos filas son el mismo hecho declarado dos veces" de
// "dos filas de la misma obra", y se usa para la deteccion de duplicados entre
// archivos del #37. La cadena vacia significa "de esta fila no se puede decir
// que registro es", y quien la reciba NO debe compararla con nada: agrupar
// bajo la cadena vacia convertiria en duplicadas entre si todas las filas de
// las que no se sabe nada.
//
// Se devuelve vacia en tres casos, y los tres son legitimos:
//
//   - la fuente no tiene clave declarada (una fuente nueva, antes de perfilar
//     su archivo);
//   - la fila no trae alguno de los componentes -- por ejemplo una emision de
//     Caracol sin hora --, porque una clave a la que le falta una pieza no
//     identifica el mismo registro que una completa: compararlas marcaria como
//     duplicadas dos emisiones distintas del mismo programa;
//   - `ids_fuente` llego fuera del contrato y [LeerIDsFuente] lo ignoro.
//
// El formato lleva la clave delante del valor (`id_ficha=871732|fecha=...`)
// para que la cadena sea legible en el detalle de una alerta sin tener que
// saber el orden, y el separador es `|` porque ni `=` ni salto de linea caben
// en un valor de ids_fuente ([EscribirIDsFuente] los rechaza) y los campos
// canonicos `fecha` y `hora` salen normalizados a `AAAA-MM-DD` y `HH:MM:SS`.
func ClaveDeRegistro(fuente, idsFuente, fecha, hora string) string {
	componentes := clavesDeRegistro[fuente]
	if len(componentes) == 0 {
		return ""
	}

	ids := LeerIDsFuente(idsFuente)
	partes := make([]string, 0, len(componentes))
	for _, c := range componentes {
		var valor string
		switch c {
		case ComponenteFecha:
			valor = strings.TrimSpace(fecha)
		case ComponenteHora:
			valor = strings.TrimSpace(hora)
		default:
			valor = ids[c]
		}
		if valor == "" {
			return ""
		}
		partes = append(partes, c+"="+valor)
	}
	return strings.Join(partes, "|")
}

// EscribirIDsFuente serializa ids en el formato del contrato. Los valores
// vacios (tras recortar espacios) se omiten: una columna del archivo sin dato
// no es un identificador. Las lineas salen ordenadas por clave, para que la
// misma fila produzca siempre el mismo texto (ADR 0005).
//
// Devuelve error, en vez de escribir algo que el lector ignoraria en silencio,
// si una clave no es del contrato, se repite, o un valor contiene un salto de
// linea o un "=".
func EscribirIDsFuente(ids ...IDFuente) (string, error) {
	vistas := map[string]bool{}
	lineas := make([]string, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(clavesIDsFuente, id.Clave) {
			return "", fmt.Errorf("ids_fuente: clave %q fuera del contrato", id.Clave)
		}
		if vistas[id.Clave] {
			return "", fmt.Errorf("ids_fuente: clave %q repetida", id.Clave)
		}
		vistas[id.Clave] = true

		valor := strings.TrimSpace(id.Valor)
		if strings.ContainsAny(valor, "=\n\r") {
			return "", fmt.Errorf("ids_fuente: el valor de %q contiene un separador: %q", id.Clave, valor)
		}
		if valor == "" {
			continue
		}
		lineas = append(lineas, id.Clave+"="+valor)
	}
	slices.Sort(lineas)
	return strings.Join(lineas, "\n"), nil
}

// LeerIDsFuente parsea ids_fuente de forma estricta: solo cuentan las lineas
// "clave=valor" con clave del contrato y valor no vacio tras recortar
// espacios. Cualquier otra linea -un valor sin clave, una clave desconocida o
// con otra grafia- se ignora: adivinar de que columna salio un valor es la
// forma de aprender un alias falso y atribuir usos a la obra equivocada.
//
// Si una clave aparece dos veces gana la ultima, que es determinista.
func LeerIDsFuente(s string) map[string]string {
	ids := map[string]string{}
	for _, linea := range strings.Split(s, "\n") {
		clave, valor, ok := strings.Cut(linea, "=")
		clave, valor = strings.TrimSpace(clave), strings.TrimSpace(valor)
		if !ok || valor == "" || !slices.Contains(clavesIDsFuente, clave) {
			continue
		}
		ids[clave] = valor
	}
	return ids
}
