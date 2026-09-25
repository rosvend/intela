// Package ingesta adapta el puerto aplicacion.LectorReporte: convierte los
// bytes de una entrega en filas del esquema canonico.
//
// # La division en tres, que es todo el diseno
//
// Los bytes pasan por dos etapas independientes -- el formato y despues el mapa
// -- y solo la segunda sale al esquema canonico:
//
//   - El FORMATO (`tabla.go`) sabe abrir un .xlsx, un CSV o un JSON y no sabe
//     que es un `ID_Ficha`. Entrega una `Tabla`: cabecera y filas, todo texto.
//   - El MAPA (`mapa.go`) sabe que `ID_Ficha` es el identificador de obra de
//     Caracol y no sabe de donde salio la celda. Es DATO.
//   - El LECTOR (este fichero) los junta, y es lo unico que satisface el puerto.
//
// La consecuencia practica es la que pide el issue #25: una fuente nueva es una
// entrada en `fuentes.go`, y la MISMA entrada sirve para los tres formatos.
// Cuando Caracol mande su parrilla en CSV en vez de en Excel no hay que tocar
// una linea de codigo.
//
// # Lo que este paquete NO hace
//
//   - No decide a que obra corresponde una fila. Eso es la cascada del ADR
//     0007, y `validarUso` rechaza cualquier fila que llegue ya identificada.
//   - No normaliza formatos regionales de numero, moneda ni fecha: eso es el
//     issue #26. Aqui una celda que no se puede convertir cae al log de
//     rechazos NOMBRANDO el campo, que es lo que permite pedir el formato bueno.
//   - No agrupa emisiones por obra. La parrilla de Caracol tiene granularidad
//     de EMISION -- 29 `ID_Ficha` en 59 filas -- y cada emision es una fila de
//     `usos`; agrupar antes de valorizar es trabajo del motor de reparto.
//   - No lee importes. Ninguna fuente los trae, y una columna de dinero en un
//     reporte de uso casi seguro esta mal interpretada.
package ingesta

import (
	"fmt"
	"slices"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Lector es un adaptador de formato: el par (fuente, formato) del issue.
//
// Se construye con [NuevoLector] y no como literal para que ningun lector
// pueda existir con un mapa que no compila contra el esquema canonico. Ver
// [Mapa.Validar].
type Lector struct {
	mapa    Mapa
	formato string
}

// Compila-o-no: si Lector deja de satisfacer el puerto, falla aqui y no en el
// cableado de cmd/api.
var _ aplicacion.LectorReporte = (*Lector)(nil)

// NuevoLector ata un mapa a un formato.
func NuevoLector(m Mapa, formato string) (*Lector, error) {
	if err := m.Validar(); err != nil {
		return nil, err
	}
	switch formato {
	case aplicacion.FormatoXLSX, aplicacion.FormatoCSV, aplicacion.FormatoJSON:
	default:
		return nil, fmt.Errorf("%w: formato %q desconocido para la fuente %q; hay %s, %s y %s",
			aplicacion.ErrReporteInvalido, formato, m.Fuente,
			aplicacion.FormatoXLSX, aplicacion.FormatoCSV, aplicacion.FormatoJSON)
	}
	return &Lector{mapa: m, formato: formato}, nil
}

// Leer satisface [aplicacion.LectorReporte].
//
// Devuelve error solo ante un fallo ESTRUCTURAL -- el archivo no se abre, falta
// una columna requerida --, y en ese caso el caso de uso no persiste nada, ni
// siquiera la evidencia cruda. Las filas malas viajan en el resultado con su
// motivo puesto.
func (l *Lector) Leer(datos []byte) ([]aplicacion.UsoPersistido, error) {
	var (
		t   Tabla
		err error
	)
	switch l.formato {
	case aplicacion.FormatoXLSX:
		t, err = TablaXLSX(datos, l.mapa.Hoja)
	case aplicacion.FormatoCSV:
		t, err = TablaCSV(datos)
	case aplicacion.FormatoJSON:
		t, err = TablaJSON(datos)
	default:
		// Inalcanzable: NuevoLector es el unico constructor y ya lo rechazo.
		// Se deja porque un `switch` sin default sobre una cadena devolveria
		// una tabla vacia, y una entrega vacia se parece demasiado a una que
		// entro bien.
		return nil, fmt.Errorf("%w: formato %q desconocido", aplicacion.ErrReporteInvalido, l.formato)
	}
	if err != nil {
		return nil, err
	}
	return l.mapa.Aplicar(t)
}

// Fuente y Formato describen al lector. Los usa el cableado para componer su
// clave sin tener que repetirla al lado del mapa, que es donde se
// desincronizaria.
func (l *Lector) Fuente() string  { return l.mapa.Fuente }
func (l *Lector) Formato() string { return l.formato }

// Catalogo construye el juego completo de lectores: cada mapa en los tres
// formatos.
//
// Los tres, y no solo el que la fuente usa hoy. El mapa de columnas es el
// mismo -- un CSV de Caracol tiene las columnas de Caracol -- y registrar solo
// el .xlsx obligaria a tocar codigo el dia que el cliente mande el mismo
// contenido de otra forma, que es exactamente lo que el mapa declarativo existe
// para evitar. El coste es un puntero por formato.
//
// Devuelve el mapa que [aplicacion.Ingesta] espera en su campo Lectores, asi
// que el cableado de cmd/api es una asignacion.
func Catalogo(mapas ...Mapa) (map[aplicacion.ClaveLector]aplicacion.LectorReporte, error) {
	formatos := []string{aplicacion.FormatoXLSX, aplicacion.FormatoCSV, aplicacion.FormatoJSON}
	cat := make(map[aplicacion.ClaveLector]aplicacion.LectorReporte, len(mapas)*len(formatos))

	for _, m := range mapas {
		for _, f := range formatos {
			l, err := NuevoLector(m, f)
			if err != nil {
				return nil, err
			}
			clave := aplicacion.ClaveLector{Fuente: m.Fuente, Formato: f}
			// Dos mapas para la misma fuente es un defecto de configuracion, y
			// silencioso: ganaria el ultimo y el primero dejaria de aplicarse
			// sin que nada fallara. Las entregas seguirian entrando, con las
			// columnas del mapa equivocado.
			if _, repe := cat[clave]; repe {
				return nil, fmt.Errorf("%w: hay dos mapas para la fuente %q",
					aplicacion.ErrReporteInvalido, m.Fuente)
			}
			cat[clave] = l
		}
	}
	return cat, nil
}

// Fuentes describe el catalogo en texto ordenado, para arrancar y para
// diagnosticar.
func Fuentes(cat map[aplicacion.ClaveLector]aplicacion.LectorReporte) []string {
	out := make([]string, 0, len(cat))
	for c := range cat {
		out = append(out, c.Fuente+"/"+c.Formato)
	}
	slices.Sort(out)
	return out
}
