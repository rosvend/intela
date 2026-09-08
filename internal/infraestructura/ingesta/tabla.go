package ingesta

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// Filas NO incluye la cabecera, y todas tienen exactamente len(Columnas)
// celdas: los lectores rellenan las que falten. Un formato tabular real
// entrega filas cortas -- excelize recorta las celdas vacias del final, y un
// CSV escrito a mano se queda sin comas -- y sin ese relleno cada lectura de
// una columna del final tendria que comprobar el limite por su cuenta.
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
	libro, err := excelize.OpenReader(bytes.NewReader(datos))
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

	filas, err := libro.GetRows(hoja)
	if err != nil {
		return Tabla{}, fmt.Errorf("%w: no se pudo leer la hoja %q: %w", ErrFormato, hoja, err)
	}
	return desdeFilas(filas)
}

// TablaCSV lee un CSV con cabecera.
//
// FieldsPerRecord = -1 desactiva la comprobacion de que todas las filas tengan
// el mismo numero de campos, y es deliberado: con la comprobacion puesta, UNA
// fila con una coma de mas aborta la lectura del archivo ENTERO y las demas se
// pierden sin motivo por fila. El desajuste no se ignora -- [desdeFilas] recorta
// o rellena, y una fila a la que le falte una columna requerida acaba rechazada
// con su motivo --, pero deja de llevarse por delante a las que estaban bien.
func TablaCSV(datos []byte) (Tabla, error) {
	lector := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(datos), bom)))
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
		fila := make([]string, len(columnas))
		copy(fila, f)
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

func vacia(fila []string) bool {
	for _, c := range fila {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
