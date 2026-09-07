package main

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
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
