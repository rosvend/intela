// Package anthropic adapta aplicacion.ModeloLenguaje a la API de mensajes de Anthropic (tool use nativo).
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ModeloPorDefecto es Haiku 4.5: rapido y barato para un asistente de consulta.
const ModeloPorDefecto = "claude-haiku-4-5-20251001"

const (
	maxTokensRespuesta = 2048
	timeoutPeticion    = 25 * time.Second
)

// Modelo es el adaptador; el bucle (turnos, reintento, cuarentena) es de AgenteConsulta, no de aqui.
type Modelo struct {
	cliente sdk.Client
	modelo  string
}

// Opcion ajusta el cliente (en pruebas, la URL del servidor falso).
type Opcion func(*[]option.RequestOption)

// ConURLBase apunta el cliente a otra URL.
func ConURLBase(url string) Opcion {
	return func(o *[]option.RequestOption) { *o = append(*o, option.WithBaseURL(url)) }
}

// Nuevo construye el adaptador; sin reintentos del SDK porque el reintento lo decide el bucle.
func Nuevo(clave, modelo string, opciones ...Opcion) Modelo {
	if modelo == "" {
		modelo = ModeloPorDefecto
	}
	opts := []option.RequestOption{
		option.WithAPIKey(clave),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(timeoutPeticion),
	}
	for _, o := range opciones {
		o(&opts)
	}
	return Modelo{cliente: sdk.NewClient(opts...), modelo: modelo}
}

// Responder implementa aplicacion.ModeloLenguaje.
func (m Modelo) Responder(ctx context.Context, p aplicacion.PeticionModelo) (aplicacion.RespuestaModelo, error) {
	herramientas, err := herramientasDe(p.Herramientas)
	if err != nil {
		return aplicacion.RespuestaModelo{}, err
	}
	sistema := []sdk.TextBlockParam{{Text: p.Sistema, CacheControl: sdk.NewCacheControlEphemeralParam()}}
	if p.Contexto != "" {
		sistema = append(sistema, sdk.TextBlockParam{Text: p.Contexto})
	}
	msg, err := m.cliente.Messages.New(ctx, sdk.MessageNewParams{
		Model:     sdk.Model(m.modelo),
		MaxTokens: maxTokensRespuesta,
		System:    sistema,
		Tools:     herramientas,
		Messages:  mensajesDe(p.Mensajes),
	})
	if err != nil {
		return aplicacion.RespuestaModelo{}, fmt.Errorf("anthropic: %w", err)
	}
	if msg.StopReason == sdk.StopReasonRefusal {
		return aplicacion.RespuestaModelo{Texto: "No puedo ayudar con esa consulta."}, nil
	}

	var resp aplicacion.RespuestaModelo
	for _, bloque := range msg.Content {
		switch b := bloque.AsAny().(type) {
		case sdk.TextBlock:
			resp.Texto += b.Text
		case sdk.ToolUseBlock:
			resp.Llamadas = append(resp.Llamadas, aplicacion.LlamadaHerramienta{ID: b.ID, Nombre: b.Name, Argumentos: b.Input})
		}
	}
	return resp, nil
}

func herramientasDe(es []aplicacion.EsquemaHerramienta) ([]sdk.ToolUnionParam, error) {
	out := make([]sdk.ToolUnionParam, 0, len(es))
	for _, e := range es {
		var esquema struct {
			Properties           map[string]any `json:"properties"`
			Required             []string       `json:"required"`
			AdditionalProperties *bool          `json:"additionalProperties"`
		}
		if err := json.Unmarshal(e.Parametros, &esquema); err != nil {
			return nil, fmt.Errorf("anthropic: esquema de %q: %w", e.Nombre, err)
		}
		entrada := sdk.ToolInputSchemaParam{Properties: esquema.Properties, Required: esquema.Required}
		if esquema.Properties == nil {
			entrada.Properties = map[string]any{}
		}
		if esquema.AdditionalProperties != nil {
			entrada.ExtraFields = map[string]any{"additionalProperties": *esquema.AdditionalProperties}
		}
		out = append(out, sdk.ToolUnionParam{OfTool: &sdk.ToolParam{
			Name:        e.Nombre,
			Description: sdk.String(e.Descripcion),
			InputSchema: entrada,
		}})
	}
	return out, nil
}

func mensajesDe(ms []aplicacion.MensajeModelo) []sdk.MessageParam {
	out := make([]sdk.MessageParam, 0, len(ms))
	for _, m := range ms {
		var bloques []sdk.ContentBlockParamUnion
		if m.Texto != "" {
			bloques = append(bloques, sdk.NewTextBlock(m.Texto))
		}
		for _, ll := range m.Llamadas {
			bloques = append(bloques, sdk.NewToolUseBlock(ll.ID, ll.Argumentos, ll.Nombre))
		}
		for _, r := range m.Resultados {
			bloques = append(bloques, sdk.NewToolResultBlock(r.LlamadaID, r.Contenido, r.EsError))
		}
		if m.Rol == aplicacion.RolMensajeAsistente {
			out = append(out, sdk.NewAssistantMessage(bloques...))
			continue
		}
		out = append(out, sdk.NewUserMessage(bloques...))
	}
	return out
}
