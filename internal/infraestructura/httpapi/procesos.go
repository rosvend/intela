package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Procesos es lo que la capa HTTP necesita del flujo de aprobaciones de
// RD 13.5, declarado aqui igual que [Recaudo] y [Declaraciones].
//
// aplicacion.Procesos la satisface sin nombrarla.
type Procesos interface {
	IniciarProceso(ctx context.Context, id, periodo string, circuito reparto.Circuito, bolsaID string) (aplicacion.ProcesoVista, error)
	AvanzarEtapa(ctx context.Context, id string) (aplicacion.ProcesoVista, error)
	Firmar(ctx context.Context, id string, rol reparto.RolAcompuerta, actorID string) (aplicacion.ProcesoVista, error)
	RechazarGate(ctx context.Context, id, motivo string) (aplicacion.ProcesoVista, error)
	ConsultarEstadoProceso(ctx context.Context, id string) (aplicacion.ProcesoVista, error)
	ListarProcesos(ctx context.Context) ([]aplicacion.ProcesoVista, error)
}

// ---------------------------------------------------------------------------
// Formas de red

// firmaJSON es una firma tal como viaja por la red. Sin campo de fecha: el
// agregado de dominio no la guarda (dominio no importa "time"), y `cuando`
// es procedencia de infraestructura, no algo que el cliente necesite para
// decidir si la compuerta esta completa.
type firmaJSON struct {
	Rol      string `json:"rol"`
	ActorID  string `json:"actor_id"`
	Revision int    `json:"revision"`
}

// procesoJSON es un ProcesoDeReparto tal como lo ve el panel de corridas.
type procesoJSON struct {
	ID            string      `json:"id"`
	Circuito      string      `json:"circuito"`
	Etapa         string      `json:"etapa"`
	Periodo       string      `json:"periodo"`
	BolsaID       string      `json:"bolsa_id"`
	SnapshotID    string      `json:"snapshot_id"`
	Reglamento    string      `json:"reglamento"`
	Revision      int         `json:"revision"`
	Firmas        []firmaJSON `json:"firmas"`
	RechazoMotivo string      `json:"rechazo_motivo,omitempty"`
}

func aProcesoJSON(p aplicacion.ProcesoVista) procesoJSON {
	firmas := make([]firmaJSON, 0, len(p.Firmas))
	for _, f := range p.Firmas {
		firmas = append(firmas, firmaJSON{Rol: f.Rol, ActorID: f.ActorID, Revision: f.SobreRev})
	}
	return procesoJSON{
		ID:            p.ID,
		Circuito:      string(p.Circuito),
		Etapa:         string(p.Etapa),
		Periodo:       p.Periodo,
		BolsaID:       p.BolsaID,
		SnapshotID:    p.SnapshotID,
		Reglamento:    p.Reglamento,
		Revision:      p.Revision,
		Firmas:        firmas,
		RechazoMotivo: p.RechazoMotivo,
	}
}

// abrirProcesoJSON es el cuerpo de POST /procesos. El id lo trae el cliente,
// igual que el de una bolsa: el trabajo de calendario lo genera por bolsa y
// corrida (ver aplicacion.Procesos.AbrirCorridaDelPeriodo), y una apertura
// manual necesita poder elegir el mismo esquema para seguir siendo
// idempotente.
type abrirProcesoJSON struct {
	ID       string `json:"id"`
	Periodo  string `json:"periodo"`
	Circuito string `json:"circuito"`
	BolsaID  string `json:"bolsa_id"`
}

// rechazarGateJSON es el cuerpo de POST /procesos/{id}/rechazar.
type rechazarGateJSON struct {
	Motivo string `json:"motivo"`
}

// ---------------------------------------------------------------------------
// Handlers

const maxCuerpoProceso = 16 << 10

// abrirProceso inicia una corrida (RD 13.5). Reservado a administrador -es
// quien opera el pipeline (roles.md)-, igual que /reportes y /obras.
func (a *API) abrirProceso(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoProceso)

	var cuerpo abrirProcesoJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con el proceso a abrir")
		return
	}

	p, err := a.procesos.IniciarProceso(r.Context(), cuerpo.ID, cuerpo.Periodo, reparto.Circuito(cuerpo.Circuito), cuerpo.BolsaID)
	if err := escribirErrorDeProceso(w, r, a.log, err, "abrir el proceso"); err != nil {
		return
	}

	w.Header().Set("Location", "/procesos/"+p.ID)
	escribirJSON(w, http.StatusCreated, aProcesoJSON(p))
}

// listarProcesos sirve el panel de corridas: etapa, circuito, periodo y
// firmas de cada proceso.
func (a *API) listarProcesos(w http.ResponseWriter, r *http.Request) {
	lista, err := a.procesos.ListarProcesos(r.Context())
	if err := escribirErrorDeProceso(w, r, a.log, err, "listar los procesos"); err != nil {
		return
	}
	cuerpo := make([]procesoJSON, 0, len(lista))
	for _, p := range lista {
		cuerpo = append(cuerpo, aProcesoJSON(p))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

// procesoPorID es la lectura de solo-consulta que la #34 pidio para el
// agente de staff (#69): en que etapa esta una corrida, sin escribir en ella.
func (a *API) procesoPorID(w http.ResponseWriter, r *http.Request) {
	p, err := a.procesos.ConsultarEstadoProceso(r.Context(), chi.URLParam(r, "id"))
	if err := escribirErrorDeProceso(w, r, a.log, err, "consultar el proceso"); err != nil {
		return
	}
	escribirJSON(w, http.StatusOK, aProcesoJSON(p))
}

// avanzarEtapaProceso mueve el proceso a la siguiente etapa de RD 13.5.
// Reservado a administrador, igual que abrirProceso.
func (a *API) avanzarEtapaProceso(w http.ResponseWriter, r *http.Request) {
	p, err := a.procesos.AvanzarEtapa(r.Context(), chi.URLParam(r, "id"))
	if err := escribirErrorDeProceso(w, r, a.log, err, "avanzar el proceso"); err != nil {
		return
	}
	escribirJSON(w, http.StatusOK, aProcesoJSON(p))
}

// firmarProceso agrega una firma a la compuerta actual.
//
// El rol y el actor salen de la SESION, nunca del cuerpo: si el cliente
// pudiera elegir el rol con el que firma, un actor de contabilidad podria
// firmar "como distribucion" y la doble firma de RD 13.5 dejaria de separar
// dos personas. La ruta ya exige distribucion o contabilidad
// (roles.md), asi que el rol de la sesion siempre es uno de los dos validos.
func (a *API) firmarProceso(w http.ResponseWriter, r *http.Request) {
	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	p, err := a.procesos.Firmar(r.Context(), chi.URLParam(r, "id"), reparto.RolAcompuerta(usuario.Rol), usuario.ID)
	if err := escribirErrorDeProceso(w, r, a.log, err, "firmar el proceso"); err != nil {
		return
	}
	escribirJSON(w, http.StatusOK, aProcesoJSON(p))
}

// rechazarGateProceso rechaza la compuerta actual: el proceso retrocede una
// etapa, no aborta (ADR 0008).
func (a *API) rechazarGateProceso(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoProceso)

	var cuerpo rechazarGateJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con el motivo del rechazo")
		return
	}

	p, err := a.procesos.RechazarGate(r.Context(), chi.URLParam(r, "id"), cuerpo.Motivo)
	if err := escribirErrorDeProceso(w, r, a.log, err, "rechazar la compuerta"); err != nil {
		return
	}
	escribirJSON(w, http.StatusOK, aProcesoJSON(p))
}

// escribirErrorDeProceso traduce un error de aplicacion.Procesos a su
// codigo HTTP y lo escribe. Devuelve el mismo error para que el handler
// pueda usarlo como guarda de retorno temprano; nil significa que no hubo
// error y el handler sigue.
func escribirErrorDeProceso(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error, accion string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "ese proceso no existe")
	case errors.Is(err, reparto.ErrRepartoInvalido):
		// 409 y no 400: el cuerpo de la peticion es correcto, lo que no
		// cuadra es el estado del proceso contra RD 13.5 -una compuerta sin
		// las dos firmas, una etapa que no es compuerta, un rol que ya firmo.
		escribirError(w, http.StatusConflict, err.Error())
	default:
		log.ErrorContext(r.Context(), "fallo al "+accion, slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo "+accion)
	}
	return err
}
