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
