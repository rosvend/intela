package ingesta

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Tabla es un archivo tabular ya leido: una cabecera y sus filas, todo texto.
//
// Es la frontera entre el FORMATO y la FUENTE, que son los dos ejes que este
// paquete separa. Un lector de formato no sabe que es un `ID_Ficha`; un mapa de
// columnas no sabe si sus columnas venian de una hoja de calculo o de un JSON.
// Sin esa frontera, dar de alta el CSV de una fuente que ya entregaba .xlsx
// obligaria a duplicar su mapa de columnas, y los dos se separarian el dia que
// alguien tocara uno solo.
//
// Todo es texto a proposito. La coercion de tipos la hace [Mapa], una vez, con
// el campo canonico de destino delante: el mismo `20241231` es una fecha en la
// parrilla y seria un entero en cualquier otra columna, y solo el mapa sabe
// cual de las dos cosas se le esta pidiendo.
//
// Filas NO incluye la cabecera. Las filas cortas se rellenan hasta
// len(Columnas): excelize recorta las celdas vacias del final, y un CSV
// escrito a mano se queda sin comas. Una fila MAS ancha que la cabecera se
// deja con los campos de mas -- [Mapa.Aplicar] la rechaza -- porque recortarlos
// en silencio es como se persiste un identificador corrido por una coma sin
// entrecomillar.
type Tabla struct {
	Columnas []string
	Filas    [][]string

	// Lineas es, para cada elemento de Filas, el numero de fila DEL ARCHIVO del
	// que salio, con la cabecera como 1.
	//
	// Existe porque Filas ya no es el archivo: [desdeFilas] descarta las filas
	// enteras en blanco, que son relleno del export y no registros. Sin esta
	// correspondencia, el unico numero disponible aguas arriba es la posicion en
	// la lista YA FILTRADA, y entonces cada blanco corre la numeracion de todo lo
	// que viene detras: la fila mala de la linea 4 de la hoja se reporta como
	// "fila 3" con un blanco delante, y como "fila 2" con dos.
	//
	// No es cosmetico. El motivo de rechazo existe para pedirle al cliente LA
	// LINEA EXACTA que hay que arreglar, y un numero corrido lo manda a mirar una
	// fila que esta bien -- o peor, justo a la que se salto por venir vacia.
	//
	// Es una lista paralela a Filas y no un campo por fila porque una `Tabla` es
	// la frontera entre el formato y el mapa, y ese contrato es "cabecera y
	// celdas, todo texto": meter un numero dentro de la fila obligaria a todo el
	// mapeo a distinguir la celda de la anotacion. Se lee por [Tabla.Linea], que
	// es lo unico que la consulta.
	Lineas []int
}

// Linea devuelve el numero de fila del archivo del que salio Filas[n].
//
// Una tabla sin numeracion -- las que se componen a mano en las pruebas -- cae a
// la posicion, que es lo que hay: la cabecera es la 1 y los datos empiezan en la
// 2. Los tres lectores de formato de este fichero SI la rellenan, asi que por el
// camino real nunca se usa esa salida.
func (t Tabla) Linea(n int) int {
	if n >= 0 && n < len(t.Lineas) {
		return t.Lineas[n]
	}
	return n + 2
}

// ErrFormato: los bytes no son del formato que se esperaba.
//
// Envuelve [aplicacion.ErrReporteInvalido] para que el caso de uso y el
// adaptador HTTP no tengan que conocer este paquete: un archivo ilegible es un
// 400 con un mensaje para el cliente, no un 500.
var ErrFormato = fmt.Errorf("%w: formato ilegible", aplicacion.ErrReporteInvalido)

// bom es la marca de orden de bytes que Excel antepone a todo CSV que exporta.
//
// Sin quitarla, la PRIMERA columna de la cabecera se llama "\ufeff" + "titulo" y no
// casa con ninguna del mapa. El sintoma es el peor posible: falta justo la
// columna requerida que da el titulo, asi que el archivo entero se rechaza con
// un mensaje que nombra una columna que el cliente ve escrita en su pantalla.
const bom = "\ufeff"

// TablaXLSX lee una hoja de un .xlsx.
//
// hoja vacia significa la primera. Es lo razonable para los archivos del
// cliente -- la parrilla de Caracol trae UNA hoja, con un nombre truncado a 31
// caracteres por el propio Excel -- y evita que el mapa tenga que declarar un
// nombre que puede cambiar entre entregas. El de Netflix si lo declara
// (`NETFLIX_REDES_2018`), y ahi el nombre exacto es la comprobacion de que
// llego el archivo que se creia.
func TablaXLSX(datos []byte, hoja string) (Tabla, error) {
	// UnzipSizeLimit acota lo DESCOMPRIMIDO: el tope de 32 MiB de la subida es
	// sobre el archivo comprimido, y sin esto excelize acepta hasta 16 GB. 256
	// MiB es 8 veces el tope comprimido -holgado para el XML verboso de una
	// parrilla real, que son KB- y cierra la puerta a la bomba de zip.
	// UnzipXMLSizeLimit se queda en su defecto (16 MB derramados a disco):
	// cumple UnzipXMLSizeLimit <= UnzipSizeLimit, que excelize exige.
	libro, err := excelize.OpenReader(bytes.NewReader(datos), excelize.Options{UnzipSizeLimit: 256 << 20})
	if err != nil {
		return Tabla{}, fmt.Errorf("%w: no se pudo abrir como .xlsx: %w", ErrFormato, err)
	}
	// El Close de excelize libera los ficheros temporales que crea para las
	// hojas grandes. No devuelve nada que se pueda hacer aqui.
	defer func() { _ = libro.Close() }()

	hojas := libro.GetSheetList()
	if len(hojas) == 0 {
		return Tabla{}, fmt.Errorf("%w: el libro no tiene ninguna hoja", ErrFormato)
	}
	if hoja == "" {
		hoja = hojas[0]
	} else if !slices.Contains(hojas, hoja) {
		// Con la lista de las que SI hay. Una hoja renombrada entre entregas es
		// de los fallos mas comunes de un export manual, y el nombre correcto
		// esta a la vista de quien lee el error.
		return Tabla{}, fmt.Errorf("%w: falta la hoja %q; el libro trae %s",
			ErrFormato, hoja, strings.Join(hojas, ", "))
	}

	filasIter, err := libro.Rows(hoja)
	if err != nil {
		return Tabla{}, fmt.Errorf("%w: no se pudo leer la hoja %q: %w", ErrFormato, hoja, err)
	}
	defer func() { _ = filasIter.Close() }()

	// No se usa GetRows: compacta las filas fisicas vacias y, peor, rellena
	// huecos hasta el atributo r del XML. Un r por encima de 1_048_576
	// (GHSA-q5j5-6p94-4gwc, excelize v2.10.1) materializa filas hasta ese
	// indice. Subir a v2.11.0 exigiria Go 1.25 y romperia el Dockerfile.
	crudas := make([][]string, 0, 64)
	fisicas := make([]int, 0, 64)
	iteradas := 0
	for filasIter.Next() {
		iteradas++
		if iteradas > maxFilasExcel {
			return Tabla{}, fmt.Errorf("%w: la hoja %q supera el tope de %d filas",
				ErrFormato, hoja, maxFilasExcel)
		}
		n, err := filaFisica(filasIter)
		if err != nil {
			return Tabla{}, fmt.Errorf("%w: %v", ErrFormato, err)
		}
		if n < 1 || n > maxFilasExcel {
			return Tabla{}, fmt.Errorf(
				"%w: la hoja %q declara la fila %d y Excel no admite mas de %d",
				ErrFormato, hoja, n, maxFilasExcel)
		}
		row, err := filasIter.Columns()
		if err != nil {
			return Tabla{}, fmt.Errorf("%w: no se pudo leer la hoja %q: %w", ErrFormato, hoja, err)
		}
		if vacia(row) {
			continue
		}
		crudas = append(crudas, row)
		fisicas = append(fisicas, n)
	}
	if err := filasIter.Error(); err != nil {
		return Tabla{}, fmt.Errorf("%w: no se pudo leer la hoja %q: %w", ErrFormato, hoja, err)
	}
	return desdeFilasNumeradas(crudas, fisicas)
}

// maxFilasExcel es el tope de filas de una hoja .xlsx (2^20). Es el mismo
// TotalRows de excelize. Un atributo r por encima es el vector de
// GHSA-q5j5-6p94-4gwc: GetRows rellenaba huecos hasta ese indice.
const maxFilasExcel = 1_048_576

// filaFisica lee el numero de fila que excelize guardo al parsear el atributo
// r del XML. No esta exportado en v2.10.1; si lo renombran, la prueba de la
// fila fisica tras un hueco se pone roja.
func filaFisica(rows *excelize.Rows) (int, error) {
	v := reflect.ValueOf(rows).Elem().FieldByName("curRow")
	if !v.IsValid() || v.Kind() != reflect.Int {
		return 0, fmt.Errorf("excelize.Rows ya no expone curRow; hay que actualizar el lector de xlsx")
	}
	return int(v.Int()), nil
}

// TablaCSV lee un CSV con cabecera.
//
// FieldsPerRecord = -1 desactiva la comprobacion de que todas las filas tengan
// el mismo numero de campos, y es deliberado: con la comprobacion puesta, UNA
// fila con una coma de mas aborta la lectura del archivo ENTERO y las demas se
// pierden sin motivo por fila. El desajuste no se ignora: la fila corta se
// rellena, la fila ancha se deja con los campos de mas y [Mapa.Aplicar] la
// rechaza nombrando el desajuste, que es lo que convierte un corrimiento por
// coma sin entrecomillar en un rechazo de fila y no en un identificador
// persistido.
func TablaCSV(datos []byte) (Tabla, error) {
	texto := strings.TrimPrefix(string(datos), bom)
	// LazyQuotes se queda: una sinopsis con una comilla a medias DENTRO del
	// campo -- `dijo "hola" y siguio` -- es texto, no estructura, y tumbar el
	// archivo por ella haria ilegibles las parrillas. Lo que LazyQuotes no
	// puede hacer es dejar abierta la comilla de arranque hasta el final: en
	// vez de devolver ErrQuote, encoding/csv mete en ese campo todo lo que
	// falta del archivo. Las filas de despues no existen, ni siquiera en el log
	// de rechazos, y el recuento de la entrega no cuadra con el archivo sin que
	// nadie lo diga (issue #167, la misma regla de ADR 0016: ni se pierde ni se
	// cuela). Eso ya no es una fila mala. Es un archivo que no se puede contar.
	// Se mira ANTES de leer y se rechaza la entrega nombrando la linea donde
	// abre la comilla, que es la que el cliente tiene que corregir.
	if linea := lineaDeComillaSinCerrar(texto); linea > 0 {
		return Tabla{}, fmt.Errorf(
			"%w: la linea %d abre una comilla que no se cierra y el resto del archivo queda dentro de ese campo",
			ErrFormato, linea)
	}
	lector := csv.NewReader(strings.NewReader(texto))
	lector.FieldsPerRecord = -1
	// Las parrillas traen texto libre -- sinopsis con comillas dentro -- y un
	// campo entrecomillado a medias no puede tumbar el archivo.
	lector.LazyQuotes = true
	// Excel escribe CSV con `;` en configuraciones regionales europeas, pero el
	// separador NO se adivina: adivinarlo mal parte los titulos por la mitad en
	// silencio. Si algun dia hace falta, entra como campo declarado del mapa.
	filas, err := lector.ReadAll()
	if err != nil {
		return Tabla{}, fmt.Errorf("%w: no se pudo leer como CSV: %w", ErrFormato, err)
	}
	return desdeFilas(filas)
}

// lineaDeComillaSinCerrar devuelve la linea fisica (la 1 es la primera del
// archivo) donde empieza un campo cuya comilla de apertura no llega a
// cerrarse, o 0 si no hay ninguna.
//
// Replica el automata de encoding/csv con LazyQuotes, no una lectura RFC
// estricta. La diferencia importa: con LazyQuotes una comilla suelta en medio
// de un campo ya entrecomillado es texto, y el campo sigue abierto hasta una
// comilla seguida de coma, de fin de linea o de fin de archivo. Rechazar esa
// comilla suelta tumbaria las sinopsis que LazyQuotes esta puesto para
// aceptar. Lo que se rechaza es llegar al final SIN haber encontrado ese
// cierre: es el unico caso en el que el lector se traga el resto del archivo.
//
// El numero es el de la linea donde ABRE la comilla, no el de la fila del
// registro. Un campo entrecomillado de verdad puede ocupar varias lineas
// antes, y la comilla que no cierra puede no ser el primer campo de la suya.
func lineaDeComillaSinCerrar(datos string) int {
	if !strings.Contains(datos, `"`) {
		return 0
	}
	c := &cursorCSV{b: []byte(datos)}
	for {
		line, err := c.primerRegistro()
		if err != nil {
			return 0
		}
		for {
			if len(line) == 0 || line[0] != '"' {
				coma := bytes.IndexByte(line, ',')
				if coma < 0 {
					break
				}
				line = line[coma+1:]
				continue
			}
			inicio := c.linea
			line = line[1:]
			sigue := false
			for {
				q := bytes.IndexByte(line, '"')
				if q < 0 {
					if len(line) == 0 {
						// EOF dentro del campo: la comilla de arranque no
						// llego a cerrarse.
						return inicio
					}
					line, err = c.leerLinea()
					if err != nil {
						return inicio
					}
					continue
				}
				line = line[q+1:]
				if len(line) > 0 && line[0] == '"' {
					// `""` es una comilla literal. El campo sigue abierto.
					line = line[1:]
					continue
				}
				if len(line) > 0 && line[0] == ',' {
					line = line[1:]
					sigue = true
					break
				}
				if len(line) == longitudFinDeLinea(line) {
					// La comilla cierra el campo y el registro.
					break
				}
				// LazyQuotes: la comilla es texto. Sigue buscando el cierre.
				continue
			}
			if !sigue {
				break
			}
		}
	}
}

// cursorCSV recorre los bytes con la misma nocion de linea que encoding/csv:
// corta por '\n', un "\r\n" es una sola linea, y un '\r' pegado al final
// antes de EOF no abre una linea vacia.
type cursorCSV struct {
	b     []byte
	i     int
	linea int
}

func (c *cursorCSV) primerRegistro() ([]byte, error) {
	for {
		line, err := c.leerLinea()
		if err != nil {
			return nil, err
		}
		// encoding/csv descarta las lineas fisicamente vacias ENTRE registros.
		// Dentro de un campo entrecomillado no: ahi el salto es contenido, y
		// lo consume [cursorCSV.leerLinea] desde el bucle de la comilla.
		if len(line) == longitudFinDeLinea(line) {
			continue
		}
		return line, nil
	}
}

func (c *cursorCSV) leerLinea() ([]byte, error) {
	if c.i >= len(c.b) {
		return nil, io.EOF
	}
	ini := c.i
	for c.i < len(c.b) && c.b[c.i] != '\n' {
		c.i++
	}
	var line []byte
	if c.i < len(c.b) && c.b[c.i] == '\n' {
		line = c.b[ini : c.i+1]
		c.i++
		// No se muta el buffer de origen: la linea puede seguir apuntando a el.
		if n := len(line); n >= 2 && line[n-2] == '\r' {
			line = append(append([]byte{}, line[:n-2]...), '\n')
		}
	} else {
		line = c.b[ini:c.i]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
	}
	c.linea++
	return line, nil
}

func longitudFinDeLinea(line []byte) int {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		return 1
	}
	return 0
}

// TablaJSON lee un array de objetos planos.
//
// # Las columnas se ordenan
//
// Un objeto JSON no tiene orden de claves, asi que la cabecera se compone con
// la UNION de las claves de todos los registros, ordenada. Sacarla del primer
// objeto dejaria fuera las columnas que solo aparecen mas abajo, y sin ordenar
// el mensaje de "falta la columna X; el archivo trae ..." cambiaria de texto en
// cada corrida.
//
// # Los valores se convierten a su texto, no a su tipo
//
// json.Number conserva el literal tal como venia: 80197856 no se convierte a
// float64 y vuelve como 8.0197856e+07, que es exactamente como se estropea un
// identificador al pasar por un JSON. Un valor compuesto -- objeto o array --
// se conserva como su JSON compacto: si la columna no esta mapeada da igual, y
// si lo esta, la coercion la rechaza NOMBRANDO el campo, que es mejor que
// convertirla en cadena vacia sin decirlo.
func TablaJSON(datos []byte) (Tabla, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(datos, []byte(bom))))
	dec.UseNumber()

	var registros []map[string]json.RawMessage
	if err := dec.Decode(&registros); err != nil {
		return Tabla{}, fmt.Errorf(
			"%w: no se pudo leer como JSON; se esperaba un array de objetos: %w", ErrFormato, err)
	}
	// Un segundo valor despues del array es basura pegada al final, y casi
	// siempre significa dos exports concatenados. Aceptarlo cargaria solo el
	// primero y diria que todo fue bien.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Tabla{}, fmt.Errorf(
			"%w: el JSON trae contenido despues del array de registros", ErrFormato)
	}

	claves := map[string]struct{}{}
	for _, r := range registros {
		for k := range r {
			claves[k] = struct{}{}
		}
	}
	columnas := make([]string, 0, len(claves))
	for k := range claves {
		columnas = append(columnas, k)
	}
	slices.Sort(columnas)

	filas := make([][]string, 0, len(registros))
	lineas := make([]int, 0, len(registros))
	for n, r := range registros {
		fila := make([]string, len(columnas))
		for i, c := range columnas {
			fila[i] = textoJSON(r[c])
		}
		filas = append(filas, fila)
		// Un array JSON no tiene cabecera, pero el registro n-esimo se numera
		// como la fila n-esima de una tabla que si la tiene: aqui no se descarta
		// ningun registro, asi que la correspondencia es directa, y mantenerla
		// deja UN solo formato de motivo para los tres formatos.
		lineas = append(lineas, n+2)
	}
	return Tabla{Columnas: columnas, Filas: filas, Lineas: lineas}, nil
}

// textoJSON reduce un valor JSON a su texto de celda.
func textoJSON(crudo json.RawMessage) string {
	if len(crudo) == 0 {
		return ""
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(crudo))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return string(crudo)
	}
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		// Objeto o array. Se conserva su texto para que la coercion lo pueda
		// rechazar nombrando el campo. Ver el doc de TablaJSON.
		return string(crudo)
	}
}

// desdeFilas parte la cabecera del cuerpo y cuadra el ancho de las filas.
//
// La cabecera se recorta con TrimSpace y se le quita el BOM. Los tres son
// blancos que no se ven: una columna que en la pantalla del cliente se llama
// `Titulo ` no casaria con `Titulo`, y el archivo se rechazaria entero
// nombrando una columna que esta ahi.
func desdeFilas(filas [][]string) (Tabla, error) {
	if len(filas) == 0 {
		return Tabla{}, fmt.Errorf("%w: el archivo no tiene ni cabecera", ErrFormato)
	}
	columnas := make([]string, len(filas[0]))
	for i, c := range filas[0] {
		columnas[i] = strings.TrimSpace(strings.TrimPrefix(c, bom))
	}

	cuerpo := make([][]string, 0, len(filas)-1)
	lineas := make([]int, 0, len(filas)-1)
	for i, f := range filas[1:] {
		if vacia(f) {
			// Una fila entera en blanco es relleno del export, no un registro.
			// Mandarla al log de rechazos llenaria el log de ruido y taparia
			// los rechazos de verdad, que son los que hay que pedirle al
			// cliente.
			continue
		}
		var fila []string
		if len(f) > len(columnas) {
			// Se conserva el ancho de mas. Recortar en silencio es como se
			// persiste un valor corrido: la coma extra desplaza las celdas y,
			// si lo corrido sigue siendo valido para su tipo, la fila entra
			// con el identificador de otra columna.
			fila = f
		} else {
			fila = make([]string, len(columnas))
			copy(fila, f)
		}
		cuerpo = append(cuerpo, fila)
		// El numero que ve el cliente en su hoja: `filas` incluye la cabecera,
		// asi que el primer registro del archivo es la linea 2. Se anota AQUI,
		// que es el unico sitio donde todavia se sabe de que linea salio: a
		// partir de este return la fila descartada ya no existe y la posicion en
		// `cuerpo` esta corrida.
		lineas = append(lineas, i+2)
	}
	return Tabla{Columnas: columnas, Filas: cuerpo, Lineas: lineas}, nil
}

// desdeFilasNumeradas es [desdeFilas] cuando el lector YA conoce el numero
// fisico de cada fila -- el .xlsx, cuyo GetRows compacta huecos y no se usa.
func desdeFilasNumeradas(filas [][]string, fisicas []int) (Tabla, error) {
	if len(filas) != len(fisicas) {
		return Tabla{}, fmt.Errorf("%w: el lector de xlsx desalineo filas y numeros de linea", ErrFormato)
	}
	t, err := desdeFilas(filas)
	if err != nil {
		return Tabla{}, err
	}
	if len(fisicas) == 0 {
		return t, nil
	}
	// fisicas[0] es la cabecera. El cuerpo hereda el resto, que ya salio del
	// XML y no se puede reconstruir como i+2.
	t.Lineas = append([]int(nil), fisicas[1:]...)
	return t, nil
}

func vacia(fila []string) bool {
	for _, c := range fila {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
