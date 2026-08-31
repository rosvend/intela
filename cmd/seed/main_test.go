package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestEjecutarSinDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := ejecutar(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || err.Error() != "falta DATABASE_URL" {
		t.Fatalf("se esperaba falta DATABASE_URL, se obtuvo %v", err)
	}
}
