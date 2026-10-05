package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

// Limites del agente: turnos de modelo por pregunta, historial y tamano del mensaje.
const (
	MaxTurnosAgente       = 5
	MaxHistorialAgente    = 20
	MaxRunasMensajeAgente = 4000
	esperaReintentoModelo = 500 * time.Millisecond
)

// Textos fijos que el bucle impone sin pasar por el modelo.
const (
	TextoConsultaRestringida   = "Esa consulta está fuera de lo que tu rol puede ver, así que no la puedo responder. Si crees que deberías tener acceso, habla con un administrador de REDES SGC."
	AvisoRespuestaParcial      = "No pude terminar de verificar esto en los pasos permitidos: tómalo como una respuesta parcial, no confirmada."
	TextoAsistenteNoDisponible = "El asistente no está disponible en este momento. Intenta de nuevo en unos minutos."
)

// ErrConsultaInvalida: el mensaje o el historial no cumplen los limites del agente.
var ErrConsultaInvalida = errors.New("consulta invalida")

// RolMensaje distingue los turnos de quien pregunta de los del asistente.
type RolMensaje string

const (
	RolMensajeUsuario   RolMensaje = "usuario"
	RolMensajeAsistente RolMensaje = "asistente"
)

// TurnoConversacion es un turno previo que el navegador reenvia; no hay almacen en el servidor.
type TurnoConversacion struct {
	Rol   RolMensaje
	Texto string
}

// LlamadaHerramienta es una invocacion que pide el modelo.
type LlamadaHerramienta struct {
	ID         string
	Nombre     string
	Argumentos json.RawMessage
}

// ResultadoHerramienta vuelve al modelo ya envuelto en el sobre de cuarentena.
type ResultadoHerramienta struct {
	LlamadaID string
	Contenido string
	EsError   bool
}

// MensajeModelo es un turno de la conversacion tal como lo ve el modelo.
type MensajeModelo struct {
	Rol        RolMensaje
	Texto      string
	Llamadas   []LlamadaHerramienta
	Resultados []ResultadoHerramienta
}

// EsquemaHerramienta es lo que el modelo lee de cada herramienta.
type EsquemaHerramienta struct {
	Nombre      string
	Descripcion string
	Parametros  json.RawMessage
}

// PeticionModelo: Sistema es el prefijo estatico (cacheable) y Contexto lo que cambia por usuario.
type PeticionModelo struct {
	Sistema      string
	Contexto     string
	Herramientas []EsquemaHerramienta
	Mensajes     []MensajeModelo
}

// RespuestaModelo es texto final o llamadas a herramienta (o ambas).
type RespuestaModelo struct {
	Texto    string
	Llamadas []LlamadaHerramienta
}

// ModeloLenguaje es el puerto neutral al proveedor: mensajes y esquemas entran, respuesta o llamadas salen.
type ModeloLenguaje interface {
	Responder(ctx context.Context, p PeticionModelo) (RespuestaModelo, error)
}

// TipoEvento es lo que el bucle emite mientras resuelve una pregunta.
type TipoEvento string

const (
	EventoHerramienta TipoEvento = "herramienta"
	EventoRespuesta   TipoEvento = "respuesta"
	EventoFallo       TipoEvento = "fallo"
)

// Evento sale por un callback para que el transporte (SSE, MCP) sea un adaptador.
type Evento struct {
	Tipo        TipoEvento
	Herramienta string
	Texto       string
	Parcial     bool
	Restringida bool
}

// AgenteConsulta es el bucle razonar-actuar-observar de solo lectura (RD 13.5: nunca firma ni reparte).
type AgenteConsulta struct {
	Modelo       ModeloLenguaje
	Herramientas CatalogoHerramientas
	Reloj        Reloj
	Log          *slog.Logger
	// Esperar es el backoff del reintento; inyectable para que las pruebas no duerman.
	Esperar func(ctx context.Context, d time.Duration) error
	// Plazo acota la pregunta entera (0 = sin plazo propio); en Lambda tiene que caber en su timeout.
	Plazo time.Duration
}

// Responder resuelve una pregunta y emite eventos; solo devuelve error si la consulta es invalida (antes de emitir nada).
func (a AgenteConsulta) Responder(ctx context.Context, actor Usuario, historial []TurnoConversacion, mensaje string, emitir func(Evento)) error {
	if actor.ID == "" || actor.Rol == "" {
		return ErrNoAutorizado
	}
	mensajes, err := mensajesIniciales(historial, mensaje)
	if err != nil {
		return err
	}
	if a.Plazo > 0 {
		var cancelar context.CancelFunc
		ctx, cancelar = context.WithTimeout(ctx, a.Plazo)
		defer cancelar()
	}

	peticion := PeticionModelo{
		Sistema:      SistemaAgente,
		Contexto:     contextoDeActor(actor),
		Herramientas: a.Herramientas.Esquemas(),
		Mensajes:     mensajes,
	}
	ultimoTexto := ""
	for turno := 1; turno <= MaxTurnosAgente; turno++ {
		resp, err := a.llamarModelo(ctx, peticion)
		if err != nil {
			a.log().ErrorContext(ctx, "agente: el modelo no respondio", slog.Int("turno", turno), slog.String("actor", actor.ID), slog.Any("error", err))
			emitir(Evento{Tipo: EventoFallo, Texto: TextoAsistenteNoDisponible})
			return nil
		}
		if strings.TrimSpace(resp.Texto) != "" {
			ultimoTexto = resp.Texto
		}
		if len(resp.Llamadas) == 0 {
			emitir(Evento{Tipo: EventoRespuesta, Texto: resp.Texto})
			return nil
		}
		peticion.Mensajes = append(peticion.Mensajes, MensajeModelo{Rol: RolMensajeAsistente, Texto: resp.Texto, Llamadas: resp.Llamadas})
		resultados := make([]ResultadoHerramienta, 0, len(resp.Llamadas))
		for _, ll := range resp.Llamadas {
			emitir(Evento{Tipo: EventoHerramienta, Herramienta: ll.Nombre})
			res, denegada := a.ejecutar(ctx, turno, actor, ll)
			if denegada {
				emitir(Evento{Tipo: EventoRespuesta, Texto: TextoConsultaRestringida, Restringida: true})
				return nil
			}
			resultados = append(resultados, res)
		}
		peticion.Mensajes = append(peticion.Mensajes, MensajeModelo{Rol: RolMensajeUsuario, Resultados: resultados})
	}

	texto := AvisoRespuestaParcial
	if ultimoTexto != "" {
		texto = ultimoTexto + "\n\n" + AvisoRespuestaParcial
	}
	emitir(Evento{Tipo: EventoRespuesta, Texto: texto, Parcial: true})
	return nil
}

// ejecutar corre una llamada; el bool dice que fue denegada y el bucle tiene que cortar sin volver al modelo.
func (a AgenteConsulta) ejecutar(ctx context.Context, turno int, actor Usuario, ll LlamadaHerramienta) (ResultadoHerramienta, bool) {
	inicio := a.Reloj.Ahora()
	datos, err := a.Herramientas.Ejecutar(ctx, actor, ll.Nombre, ll.Argumentos)
	resultado := "ok"
	switch {
	case errors.Is(err, ErrNoAutorizado):
		resultado = "no_autorizado"
	case err != nil:
		resultado = "error"
	}
	a.log().InfoContext(ctx, "agente: herramienta",
		slog.Int("turno", turno),
		slog.String("herramienta", ll.Nombre),
		slog.Int("argumentos_bytes", len(ll.Argumentos)),
		slog.String("actor", actor.ID),
		slog.String("rol", string(actor.Rol)),
		slog.Int64("latencia_ms", a.Reloj.Ahora().Sub(inicio).Milliseconds()),
		slog.String("resultado", resultado),
		slog.Any("error", err),
	)
	if resultado == "no_autorizado" {
		return ResultadoHerramienta{}, true
	}

	nombre := ll.Nombre
	if errors.Is(err, ErrHerramientaDesconocida) {
		nombre = "desconocida"
	}
	if err != nil {
		return ResultadoHerramienta{LlamadaID: ll.ID, Contenido: cuarentena(nombre, map[string]string{"error": mensajeParaModelo(err)}), EsError: true}, false
	}
	return ResultadoHerramienta{LlamadaID: ll.ID, Contenido: cuarentena(nombre, datos)}, false
}

// llamarModelo hace un intento y, si falla, un reintento tras el backoff.
func (a AgenteConsulta) llamarModelo(ctx context.Context, p PeticionModelo) (RespuestaModelo, error) {
	resp, err := a.Modelo.Responder(ctx, p)
	if err == nil {
		return resp, nil
	}
	if errEspera := a.esperar(ctx, esperaReintentoModelo); errEspera != nil {
		return RespuestaModelo{}, errors.Join(err, errEspera)
	}
	return a.Modelo.Responder(ctx, p)
}

func (a AgenteConsulta) esperar(ctx context.Context, d time.Duration) error {
	if a.Esperar != nil {
		return a.Esperar(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (a AgenteConsulta) log() *slog.Logger {
	if a.Log != nil {
		return a.Log
	}
	return slog.Default()
}

// cuarentena envuelve datos como JSON escapado: json.Marshal escapa < y >, asi que los datos no pueden cerrar el sobre.
func cuarentena(nombre string, datos any) string {
	cuerpo, err := json.Marshal(datos)
	if err != nil {
		cuerpo = []byte(`{"error":"resultado no serializable"}`)
	}
	return fmt.Sprintf("<tool_result name=%q>\n%s\n</tool_result>", nombre, cuerpo)
}

// erroresVisiblesAlModelo son los errores de dominio que el modelo puede explicar; el resto se oculta.
var erroresVisiblesAlModelo = []error{
	ErrNoEncontrado, ErrArgumentosInvalidos, ErrHerramientaDesconocida,
	ErrPeriodoInvalido, ErrFiltroInvalido,
}

func mensajeParaModelo(err error) string {
	for _, visible := range erroresVisiblesAlModelo {
		if errors.Is(err, visible) {
			if errors.Is(err, ErrArgumentosInvalidos) {
				return err.Error()
			}
			return visible.Error()
		}
	}
	return "la consulta fallo por un error interno; no la repitas con los mismos argumentos"
}

func mensajesIniciales(historial []TurnoConversacion, mensaje string) ([]MensajeModelo, error) {
	mensaje = strings.TrimSpace(mensaje)
	if mensaje == "" || utf8.RuneCountInString(mensaje) > MaxRunasMensajeAgente {
		return nil, fmt.Errorf("%w: el mensaje tiene que tener entre 1 y %d caracteres", ErrConsultaInvalida, MaxRunasMensajeAgente)
	}
	if len(historial) > MaxHistorialAgente {
		return nil, fmt.Errorf("%w: el historial admite como maximo %d turnos", ErrConsultaInvalida, MaxHistorialAgente)
	}
	out := make([]MensajeModelo, 0, len(historial)+1)
	for _, t := range historial {
		if t.Rol != RolMensajeUsuario && t.Rol != RolMensajeAsistente {
			return nil, fmt.Errorf("%w: rol de historial %q", ErrConsultaInvalida, t.Rol)
		}
		if utf8.RuneCountInString(t.Texto) > MaxRunasMensajeAgente {
			return nil, fmt.Errorf("%w: un turno del historial es demasiado largo", ErrConsultaInvalida)
		}
		if strings.TrimSpace(t.Texto) == "" || (len(out) == 0 && t.Rol == RolMensajeAsistente) {
			continue
		}
		out = append(out, MensajeModelo{Rol: t.Rol, Texto: t.Texto})
	}
	return append(out, MensajeModelo{Rol: RolMensajeUsuario, Texto: mensaje}), nil
}

// contextoDeActor manda solo el rol: nombre y correo no salen al proveedor (minimizacion, Ley 1581, ADR 0025).
func contextoDeActor(u Usuario) string {
	return fmt.Sprintf("Quien pregunta tiene rol %q. Responde solo con lo que ese rol puede ver.", u.Rol)
}
