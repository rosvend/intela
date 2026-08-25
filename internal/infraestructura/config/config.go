// Package config lee la configuracion del entorno.
//
// Existe porque getenv estaba copiado literalmente en los tres main.go, y
// tres copias de la misma funcion divergen.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Cadena devuelve la variable, o pordefecto si esta vacia o no existe.
func Cadena(clave, pordefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return pordefecto
}

// Lista parte la variable por comas y descarta los vacios. Para listas
// blancas de origenes CORS y similares.
func Lista(clave string) []string {
	crudo := os.Getenv(clave)
	if strings.TrimSpace(crudo) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(crudo, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Duracion acepta el formato de time.ParseDuration ("30s", "5m").
func Duracion(clave string, pordefecto time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(clave))
	if v == "" {
		return pordefecto
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("duracion invalida, se usa el valor por defecto",
			slog.String("clave", clave), slog.String("valor", v),
			slog.Duration("pordefecto", pordefecto))
		return pordefecto
	}
	return d
}

// Bool acepta lo que entiende strconv.ParseBool.
func Bool(clave string, pordefecto bool) bool {
	v := strings.TrimSpace(os.Getenv(clave))
	if v == "" {
		return pordefecto
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return pordefecto
	}
	return b
}

// Logger construye el logger del proceso. JSON en produccion, texto en
// desarrollo.
func Logger(componente string) *slog.Logger {
	nivel := slog.LevelInfo
	if Bool("DEBUG", false) {
		nivel = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: nivel}

	var h slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if Cadena("LOG_FORMATO", "json") == "texto" {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h).With(slog.String("componente", componente))
}
