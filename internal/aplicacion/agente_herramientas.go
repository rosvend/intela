package aplicacion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// ErrHerramientaDesconocida: el modelo pidio una herramienta que no esta en el catalogo.
var ErrHerramientaDesconocida = errors.New("herramienta desconocida")

// ErrArgumentosInvalidos: los argumentos del modelo no cumplen el esquema de la herramienta.
var ErrArgumentosInvalidos = errors.New("argumentos invalidos")

// Herramienta es una lectura que el agente puede invocar; forma compatible con una tool de MCP.
type Herramienta struct {
	Nombre      string
	Descripcion string
	// Esquema es el JSON Schema (subconjunto: object, properties, required, additionalProperties, enum) de los argumentos.
	Esquema json.RawMessage
	// Ejecutar llama a un caso de uso de lectura con el actor de la sesion; el RBAC vive en ese caso de uso.
	Ejecutar func(ctx context.Context, actor Usuario, args json.RawMessage) (any, error)
}

// CatalogoHerramientas es el registro de solo lectura que el bucle despacha.
type CatalogoHerramientas struct {
	herramientas []herramientaValidada
}

type herramientaValidada struct {
	Herramienta
	esquema esquemaArgumentos
}

type esquemaArgumentos struct {
	Type                 string                      `json:"type"`
	Properties           map[string]propiedadEsquema `json:"properties"`
	Required             []string                    `json:"required"`
	AdditionalProperties *bool                       `json:"additionalProperties"`
}

type propiedadEsquema struct {
	Type string `json:"type"`
	Enum []any  `json:"enum"`
}

var nombreHerramientaRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// NuevoCatalogoHerramientas valida cada definicion al arrancar: un esquema roto es un defecto del programa.
func NuevoCatalogoHerramientas(hs ...Herramienta) (CatalogoHerramientas, error) {
	var c CatalogoHerramientas
	for _, h := range hs {
		if !nombreHerramientaRe.MatchString(h.Nombre) {
			return CatalogoHerramientas{}, fmt.Errorf("herramienta %q: nombre invalido", h.Nombre)
		}
		if h.Ejecutar == nil {
			return CatalogoHerramientas{}, fmt.Errorf("herramienta %q: sin Ejecutar", h.Nombre)
		}
		if _, ya := c.buscar(h.Nombre); ya {
			return CatalogoHerramientas{}, fmt.Errorf("herramienta %q: duplicada", h.Nombre)
		}
		var e esquemaArgumentos
		if err := json.Unmarshal(h.Esquema, &e); err != nil || e.Type != "object" {
			return CatalogoHerramientas{}, fmt.Errorf("herramienta %q: el esquema tiene que ser un objeto JSON Schema de tipo object", h.Nombre)
		}
		for _, req := range e.Required {
			if _, hay := e.Properties[req]; !hay {
				return CatalogoHerramientas{}, fmt.Errorf("herramienta %q: requerido %q sin propiedad", h.Nombre, req)
			}
		}
		c.herramientas = append(c.herramientas, herramientaValidada{Herramienta: h, esquema: e})
	}
	return c, nil
}

// Esquemas devuelve lo que el modelo lee para decidir, en orden de registro (estable para la cache de prompt).
func (c CatalogoHerramientas) Esquemas() []EsquemaHerramienta {
	out := make([]EsquemaHerramienta, 0, len(c.herramientas))
	for _, h := range c.herramientas {
		out = append(out, EsquemaHerramienta{Nombre: h.Nombre, Descripcion: h.Descripcion, Parametros: h.Esquema})
	}
	return out
}

// Ejecutar valida los argumentos contra el esquema y solo entonces llama a la herramienta.
func (c CatalogoHerramientas) Ejecutar(ctx context.Context, actor Usuario, nombre string, args json.RawMessage) (any, error) {
	h, ok := c.buscar(nombre)
	if !ok {
		return nil, ErrHerramientaDesconocida
	}
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := h.esquema.validar(args); err != nil {
		return nil, err
	}
	return h.Ejecutar(ctx, actor, args)
}

func (c CatalogoHerramientas) buscar(nombre string) (herramientaValidada, bool) {
	i := slices.IndexFunc(c.herramientas, func(h herramientaValidada) bool { return h.Nombre == nombre })
	if i < 0 {
		return herramientaValidada{}, false
	}
	return c.herramientas[i], true
}

func (e esquemaArgumentos) validar(args json.RawMessage) error {
	var obj map[string]any
	if err := json.Unmarshal(args, &obj); err != nil || obj == nil {
		return fmt.Errorf("%w: se esperaba un objeto JSON", ErrArgumentosInvalidos)
	}
	for _, req := range e.Required {
		if _, hay := obj[req]; !hay {
			return fmt.Errorf("%w: falta %q", ErrArgumentosInvalidos, req)
		}
	}
	for clave, valor := range obj {
		p, conocida := e.Properties[clave]
		if !conocida {
			if e.AdditionalProperties != nil && !*e.AdditionalProperties {
				return fmt.Errorf("%w: campo no permitido %q", ErrArgumentosInvalidos, clave)
			}
			continue
		}
		if !tipoCuadra(p.Type, valor) {
			return fmt.Errorf("%w: %q tiene que ser %s", ErrArgumentosInvalidos, clave, p.Type)
		}
		if len(p.Enum) > 0 && !slices.Contains(p.Enum, valor) {
			return fmt.Errorf("%w: %q fuera de los valores permitidos", ErrArgumentosInvalidos, clave)
		}
	}
	return nil
}

func tipoCuadra(tipo string, v any) bool {
	switch tipo {
	case "string":
		_, ok := v.(string)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	case "number":
		_, ok := v.(float64)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "":
		return true
	default:
		return false
	}
}
