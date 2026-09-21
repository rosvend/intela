package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

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
// Los metodos hablan en [repertorio.Metadatos] y devuelven
// [aplicacion.ObraDelCatalogo], que es la entidad con el estado de su
// declaracion ya compuesto. La forma que viaja por la red la decide este
// fichero: los cuatro devuelven la misma proyeccion para que las cuatro
// respuestas que llevan el schema `Obra` digan exactamente lo mismo.
//
// Las dos escrituras piden un actorID, igual que [Declaraciones.GuardarSplits]
// y por lo mismo: es quien FIRMA el asiento de bitacora (ADR 0006). Sale de la
// sesion y nunca del cuerpo -- un actor que llegue por la red es un actor que
// se puede falsificar --, y por eso las dos rutas van detras de conSesion.
type Catalogo interface {
	RegistrarObra(ctx context.Context, id string, m repertorio.Metadatos, actorID string) (aplicacion.ObraDelCatalogo, error)
	ActualizarMetadatosObra(ctx context.Context, id string, m repertorio.Metadatos, actorID string) (aplicacion.ObraDelCatalogo, error)
	ObraPorID(ctx context.Context, id string) (aplicacion.ObraDelCatalogo, error)
	BuscarObras(ctx context.Context, f aplicacion.FiltroObras) ([]aplicacion.ObraDelCatalogo, error)
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

// nuevaObraJSON es el cuerpo de un ALTA: identidad y metadatos, y nada mas.
//
// No es [obraJSON] aunque se le parezca -y por eso son dos tipos y no uno-:
// obraJSON lleva ademas los tres campos derivados de la Declaracion de Obra, y
// decodificar el alta en el aceptaria en silencio unos datos que el alta no
// mira. Un cliente que mandara `estado_declaracion: completa` se quedaria
// creyendo que puede declarar el estado a mano, y el estado lo calcula el
// backend a partir de las partes guardadas (`R-04`, `RD 13.1.3`). El schema
// del contrato es el mismo reparto: `NuevaObra` es lo que se puede MANDAR,
// `Obra` es lo que se recibe.
type nuevaObraJSON struct {
	ID string `json:"id"`
	metadatosJSON
}

// decimalComoNumeroJSON es un decimal que viaja como NUMERO JSON, alla donde
// el contrato dice `type: number`.
//
// Nombra la FORMA y no un campo porque son dos los campos del contrato con
// esta forma -`porcentaje` de una parte y `suma_porcentajes` de una obra-, y
// porque el motivo es el mismo en los dos: son cifras con las que el cliente
// hace ARITMETICA. El editor de splits suma los porcentajes de las partes, y
// sobre "60" esa suma concatena texto en vez de sumar -"60" y "40" dan
// "6040"-, que es un total equivocado y en silencio en un sistema que reparte
// dinero de terceros.
//
// # Por que no basta `decimal.Decimal` a secas
//
// Porque serializa ENTRE COMILLAS: su MarshalJSON consulta el interruptor
// global de la libreria (`MarshalJSONWithoutQuotes`), que viene en false. Ese
// interruptor no se toca desde aqui: es del proceso entero, y hay otras
// respuestas que el contrato declara como cadena -`bruto` de las bolsas, el
// dinero de terceros del ADR 0005-, asi que encenderlo cambiaria la forma de
// media API desde un sitio que no tiene nada que ver con esta.
//
// El valor lo escribe decimal.String(), que nunca usa notacion cientifica -lo
// que sale de ahi es un numero JSON valido- y no redondea: los decimales los
// decide quien lo muestra.
type decimalComoNumeroJSON decimal.Decimal

func (d decimalComoNumeroJSON) MarshalJSON() ([]byte, error) {
	return []byte(decimal.Decimal(d).String()), nil
}

// UnmarshalJSON delega en la libreria en vez de reimplementar el parseo: el
// decimal de verdad sabe leerlo -numero JSON y cadena entrecomillada-, y este
// tipo no tiene por que saber mas. Apretar el lado que ENTRA no es parte de
// este arreglo, y hacerlo rechazaria cuerpos con `"porcentaje":"60"` que hoy
// se guardan.
func (d *decimalComoNumeroJSON) UnmarshalJSON(b []byte) error {
	return (*decimal.Decimal)(d).UnmarshalJSON(b)
}

// obraJSON es la obra entera tal como la devuelve una lectura: lo que recibe
// un alta, mas lo que el sistema sabe de su declaracion.
//
// Los tres ultimos campos son de SOLO LECTURA: son la proyeccion de la
// version vigente de la declaracion, y no se aceptan por ningun cuerpo.
type obraJSON struct {
	ID string `json:"id"`
	metadatosJSON

	// EstadoDeclaracion es "completa" o "incompleta", y lo calcula el backend
	// ([repertorio.Declaracion.Estado]). Dos valores, no tres: `invalida` no
	// es un estado del modelo sino lo que el guardado RECHAZA -una suma por
	// encima de 100-, asi que no puede llegar aqui.
	EstadoDeclaracion string `json:"estado_declaracion"`

	// SumaPorcentajes es lo declarado, no lo repartido: por debajo de 100 la
	// obra queda retenida entera (`R-04`).
	SumaPorcentajes decimalComoNumeroJSON `json:"suma_porcentajes"`

	// VersionVigente es la version abierta de la declaracion, y null quiere
	// decir que la obra NO tiene ninguna. Existe porque
	// `estado_declaracion` no distingue "sin declaracion" de "declarada a
	// medias": las dos son `incompleta`. Sin este campo habria que pintar
	// "incompleta" sobre obras que nadie declaro, que es afirmar una
	// declaracion inexistente. No es un tercer estado: es el origen del
	// estado.
	VersionVigente *int `json:"version_vigente"`
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

func aObraJSON(oc aplicacion.ObraDelCatalogo) obraJSON {
	o := oc.Obra
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
		EstadoDeclaracion: oc.EstadoDecl,
		SumaPorcentajes:   decimalComoNumeroJSON(oc.SumaPorcentajes),
		VersionVigente:    oc.VersionVigente,
	}
}

// ---------------------------------------------------------------------------
// Handlers

// buscarObras sirve el catalogo, con o sin filtros, siempre paginado.
//
// Sin filtros de titulo/genero/IPI/anio devuelve la primera pagina: el tope
// evita servir el catalogo real de REDES SGC de un golpe (issue #90).
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

	pag, ok := leerPaginacion(w, q)
	if !ok {
		return
	}
	filtro.Paginacion = pag

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

// leerPaginacion interpreta limite y desplazamiento. Misma forma que
// ListarObras, y la comparten las CUATRO rutas que paginan -`GET /obras`,
// `GET /reportes/{id}/rechazos`, `GET /titulares` y
// `GET /obras/{id}/declaracion/historial`-: una sola forma de decir "limite"
// evita la traduccion que se desvia. Ausente = defecto; mal formado o fuera de
// rango = 400, nunca un recorte en silencio de lo que se pidio.
//
// Eran dos hasta que la #30 sirvio el padron por paginas, y cuatro cuando el
// historial de versiones dejo de tener un tope duro. Si aparece una quinta, se
// dice -el numero es la unica parte de este comentario que envejece solo-.
func leerPaginacion(w http.ResponseWriter, q url.Values) (aplicacion.Paginacion, bool) {
	p := aplicacion.Paginacion{}
	if bruto := q.Get("limite"); bruto != "" {
		n, err := strconv.Atoi(bruto)
		if err != nil || n <= 0 {
			escribirError(w, http.StatusBadRequest, "limite tiene que ser un entero positivo")
			return aplicacion.Paginacion{}, false
		}
		if n > aplicacion.LimiteObrasMaximo {
			escribirError(w, http.StatusBadRequest,
				"limite no puede ser mayor que "+strconv.Itoa(aplicacion.LimiteObrasMaximo))
			return aplicacion.Paginacion{}, false
		}
		p.Limite = n
	}
	if bruto := q.Get("desplazamiento"); bruto != "" {
		n, err := strconv.Atoi(bruto)
		if err != nil || n < 0 {
			escribirError(w, http.StatusBadRequest, "desplazamiento tiene que ser un entero no negativo")
			return aplicacion.Paginacion{}, false
		}
		p.Desplazamiento = n
	}
	return p.ConDefecto(), true
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

// maxCuerpoObra acota el JSON del cuerpo antes de decodificarlo: sin limite,
// un array arbitrariamente grande de coautores se asigna entero en memoria
// antes de que repertorio.NuevaObra tenga oportunidad de rechazarlo, y desde
// #91 cada PATCH escribe esos coautores en `asientos.payload` -- una tabla
// append-only, indexada por GIN entera y conservada diez anos (ADR 0006).
// Mismo tope que maxCuerpoDeclaracion, en declaraciones.go: ninguna obra real
// tiene miles de coautores.
const maxCuerpoObra = 1 << 20 // 1 MiB

// registrarObra da de alta una obra.
//
// El identificador lo trae el cuerpo: es el numero de obra de REDES-SYS, que
// se asigna fuera de este sistema. Por eso el duplicado es 409 y no un id
// nuevo inventado en silencio.
//
// El cuerpo se decodifica en [nuevaObraJSON] y no en [obraJSON]: el estado de
// la declaracion que la respuesta lleva no entra por aqui.
func (a *API) registrarObra(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoObra)

	var cuerpo nuevaObraJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con la obra")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		// Inalcanzable detras de conSesion, igual que en guardarDeclaracion.
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	obra, err := a.catalogo.RegistrarObra(r.Context(), cuerpo.ID, cuerpo.aDominio(), usuario.ID)
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
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoObra)

	var cuerpo metadatosJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con los metadatos")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	obra, err := a.catalogo.ActualizarMetadatosObra(
		r.Context(), chi.URLParam(r, "id"), cuerpo.aDominio(), usuario.ID)
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
