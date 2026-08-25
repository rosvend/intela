// Package reloj adapta el puerto aplicacion.Reloj al reloj del sistema.
//
// Es un paquete propio y no una funcion suelta dentro del adaptador de
// PostgreSQL: un reloj no tiene nada que ver con una base de datos, y
// leer postgres.RelojReal{} en el cableado delata la frontera mal puesta.
package reloj

import (
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Sistema devuelve la hora del proceso. Es el unico sitio del binario donde
// se llama a time.Now().
type Sistema struct{}

func (Sistema) Ahora() time.Time { return time.Now().UTC() }

// Fijo devuelve siempre el mismo instante. Para pruebas: las prescripciones
// de 3 y 10 anos de RD 15 se comprueban moviendo esto, no esperando.
type Fijo struct{ Instante time.Time }

func (f Fijo) Ahora() time.Time { return f.Instante }

var (
	_ aplicacion.Reloj = Sistema{}
	_ aplicacion.Reloj = Fijo{}
)
