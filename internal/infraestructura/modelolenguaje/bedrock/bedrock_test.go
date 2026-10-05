package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/rosvend/intela/internal/aplicacion"
)

// clienteFalso guarda la peticion y devuelve la salida o el error fijados.
type clienteFalso struct {
	recibido *bedrockruntime.ConverseInput
	salida   *bedrockruntime.ConverseOutput
	err      error
}

func (c *clienteFalso) Converse(_ context.Context, in *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	c.recibido = in
	return c.salida, c.err
}

func peticion() aplicacion.PeticionModelo {
	return aplicacion.PeticionModelo{
		Sistema:  "eres el asistente",
		Contexto: "rol administrador",
		Herramientas: []aplicacion.EsquemaHerramienta{{
			Nombre: "eco", Descripcion: "devuelve",
			Parametros: json.RawMessage(`{"type":"object","properties":{"texto":{"type":"string"}},"required":["texto"],"additionalProperties":false}`),
		}},
		Mensajes: []aplicacion.MensajeModelo{
			{Rol: aplicacion.RolMensajeUsuario, Texto: "hola"},
			{Rol: aplicacion.RolMensajeAsistente, Texto: "miro", Llamadas: []aplicacion.LlamadaHerramienta{{ID: "toolu_1", Nombre: "eco", Argumentos: json.RawMessage(`{"texto":"a"}`)}}},
			{Rol: aplicacion.RolMensajeUsuario, Resultados: []aplicacion.ResultadoHerramienta{{LlamadaID: "toolu_1", Contenido: "<tool_result name=\"eco\">{}</tool_result>", EsError: true}}},
		},
	}
}

func salidaConMensaje(stop types.StopReason, bloques ...types.ContentBlock) *bedrockruntime.ConverseOutput {
	return &bedrockruntime.ConverseOutput{
		StopReason: stop,
		Output:     &types.ConverseOutputMemberMessage{Value: types.Message{Role: types.ConversationRoleAssistant, Content: bloques}},
	}
}

func jsonDe(t *testing.T, d document.Interface) string {
	t.Helper()
	b, err := d.MarshalSmithyDocument()
	if err != nil {
		t.Fatalf("documento: %v", err)
	}
	return string(b)
}

func TestTraduceLaPeticionAConverse(t *testing.T) {
	c := &clienteFalso{salida: salidaConMensaje(types.StopReasonToolUse,
		&types.ContentBlockMemberText{Value: "voy a buscar"},
		&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{ToolUseId: aws.String("toolu_2"), Name: aws.String("eco"), Input: document.NewLazyDocument(map[string]any{"texto": "b"})}},
	)}
	m := Modelo{Cliente: c, Modelo: "modelo-x"}

	resp, err := m.Responder(t.Context(), peticion())
	if err != nil {
		t.Fatalf("Responder: %v", err)
	}

	in := c.recibido
	if aws.ToString(in.ModelId) != "modelo-x" {
		t.Errorf("ModelId = %q", aws.ToString(in.ModelId))
	}
	if len(in.System) != 2 || in.System[1].(*types.SystemContentBlockMemberText).Value != "rol administrador" {
		t.Errorf("System = %+v", in.System)
	}
	spec := in.ToolConfig.Tools[0].(*types.ToolMemberToolSpec).Value
	if aws.ToString(spec.Name) != "eco" || aws.ToString(spec.Description) != "devuelve" {
		t.Errorf("tool = %+v", spec)
	}
	if got := jsonDe(t, spec.InputSchema.(*types.ToolInputSchemaMemberJson).Value); got != `{"additionalProperties":false,"properties":{"texto":{"type":"string"}},"required":["texto"],"type":"object"}` {
		t.Errorf("esquema = %s", got)
	}
	if len(in.Messages) != 3 || in.Messages[1].Role != types.ConversationRoleAssistant {
		t.Fatalf("mensajes = %+v", in.Messages)
	}
	uso := in.Messages[1].Content[1].(*types.ContentBlockMemberToolUse).Value
	if aws.ToString(uso.ToolUseId) != "toolu_1" || jsonDe(t, uso.Input) != `{"texto":"a"}` {
		t.Errorf("toolUse = %+v", uso)
	}
	res := in.Messages[2].Content[0].(*types.ContentBlockMemberToolResult).Value
	if aws.ToString(res.ToolUseId) != "toolu_1" || res.Status != types.ToolResultStatusError || res.Content[0].(*types.ToolResultContentBlockMemberText).Value == "" {
		t.Errorf("toolResult = %+v", res)
	}

	if resp.Texto != "voy a buscar" || len(resp.Llamadas) != 1 || resp.Llamadas[0].ID != "toolu_2" || string(resp.Llamadas[0].Argumentos) != `{"texto":"b"}` {
		t.Errorf("respuesta = %+v", resp)
	}
}

func TestFusionaTurnosSeguidosDelMismoRolYOmiteLosVacios(t *testing.T) {
	c := &clienteFalso{salida: salidaConMensaje(types.StopReasonEndTurn, &types.ContentBlockMemberText{Value: "ok"})}
	p := aplicacion.PeticionModelo{Sistema: "s", Mensajes: []aplicacion.MensajeModelo{
		{Rol: aplicacion.RolMensajeUsuario, Texto: "primera"},
		{Rol: aplicacion.RolMensajeAsistente},
		{Rol: aplicacion.RolMensajeUsuario, Texto: "segunda"},
	}}
	if _, err := (Modelo{Cliente: c, Modelo: "m"}).Responder(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if len(c.recibido.Messages) != 1 || len(c.recibido.Messages[0].Content) != 2 {
		t.Fatalf("Converse exige turnos alternados y sin bloques vacios: %+v", c.recibido.Messages)
	}
	if c.recibido.ToolConfig != nil {
		t.Error("sin herramientas no se manda toolConfig")
	}
}

func TestUnErrorDelProveedorSubeComoError(t *testing.T) {
	c := &clienteFalso{err: errors.New("ThrottlingException")}
	if _, err := (Modelo{Cliente: c, Modelo: "m"}).Responder(t.Context(), peticion()); err == nil {
		t.Fatal("se esperaba error")
	}
}

func TestUnContenidoFiltradoVuelveComoTextoSinLlamadas(t *testing.T) {
	c := &clienteFalso{salida: salidaConMensaje(types.StopReasonContentFiltered)}
	resp, err := (Modelo{Cliente: c, Modelo: "m"}).Responder(t.Context(), peticion())
	if err != nil || resp.Texto == "" || len(resp.Llamadas) != 0 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}
