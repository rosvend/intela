package ingesta

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Campo es una casilla del esquema canonico que un mapa puede rellenar.
//
// Es un conjunto CERRADO, y esa es su razon de ser: un mapa de columnas es
// dato, y sin un vocabulario cerrado nada impide que una fuente nueva declare
// un destino que no existe y se descubra en produccion. Aqui, un campo que no
// este en esta lista no compila si es constante y lo rechaza [Mapa.Validar] si
// viene de otro sitio.
//
// No hay campo para el dinero y no lo va a haber: un reporte de uso PONDERA la
// bolsa, no la aporta. Tampoco hay campo para porcentajes de autor -- salen
// solo de la Declaracion de Obra (`R-03`) -- ni para obra_id: identificar es
// trabajo de la cascada (ADR 0007) y `validarUso` rechaza la fila que lo traiga.
type Campo string

const (
	// CampoTitulo es lo unico que TODA fuente tiene que traer: sin titulo no
	// hay nada que identificar.
	CampoTitulo Campo = "titulo"

	// CampoTituloOrig es el titulo original cuando la fuente trae los dos.
	// Opcional: el escalon 3 lo prueba ademas de CampoTitulo.
	CampoTituloOrig Campo = "titulo_original"

	// CampoIDsFuente es un identificador de la fuente. El VALOR va tal como
	// viene, sin normalizar; la CLAVE no: [Columna.ClaveID] tiene que ser una
	// constante `aplicacion.Clave*`, y al persistir se juntan con
	// [aplicacion.EscribirIDsFuente] en lineas `clave=valor` (ADR 0018).
	// Sin la clave, la cascada tira la linea y el escalon 1 no casa nunca.
	CampoIDsFuente Campo = "ids_fuente"

	// CampoModalidad deja que la modalidad venga por fila. Casi ninguna fuente
	// la necesita -- una parrilla de TV es toda TV --, pero un export que mezcle
	// varias sin ella tendria que partirse en varios archivos.
	CampoModalidad Campo = "modalidad"

	CampoTipoObra       Campo = "tipo_obra"
	CampoDuracionMin    Campo = "duracion_min"
	CampoEmisiones      Campo = "emisiones"
	CampoRating         Campo = "rating"
	CampoTaquilla       Campo = "taquilla"
	CampoEspectadores   Campo = "espectadores"
	CampoExhibiciones   Campo = "exhibiciones"
	CampoVistas         Campo = "vistas"
	CampoMinutosVistos  Campo = "minutos_vistos"
	CampoPB             Campo = "pb"
	CampoCanalID        Campo = "canal_id"
	CampoFecha          Campo = "fecha"
	CampoHora           Campo = "hora"
	CampoMoneda         Campo = "moneda"
	CampoUnidadDuracion Campo = "unidad_duracion"
)

// tipo es como se convierte el texto de la celda al llegar a este campo.
type tipo int

const (
	texto tipo = iota
	entero
	numero
)

// tipos es la tabla de coercion, una entrada por campo.
//
// Es una tabla y no un `switch` repartido por el parseo por lo mismo que las
// medidas de `validarUso` estan en una tabla: un campo nuevo es una fila, y no
// hay forma de anadirlo en un sitio y olvidarlo en otro. Ser tambien la lista
// de campos validos -- [Mapa.Validar] consulta este mapa -- garantiza que las
// dos cosas no se puedan separar.
var tipos = map[Campo]tipo{
	CampoTitulo:         texto,
	CampoTituloOrig:     texto,
	CampoIDsFuente:      texto,
	CampoModalidad:      texto,
	CampoTipoObra:       texto,
	CampoDuracionMin:    numero,
	CampoEmisiones:      entero,
	CampoRating:         numero,
	CampoTaquilla:       numero,
	CampoEspectadores:   numero,
	CampoExhibiciones:   entero,
	CampoVistas:         numero,
	CampoMinutosVistos:  numero,
	CampoPB:             numero,
	CampoCanalID:        texto,
	CampoFecha:          texto,
	CampoHora:           texto,
	CampoMoneda:         texto,
	CampoUnidadDuracion: texto,
}

// Columna ata una columna del archivo a un campo canonico.
type Columna struct {
	// Campo de destino.
	Campo Campo

	// Nombre EXACTO de la columna en el archivo, tal como la escribe la
	// fuente. No se normaliza -- ni acentos ni mayusculas --: `Año` y `Anio` son
	// columnas distintas, y adivinar cual quiso decir el cliente es como se
	// carga una columna equivocada sin que nadie se entere.
	Nombre string

	// Requerida dice dos cosas, a dos niveles:
	//
	//   - La columna tiene que ESTAR en la cabecera. Si falta, no se persiste
	//     nada; ver [Mapa.Aplicar].
	//   - Cada fila tiene que traerle un VALOR. Una celda vacia -- o con un
	//     placeholder: en una columna requerida `--` y `N/A` son lo mismo que el
	//     blanco -- es un rechazo DE FILA, con linea y columna en el motivo, no
	//     una entrega perdida. Lo contrario era peor que fallar: un `id` vacio
	//     entraba y la cascada no podia casar ni aprender alias con esa fila, y
	//     una `taquilla` vacia entraba en cero sin ponderar nada ni dejar rastro
	//     (issue #113).
	//
	// Las columnas OPCIONALES conservan los placeholders como hueco declarado:
	// los archivos reales traen 18 de 48 columnas vacias al 100%, y ahi una
	// celda en blanco es el caso normal, no la excepcion.
	Requerida bool

	// ClaveID es la clave del contrato de ids_fuente (ADR 0018) cuando
	// [Campo] es [CampoIDsFuente]. Tiene que ser una constante
	// `aplicacion.Clave*` -- `id_ficha`, `show_id`, `imdb` --, no el nombre
	// de la columna del archivo. En cualquier otro campo tiene que quedar
	// vacia.
	ClaveID string
}

// Mapa es el mapeo declarativo de UNA fuente. Es DATO, no codigo.
//
// Que sea dato es el requisito del issue #25 -- "un mapa de columnas
// declarativo, para que una fuente nueva sea una entrada de configuracion" --
// y lo que hace que los tres formatos compartan un solo mapa por fuente: el
// CSV de Caracol y su .xlsx tienen las mismas columnas, y con la logica escrita
// en un `if` por fuente habria que copiarlas.
type Mapa struct {
	// Fuente es quien entrego. Es lo que se estampa en `usos.fuente` y lo que
	// indexa `alias_obra`.
	Fuente string

	// Modalidad de todas las filas, salvo que el archivo traiga
	// [CampoModalidad] y la celda venga con valor.
	Modalidad reparto.Modalidad

	// Hoja del .xlsx. Vacia significa la primera; se ignora en CSV y JSON.
	Hoja string

	// Columnas, en el orden en que se nombran en los mensajes de error. Es una
	// lista y no un mapa para que ese orden sea estable: el recorrido de un
	// mapa de Go es aleatorio, y un mensaje que cambia de texto en cada
	// llamada no se puede probar ni buscar en un log.
	Columnas []Columna

	// ClaveRegistro son las columnas DEL ARCHIVO que juntas identifican un
	// registro. Dos filas con la misma clave son el mismo hecho declarado dos
	// veces, y la segunda se rechaza con su motivo.
	//
	// Es la deteccion de duplicados POR REGISTRO, que es un fallo distinto del
	// duplicado por huella del archivo: aquel lo resuelve solo el
	// UNIQUE (sha256, fuente) de la boveda, este solo se ve leyendo dentro.
	//
	// La clave NO es el identificador de obra, y confundirlos rompe la
	// parrilla: en Caracol la granularidad es la EMISION, con 29 `ID_Ficha`
	// distintos en 59 filas y titulos que se emiten hasta 4 veces. Con
	// `ID_Ficha` sola como clave, 30 emisiones de verdad acabarian en el log de
	// rechazos. Por eso la clave lleva ademas fecha y hora.
	//
	// Vacia desactiva la comprobacion.
	ClaveRegistro []string
}

// Validar comprueba que el mapa esta bien formado.
//
// Se llama al CONSTRUIR el lector, no al leer un archivo: un mapa mal escrito
// es un defecto del programa, y descubrirlo cuando ya hay una entrega en vuelo
// significa descubrirlo con el cliente esperando.
func (m Mapa) Validar() error {
	if strings.TrimSpace(m.Fuente) == "" {
		return fmt.Errorf("%w: el mapa no dice de que fuente es", aplicacion.ErrReporteInvalido)
	}
	if len(m.Columnas) == 0 {
		return fmt.Errorf("%w: el mapa de %q no declara ninguna columna",
			aplicacion.ErrReporteInvalido, m.Fuente)
	}
	vistos := map[Campo]string{}
	nombres := map[string]Campo{}
	clavesID := map[string]string{}
	for _, c := range m.Columnas {
		if _, ok := tipos[c.Campo]; !ok {
			return fmt.Errorf("%w: el mapa de %q manda %q a un campo canonico que no existe (%q)",
				aplicacion.ErrReporteInvalido, m.Fuente, c.Nombre, c.Campo)
		}
		if strings.TrimSpace(c.Nombre) == "" {
			return fmt.Errorf("%w: el mapa de %q deja sin nombre la columna del campo %q",
				aplicacion.ErrReporteInvalido, m.Fuente, c.Campo)
		}
		// La misma columna de origen a dos campos no es ambiguo de leer --
		// esa posicion alimentaria los dos campos con la misma celda -- pero
		// si de mantener: la identidad de la fuente quedaria copiada del
		// titulo (o al reves) sin que nadie lo viera.
		if otro, repe := nombres[c.Nombre]; repe {
			return fmt.Errorf("%w: el mapa de %q usa la columna %q para los campos %q y %q",
				aplicacion.ErrReporteInvalido, m.Fuente, c.Nombre, otro, c.Campo)
		}
		nombres[c.Nombre] = c.Campo

		claveID := strings.TrimSpace(c.ClaveID)
		if c.Campo == CampoIDsFuente {
			if claveID == "" {
				return fmt.Errorf("%w: el mapa de %q manda %q a ids_fuente sin clave del contrato",
					aplicacion.ErrReporteInvalido, m.Fuente, c.Nombre)
			}
			if !aplicacion.EsClaveIDsFuente(claveID) {
				return fmt.Errorf("%w: el mapa de %q manda %q a ids_fuente con clave %q, que no es del contrato",
					aplicacion.ErrReporteInvalido, m.Fuente, c.Nombre, claveID)
			}
			if otra, repe := clavesID[claveID]; repe {
				return fmt.Errorf("%w: el mapa de %q declara la clave %q dos veces (%q y %q)",
					aplicacion.ErrReporteInvalido, m.Fuente, claveID, otra, c.Nombre)
			}
			clavesID[claveID] = c.Nombre
			continue
		}
		if claveID != "" {
			return fmt.Errorf("%w: el mapa de %q pone clave de ids_fuente %q en el campo %q, que no es ids_fuente",
				aplicacion.ErrReporteInvalido, m.Fuente, claveID, c.Campo)
		}
		// Dos columnas al mismo campo no es ambiguo de leer -- ganaria la
		// ultima -- pero si de mantener: nadie sabria cual de las dos es la que
		// vale, y la respuesta cambiaria al reordenar la lista.
		//
		// CampoIDsFuente es la excepcion de arriba: una fila trae varios ids
		// (show_id, series_id, netflix_id) y cada uno es una columna distinta
		// con su clave del contrato.
		if otra, repe := vistos[c.Campo]; repe {
			return fmt.Errorf("%w: el mapa de %q manda dos columnas al campo %q (%q y %q)",
				aplicacion.ErrReporteInvalido, m.Fuente, c.Campo, otra, c.Nombre)
		}
		vistos[c.Campo] = c.Nombre
	}
	if _, hayTitulo := vistos[CampoTitulo]; !hayTitulo {
		// Sin titulo no hay nada que identificar, y `validarUso` rechazaria
		// TODAS las filas de la fuente. Mejor no dejar que exista el lector.
		return fmt.Errorf("%w: el mapa de %q no dice de que columna sale el titulo",
			aplicacion.ErrReporteInvalido, m.Fuente)
	}
	if m.Modalidad == "" {
		if _, porFila := vistos[CampoModalidad]; !porFila {
			return fmt.Errorf(
				"%w: el mapa de %q no fija modalidad ni la mapea desde una columna",
				aplicacion.ErrReporteInvalido, m.Fuente)
		}
	}
	return nil
}

// Aplicar lleva una [Tabla] al esquema canonico.
//
// # Lo que aborta la entrega y lo que rechaza una fila
//
// Hay exactamente UNA clase de fallo que se lleva el archivo entero: que falte
// una columna REQUERIDA. Es estructural -- ninguna fila se puede leer -- y el
// criterio de aceptacion es que no se persista nada.
//
// Todo lo demas es de fila y viaja en el resultado con su motivo: una celda
// que no se puede convertir, un registro repetido, una modalidad que no
// existe. Un lote con filas malas no es un error, es el caso normal.
//
// # Las columnas que faltan se nombran TODAS
//
// Y no la primera. El error se lee para preparar el reenvio del archivo, y de
// una en una hacen falta tantas rondas con el cliente como columnas falten.
func (m Mapa) Aplicar(t Tabla) ([]aplicacion.UsoPersistido, error) {
	indices, err := m.indices(t)
	if err != nil {
		return nil, err
	}

	// La clave de registro se resuelve contra la cabecera del archivo, no
	// contra los campos canonicos: identifica el REGISTRO tal como lo declara
	// la fuente. Sus columnas PUEDEN estar tambien mapeadas (Fecha y Hora de
	// Caracol van a campos canonicos para normalizacion y a la vez distinguen
	// dos emisiones del mismo programa).
	clave, err := m.indicesClave(t)
	if err != nil {
		return nil, err
	}

	usos := make([]aplicacion.UsoPersistido, 0, len(t.Filas))
	// Fila del archivo en la que se vio cada clave por primera vez, para poder
	// decir en el motivo con cual choca.
	vistas := make(map[string]int, len(t.Filas))

	for n, fila := range t.Filas {
		// Numero de fila TAL COMO LO VE EL CLIENTE en su hoja de calculo: la
		// cabecera es la 1 y los datos empiezan en la 2. Un motivo que diga
		// "fila 0" obliga a quien lo lee a traducirlo, y es la clase de detalle
		// que se traduce mal.
		//
		// Se lo pregunta a la tabla y NO se calcula como `n + 2`. La diferencia
		// es que `t.Filas` no es el archivo: el lector de formato ya descarto las
		// filas enteras en blanco, asi que `n` es la posicion en la lista
		// FILTRADA y cada blanco corre la numeracion de todo lo que viene detras.
		// Con `n + 2`, la fila mala de la linea 4 de la hoja se reporta como
		// "fila 3" si tenia un blanco delante y como "fila 2" si tenia dos, que
		// es mandar al cliente a arreglar una fila que esta bien.
		linea := t.Linea(n)

		u, motivo := m.fila(fila, indices, linea, func(col int) bool { return t.compuesta(n, col) })
		if ancho := t.ancho(n); ancho < len(t.Columnas) {
			// Una coma PERDIDA corre los valores a la izquierda igual que una
			// de mas los corre a la derecha (issue #113). Va ANTES que el motivo
			// de celda y lo pisa: con el corrimiento, la celda que "falla" es
			// un sintoma, y su motivo mandaria al cliente a rellenar una celda
			// cuando lo que falta es una coma.
			motivo = fmt.Sprintf(
				"fila %d: trae %d campos y la cabecera tiene %d; una fila corta no se rellena porque suele ser una coma perdida que corre los valores a la izquierda",
				linea, ancho, len(t.Columnas))
		}
		if motivo == "" && len(fila) > len(t.Columnas) {
			// Un campo de mas no se recorta: en CSV suele ser una coma sin
			// entrecomillar que recorre todos los valores de la fila, y si los
			// corridos siguen siendo validos para su tipo, la fila entraria
			// con identificadores o medidas de otra columna. El rechazo
			// conserva lo que se pudo leer de las columnas de la cabecera
			// para poder pedirle al cliente la linea exacta.
			motivo = fmt.Sprintf(
				"fila %d: trae %d campos y la cabecera tiene %d; un campo de mas no se recorta porque suele ser una coma sin entrecomillar que recorre los valores",
				linea, len(fila), len(t.Columnas))
		}
		if motivo == "" && len(clave) > 0 {
			k := claveDe(fila, clave)
			if antes, repe := vistas[k]; repe {
				motivo = fmt.Sprintf(
					"registro duplicado: %s ya venia en la fila %d de este mismo archivo",
					descripcionClave(m.ClaveRegistro, fila, clave), antes)
			} else {
				vistas[k] = linea
			}
		}
		u.RechazoMotivo = motivo
		usos = append(usos, u)
	}
	return usos, nil
}

// indices resuelve cada columna del mapa a su posicion en la cabecera.
//
// -1 significa que la columna no esta y no era requerida: sus celdas se leen
// como vacias, que es lo que son. La clave es el nombre de la columna del
// archivo, no el campo canonico: ids_fuente admite varias columnas (show_id,
// series_id, netflix_id) y un mapa por campo se pisaria.
func (m Mapa) indices(t Tabla) (map[string]int, error) {
	indices := make(map[string]int, len(m.Columnas))
	var faltan []string
	for _, c := range m.Columnas {
		i := posicion(t.Columnas, c.Nombre)
		if i < 0 && c.Requerida {
			faltan = append(faltan, c.Nombre)
		}
		indices[c.Nombre] = i
	}
	if len(faltan) > 0 {
		// Con la cabecera que SI trae el archivo. Es lo que convierte "falta
		// Duracion_total" en un diagnostico: casi siempre la columna esta con
		// otro nombre, y verlas juntas lo resuelve sin abrir el archivo.
		return nil, fmt.Errorf(
			"%w: a la entrega de %q le faltan columnas requeridas: %s. El archivo trae: %s",
			aplicacion.ErrReporteInvalido, m.Fuente,
			strings.Join(faltan, ", "), strings.Join(t.Columnas, ", "))
	}
	return indices, nil
}

// indicesClave resuelve las columnas de [Mapa.ClaveRegistro].
//
// Una columna de la clave que no este en el archivo SI aborta la entrega,
// aunque no sea "requerida" en el sentido de [Columna]: sin ella la deteccion
// de duplicados por registro deja de funcionar, y hacerlo en silencio es
// exactamente el fallo que el ADR 0016 llama peor que fallar -- el archivo
// entraria entero, con sus repetidos ponderando dos veces, y nadie lo sabria.
func (m Mapa) indicesClave(t Tabla) ([]int, error) {
	if len(m.ClaveRegistro) == 0 {
		return nil, nil
	}
	indices := make([]int, 0, len(m.ClaveRegistro))
	var faltan []string
	for _, nombre := range m.ClaveRegistro {
		i := posicion(t.Columnas, nombre)
		if i < 0 {
			faltan = append(faltan, nombre)
			continue
		}
		indices = append(indices, i)
	}
	if len(faltan) > 0 {
		return nil, fmt.Errorf(
			"%w: a la entrega de %q le faltan las columnas que identifican un registro: %s. "+
				"Sin ellas no se pueden detectar filas repetidas. El archivo trae: %s",
			aplicacion.ErrReporteInvalido, m.Fuente,
			strings.Join(faltan, ", "), strings.Join(t.Columnas, ", "))
	}
	return indices, nil
}

// fila convierte una fila del archivo. Devuelve el motivo de rechazo, o "".
//
// El uso se devuelve SIEMPRE, rechazado o no: el log de rechazos guarda lo
// identificatorio de la fila -- fuente, titulo, ids_fuente, modalidad -- para
// poder pedirle al cliente la linea exacta, asi que una fila rechazada tiene
// que llevar todo lo que se le pudo leer antes de fallar.
//
// Solo se devuelve el PRIMER motivo. Un rechazo se lee para arreglar la fila y
// volver a mandarla; acumular los cinco fallos de una fila rota entera no
// ayuda mas y no cabe en el CHECK de un motivo por fila.
//
// compuesta dice si la celda de una columna era un objeto o array JSON (ver
// [TablaJSON]); en los demas formatos no lo es nunca.
func (m Mapa) fila(fila []string, indices map[string]int, linea int, compuesta func(col int) bool) (aplicacion.UsoPersistido, string) {
	u := aplicacion.UsoPersistido{Modalidad: m.Modalidad}
	var motivo string
	var ids []aplicacion.IDFuente

	// Recorre m.Columnas y no el mapa `indices` porque el orden importa: es el
	// que decide CUAL fallo se reporta cuando una fila tiene varios, y con el
	// recorrido aleatorio de un mapa de Go seria otro en cada corrida.
	for _, c := range m.Columnas {
		bruto := celda(fila, indices[c.Nombre])
		if compuesta(indices[c.Nombre]) && motivo == "" {
			// Antes que nada: como texto, un objeto pasa por titulo y un array
			// por identificador, y ninguna comprobacion de abajo lo veria.
			motivo = fmt.Sprintf("fila %d, %s (columna %q): valor JSON compuesto (objeto o array) donde va un valor simple: %s",
				linea, c.Campo, c.Nombre, bruto)
		}
		if c.Requerida && esPlaceholder(bruto) && motivo == "" {
			// Antes que la coercion: aDecimal convertiria el hueco en un cero
			// valido y nadie volveria a ver que faltaba.
			motivo = fmt.Sprintf("fila %d, %s (columna %q): la columna es requerida y la celda viene vacia (%q)",
				linea, c.Campo, c.Nombre, strings.TrimSpace(bruto))
		}
		if c.Campo == CampoIDsFuente {
			ids = append(ids, aplicacion.IDFuente{
				Clave: strings.TrimSpace(c.ClaveID),
				Valor: bruto,
			})
			continue
		}
		switch tipos[c.Campo] {
		case texto:
			m.asignarTexto(&u, c.Campo, bruto)
		case entero:
			v, err := aEntero(bruto)
			if err != nil && motivo == "" {
				motivo = fmt.Sprintf("fila %d, %s (columna %q): %v", linea, c.Campo, c.Nombre, err)
			}
			m.asignarEntero(&u, c.Campo, v)
		case numero:
			v, err := aDecimal(bruto)
			if err != nil && motivo == "" {
				motivo = fmt.Sprintf("fila %d, %s (columna %q): %v", linea, c.Campo, c.Nombre, err)
			}
			m.asignarDecimal(&u, c.Campo, v)
		}
	}
	if len(ids) > 0 {
		texto, err := aplicacion.EscribirIDsFuente(ids...)
		if err != nil {
			if motivo == "" {
				motivo = fmt.Sprintf("fila %d, ids_fuente: %v", linea, err)
			}
		} else {
			u.IDsFuente = texto
		}
	}
	return u, motivo
}

func (m Mapa) asignarTexto(u *aplicacion.UsoPersistido, c Campo, v string) {
	v = strings.TrimSpace(v)
	switch c {
	case CampoTitulo:
		u.Titulo = v
	case CampoTituloOrig:
		u.TituloOrig = v
	case CampoTipoObra:
		u.TipoObra = v
	case CampoCanalID:
		u.CanalID = v
	case CampoFecha:
		u.Fecha = v
	case CampoHora:
		u.Hora = v
	case CampoMoneda:
		u.Moneda = v
	case CampoUnidadDuracion:
		u.UnidadDuracion = v
	case CampoModalidad:
		// Una celda vacia deja la modalidad fija del mapa. Es lo que hace que
		// un archivo que trae la columna a medias no se convierta en medio
		// archivo rechazado; y si el mapa tampoco la fija, `validarUso` la
		// rechaza nombrando el campo, que es la unica comprobacion de este
		// valor y esta escrita una sola vez.
		if v != "" {
			u.Modalidad = reparto.Modalidad(strings.ToLower(v))
		}
	}
}

func (m Mapa) asignarEntero(u *aplicacion.UsoPersistido, c Campo, v int64) {
	switch c {
	case CampoEmisiones:
		u.Emisiones = v
	case CampoExhibiciones:
		u.Exhibiciones = v
	}
}

func (m Mapa) asignarDecimal(u *aplicacion.UsoPersistido, c Campo, v decimal.Decimal) {
	switch c {
	case CampoDuracionMin:
		u.DuracionMin = v
	case CampoRating:
		u.Rating = v
	case CampoTaquilla:
		u.Taquilla = v
	case CampoEspectadores:
		u.Espectadores = v
	case CampoVistas:
		u.Vistas = v
	case CampoMinutosVistos:
		u.MinutosVistos = v
	case CampoPB:
		u.PB = v
	}
}

// placeholders son los textos con los que las fuentes escriben "aqui no hay
// dato" en una columna numerica.
//
// `--` esta medido en el archivo de Netflix (`episode_nbr`). Los demas son los
// que escriben Excel y los exports de SQL. Se comparan en minusculas y ya
// recortados.
//
// Tratarlos como vacio y no como error es deliberado: un hueco declarado no es
// un dato roto. Lo que NO se hace es tratar como vacio cualquier cosa que no
// parsee -- eso convertiria un `1.234,56` mal formateado en un cero, que es una
// cifra falsa entrando en el reparto sin que nadie lo vea.
var placeholders = map[string]struct{}{
	"":     {},
	"--":   {},
	"-":    {},
	"n/a":  {},
	"na":   {},
	"null": {},
	"nan":  {},
	"#n/a": {},
}

func esPlaceholder(v string) bool {
	_, ok := placeholders[strings.ToLower(strings.TrimSpace(v))]
	return ok
}

// aDecimal convierte una celda a la medida que espera el esquema.
//
// El cero de un placeholder NO es una mentira: las columnas de medida de `usos`
// son NOT NULL DEFAULT 0, asi que "no declarado" y "cero" ya son el mismo
// estado en la base, y esta funcion no inventa nada que la tabla no dijera.
//
// Lo que NO hace es normalizar formatos regionales -- `1.234,56`, `$ 1.000`, un
// `48 min` --. Eso es el issue #26, y adivinarlo aqui es peor que fallar: leer
// `1.234,56` como 1,23456 pondera mil veces menos y no falla nada. Mientras
// tanto la celda cae al log de rechazos NOMBRANDO el campo y el valor, que es
// lo que permite pedirle al cliente el formato bueno.
func aDecimal(bruto string) (decimal.Decimal, error) {
	if esPlaceholder(bruto) {
		return decimal.Zero, nil
	}
	v, err := decimal.NewFromString(strings.TrimSpace(bruto))
	if err != nil {
		return decimal.Zero, fmt.Errorf("%q no es un numero", strings.TrimSpace(bruto))
	}
	return v, nil
}

// aEntero convierte una celda a un recuento.
//
// Acepta un decimal con parte fraccionaria NULA porque es como una hoja de
// calculo escribe los enteros: `stream_starts` sale de excelize como `2172`
// pero de un JSON exportado por pandas como `2172.0`, y rechazar el segundo
// seria rechazar el mismo dato por el formato del archivo. Un `2172.5` si es
// un error: un recuento de emisiones fraccionario no significa nada, y
// truncarlo en silencio perderia el aviso de que la columna no es la que se
// creia.
func aEntero(bruto string) (int64, error) {
	if esPlaceholder(bruto) {
		return 0, nil
	}
	bruto = strings.TrimSpace(bruto)
	if n, err := strconv.ParseInt(bruto, 10, 64); err == nil {
		return n, nil
	}
	d, err := decimal.NewFromString(bruto)
	if err != nil {
		return 0, fmt.Errorf("%q no es un numero entero", bruto)
	}
	if !d.Equal(d.Truncate(0)) {
		return 0, fmt.Errorf("%q no es entero y un recuento no se puede partir", bruto)
	}
	// IntPart no avisa fuera de rango: desborda, y con el signo cambiado
	// (`9223372036854775808` sale como MinInt64). Es la unica salida por la que
	// un recuento podria entrar como OTRO numero sin motivo (issue #113).
	if d.GreaterThan(maxInt64) || d.LessThan(minInt64) {
		return 0, fmt.Errorf("%q no cabe en un recuento (maximo %d)", bruto, int64(math.MaxInt64))
	}
	return d.IntPart(), nil
}

// Los bordes de int64 como decimales, para comparar antes de IntPart.
var (
	maxInt64 = decimal.NewFromInt(math.MaxInt64)
	minInt64 = decimal.NewFromInt(math.MinInt64)
)

// celda lee una posicion de la fila. Una columna ausente -- indice -1 -- es una
// celda vacia, no un panico.
func celda(fila []string, i int) string {
	if i < 0 || i >= len(fila) {
		return ""
	}
	return fila[i]
}

func posicion(columnas []string, nombre string) int {
	for i, c := range columnas {
		if c == nombre {
			return i
		}
	}
	return -1
}

// claveDe compone la clave de registro de una fila.
//
// El separador es un byte nulo porque no puede aparecer en una celda de texto:
// con un guion, las claves ("a-b", "c") y ("a", "b-c") serian la misma y dos
// registros distintos se reportarian como duplicados.
//
// Se comparan los valores RECORTADOS y en minusculas: un identificador con un
// espacio de mas es el mismo identificador, y esta comparacion no decide nada
// que se persista -- solo si una fila se repite --, asi que puede permitirse ser
// tolerante donde `alias_obra` no puede.
func claveDe(fila []string, indices []int) string {
	partes := make([]string, len(indices))
	for i, idx := range indices {
		partes[i] = strings.ToLower(strings.TrimSpace(celda(fila, idx)))
	}
	return strings.Join(partes, "\x00")
}

// descripcionClave escribe la clave para un humano: `ID_Ficha=55174,
// Fecha=20241231`.
func descripcionClave(nombres []string, fila []string, indices []int) string {
	partes := make([]string, 0, len(indices))
	for i, idx := range indices {
		partes = append(partes, nombres[i]+"="+strings.TrimSpace(celda(fila, idx)))
	}
	return strings.Join(partes, ", ")
}
