package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// Anomalias es lo que la capa HTTP necesita del nucleo para la bandeja de
// alertas (#37), declarado aqui igual que [Catalogo] y [Recaudo].
//
// aplicacion.Anomalias la satisface sin nombrarla, y las pruebas pasan un
// doble sin levantar Postgres.
type Anomalias interface {
	Evaluar(ctx context.Context, periodo, actorID string) (aplicacion.ResumenEvaluacion, error)
	Listar(ctx context.Context, f aplicacion.FiltroAlertas) ([]aplicacion.Alerta, error)
	Resolver(ctx context.Context, id, actorID, nota string) (aplicacion.Alerta, error)
}

// ---------------------------------------------------------------------------
// Formas de red

// alertaJSON es una alerta tal como viaja por la red.
//
// Los seis primeros campos son el contrato que el tablero de la #104 ya fijo
// (`web/src/reparto/tipos.ts`, `type Alerta`): id, tipo, detalle, periodo,
// referencia y resuelta. Los demas son aditivos -- TypeScript los ignora si no
// los declara -- y estan porque el tablero solo PINTA, y quien tenga que
// RESOLVER necesita mas.
//
// # Por que viajan `referencia` y ademas `ref_tipo` / `ref_id`
//
// Parece redundante y no lo es: responden a dos consumidores. `referencia` es
// la cadena que el tablero mete en una celda de tabla -- el tablero no sabe
// componerla y un `ref_id` suelto no dice de que es --. `ref_tipo` y `ref_id`
// son el par legible por maquina que la bandeja de resolucion de #39 necesita
// para llevar a quien resuelve al registro ofensor: con la cadena compuesta
// tendria que partirla por el separador, que es exactamente la clase de
// re-derivacion que el `codigo` tipado de la cola de revision ya descarto.
//
// No hay campo de dinero, y por la misma razon que no lo tiene un uso: una
// alerta dice que una fila no esta en condiciones de ponderar, nunca cuanto
// vale.
type alertaJSON struct {
	ID          string     `json:"id"`
	Tipo        string     `json:"tipo"`
	Detalle     string     `json:"detalle"`
	Periodo     string     `json:"periodo"`
	Referencia  string     `json:"referencia"`
	RefTipo     string     `json:"ref_tipo"`
	RefID       string     `json:"ref_id"`
	RefTitular  string     `json:"ref_titular,omitempty"`
	Critica     bool       `json:"critica"`
	Detectada   time.Time  `json:"detectada"`
	Resuelta    bool       `json:"resuelta"`
	ResueltaPor string     `json:"resuelta_por,omitempty"`
	ResueltaEn  *time.Time `json:"resuelta_en,omitempty"`
	Nota        string     `json:"nota,omitempty"`
}

// resumenEvaluacionJSON es lo que devuelve una pasada de deteccion.
//
// `usos_sin_cotejar` no cuenta anomalias: cuenta las filas a las que no se les
// pudo componer clave de registro, asi que el detector de duplicados no las
// comparo con ninguna. Viaja porque sin ella "cero duplicados" y "no se miro"
// se leen igual en el tablero.
type resumenEvaluacionJSON struct {
	Periodo          string         `json:"periodo"`
	Detectadas       int            `json:"detectadas"`
	Nuevas           int            `json:"nuevas"`
	PorTipo          map[string]int `json:"por_tipo"`
	CriticasAbiertas int            `json:"criticas_abiertas"`
	UsosSinCotejar   int            `json:"usos_sin_cotejar"`
}

// evaluacionJSON es el cuerpo de la pasada. Un objeto y no un `?periodo=` en
// la ruta porque esto ESCRIBE: un POST cuyo unico argumento viaja en la query
// se copia y se repite desde la barra del navegador con demasiada facilidad.
type evaluacionJSON struct {
	Periodo string `json:"periodo"`
}

// resolucionJSON es el cuerpo de una resolucion. La nota es opcional: el
// hecho obligatorio es QUIEN resolvio, y ese sale de la sesion, no del cuerpo
// -- dejarlo llegar por JSON permitiria firmar a nombre de otro.
type resolucionJSON struct {
	Nota string `json:"nota"`
}

// separadorReferencia une ref_tipo y ref_id en la cadena que se pinta.
//
// `:` y no `/`: la referencia acaba en una celda de tabla y tambien en un
// mensaje de log, y una barra invita a leerla como una ruta que no existe.
const separadorReferencia = ":"

// separadorRefTitular cuelga la segunda coordenada de la primera, con la misma
// forma que un fragmento: `obra:obra-1#ipi-2` se lee como "dentro de la obra
// obra-1, el titular ipi-2".
const separadorRefTitular = "#"

func aAlertaJSON(a aplicacion.Alerta) alertaJSON {
	referencia := a.RefTipo + separadorReferencia + a.RefID
	if a.RefTitular != "" {
		referencia += separadorRefTitular + a.RefTitular
	}
	return alertaJSON{
		ID:          a.ID,
		Tipo:        a.Tipo,
		Detalle:     a.Detalle,
		Periodo:     a.Periodo,
		Referencia:  referencia,
		RefTipo:     a.RefTipo,
		RefID:       a.RefID,
		RefTitular:  a.RefTitular,
		Critica:     a.Critica,
		Detectada:   a.Detectada,
		Resuelta:    a.Resuelta,
		ResueltaPor: a.ResueltaPor,
		ResueltaEn:  a.ResueltaEn,
		Nota:        a.Nota,
	}
}

// ---------------------------------------------------------------------------
// Handlers

// maxCuerpoAlertas acota el cuerpo. Los dos POST de este fichero son objetos
// de tamano fijo; el limite esta para que un cuerpo enorme no se lea entero
// antes de rechazarlo.
const maxCuerpoAlertas = 64 << 10

// conAnomalias responde 503 si el binario no cableo el caso de uso, igual que
// [API.conIngesta] y por lo mismo: sin esto la ruta existe, el handler llama a
// una interfaz nil y el Recoverer lo convierte en un 500 sin cuerpo, que se
// lee como "el servidor esta roto" cuando lo que pasa es que a ESA instalacion
// le falta la pieza.
func (a *API) conAnomalias(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.anomalias == nil {
			escribirError(w, http.StatusServiceUnavailable,
				"la deteccion de anomalias no esta configurada en esta instalacion")
			return
		}
		h(w, r)
	}
}

// listarAlertas sirve la bandeja de anomalias.
//
// Los tres filtros son opcionales y ninguno se ignora cuando viene mal: un
// `?periodo=2025-13` o un `?tipo=onni` salen con 400, por el mismo criterio
// que `GET /bolsas`. Ignorados devolverian la lista de TODO -- o la lista
// vacia -- y quien pregunta lo leeria como "ese periodo no tuvo anomalias".
func (a *API) listarAlertas(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filtro := aplicacion.FiltroAlertas{
		Periodo: q.Get("periodo"),
		Tipo:    q.Get("tipo"),
	}
	if crudo := q.Get("resueltas"); crudo != "" {
		valor, err := strconv.ParseBool(crudo)
		if err != nil {
			escribirError(w, http.StatusBadRequest, "resueltas tiene que ser true o false")
			return
		}
		filtro.Resueltas = &valor
	}

	// El mismo `leerPaginacion` que `/obras`, `/titulares` y el historial de
	// declaraciones, no una copia: los cuatro listados aceptan los mismos dos
	// parametros y rechazan lo mismo con el mismo 400.
	paginacion, ok := leerPaginacion(w, q)
	if !ok {
		return
	}
	filtro.Paginacion = paginacion

	alertas, err := a.anomalias.Listar(r.Context(), filtro)
	switch {
	case err == nil:
	case errors.Is(err, recaudo.ErrBolsaInvalida):
		// El centinela es el de recaudo porque el validador de periodo es el
		// suyo: `recaudo.ValidarPeriodo` es UNA regla y tener una copia aqui
		// es como `?periodo=2025-13` acababa devolviendo 200 en #27.
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrFiltroInvalido):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al listar alertas", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudieron consultar las alertas")
		return
	}

	// make y no var: sin coincidencias tiene que salir [] y no null, o el
	// tablero que itera la respuesta revienta.
	cuerpo := make([]alertaJSON, 0, len(alertas))
	for _, al := range alertas {
		cuerpo = append(cuerpo, aAlertaJSON(al))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

// evaluarAnomalias corre los seis detectores sobre un periodo.
//
// # Por que existe esta ruta
//
// Porque sin ella nada puebla `alertas` y el criterio "las alertas de un
// periodo se listan centralmente antes de que corra la distribucion" no se
// puede cumplir: `GET /alertas` devolveria siempre la lista vacia.
//
// No es el cableado de la compuerta de #34, que es otra cosa y no existe
// todavia (`/admin/pipeline` es un stub). Es el disparador MANUAL mientras esa
// compuerta llega, y por eso escribe quien opera el periodo. Ver ADR 0021.
//
// # Por que no se evalua dentro del GET
//
// Porque un GET no escribe. Un listado que dispara una pasada de deteccion
// deja asientos de bitacora cada vez que alguien refresca el tablero -- el
// panel de #104 sondea cada 15 segundos -- y ademas convierte una lectura
// concurrente en una escritura concurrente.
//
// Responde 200 y no 201: la pasada no crea UN recurso con una URL, crea N
// alertas o ninguna, y lo que devuelve es el recuento.
func (a *API) evaluarAnomalias(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoAlertas)

	var cuerpo evaluacionJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con el periodo")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	resumen, err := a.anomalias.Evaluar(r.Context(), cuerpo.Periodo, usuario.ID)
	switch {
	case err == nil:
	case errors.Is(err, recaudo.ErrBolsaInvalida):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al evaluar anomalias", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo evaluar el periodo")
		return
	}

	escribirJSON(w, http.StatusOK, resumenEvaluacionJSON{
		Periodo:          resumen.Periodo,
		Detectadas:       resumen.Detectadas,
		Nuevas:           resumen.Nuevas,
		PorTipo:          resumen.PorTipo,
		CriticasAbiertas: resumen.CriticasAbiertas,
		UsosSinCotejar:   resumen.UsosSinCotejar,
	})
}

// resolverAlerta marca una alerta como atendida a nombre de quien la atiende.
//
// El actor sale de la SESION y no del cuerpo: dejarlo llegar por JSON
// permitiria firmar la decision a nombre de otro, y el asiento del ADR 0006
// tiene que nombrar a quien la tomo de verdad.
//
// El cuerpo es opcional -- la nota lo es -- asi que un POST sin cuerpo se
// acepta: `io.EOF` al decodificar no es un cuerpo mal formado, es la ausencia
// de cuerpo, y rechazarla obligaria a mandar `{}` para no decir nada.
func (a *API) resolverAlerta(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoAlertas)

	var cuerpo resolucionJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil && !errors.Is(err, io.EOF) {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con la nota")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	alerta, err := a.anomalias.Resolver(r.Context(), chi.URLParam(r, "id"), usuario.ID, cuerpo.Nota)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "esa alerta no existe")
		return
	case errors.Is(err, aplicacion.ErrAlertaYaResuelta):
		// 409 y no 200: quien pulso el boton el segundo tiene que saber que la
		// firma que quedo escrita no es la suya.
		escribirError(w, http.StatusConflict, aplicacion.ErrAlertaYaResuelta.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al resolver una alerta", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo resolver la alerta")
		return
	}

	escribirJSON(w, http.StatusOK, aAlertaJSON(alerta))
}
