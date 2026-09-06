package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
)

// Traduccion entre el evento de una Function URL y net/http, en las dos
// direcciones.
//
// Escrito a mano, y no con awslabs/aws-lambda-go-api-proxy, porque el go.mod de
// esa libreria declara gin, fiber, iris, echo, gorilla/mux, negroni y fasthttp
// como dependencias directas: trae todos los adaptadores de framework en un
// solo modulo. Este repositorio tiene siete dependencias directas elegidas a
// mano, y setenta lineas propias salen mas baratas que ese arbol.

// aHTTP construye la peticion que vera el router de chi.
func aHTTP(ctx context.Context, ev events.LambdaFunctionURLRequest) (*http.Request, error) {
	cuerpo := []byte(ev.Body)
	if ev.IsBase64Encoded {
		descifrado, err := base64.StdEncoding.DecodeString(ev.Body)
		if err != nil {
			return nil, fmt.Errorf("cuerpo en base64 invalido: %w", err)
		}
		cuerpo = descifrado
	}

	ruta := ev.RawPath
	if ruta == "" {
		ruta = "/"
	}
	if ev.RawQueryString != "" {
		ruta += "?" + ev.RawQueryString
	}

	// Las claves de Headers llegan en minusculas desde el runtime.
	anfitrion := ev.Headers["host"]
	if anfitrion == "" {
		anfitrion = "localhost"
	}

	// URL absoluta, no solo la ruta: asi r.Host y r.URL.Scheme quedan
	// poblados, que es lo que espera cualquier handler que construya un enlace
	// de vuelta.
	r, err := http.NewRequestWithContext(ctx,
		ev.RequestContext.HTTP.Method,
		"https://"+anfitrion+ruta,
		bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("construir peticion: %w", err)
	}

	for clave, valor := range ev.Headers {
		r.Header.Set(clave, valor)
	}

	// Las cookies viajan en su propio campo, fuera de Headers.
	if len(ev.Cookies) > 0 {
		r.Header.Set("Cookie", strings.Join(ev.Cookies, "; "))
	}

	// Sin esto la bitacora ve todas las peticiones como si vinieran del mismo
	// sitio, y el ADR 0006 pide poder responder quien hizo que. Cuando hay un
	// proxy delante, chi (middleware.RealIP) prefiere X-Forwarded-For, que ya
	// viene en Headers.
	r.RemoteAddr = ev.RequestContext.HTTP.SourceIP
	r.RequestURI = ruta
	r.ContentLength = int64(len(cuerpo))

	return r, nil
}

// grabador es un http.ResponseWriter que acumula en memoria.
//
// Deliberadamente no es httptest.NewRecorder: httptest es un paquete de pruebas
// y registra flags propios: no tiene que entrar en un binario de produccion por
// veinte lineas que se escriben solas.
type grabador struct {
	codigo  int
	cabezas http.Header
	cuerpo  bytes.Buffer
}

func nuevoGrabador() *grabador {
	return &grabador{cabezas: make(http.Header)}
}

func (g *grabador) Header() http.Header { return g.cabezas }

func (g *grabador) WriteHeader(codigo int) {
	if g.codigo == 0 {
		g.codigo = codigo
	}
}

func (g *grabador) Write(p []byte) (int, error) {
	if g.codigo == 0 {
		g.codigo = http.StatusOK
	}
	return g.cuerpo.Write(p)
}

// aEvento convierte lo grabado en la respuesta que espera la Function URL.
func (g *grabador) aEvento() events.LambdaFunctionURLResponse {
	codigo := g.codigo
	if codigo == 0 {
		// El handler no escribio nada. net/http lo trataria como 200.
		codigo = http.StatusOK
	}

	cabezas := make(map[string]string, len(g.cabezas))
	var galletas []string

	for clave, valores := range g.cabezas {
		// Set-Cookie es la unica cabecera que se repite legitimamente, y
		// aplastarla con una coma rompe las cookies. Va en su propio campo.
		if http.CanonicalHeaderKey(clave) == "Set-Cookie" {
			galletas = append(galletas, valores...)
			continue
		}
		cabezas[clave] = strings.Join(valores, ", ")
	}

	crudo := g.cuerpo.Bytes()

	// Texto valido va en claro, para que el cuerpo se pueda leer en los logs
	// cuando algo falla. Lo demas va en base64, que es lo unico que sobrevive
	// al transporte JSON.
	if utf8.Valid(crudo) {
		return events.LambdaFunctionURLResponse{
			StatusCode: codigo,
			Headers:    cabezas,
			Cookies:    galletas,
			Body:       string(crudo),
		}
	}

	return events.LambdaFunctionURLResponse{
		StatusCode:      codigo,
		Headers:         cabezas,
		Cookies:         galletas,
		Body:            base64.StdEncoding.EncodeToString(crudo),
		IsBase64Encoded: true,
	}
}
