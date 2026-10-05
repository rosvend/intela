// Package bedrock adapta aplicacion.ModeloLenguaje a la API Converse de Amazon Bedrock (tool use nativo).
package bedrock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ModeloPorDefecto es Haiku 4.5 por el perfil de inferencia us. (us-east-1, us-east-2, us-west-2); ADR 0025.
const ModeloPorDefecto = "us.anthropic.claude-haiku-4-5-20251001-v1:0"

const (
	maxTokensRespuesta = 2048
	textoRechazo       = "No puedo ayudar con esa consulta."
)

// Cliente es lo unico que el adaptador usa de bedrockruntime.Client; las pruebas lo sustituyen.
type Cliente interface {
	Converse(ctx context.Context, in *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

// Modelo es el adaptador; el bucle (turnos, reintento, cuarentena) es de AgenteConsulta, no de aqui.
type Modelo struct {
	Cliente Cliente
	Modelo  string
}

// Responder implementa aplicacion.ModeloLenguaje.
func (m Modelo) Responder(ctx context.Context, p aplicacion.PeticionModelo) (aplicacion.RespuestaModelo, error) {
	in := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(m.Modelo),
		System:          sistemaDe(p),
		Messages:        mensajesDe(p.Mensajes),
		InferenceConfig: &types.InferenceConfiguration{MaxTokens: aws.Int32(maxTokensRespuesta)},
	}
	if len(p.Herramientas) > 0 {
		herramientas, err := herramientasDe(p.Herramientas)
		if err != nil {
			return aplicacion.RespuestaModelo{}, err
		}
		in.ToolConfig = &types.ToolConfiguration{Tools: herramientas}
	}
	out, err := m.Cliente.Converse(ctx, in)
	if err != nil {
		return aplicacion.RespuestaModelo{}, fmt.Errorf("bedrock %s: %w", m.Modelo, err)
	}
	return respuestaDe(out)
}

func respuestaDe(out *bedrockruntime.ConverseOutput) (aplicacion.RespuestaModelo, error) {
	msg, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return aplicacion.RespuestaModelo{}, fmt.Errorf("bedrock: salida inesperada %T", out.Output)
	}
	var resp aplicacion.RespuestaModelo
	for _, bloque := range msg.Value.Content {
		switch b := bloque.(type) {
		case *types.ContentBlockMemberText:
			resp.Texto += b.Value
		case *types.ContentBlockMemberToolUse:
			args, err := b.Value.Input.MarshalSmithyDocument()
			if err != nil {
				return aplicacion.RespuestaModelo{}, fmt.Errorf("bedrock: argumentos de %q: %w", aws.ToString(b.Value.Name), err)
			}
			resp.Llamadas = append(resp.Llamadas, aplicacion.LlamadaHerramienta{ID: aws.ToString(b.Value.ToolUseId), Nombre: aws.ToString(b.Value.Name), Argumentos: args})
		}
	}
	filtrado := out.StopReason == types.StopReasonContentFiltered || out.StopReason == types.StopReasonGuardrailIntervened
	if filtrado && resp.Texto == "" && len(resp.Llamadas) == 0 {
		resp.Texto = textoRechazo
	}
	return resp, nil
}

func sistemaDe(p aplicacion.PeticionModelo) []types.SystemContentBlock {
	var out []types.SystemContentBlock
	for _, s := range []string{p.Sistema, p.Contexto} {
		if s != "" {
			out = append(out, &types.SystemContentBlockMemberText{Value: s})
		}
	}
	return out
}

func herramientasDe(es []aplicacion.EsquemaHerramienta) ([]types.Tool, error) {
	out := make([]types.Tool, 0, len(es))
	for _, e := range es {
		var esquema map[string]any
		if err := json.Unmarshal(e.Parametros, &esquema); err != nil {
			return nil, fmt.Errorf("bedrock: esquema de %q: %w", e.Nombre, err)
		}
		spec := types.ToolSpecification{Name: aws.String(e.Nombre), InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(esquema)}}
		if e.Descripcion != "" {
			spec.Description = aws.String(e.Descripcion)
		}
		out = append(out, &types.ToolMemberToolSpec{Value: spec})
	}
	return out, nil
}

// mensajesDe fusiona turnos seguidos del mismo rol y omite los vacios: Converse exige alternancia.
func mensajesDe(ms []aplicacion.MensajeModelo) []types.Message {
	out := make([]types.Message, 0, len(ms))
	for _, m := range ms {
		bloques := bloquesDe(m)
		if len(bloques) == 0 {
			continue
		}
		rol := types.ConversationRoleUser
		if m.Rol == aplicacion.RolMensajeAsistente {
			rol = types.ConversationRoleAssistant
		}
		if n := len(out); n > 0 && out[n-1].Role == rol {
			out[n-1].Content = append(out[n-1].Content, bloques...)
			continue
		}
		out = append(out, types.Message{Role: rol, Content: bloques})
	}
	return out
}

func bloquesDe(m aplicacion.MensajeModelo) []types.ContentBlock {
	var bloques []types.ContentBlock
	if m.Texto != "" {
		bloques = append(bloques, &types.ContentBlockMemberText{Value: m.Texto})
	}
	for _, ll := range m.Llamadas {
		bloques = append(bloques, &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String(ll.ID), Name: aws.String(ll.Nombre), Input: document.NewLazyDocument(argumentosDe(ll.Argumentos)),
		}})
	}
	for _, r := range m.Resultados {
		estado := types.ToolResultStatusSuccess
		if r.EsError {
			estado = types.ToolResultStatusError
		}
		bloques = append(bloques, &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
			ToolUseId: aws.String(r.LlamadaID), Status: estado,
			Content: []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{Value: r.Contenido}},
		}})
	}
	return bloques
}

// argumentosDe devuelve el objeto JSON de la llamada; Converse exige un objeto, asi que lo invalido va como {}.
func argumentosDe(raw json.RawMessage) any {
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return map[string]any{}
	}
	return v
}
