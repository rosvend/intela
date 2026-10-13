package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Agente es lo que HTTP necesita de AgenteConsulta.
type Agente interface {
	Responder(ctx context.Context, actor aplicacion.Usuario, historial []aplicacion.TurnoConversacion, mensaje string, emitir func(aplicacion.Evento)) error
}

var _ Agente = aplicacion.AgenteConsulta{}

// Cuerpo y ritmo maximos de /agente/consulta: cada pregunta cuesta hasta cinco llamadas al modelo.
const (
	maxCuerpoAgente       = 64 << 10
	limiteAgentePorMinuto = 20
)

type consultaAgenteJSON struct {
	Mensaje   string      `json:"mensaje"`
	Historial []turnoJSON `json:"historial"`
}

type turnoJSON struct {
	Rol   string `json:"rol"`
	Texto string `json:"texto"`
}

type eventoHerramientaJSON struct {
	Herramienta string `json:"herramienta"`
}

type eventoRespuestaJSON struct {
	Texto       string `json:"texto"`
	Parcial     bool   `json:"parcial"`
	Restringida bool   `json:"restringida"`
}

type eventoErrorJSON struct {
	Mensaje string `json:"mensaje"`
}

// consultarAgente valida con JSON y, desde el primer evento, transmite SSE; en Lambda el mismo cuerpo llega de una vez.
func (a *API) consultarAgente(w http.ResponseWriter, r *http.Request) {
	if a.agente == nil {
		escribirError(w, http.StatusServiceUnavailable, "el asistente no esta configurado en esta instalacion")
		return
	}
	usuario, ok := UsuarioDe(r.Context())
	if !ok {
		noAutenticado(w, "falta la sesion")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoAgente)
	var c consultaAgenteJSON
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		var demasiado *http.MaxBytesError
		if errors.As(err, &demasiado) {
			escribirError(w, http.StatusRequestEntityTooLarge, "la consulta es demasiado grande")
			return
		}
		escribirError(w, http.StatusBadRequest, "cuerpo invalido: se esperaba {mensaje, historial}")
		return
	}
	historial := make([]aplicacion.TurnoConversacion, 0, len(c.Historial))
	for _, t := range c.Historial {
		historial = append(historial, aplicacion.TurnoConversacion{Rol: aplicacion.RolMensaje(t.Rol), Texto: t.Texto})
	}

	sse := &escritorSSE{w: w, log: a.log}
	err := a.agente.Responder(r.Context(), usuario, historial, c.Mensaje, sse.emitir)
	if err == nil || sse.empezado {
		return
	}
	switch {
	case errors.Is(err, aplicacion.ErrConsultaInvalida):
		escribirError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, "no autorizado")
	default:
		a.log.ErrorContext(r.Context(), "agente: consulta fallida", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo atender la consulta")
	}
}

// escritorSSE fija las cabeceras con el primer evento y vacia el buffer si el transporte lo permite.
type escritorSSE struct {
	w        http.ResponseWriter
	log      *slog.Logger
	empezado bool
}

func (s *escritorSSE) emitir(e aplicacion.Evento) {
	if !s.empezado {
		h := s.w.Header()
		h.Set("Content-Type", "text/event-stream; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no")
		s.w.WriteHeader(http.StatusOK)
		s.empezado = true
	}
	nombre, datos := eventoSSE(e)
	cuerpo, _ := json.Marshal(datos)
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", nombre, cuerpo); err != nil {
		s.log.Warn("agente: no se pudo escribir el evento", slog.Any("error", err))
		return
	}
	_ = http.NewResponseController(s.w).Flush()
}

func eventoSSE(e aplicacion.Evento) (string, any) {
	switch e.Tipo {
	case aplicacion.EventoHerramienta:
		return "tool_call", eventoHerramientaJSON{Herramienta: e.Herramienta}
	case aplicacion.EventoRespuesta:
		return "answer", eventoRespuestaJSON{Texto: e.Texto, Parcial: e.Parcial, Restringida: e.Restringida}
	default:
		return "error", eventoErrorJSON{Mensaje: e.Texto}
	}
}
