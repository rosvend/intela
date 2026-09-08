package ingesta

import (
	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Nombres de fuente. Son lo que se estampa en `usos.fuente` y lo que indexa
// `alias_obra`, asi que cambiarlos rompe la correspondencia aprendida de todas
// las entregas anteriores.
const (
	FuenteCaracol = "caracol"
	FuenteNetflix = "netflix"
	FuenteCine    = "cine"
)

// MapasDelCliente son los mapas de columnas de las fuentes perfiladas.
//
// Cubren las tres modalidades que pide el criterio de aceptacion de OE-1 -- TV,
// OTT y cine -- y estan medidos, no supuestos: los dos primeros contra los
// archivos reales de `data/files/`, perfilados con
// `uv run --script src/scripts/sample.py` y documentados en
// `docs/dominio/fuentes-datos.md`.
//
// Anadir una fuente es anadir un [Mapa] a esta lista. No hace falta tocar nada
// mas: [Catalogo] la registra en los tres formatos.
func MapasDelCliente() []Mapa {
	return []Mapa{MapaCaracol(), MapaNetflix(), MapaCine()}
}

// CatalogoDelCliente es [Catalogo] sobre [MapasDelCliente].
//
// Devuelve error porque los mapas se validan al construirse. Que un mapa de
// este fichero este mal es un defecto del programa, no de la entrega: por eso
// se descubre al arrancar el binario y no cuando llega un archivo.
func CatalogoDelCliente() (map[aplicacion.ClaveLector]aplicacion.LectorReporte, error) {
	return Catalogo(MapasDelCliente()...)
}

// MapaCaracol es la parrilla de television de CARACOL.
//
// Medido sobre `data/files/CARACOL_REDES-SGC_(COLOMBIA)_20250202.xlsx`: 59
// filas x 48 columnas, una hoja.
//
// # Lo que NO se mapea, que es la mitad de las decisiones
//
//   - `tipo_obra`. La parrilla trae `TIPO` (PR/SE/PE) y `SubGenero`
//     (Telenovela, Magazine, Noticiero, Agro...), y NINGUNA de las dos es la
//     clasificacion del reglamento -- cinematografica, unitario, serie,
//     telenovela, sketches --. La tabla de correspondencia es la pregunta 5 de
//     `docs/dominio/fuentes-datos.md` y todavia no la ha contestado el cliente.
//     Escribir `SubGenero` en `tipo_obra` inventaria esa correspondencia y
//     despues no habria forma de distinguir lo mapeado de lo supuesto; la
//     columna se queda en su DEFAULT vacio hasta que la respuesta llegue.
//   - `rating`. La parrilla NO lo trae, y es lo que bloquea `RD 9.1.1`
//     completo: hace falta el feed del proveedor de audiencia, que es la
//     pregunta 3.
//   - Los cuatro campos de episodio (`Titulo_capitulo`, `Temporada`,
//     `ID_Ficha_Capitulo`, `Numero_Capitulo`) estan vacios al 100% pese a que
//     18 filas son series. Hoy solo se puede identificar el programa.
//   - Los creditos (`Autor*`, `Guionista*`, `Director*`). Son features de
//     matching (`R-02`), no insumo de pago: los autores y los porcentajes salen
//     SOLO de la Declaracion de Obra (`R-03`). 36 de 59 filas no traen ni autor
//     ni guionista y eso no bloquea nada.
//
// # emisiones no se mapea porque cada fila YA es una emision
//
// La granularidad de la parrilla es la emision, no la obra: 29 `ID_Ficha`
// distintos en 59 filas, con titulos que se emiten hasta 4 veces. Cada fila
// entra como un uso con `emisiones` en su valor por defecto (1, que rellena
// GuardarUsos), y contarlas es trabajo del motor de reparto. Mapear aqui una
// columna de emisiones multiplicaria por si misma la cuenta.
func MapaCaracol() Mapa {
	return Mapa{
		Fuente:    FuenteCaracol,
		Modalidad: reparto.TV,
		// Vacia: la primera. El nombre real esta truncado a 31 caracteres por
		// Excel (`CARACOL_REDES-SGC_(COLOMBIA)_20`) y depende de como se llamara
		// el archivo al exportarlo, asi que exigirlo rechazaria la entrega del
		// mes que viene por un motivo que no es.
		Hoja: "",
		Columnas: []Columna{
			// `Titulo` y no `Titulo_original`: es el titulo con el que se emitio
			// en Colombia, que es contra el que resuelve la cascada. Los dos
			// difieren en 16 de 59 filas; el original se recuperara cuando el
			// esquema canonico admita las dos variantes.
			{Campo: CampoTitulo, Nombre: "Titulo", Requerida: true},
			// `ID_Ficha` es la clave de obra de la fuente. Va a `ids_fuente` tal
			// cual, sin normalizar, porque es lo que indexa `alias_obra`.
			{Campo: CampoIDsFuente, Nombre: "ID_Ficha", Requerida: true},
			// Minutos. Poblada en las 59 filas. Alimenta `Duracion` de
			// `RD 9.1.1`.
			{Campo: CampoDuracionMin, Nombre: "Duracion_total", Requerida: true},
		},
		// La emision, que es la granularidad real. `ID_Ficha` sola mandaria 30
		// emisiones legitimas al log de rechazos; con la fecha y la hora, las 59
		// filas del archivo real son 59 claves distintas.
		ClaveRegistro: []string{"ID_Ficha", "Fecha", "Hora"},
	}
}

// MapaNetflix es el reporte de consumo OTT.
//
// Medido sobre `data/files/Modulo identificación de Obras - Parrilla
// Netflix.xlsx`: 49 filas x 19 columnas, hoja `NETFLIX_REDES_2018`.
//
// # El titulo es el del SHOW, no el del episodio
//
// `episode_name` es mas preciso y no sirve: se resuelve contra el catalogo
// maestro, y los campos de episodio de la parrilla de Caracol estan vacios al
// 100%, asi que el episodio no existe en ninguno de los dos lados de la
// comparacion. El show es la unidad comparable que hay hoy. La identidad del
// episodio no se pierde: `netflix_id` es de episodio y va a `ids_fuente`.
//
// # `Id_Ntx` no se mapea, y no es un olvido
//
// Es un CONTADOR DE FILA del export, de 1 a 49, que se renumera en cada
// entrega. Persistirlo como identificador ataria un alias aprendido a una
// posicion en un archivo, y la entrega siguiente lo apuntaria a otra obra.
//
// # Lo que no cubre
//
//   - `eidr` esta VACIA en las 49 filas. Era el identificador que habria
//     resuelto el cruce entre fuentes; es la pregunta 6 al cliente.
//   - `minutos_vistos` no se mapea. `episode_runtime` es la duracion del
//     episodio, no el tiempo efectivamente visto, que es lo que pide `RD 9.7`;
//     multiplicarlo por `stream_starts` daria una cota superior, no la
//     magnitud. Un numero plausible en la columna equivocada es peor que la
//     columna vacia, porque el reparto no lo distingue.
//   - `term_end_date`, `year` y `viewing_country` son constantes del export --
//     metadatos del archivo, no datos de la fila -- y por eso no son columnas
//     del esquema canonico.
func MapaNetflix() Mapa {
	return Mapa{
		Fuente:    FuenteNetflix,
		Modalidad: reparto.OTT,
		// Aqui SI se exige el nombre: es fijo entre entregas y sirve de
		// comprobacion de que llego el libro que se creia.
		Hoja: "NETFLIX_REDES_2018",
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "show_name", Requerida: true},
			{Campo: CampoIDsFuente, Nombre: "netflix_id", Requerida: true},
			// La metrica de uso. Alimenta `V` de `RD 9.7`.
			{Campo: CampoVistas, Nombre: "stream_starts", Requerida: true},
			// La duracion del episodio, que es lo que la columna dice ser. NO es
			// el `DU` de `RD 9.7`; ver el doc de arriba.
			{Campo: CampoDuracionMin, Nombre: "episode_runtime", Requerida: false},
		},
		// `netflix_id` es unico en las 49 filas: es identificador de episodio,
		// no de show, asi que dos filas con el mismo son el mismo hecho dos
		// veces.
		ClaveRegistro: []string{"netflix_id"},
	}
}

// MapaCine es la taquilla de sala.
//
// La tercera modalidad que pide OE-1. El cliente no ha entregado un archivo
// real de exhibicion todavia, asi que el mapa se fija contra la muestra
// SINTETICA `data/samples/cine.csv` -- escrita a mano, con titulos como
// "Pelicula X" --, y hay que leerlo como lo que es: la forma del adaptador
// esta probada, las cifras no significan nada. Cuando llegue el archivo real
// se perfila con `sample.py` y se corrigen los nombres de columna aqui, sin
// tocar codigo.
//
// Mapea `modalidad` desde el archivo ADEMAS de fijarla. No es redundante: la
// muestra la trae como columna, y dejarla sin mapear haria que un archivo con
// filas de hotel entrara entero declarado como cine. Con la columna mapeada, la
// fila dice lo que es y `validarUso` rechaza lo que no sea una de las cuatro
// modalidades; la modalidad fija sigue valiendo para las celdas vacias.
func MapaCine() Mapa {
	return Mapa{
		Fuente:    FuenteCine,
		Modalidad: reparto.Cine,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "titulo", Requerida: true},
			{Campo: CampoIDsFuente, Nombre: "id", Requerida: true},
			{Campo: CampoModalidad, Nombre: "modalidad", Requerida: false},
			{Campo: CampoTipoObra, Nombre: "tipo_obra", Requerida: false},
			// La metrica de la modalidad. Sin ella la fila no pondera nada.
			{Campo: CampoTaquilla, Nombre: "taquilla", Requerida: true},
		},
		ClaveRegistro: []string{"id"},
	}
}
