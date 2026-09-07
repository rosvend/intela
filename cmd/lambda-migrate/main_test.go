package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func mudo() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Las ordenes destructivas se rechazan ANTES de conectar.
//
// Esta funcion vive dentro de la VPC y su nombre esta publicado en los outputs
// de Terraform. Quien tenga lambda:InvokeFunction puede llamarla; sin esta
// comprobacion, `{"orden":"reset"}` tira el esquema entero sin pasar por
// Terraform y sin que la guarda de destruccion vea nada.
//
// Que no haya base levantada en esta prueba ES la prueba: si el rechazo llegara
// tarde, Aplicar intentaria conectar y el error hablaria de la conexion, no de
// la orden.
func TestOrdenesDestructivasRechazadas(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	for _, orden := range []string{"reset", "down", "down-to", "redo", "create", "fix"} {
		t.Run(orden, func(t *testing.T) {
			_, err := atender(mudo())(t.Context(), peticion{Orden: orden})
			if err == nil {
				t.Fatalf("la orden %q tiene que rechazarse", orden)
			}
			if !strings.Contains(err.Error(), "no permitida") {
				t.Errorf("tiene que rechazarse por la lista, no por otra cosa: %v", err)
			}
		})
	}
}

// Las permitidas pasan el filtro y llegan a intentar conectar. El DSN apunta a
// un puerto donde no hay nadie, asi que el error esperado es de conexion: eso
// demuestra que la orden se acepto y que el fallo esta mas abajo.
func TestOrdenesPermitidasPasanElFiltro(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	for _, orden := range ordenesPermitidas {
		t.Run(orden, func(t *testing.T) {
			_, err := atender(mudo())(t.Context(), peticion{Orden: orden})
			if err != nil && strings.Contains(err.Error(), "no permitida") {
				t.Fatalf("la orden %q esta en la lista y aun asi se rechazo: %v", orden, err)
			}
		})
	}
}

// Terraform manda {"orden":"up"}, pero una invocacion sin cuerpo tiene que
// seguir aplicando el esquema y no rechazarse por vacia.
func TestOrdenVaciaUsaLaPorDefecto(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	_, err := atender(mudo())(t.Context(), peticion{Orden: ""})
	if err != nil && strings.Contains(err.Error(), "no permitida") {
		t.Fatalf("la orden por defecto tiene que estar permitida: %v", err)
	}
}

// La provision inicial valida ANTES de conectar, igual que la lista de ordenes.
//
// Mismo razonamiento que TestOrdenesDestructivasRechazadas: no hay base
// levantada, asi que si la validacion llegara tarde el error hablaria de la
// conexion. Y el mensaje tiene que nombrar el campo, porque esta superficie la
// invoca una persona con la CLI delante, sin formulario que le marque nada.
func TestPrimerAdministradorValidaAntesDeConectar(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")

	casos := map[string]struct {
		p     peticion
		campo string
	}{
		"sin email":  {peticion{Orden: ordenPrimerAdministrador, Hash: hashDePrueba, Nombre: "A", ID: "usr-a"}, "email"},
		"sin hash":   {peticion{Orden: ordenPrimerAdministrador, Email: "a@b.co", Nombre: "A", ID: "usr-a"}, "hash"},
		"sin nombre": {peticion{Orden: ordenPrimerAdministrador, Email: "a@b.co", Hash: hashDePrueba, ID: "usr-a"}, "nombre"},
		"email sin arroba": {
			peticion{Orden: ordenPrimerAdministrador, Email: "a.b.co", Hash: hashDePrueba, Nombre: "A", ID: "usr-a"},
			"email",
		},
		// La clave en claro mandada por error en vez del hash.
		"hash corto": {
			peticion{Orden: ordenPrimerAdministrador, Email: "a@b.co", Hash: "secreta", Nombre: "A", ID: "usr-a"},
			"hash",
		},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, err := atender(mudo())(t.Context(), c.p)
			if err == nil {
				t.Fatal("se esperaba error")
			}
			if !strings.Contains(err.Error(), c.campo) {
				t.Errorf("mensaje = %q, se esperaba que nombrara %q", err.Error(), c.campo)
			}
			// Si hubiera llegado a conectar, el error seria de red.
			for _, señal := range []string{"connect", "dial", "127.0.0.1", "refused"} {
				if strings.Contains(err.Error(), señal) {
					t.Errorf("el error habla de la conexion (%q): la validacion llego tarde", señal)
				}
			}
		})
	}
}

// La orden de provision NO es una orden de goose y no puede colarse por la
// lista: si estuviera en ordenesPermitidas, llegaria a Aplicar y goose fallaria
// con "not a valid command", que no dice nada util.
func TestPrimerAdministradorNoEsUnaOrdenDeGoose(t *testing.T) {
	for _, orden := range ordenesPermitidas {
		if orden == ordenPrimerAdministrador {
			t.Fatalf("%q no debe estar en ordenesPermitidas", ordenPrimerAdministrador)
		}
	}
}

const hashDePrueba = "$2a$10$0123456789012345678901234567890123456789012345678901"
