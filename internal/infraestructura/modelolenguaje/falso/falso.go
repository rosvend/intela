// Package falso es un ModeloLenguaje determinista para pruebas, CI y desarrollo local sin proveedor.
package falso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Marcas de prueba en la pregunta: fuerzan el fallo del modelo o el limite de turnos.
const (
	MarcaFallo   = "#fallo"
	MarcaParcial = "#parcial"
)

var (
	periodoRe = regexp.MustCompile(`\b\d{4}(-\d{2})?\b`)
	sobreRe   = regexp.MustCompile(`(?s)<tool_result name="[^"]*">\n(.*)\n</tool_result>`)
)

// Modelo elige la herramienta cuyo nombre aparece en la pregunta y resume sus resultados.
type Modelo struct{}

// Responder implementa aplicacion.ModeloLenguaje.
func (Modelo) Responder(_ context.Context, p aplicacion.PeticionModelo) (aplicacion.RespuestaModelo, error) {
	if len(p.Mensajes) == 0 {
		return aplicacion.RespuestaModelo{}, errors.New("falso: peticion sin mensajes")
	}
	preguntaOriginal := strings.ToLower(ultimaPregunta(p.Mensajes))
	if strings.Contains(preguntaOriginal, MarcaFallo) {
		return aplicacion.RespuestaModelo{}, errors.New("falso: fallo forzado por la marca #fallo")
	}
	ultimo := p.Mensajes[len(p.Mensajes)-1]
	if len(ultimo.Resultados) > 0 && !strings.Contains(preguntaOriginal, MarcaParcial) {
		return aplicacion.RespuestaModelo{Texto: resumir(ultimo.Resultados)}, nil
	}
	for i, h := range p.Herramientas {
		if mencionada(preguntaOriginal, h.Nombre) {
			return aplicacion.RespuestaModelo{
				Texto:    "Consulto " + h.Nombre + ".",
				Llamadas: []aplicacion.LlamadaHerramienta{{ID: fmt.Sprintf("falso-%d-%d", len(p.Mensajes), i), Nombre: h.Nombre, Argumentos: argumentos(h, preguntaOriginal)}},
			}, nil
		}
	}
	return aplicacion.RespuestaModelo{Texto: demostracion(p.Herramientas)}, nil
}

// NoDisponible responde sin error cuando la instalacion no tiene proveedor configurado.
type NoDisponible struct{}

// Responder implementa aplicacion.ModeloLenguaje.
func (NoDisponible) Responder(context.Context, aplicacion.PeticionModelo) (aplicacion.RespuestaModelo, error) {
	return aplicacion.RespuestaModelo{Texto: "El asistente no está disponible en esta instalación: falta configurar el proveedor del modelo de lenguaje. Avísale a un administrador."}, nil
}

func ultimaPregunta(ms []aplicacion.MensajeModelo) string {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Rol == aplicacion.RolMensajeUsuario && ms[i].Texto != "" {
			return ms[i].Texto
		}
	}
	return ""
}

// mencionada dice si alguna palabra significativa del nombre (buscar_REGLAMENTO, listar_ONI) aparece en la pregunta.
func mencionada(pregunta, nombre string) bool {
	partes := strings.Split(nombre, "_")
	clave := partes[len(partes)-1]
	return strings.Contains(pregunta, clave)
}

// argumentos rellena solo los requeridos: el periodo si lo pide, la pregunta para el resto de textos.
func argumentos(h aplicacion.EsquemaHerramienta, pregunta string) json.RawMessage {
	var esquema struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(h.Parametros, &esquema)
	args := map[string]string{}
	for _, req := range esquema.Required {
		if strings.Contains(req, "periodo") {
			args[req] = periodoRe.FindString(pregunta)
			continue
		}
		args[req] = pregunta
	}
	b, _ := json.Marshal(args)
	return b
}

func resumir(rs []aplicacion.ResultadoHerramienta) string {
	var b strings.Builder
	b.WriteString("Modo de demostración. Esto es lo que devolvieron las herramientas:\n")
	for _, r := range rs {
		datos := r.Contenido
		if m := sobreRe.FindStringSubmatch(r.Contenido); m != nil {
			datos = m[1]
		}
		if len(datos) > 1200 {
			datos = datos[:1200] + "..."
		}
		b.WriteString("\n" + datos + "\n")
	}
	return b.String()
}

func demostracion(hs []aplicacion.EsquemaHerramienta) string {
	nombres := make([]string, 0, len(hs))
	for _, h := range hs {
		nombres = append(nombres, h.Nombre)
	}
	if len(nombres) == 0 {
		return "Modo de demostración: no hay un modelo de lenguaje conectado y todavía no tengo herramientas de consulta."
	}
	return "Modo de demostración: no hay un modelo de lenguaje conectado. Menciona una de estas consultas y la ejecuto: " + strings.Join(nombres, ", ") + "."
}
