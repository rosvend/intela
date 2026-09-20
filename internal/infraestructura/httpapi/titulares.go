package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// Padron es lo que la capa HTTP necesita del nucleo para servir el padron de
// titulares, declarado aqui igual que [Catalogo] y [Autenticacion].
//
// Se declara en el consumidor: este paquete depende de un metodo, no del tipo
// que lo implementa, y las pruebas pasan un doble sin levantar nada.
// aplicacion.Titulares la satisface sin nombrarla.
//
// Habla en [afiliacion.Titular], que es un tipo del nucleo sin etiquetas json:
// la forma que viaja por la red la decide este fichero.
type Padron interface {
	BuscarTitulares(ctx context.Context, f aplicacion.FiltroTitulares) ([]afiliacion.Titular, error)
}

// ---------------------------------------------------------------------------
// Forma de red

// titularJSON es el contrato HTTP de una entrada del padron y el que tiene que
// cuadrar con el schema Titular de api/openapi.yaml.
//
// No lleva email, y no es un olvido: el padron se sirve para armar un reparto,
// y un listado de titulares no tiene por que sacar de la base la direccion de
// contacto de cada uno. Entra el dia que haya una pantalla que lo muestre.
//
// persona_natural SI va, aunque R-01 solo deje cobrar a quien la tenga cierta.
// Recortar aqui las personas juridicas dejaria a quien edita sin poder explicar
// por que el titular que busca en el padron no aparece entre los elegibles: la
// regla quedaria invisible y pareceria un dato que falta.
type titularJSON struct {
	ID             string `json:"id"`
	Nombre         string `json:"nombre"`
	IPI            string `json:"ipi"`
	PersonaNatural bool   `json:"persona_natural"`
	Clase          string `json:"clase"`
}

func aTitularJSON(tit afiliacion.Titular) titularJSON {
	return titularJSON{
		ID:             tit.ID(),
		Nombre:         tit.Nombre(),
		IPI:            tit.IPI(),
		PersonaNatural: tit.PersonaNatural(),
		Clase:          string(tit.Clase()),
	}
}

// ---------------------------------------------------------------------------
// Handler

// buscarTitulares sirve el padron, con o sin filtros, siempre paginado.
//
// Sin filtros devuelve la primera pagina: el tope evita servir el padron real
// de REDES SGC de un golpe, igual que en el catalogo de obras.
func (a *API) buscarTitulares(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filtro := aplicacion.FiltroTitulares{
		Nombre: q.Get("nombre"),
		IPI:    q.Get("ipi"),
	}
	// persona_natural se rechaza si no es un booleano, en vez de ignorarse.
	// Ignorado, quien pregunta por las personas juridicas -la pregunta que
	// hace R-01: quien esta en el padron y no puede cobrar- recibiria el
	// padron entero y leeria a las personas naturales como si fueran lo que
	// pidio.
	//
	// Solo `true` y `false`, que es lo unico que el contrato declara y lo que
	// serializa un booleano de verdad. Aceptar ademas "1" o "TRUE" seria
	// aceptar en silencio valores que el schema no describe.
	if bruto := q.Get("persona_natural"); bruto != "" {
		switch bruto {
		case "true":
			si := true
			filtro.PersonaNatural = &si
		case "false":
			no := false
			filtro.PersonaNatural = &no
		default:
			escribirError(w, http.StatusBadRequest, "persona_natural tiene que ser true o false")
			return
		}
	}
	// `ids` es la consulta ACOTADA por identificador: los pocos titulares que
	// nombra una declaracion y ninguno mas. Es la misma forma que usa
	// `exigirPuedenRecibirReparto` para comprobar R-01 en cada guardado, que
	// hasta ahora era el unico consumidor de `FiltroTitulares.IDs`.
	//
	// Se leen las DOS formas del parametro porque son la misma pregunta y
	// ninguna es "la rara": repetir la clave (`?ids=a&ids=b`) es lo que manda un
	// cliente generado a partir del contrato, y la coma (`?ids=a,b`) es lo que
	// escribe una mano. Se leen juntas y no como dos caminos -si una de las dos
	// se quedara sin leer, el filtro se perderia EN SILENCIO y quien pinta no
	// tendria sintoma: le llegaria el padron entero y elegiria las filas que ya
	// tiene por su cuenta, que es el defecto que este parametro existe para no
	// repetir-.
	//
	// Un elemento vacio NO viaja: una coma de mas (`?ids=a,,b`) dejaria un
	// identificador vacio en la lista, y `id = ANY('{""}')` no encuentra nada,
	// asi que pasarlo convertiria una lista de dos ids utiles en una que no
	// devuelve ninguno.
	//
	// Y una lista que se queda VACIA -`?ids=`, solo comas, solo espacios- NO se
	// asigna: es la misma pregunta que no mandar el parametro, y el puerto lo
	// documenta asi ("un IDs vacio y uno nil son la misma pregunta: sin filtro").
	// Traducirla a "sin resultados" seria romper ese contrato desde el
	// adaptador: el padron no se filtra por lo que nadie pidio.
	ids := make([]string, 0, len(q["ids"]))
	for _, bruto := range q["ids"] {
		for _, id := range strings.Split(bruto, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) > 0 {
		filtro.IDs = ids
	}

	pag, ok := leerPaginacion(w, q)
	if !ok {
		return
	}
	filtro.Paginacion = pag

	titulares, err := a.padron.BuscarTitulares(r.Context(), filtro)
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al buscar titulares", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar el padron")
		return
	}

	// make y no var: un padron sin coincidencias tiene que salir como [] y no
	// como null, o cualquier cliente que itere la respuesta revienta.
	cuerpo := make([]titularJSON, 0, len(titulares))
	for _, tit := range titulares {
		cuerpo = append(cuerpo, aTitularJSON(tit))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}
