package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Catalogo es lo que la capa HTTP necesita del nucleo para servir el catalogo
// maestro, declarado aqui igual que [Autenticacion] y [Salud].
//
// Se declara en el consumidor: este paquete depende de cuatro metodos, no del
// tipo que los implementa, y las pruebas pasan un doble sin levantar nada.
// aplicacion.Catalogo la satisface sin nombrarla.
//
// Los metodos hablan en [repertorio.Metadatos] y [repertorio.Obra], que son
// tipos del nucleo sin etiquetas json ni nada de transporte. La forma que
// viaja por la red la decide este fichero.
type Catalogo interface {
	RegistrarObra(ctx context.Context, id string, m repertorio.Metadatos) (repertorio.Obra, error)
	ActualizarMetadatosObra(ctx context.Context, id string, m repertorio.Metadatos) (repertorio.Obra, error)
	ObraPorID(ctx context.Context, id string) (repertorio.Obra, error)
	BuscarObras(ctx context.Context, f aplicacion.FiltroObras) ([]repertorio.Obra, error)
}

// ---------------------------------------------------------------------------
// Formas de red
//
// Los modelos del nucleo no llevan etiquetas json a proposito. Estos structs
// son el contrato HTTP y son los que tienen que cuadrar con los schemas de
// api/openapi.yaml: renombrar un campo del nucleo no cambia lo que ve un
// cliente.

type coautorJSON struct {
	Nombre string `json:"nombre"`
	IPI    string `json:"ipi"`
	Rol    string `json:"rol"`
}

// metadatosJSON es el cuerpo de un PATCH: todo lo de una obra MENOS su
// identificador.
//
// Que el id no este aqui no es un olvido, es la garantia. El identificador
// viaja por la ruta, asi que no hay forma de mandar uno distinto ni por
// descuido ni a proposito.
type metadatosJSON struct {
	Titulo    string        `json:"titulo"`
	Genero    string        `json:"genero"`
	Anio      int           `json:"anio"`
	Tipo      string        `json:"tipo"`
	IDA       string        `json:"ida"`
	EIDR      string        `json:"eidr"`
	IMDB      string        `json:"imdb"`
	Coautores []coautorJSON `json:"coautores"`
}

// obraJSON es la obra entera: lo que devuelve una lectura y lo que recibe un
// alta.
type obraJSON struct {
	ID string `json:"id"`
	metadatosJSON
}

func (m metadatosJSON) aDominio() repertorio.Metadatos {
	coautores := make([]repertorio.Coautor, 0, len(m.Coautores))
	for _, c := range m.Coautores {
		coautores = append(coautores, repertorio.Coautor{
			Nombre: c.Nombre,
			IPI:    c.IPI,
			Rol:    repertorio.RolAutoral(c.Rol),
		})
	}
	return repertorio.Metadatos{
		Titulo:    m.Titulo,
		Genero:    m.Genero,
		Anio:      m.Anio,
		Tipo:      repertorio.TipoObra(m.Tipo),
		IDA:       m.IDA,
		EIDR:      m.EIDR,
		IMDB:      m.IMDB,
		Coautores: coautores,
	}
}

func aObraJSON(o repertorio.Obra) obraJSON {
	m := o.Metadatos()
	coautores := make([]coautorJSON, 0, len(m.Coautores))
	for _, c := range m.Coautores {
		coautores = append(coautores, coautorJSON{
			Nombre: c.Nombre, IPI: c.IPI, Rol: string(c.Rol),
		})
	}
	return obraJSON{
		ID: o.ID(),
		metadatosJSON: metadatosJSON{
			Titulo:    m.Titulo,
			Genero:    m.Genero,
			Anio:      m.Anio,
			Tipo:      string(m.Tipo),
			IDA:       m.IDA,
			EIDR:      m.EIDR,
			IMDB:      m.IMDB,
			Coautores: coautores,
		},
	}
}

// ---------------------------------------------------------------------------
// Handlers

// buscarObras sirve el catalogo, con o sin filtros.
//
// Sin ningun parametro devuelve el catalogo entero: "sin recorte" es un
// recorte mas y no merece una ruta aparte.
func (a *API) buscarObras(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filtro := aplicacion.FiltroObras{
		Titulo: q.Get("titulo"),
		Genero: q.Get("genero"),
		IPI:    q.Get("ipi"),
	}
	// El anio se rechaza si no es un numero, en vez de ignorarse. Un
	// ?anio=dosmil silenciosamente ignorado devuelve el catalogo entero y
	// quien pregunta lo lee como "no hay ninguna de ese anio".
	if bruto := q.Get("anio"); bruto != "" {
		anio, err := strconv.Atoi(bruto)
		if err != nil || anio <= 0 {
			escribirError(w, http.StatusBadRequest, "anio tiene que ser un entero positivo")
			return
		}
		filtro.Anio = anio
	}

	obras, err := a.catalogo.BuscarObras(r.Context(), filtro)
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al buscar obras", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar el catalogo")
		return
	}

	// make y no var: un catalogo sin coincidencias tiene que salir como [] y
	// no como null, o cualquier cliente que itere la respuesta revienta.
	cuerpo := make([]obraJSON, 0, len(obras))
	for _, o := range obras {
		cuerpo = append(cuerpo, aObraJSON(o))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

func (a *API) obraPorID(w http.ResponseWriter, r *http.Request) {
	obra, err := a.catalogo.ObraPorID(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "esa obra no esta en el catalogo")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al leer una obra", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar el catalogo")
		return
	}
	escribirJSON(w, http.StatusOK, aObraJSON(obra))
}

// registrarObra da de alta una obra.
//
// El identificador lo trae el cuerpo: es el numero de obra de REDES-SYS, que
// se asigna fuera de este sistema. Por eso el duplicado es 409 y no un id
// nuevo inventado en silencio.
func (a *API) registrarObra(w http.ResponseWriter, r *http.Request) {
	var cuerpo obraJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con la obra")
		return
	}

	obra, err := a.catalogo.RegistrarObra(r.Context(), cuerpo.ID, cuerpo.aDominio())
	switch {
	case err == nil:
	case errors.Is(err, repertorio.ErrObraInvalida):
		// 400 y no 422: los datos llegaron, no forman una obra, y el mensaje
		// del dominio dice cual falta. Es informacion util y no revela nada.
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrObraDuplicada):
		escribirError(w, http.StatusConflict, aplicacion.ErrObraDuplicada.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al registrar una obra", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo registrar la obra")
		return
	}

	w.Header().Set("Location", "/obras/"+obra.ID())
	escribirJSON(w, http.StatusCreated, aObraJSON(obra))
}

// actualizarObra corrige los metadatos. Nunca el identificador: no esta en el
// cuerpo que se decodifica.
//
// No crea la obra si no existe. Un PATCH que inserta convierte un id mal
// escrito en una obra fantasma del catalogo, y contra el catalogo resuelve
// todo el matching.
func (a *API) actualizarObra(w http.ResponseWriter, r *http.Request) {
	var cuerpo metadatosJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con los metadatos")
		return
	}

	obra, err := a.catalogo.ActualizarMetadatosObra(
		r.Context(), chi.URLParam(r, "id"), cuerpo.aDominio())
	switch {
	case err == nil:
	case errors.Is(err, repertorio.ErrObraInvalida):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "esa obra no esta en el catalogo")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al actualizar una obra", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo actualizar la obra")
		return
	}
	escribirJSON(w, http.StatusOK, aObraJSON(obra))
}
