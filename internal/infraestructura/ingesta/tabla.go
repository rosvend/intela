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
// len(Columnas), para que el mapa pueda leer lo que traen. En .xlsx eso es
// todo: excelize recorta las celdas vacias del final y la fila corta es la
// forma normal. En CSV no: una coma PERDIDA corre los valores a la izquierda,
// asi que el lector anota el ancho real y [Mapa.Aplicar] rechaza la fila
// (issue #113). Una fila MAS ancha que la cabecera se deja con los campos de
// mas -- [Mapa.Aplicar] la rechaza -- porque recortarlos en silencio es como
// se persiste un identificador corrido por una coma sin entrecomillar.
type Tabla struct {
	Columnas []string
	Filas    [][]string

	// Lineas es, para cada elemento de Filas, el numero de fila DEL ARCHIVO del
	// que salio, con la cabecera como 1.
	//
	// Existe porque Filas ya no es el archivo: se descartan las filas enteras
	// en blanco, que son relleno del export y no registros -- las descarta
	// [desdeFilasNumeradas], y en CSV ya antes el propio `encoding/csv`, que se
	// salta las lineas fisicamente vacias sin avisar --. Sin esta
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

	// anchos es, cuando el formato lo hace significativo (solo CSV), cuantos
	// campos traia cada fila ANTES de rellenarla. nil en .xlsx y JSON.
	// anchoEsperado es el que tiene que alcanzar para no ser corta; ver
	// [anchoEsperado]. Se leen por [Tabla.corta].
	anchos        []int
	anchoEsperado int
	// conEsperado es cuantas filas traen anchoEsperado, de len(anchos): lo que
	// el motivo de rechazo cuenta para que diga la verdad sobre el archivo.
	conEsperado int
	// disputa son los anchos legitimos que se reparten las filas cuando
	// ninguno llega a umbralMayoriaAncho. Vacia si el archivo se decidio.
	disputa []anchoEnDisputa

	// formato es de que lector salio la tabla. Solo lo usan los motivos que
	// dependen de el: un dato sin cabecera es "una coma de mas" en CSV, una
	// columna sin encabezado en .xlsx y una clave vacia en JSON.
	formato string

	// compuestas marca, por fila, las columnas cuyo valor era un objeto o un
	// array JSON. Solo lo rellena [TablaJSON]. Existe porque la celda es
	// texto: `{"x":1}` como texto es un titulo valido y un identificador
	// valido, y sin la marca el mapa no tiene forma de saber que no lo era.
	compuestas []map[int]bool
}

// compuesta dice si la celda (n, col) era un valor JSON compuesto.
func (t Tabla) compuesta(n, col int) bool {
	return n >= 0 && n < len(t.compuestas) && t.compuestas[n][col]
}

// anchoEnDisputa es un ancho legitimo y cuantas filas lo traen.
type anchoEnDisputa struct{ ancho, filas int }

// enDisputa dice si Filas[n] tiene uno de los anchos legitimos que el archivo
// no pudo decidir (ver [anchoEsperado]). Toda fila asi se rechaza.
func (t Tabla) enDisputa(n int) bool {
	if len(t.disputa) == 0 || n < 0 || n >= len(t.anchos) {
		return false
	}
	for _, d := range t.disputa {
		if d.ancho == t.anchos[n] {
			return true
		}
	}
	return false
}

// desajuste dice si Filas[n] traia un numero de campos distinto del que
// escribe el archivo, dentro del ancho de la cabecera, y cuantos traia. Una
// fila MAS ancha que la cabecera no es desajuste: la rechaza [Mapa.Aplicar]
// por ancha. Siempre false en los formatos que no anotan el ancho.
func (t Tabla) desajuste(n int) (ancho int, es bool) {
	if n < 0 || n >= len(t.anchos) {
		return 0, false
	}
	a := t.anchos[n]
	return a, a != t.anchoEsperado && a <= len(t.Columnas)
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
	t, err := desdeFilasNumeradas(crudas, fisicas, false)
	t.formato = aplicacion.FormatoXLSX
	return t, err
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
// rellena para poder leerla pero se anota su ancho real, la fila ancha se deja
// con los campos de mas, y [Mapa.Aplicar] rechaza las dos nombrando el
// desajuste. Es lo que convierte un corrimiento por coma -- de mas o perdida --
// en un rechazo de fila y no en un identificador persistido.
func TablaCSV(datos []byte) (Tabla, error) {
	lector := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(datos), bom)))
	lector.FieldsPerRecord = -1
	// Las parrillas traen texto libre -- sinopsis con comillas dentro -- y un
	// campo entrecomillado a medias no puede tumbar el archivo.
	lector.LazyQuotes = true
	// Excel escribe CSV con `;` en configuraciones regionales europeas, pero el
	// separador NO se adivina: adivinarlo mal parte los titulos por la mitad en
	// silencio. Si algun dia hace falta, entra como campo declarado del mapa.
	//
	// Se lee registro a registro y NO con ReadAll, por la numeracion: el lector
	// descarta las lineas FISICAMENTE en blanco antes de devolver nada, asi que
	// con ReadAll la posicion en la lista ya no es la linea del archivo, y la fila
	// mala de la linea 5 con dos blancos delante se reportaba como "fila 3"
	// (issue #113). FieldPos da la linea en la que EMPIEZA el registro, que es
	// tambien lo correcto con un campo entrecomillado que abarca varias lineas.
	var (
		filas   [][]string
		fisicas []int
	)
	for {
		registro, err := lector.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Tabla{}, fmt.Errorf("%w: no se pudo leer como CSV: %w", ErrFormato, err)
		}
		linea, _ := lector.FieldPos(0)
		filas = append(filas, registro)
		fisicas = append(fisicas, linea)
	}
	t, err := desdeFilasNumeradas(filas, fisicas, true)
	t.formato = aplicacion.FormatoCSV
	return t, err
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
// se conserva como su JSON y se MARCA como compuesto: si la columna no esta
// mapeada da igual, y si lo esta, [Mapa.Aplicar] rechaza la fila NOMBRANDO el
// campo, que es mejor que convertirla en cadena vacia sin decirlo. La marca
// hace falta porque, como texto, `{"x":1}` pasa por un titulo y `[1,2]` por un
// identificador: sin ella entraban los dos sin motivo (issue #113).
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
	compuestas := make([]map[int]bool, len(registros))
	for n, r := range registros {
		fila := make([]string, len(columnas))
		for i, c := range columnas {
			var compuesto bool
			fila[i], compuesto = textoJSON(r[c])
			if compuesto {
				if compuestas[n] == nil {
					compuestas[n] = map[int]bool{}
				}
				compuestas[n][i] = true
			}
		}
		filas = append(filas, fila)
		// Un array JSON no tiene cabecera, pero el registro n-esimo se numera
		// como la fila n-esima de una tabla que si la tiene: aqui no se descarta
		// ningun registro, asi que la correspondencia es directa, y mantenerla
		// deja UN solo formato de motivo para los tres formatos.
		lineas = append(lineas, n+2)
	}
	return Tabla{Columnas: columnas, Filas: filas, Lineas: lineas, compuestas: compuestas, formato: aplicacion.FormatoJSON}, nil
}

// textoJSON reduce un valor JSON a su texto de celda, y dice si era un valor
// compuesto (objeto o array).
func textoJSON(crudo json.RawMessage) (string, bool) {
	if len(crudo) == 0 {
		return "", false
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(crudo))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return string(crudo), false
	}
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, false
	case json.Number:
		return t.String(), false
	case bool:
		if t {
			return "true", false
		}
		return "false", false
	default:
		// Objeto o array. Se conserva su texto para el log de rechazos y se
		// marca, para que el mapa lo rechace nombrando el campo. Ver el doc de
		// TablaJSON.
		return string(crudo), true
	}
}

// desdeFilasNumeradas parte la cabecera del cuerpo y cuadra el ancho de las
// filas. fisicas[i] es la linea del archivo de la que salio filas[i], con
// fisicas[0] la de la cabecera: la posicion no sirve, porque los dos lectores
// que pasan por aqui ya descartaron filas antes (el CSV las lineas en blanco,
// el .xlsx los huecos fisicos).
//
// anotarAncho guarda cuantos campos traia de verdad cada fila antes de
// rellenarla, para que [Mapa.Aplicar] rechace la corta. Solo lo pide el CSV:
// en .xlsx una fila corta es lo normal -- excelize recorta las celdas vacias
// del final -- y rechazarla tiraria toda fila con opcionales vacias al final.
//
// La cabecera se recorta con TrimSpace y se le quita el BOM. Los tres son
// blancos que no se ven: una columna que en la pantalla del cliente se llama
// `Titulo ` no casaria con `Titulo`, y el archivo se rechazaria entero
// nombrando una columna que esta ahi.
func desdeFilasNumeradas(filas [][]string, fisicas []int, anotarAncho bool) (Tabla, error) {
	if len(filas) != len(fisicas) {
		return Tabla{}, fmt.Errorf("%w: el lector desalineo filas y numeros de linea", ErrFormato)
	}
	if len(filas) == 0 {
		return Tabla{}, fmt.Errorf("%w: el archivo no tiene ni cabecera", ErrFormato)
	}
	columnas := make([]string, len(filas[0]))
	for i, c := range filas[0] {
		columnas[i] = strings.TrimSpace(strings.TrimPrefix(c, bom))
	}
	// Las columnas sin nombre se CONSERVAN, en su posicion: quitarlas correria
	// las de detras (en medio) o haria pasar por fila justa una fila con un
	// campo de mas (al final: `titulo,id,taquilla,moneda` y una fila
	// `Rapido, furioso,55,100,` solo se ve ancha contra el ancho ORIGINAL).
	// Lo que traiga una fila bajo ellas lo rechaza [Mapa.Aplicar] con motivo.

	cuerpo := make([][]string, 0, len(filas)-1)
	lineas := make([]int, 0, len(filas)-1)
	var anchos []int
	if anotarAncho {
		anchos = make([]int, 0, len(filas)-1)
	}
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
		if anotarAncho {
			anchos = append(anchos, len(f))
		}
		// El numero que ve el cliente en su hoja. Se anota AQUI, que es el
		// unico sitio donde todavia se sabe de que linea salio: a partir de
		// este return la fila descartada ya no existe y la posicion en
		// `cuerpo` esta corrida. Y se anota en el MISMO bucle que descarta,
		// no despues: numerar aparte desalinearia las dos listas en cuanto
		// hubiera un registro de solo blancos, que el lector no descarta.
		lineas = append(lineas, fisicas[i+1])
	}
	t := Tabla{Columnas: columnas, Filas: cuerpo, Lineas: lineas, anchos: anchos}
	if anotarAncho {
		t.anchoEsperado, t.conEsperado, t.disputa = anchoEsperado(columnas, anchos)
	}
	return t, nil
}

// umbralMayoriaAncho es la fraccion de las filas de datos que tiene que
// reunir un ancho para imponerse a los demas cuando el archivo los mezcla.
//
// Es alto a proposito. Por el ancho solo, una fila con la coma final de mas
// (J1: una fila rara en un archivo sano) y una fila con la coma PERDIDA (K2:
// la mayoria perdio la coma) tienen la misma forma, y con mayoria simple el
// segundo caso dejaba entrar corridas las filas de la mayoria y rechazaba la
// buena. Ante la duda, ruido y no silencio: solo una mayoria abrumadora -- la
// de un archivo sano con alguna fila suelta, como la parrilla de Caracol con
// una fila de coma final, 58 de 59 -- se toma por la forma del archivo.
const umbralMayoriaAncho = 0.9

// anchoEsperado decide, para TODO el archivo, cuantos campos tiene que traer
// una fila, cuantas filas los traen, y si el archivo no se pudo decidir.
//
// Solo es dudoso cuando la cabecera termina en columnas sin nombre -- la coma
// final de `titulo,id,taquilla,` --, porque entonces hay mas de una forma
// legitima de escribir una fila: con esas comas o sin ellas. Los anchos
// legitimos van del de la ultima columna con nombre al ancho original de la
// cabecera. Se decide por archivo porque por fila no se puede: con la coma
// final en todas las filas, una coma PERDIDA deja la fila justo en el ancho de
// las columnas con nombre.
//
// Tres casos:
//
//   - Todas las filas en un solo ancho legitimo: ese es el ancho, y se aceptan.
//     Cubre las tres formas coherentes de exportar un archivo.
//   - Anchos legitimos mezclados y uno con al menos umbralMayoriaAncho de las
//     filas de datos: ese es el ancho, y se rechaza la minoria -- tambien la
//     que se pasa, porque una celda en blanco al final bajo la columna sin
//     nombre es indistinguible de una coma de mas --.
//   - Ninguno llega al umbral: se devuelven los anchos en disputa y se rechazan
//     TODAS sus filas. No hay forma de saber cual es la buena, y aceptar la
//     mayoria es como entraban corridas las filas de K2.
//
// Lo que NO puede ver, y conviene decirlo: un archivo en un solo ancho con una
// coma perdida en todas sus filas -- el caso extremo, una sola fila (K1) -- se
// lee como coherente y entra corrido. Y una fila que pierde un campo y gana
// otro (`Rapido, furioso,2` bajo `titulo,id,taquilla`) tiene el ancho de las
// buenas. Ninguna regla de ancho distingue esas filas de una bien escrita; en
// `main` tambien entran.
func anchoEsperado(columnas []string, anchos []int) (esperado, con int, disputa []anchoEnDisputa) {
	nombradas := len(columnas)
	for nombradas > 0 && columnas[nombradas-1] == "" {
		nombradas--
	}
	cuenta := map[int]int{}
	for _, a := range anchos {
		if a >= nombradas && a <= len(columnas) {
			cuenta[a]++
		}
	}
	// De menor a mayor con >=: en empate se queda el mayor. Sin ninguna fila
	// en el rango legitimo, el ancho original.
	esperado = len(columnas)
	for a := nombradas; a <= len(columnas); a++ {
		if cuenta[a] > 0 && cuenta[a] >= con {
			esperado, con = a, cuenta[a]
		}
	}
	if len(cuenta) <= 1 || float64(con)/float64(len(anchos)) >= umbralMayoriaAncho {
		return esperado, con, nil
	}
	for a := nombradas; a <= len(columnas); a++ {
		if cuenta[a] > 0 {
			disputa = append(disputa, anchoEnDisputa{ancho: a, filas: cuenta[a]})
		}
	}
	// Lo que queda por debajo de las columnas con nombre es corto igual, y
	// su motivo cuenta contra el ancho minimo legitimo, que es un hecho.
	return nombradas, cuenta[nombradas], disputa
}

func vacia(fila []string) bool {
	for _, c := range fila {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
