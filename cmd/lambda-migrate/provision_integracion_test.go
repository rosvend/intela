package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// TestMain apaga el contenedor cuando termina el binario de pruebas. Mismo
// patron que cmd/worker: os.Exit se salta los defer.
func TestMain(m *testing.M) {
	codigo := m.Run()
	testhelp.Terminar()
	os.Exit(codigo)
}

// El camino completo, contra PostgreSQL de verdad: evento -> validacion ->
// pool -> INSERT. Las pruebas de aplicacion y de postgres cubren cada mitad;
// esta cubre el CABLEADO, que es lo que se va a invocar contra produccion.
func TestProvisionarDeExtremoAExtremo(t *testing.T) {
	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)

	p := peticion{
		Orden:  ordenPrimerAdministrador,
		ID:     "usr-admin",
		Email:  "admin@redes.co",
		Nombre: "Administrador",
		Hash:   hashDePrueba,
	}

	r, err := atender(mudo())(t.Context(), p)
	if err != nil {
		t.Fatalf("primera invocacion: %v", err)
	}
	if r.Estado != "creado" {
		t.Fatalf("estado = %q, se esperaba \"creado\"", r.Estado)
	}

	pool, err := pgxpool.New(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir pool de lectura: %v", err)
	}
	t.Cleanup(pool.Close)

	var email, rol, hash string
	if err := pool.QueryRow(t.Context(),
		`SELECT email, rol, password_hash FROM usuarios WHERE id = $1`, p.ID).
		Scan(&email, &rol, &hash); err != nil {
		t.Fatalf("leer la cuenta creada: %v", err)
	}
	if email != p.Email {
		t.Errorf("email = %q, se esperaba %q", email, p.Email)
	}
	if rol != "administrador" {
		t.Errorf("rol = %q, se esperaba \"administrador\"", rol)
	}
	if hash != p.Hash {
		t.Errorf("hash = %q, se esperaba el que se mando", hash)
	}

	// La segunda invocacion NO es un error: la operacion es de una sola vez y
	// ya se hizo. Que devolviera error convertiria un reintento inocuo -- un
	// operador que no sabe si la primera llego -- en una alarma.
	r2, err := atender(mudo())(t.Context(), p)
	if err != nil {
		t.Fatalf("segunda invocacion: %v", err)
	}
	if r2.Estado != "ya provisionada" {
		t.Errorf("estado = %q, se esperaba \"ya provisionada\"", r2.Estado)
	}

	var total int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM usuarios`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 1 {
		t.Errorf("usuarios = %d, se esperaba 1", total)
	}
}

// LA propiedad que da nombre al comando: la cuenta creada SIRVE PARA ENTRAR.
//
// La primera version del PR no la probaba. Sus pruebas leian el hash de vuelta
// y lo comparaban con el que se habia mandado (`if hash != p.Hash`), que es
// cierto tambien cuando lo que se guardo es una clave en claro -- el caso que
// dejaba la instalacion cerrada para siempre, porque desde ahi el login falla
// con la clave correcta y no hay ninguna via de arreglo.
//
// Asi que aqui se hashea con el hasher de verdad, se provisiona por la Lambda,
// y despues se INICIA SESION por el mismo caso de uso que usa la API. Nada de
// comparar cadenas.
func TestLaCuentaProvisionadaPuedeIniciarSesion(t *testing.T) {
	const clave = "una-clave-de-operador-larga"

	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)

	hasher := cripto.Bcrypt{}
	hash, err := hasher.Hash(clave)
	if err != nil {
		t.Fatalf("hashear: %v", err)
	}

	r, err := atender(mudo())(t.Context(), peticion{
		Orden:  ordenPrimerAdministrador,
		ID:     "usr-admin",
		Email:  "admin@redes.co",
		Nombre: "Administrador",
		Hash:   hash,
	})
	if err != nil {
		t.Fatalf("provisionar: %v", err)
	}
	if r.Estado != "creado" {
		t.Fatalf("estado = %q, se esperaba \"creado\"", r.Estado)
	}

	store, err := postgres.Abrir(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(store.CerrarPool)

	// El mismo cableado que cmd/lambda: si esto entra, la cuenta sirve.
	autenticacion := aplicacion.Autenticacion{
		Usuarios: store,
		Claves:   hasher,
		Sesiones: store,
		Reloj:    reloj.Sistema{},
		Tokens:   cripto.TokensAleatorios{},
		TTL:      time.Hour,
	}

	sesion, err := autenticacion.IniciarSesion(t.Context(), "admin@redes.co", clave)
	if err != nil {
		t.Fatalf("iniciar sesion con la clave correcta: %v", err)
	}
	if sesion.Usuario.Rol != aplicacion.RolAdministrador {
		t.Errorf("rol = %q, se esperaba %q", sesion.Usuario.Rol, aplicacion.RolAdministrador)
	}
	if sesion.Token == "" {
		t.Error("token vacio")
	}

	// Y la clave equivocada no entra: sin esto, un verificador que dijera
	// siempre "si" pasaria la mitad de arriba.
	if _, err := autenticacion.IniciarSesion(t.Context(), "admin@redes.co", "otra-clave"); err == nil {
		t.Error("se inicio sesion con una clave equivocada")
	}
}

// Un hash TRUNCADO no llega a la base, y esto se prueba en la provision y no
// solo en EsHash a proposito: el punto no es que el validador sepa decir no,
// es que la provision no lo deje pasar.
//
// El hueco era de un caracter. `bcrypt.Cost` valida la cabecera y no el largo,
// asi que un hash de 59 devolvia coste 10 y ningun error, la cuenta se creaba
// -- estado "creado" -- y el login fallaba despues con la clave correcta, sin
// via de arreglo. El modo de fallo no es de laboratorio: es una escritura del
// hash cortada a medias, un copiar/pegar que se come el ultimo caracter, un
// `scp` interrumpido.
func TestProvisionRechazaUnHashTruncado(t *testing.T) {
	const clave = "una-clave-de-operador-larga"

	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)

	completo, err := cripto.Bcrypt{}.Hash(clave)
	if err != nil {
		t.Fatalf("hashear: %v", err)
	}
	truncado := completo[:len(completo)-1]

	_, err = atender(mudo())(t.Context(), peticion{
		Orden:  ordenPrimerAdministrador,
		ID:     "usr-admin",
		Email:  "admin@redes.co",
		Nombre: "Administrador",
		Hash:   truncado,
	})
	if !errors.Is(err, aplicacion.ErrUsuarioInvalido) {
		t.Fatalf("err = %v, se esperaba ErrUsuarioInvalido", err)
	}

	// Y la base sigue vacia: rechazar tarde, despues de escribir, seria el
	// mismo callejon sin salida, porque la provision no corre dos veces.
	pool, err := pgxpool.New(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(pool.Close)

	var total int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM usuarios`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 0 {
		t.Fatalf("usuarios = %d, se esperaba 0: no debe crearse la cuenta", total)
	}

	// La instalacion sigue provisionable con el hash bueno: el rechazo no
	// quema el unico intento que hay.
	r, err := atender(mudo())(t.Context(), peticion{
		Orden:  ordenPrimerAdministrador,
		ID:     "usr-admin",
		Email:  "admin@redes.co",
		Nombre: "Administrador",
		Hash:   completo,
	})
	if err != nil {
		t.Fatalf("provisionar con el hash completo tras el rechazo: %v", err)
	}
	if r.Estado != "creado" {
		t.Fatalf("estado = %q, se esperaba \"creado\"", r.Estado)
	}
}
