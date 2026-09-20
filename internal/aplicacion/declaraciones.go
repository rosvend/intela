package aplicacion

import (
	"context"
	"fmt"
	"time"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Declaraciones son los casos de uso de la Declaracion de Obra: el ABM
// versionado que la #23 le debia a la #30.
//
// Un servicio con dos operaciones y no dos structs, por lo mismo que
// [Catalogo]: giran sobre el mismo agregado y comparten las mismas
// dependencias.
//
// No lleva un [BitacoraAuditoria] propio: el asiento de esta operacion es
// parte del mismo contrato atomico que [GestionDeclaraciones.Guardar], no un
// segundo puerto que este caso de uso orqueste por su cuenta -ver el
// comentario de ese puerto en puertos.go.
//
// Lleva [PadronTitulares] ademas de la gestion por lo mismo que
// [Titulares.Padron] es un puerto y no una consulta suelta: escribir una
// declaracion exige comprobar `R-01` (`RD 4.5`) sobre las partes que llegan, y
// esa comprobacion es E/S. Ver [Declaraciones.GuardarSplits].
type Declaraciones struct {
	Gestion GestionDeclaraciones
	Padron  PadronTitulares
	Reloj   Reloj
}

// GuardarSplits valida las partes que llegan, comprueba que todas puedan
// recibir reparto y las guarda como una version nueva de la declaracion de la
// obra.
//
// Es la misma operacion tanto si es la primera declaracion de la obra como si
// es una edicion: [GestionDeclaraciones.Guardar] decide por si sola si cierra
// una version anterior o abre la primera. Separarlo en dos metodos de caso de
// uso duplicaria esta funcion entera para una diferencia que ya resuelve el
// adaptador.
//
// # El orden de las tres cosas
//
// Primero [repertorio.NuevaDeclaracion], que es puro y local: unas partes que
// no forman una declaracion -la suma se pasa de 100, falta un IPI, un titular
// repetido- se rechazan sin gastar ni una consulta ni una escritura. Despues
// la comprobacion de `R-01`, que si es E/S porque hay que leer el padron, y
// por eso va detras de todo lo que se puede decidir sin salir de aqui.
//
// Y las dos ANTES de [GestionDeclaraciones.Guardar], que es la unica que
// escribe. No es preferencia de estilo: Guardar cierra la version abierta y
// abre la nueva en la misma transaccion, y una vez hecho no hay desde este
// nivel nada que deshacer -el puerto no expone un "deshaz esa version"-. Una
// comprobacion posterior dejaria guardada una declaracion con una parte que el
// reparto rechaza al pagar, que es exactamente el defecto que esto cierra.
func (d Declaraciones) GuardarSplits(ctx context.Context, obraID string, partes []repertorio.Parte, actorID string) (VersionDeclaracion, error) {
	decl, err := repertorio.NuevaDeclaracion(obraID, partes)
	if err != nil {
		return VersionDeclaracion{}, err
	}

	if err := d.exigirPuedenRecibirReparto(ctx, partes); err != nil {
		return VersionDeclaracion{}, err
	}

	version, vigenteDesde, err := d.Gestion.Guardar(ctx, decl, d.Reloj.Ahora(), actorID)
	if err != nil {
		return VersionDeclaracion{}, fmt.Errorf("guardar declaracion de la obra %q: %w", obraID, err)
	}

	return VersionDeclaracion{Version: version, VigenteDesde: vigenteDesde, Declaracion: decl}, nil
}

// exigirPuedenRecibirReparto es `R-01` (`RD 4.5`) de este lado: ninguna parte
// de una declaracion puede apuntar a un titular del padron que no sea persona
// natural. Devuelve [ErrTitularNoEsPersonaNatural] si alguna lo apunta.
//
// Y ademas concilia el IPI: la parte tiene que declarar el mismo que el padron
// tiene para ese titular, o sale [ErrIPIQueNoCuadra]. No es una regla distinta
// sino la misma pregunta con mas de una respuesta posible -el padron es lo que
// se sabe de un titular, y el IPI es parte de eso-, y por eso viaja en esta
// misma consulta acotada en vez de en una segunda.
//
// # Se pregunta por los titulares que la declaracion NOMBRA
//
// La consulta lleva los ids de las partes y ningun otro filtro: el padron
// devuelve esas filas -las personas naturales incluidas- y el veredicto sale de
// recorrerlas preguntando `PuedeRecibirReparto()`. Es la pregunta que este caso
// de uso tiene de verdad, y por eso esta acotada por el tamano de la
// declaracion y no por el del padron: leer el padron entero en cada guardado
// costaria lo que mida el padron -que no tiene tope- para decidir sobre un
// punado de ids, y con el padron real cargado eso seria cada guardado del
// editor.
//
// El recorte lo hace la consulta; el veredicto lo da la entidad. Preguntar por
// los ids y no por el filtro PersonaNatural es justo lo que deja el veredicto
// legible: lo que vuelve son las filas de verdad, no una lista ya recortada
// por una copia de la regla. La regla vive en un solo sitio -esa mitad de R-01
// que el dominio ya contesta-, y una condicion escrita dos veces es una
// condicion que algun dia discrepa: hoy `persona_natural` excluiria lo mismo
// que `PuedeRecibirReparto()` solo porque coinciden, y el dia que R-01 excluya
// a alguien que si es persona natural lo que cambia es la entidad, no esta
// consulta.
//
// # Ausencia no es rechazo
//
// Que un titular_id no venga en lo que devuelve el padron no prueba nada: la
// fila puede no existir. Aqui solo se rechaza lo que se puede afirmar -el
// titular esta en el padron y no puede recibir reparto-, y a un titular_id que
// no existe lo sigue delatando la clave foranea al escribir, que el adaptador
// de Postgres traduce a [ErrTitularInexistente]. Colapsarlos diria "no es
// persona natural" de un typo.
//
// Y un fallo al leer el padron sale de aqui envuelto para nombrar la
// operacion, SIN el centinela de la regla: no se puede rechazar una declaracion
// por una regla que no se llego a comprobar, y menos cuando lo que esta roto es
// la base y no el dato que mando quien edita.
func (d Declaraciones) exigirPuedenRecibirReparto(ctx context.Context, partes []repertorio.Parte) error {
	// Los ids van sin repetir: se pregunta por titular, no por parte, y dos
	// partes del mismo titular no son dos preguntas. Hoy
	// [repertorio.NuevaDeclaracion] ya rechaza un titular repetido antes de
	// llegar aqui, asi que esto es el contrato de esta funcion y no un caso que
	// se de en produccion: quien pregunte recibe cada id una vez.
	ids := make([]string, 0, len(partes))
	vistos := make(map[string]struct{}, len(partes))
	for _, p := range partes {
		if _, repetido := vistos[p.TitularID]; repetido {
			continue
		}
		vistos[p.TitularID] = struct{}{}
		ids = append(ids, p.TitularID)
	}

	// Y el tope es el numero de ids. Como la lista no trae repetidos y el id es
	// la clave del padron, esta consulta no puede devolver mas filas que ids se
	// pidieron; decirlo evita el tope por defecto, que recortaria la respuesta
	// de una declaracion con mas partes que el tope y dejaria titulares sin
	// comprobar en silencio, porque la lista que vuelve parece completa.
	titulares, err := d.Padron.BuscarTitulares(ctx, FiltroTitulares{
		IDs:        ids,
		Paginacion: Paginacion{Limite: len(ids)},
	})
	if err != nil {
		return fmt.Errorf("comprobar quien puede recibir reparto: %w", err)
	}

	// El IPI que la parte declara tiene que ser el que el padron tiene para ese
	// titular. El bucle de abajo ya trae las filas autoritativas -y por eso la
	// comparacion no cuesta una consulta mas-, pero hasta ahora nadie llamaba a
	// `t.IPI()`: el valor declarado llegaba intacto a `declaraciones.ipi` y de
	// ahi a `resultados_titular.ipi`, que es de donde sale a quien se le paga.
	// Un `tit-ana` con el IPI de otra persona se guardaba con 200.
	//
	// `hay` es lo que impide leer la ausencia como discrepancia: un titular_id
	// que no existe no vuelve del padron, y decir de el "el IPI no cuadra"
	// mandaria a corregir un numero cuando el error es el identificador. Ese
	// caso lo sigue delatando la clave foranea, con su propio centinela.
	porID := make(map[string]string, len(partes))
	for _, p := range partes {
		porID[p.TitularID] = p.IPI
	}

	for _, t := range titulares {
		if !t.PuedeRecibirReparto() {
			// El nombre de la fila del padron viaja en el error para que quede
			// en el log de quien opera. A quien edita le llega un mensaje fijo,
			// no este: ver el handler en httpapi/declaraciones.go.
			//
			// `R-01` va ANTES de la conciliacion del IPI, y no al reves: a un
			// titular que no puede recibir reparto no se le concilia el IPI
			// -muchas sociedades ni lo tienen, ver `sociedadDelPadron`-, y
			// decirle "el IPI no cuadra" a quien puso una productora en su
			// declaracion lo manda a corregir un numero cuando el problema es
			// que esa parte no puede cobrar nunca.
			return fmt.Errorf("la parte del titular %q (%s): %w",
				t.ID(), t.Nombre(), ErrTitularNoEsPersonaNatural)
		}
		if declarado, hay := porID[t.ID()]; hay && declarado != t.IPI() {
			return fmt.Errorf("la parte del titular %q declara el IPI %q y el padron tiene %q: %w",
				t.ID(), declarado, t.IPI(), ErrIPIQueNoCuadra)
		}
	}
	return nil
}

// Historial devuelve todas las versiones de la declaracion de una obra,
// ordenadas por version. Una obra sin ninguna declaracion aun devuelve una
// lista vacia, no un error: es el mismo criterio que [Catalogo.BuscarObras].
func (d Declaraciones) Historial(ctx context.Context, obraID string) ([]VersionDeclaracion, error) {
	historial, err := d.Gestion.Historial(ctx, obraID)
	if err != nil {
		return nil, fmt.Errorf("historial de declaraciones de la obra %q: %w", obraID, err)
	}
	return historial, nil
}

// VigenteEn resuelve que version de la declaracion regia en un instante dado.
// Es la pregunta que un reproceso necesita responder para reproducir un
// reparto pasado con el split que tenia entonces, no con el de hoy.
func (d Declaraciones) VigenteEn(ctx context.Context, obraID string, momento time.Time) (VersionDeclaracion, error) {
	vd, err := d.Gestion.VigenteEn(ctx, obraID, momento)
	if err != nil {
		return VersionDeclaracion{}, fmt.Errorf("declaracion vigente de la obra %q: %w", obraID, err)
	}
	return vd, nil
}

// AsientoDeclaracion es la forma del payload JSONB del asiento que
// [GestionDeclaraciones.Guardar] escribe. Exportada porque quien la
// serializa es el adaptador de infraestructura -es el unico que sabe en que
// transaccion asentar-, no este caso de uso; vive aqui y no como forma de red
// porque el asiento tampoco lo es: es el registro interno de la bitacora
// (ADR 0006), no una respuesta HTTP.
type AsientoDeclaracion struct {
	Version int                `json:"version"`
	Estado  string             `json:"estado"`
	Partes  []repertorio.Parte `json:"partes"`
}
