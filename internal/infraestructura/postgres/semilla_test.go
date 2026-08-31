package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// Fixtures de las pruebas de integracion.
//
// Viven aqui y no en testhelp a proposito: testhelp es ciclo de vida del
// contenedor y nada mas. En cuanto le crezca un SembrarObras, los issues que
// lo copien (#18 ingesta, #22 seed, #23 declaraciones) heredan fixtures de
// repertorio que no quieren.
//
// Los valores no son arbitrarios: el esquema los rechaza si lo son. clase
// tiene que ser 'socio' o 'administrado'; un titular persona natural necesita
// IPI no vacio; el email lleva arroba; el hash pasa de 20 caracteres; un
// usuario con rol 'titular' necesita titular_id.

const (
	// bcrypt de verdad en forma y longitud (60 caracteres). "hash" no pasa el
	// CHECK length(password_hash) >= 20.
	hashBcrypt = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

	// Las cuatro obras cubren las tres clausulas de repertorio.Declaracion.
	obraCompleta       = "obra-completa"
	obraIncompleta     = "obra-incompleta"
	obraSinDeclaracion = "obra-sin-declaracion"
	obraSinIPI         = "obra-sin-ipi"

	usuarioAdmin   = "usr-admin"
	usuarioTitular = "usr-titular"
	emailAdmin     = "admin@redes.co"
	emailTitular   = "ana@redes.co"
	titularAna     = "tit-ana"
	titularBeto    = "tit-beto"
)

// sembrar deja la base con el juego de datos completo y devuelve el Store.
//
// Cada prueba arranca de una base recien restaurada, asi que no hay orden ni
// limpieza que gestionar.
func sembrar(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	pool := testhelp.Pool(t)
	ctx := t.Context()

	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar (%s): %v", sql, err)
		}
	}

	ejecutar(`INSERT INTO titulares (id, nombre, ipi, persona_natural, clase, email)
	          VALUES ($1, 'Ana Escritora', 'IPI-00000001', TRUE, 'socio', $2)`,
		titularAna, emailTitular)
	ejecutar(`INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
	          VALUES ($1, 'Beto Libretista', 'IPI-00000002', TRUE, 'administrado')`,
		titularBeto)

	// El administrador va sin titular_id (NULL). Es el que prueba que el
	// COALESCE de afiliacion.go devuelve "" y no revienta el escaneo.
	ejecutar(`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
	          VALUES ($1, $2, 'Admin', 'administrador', $3)`,
		usuarioAdmin, emailAdmin, hashBcrypt)
	ejecutar(`INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash)
	          VALUES ($1, $2, 'Ana Escritora', 'titular', $3, $4)`,
		usuarioTitular, emailTitular, titularAna, hashBcrypt)

	ejecutar(`INSERT INTO obras (id, titulo, ida, eidr, imdb, tipo) VALUES
	            ($1, 'La Casa de las Dos Palmas', 'IDA-1', 'EIDR-1', 'tt0001', 'serie'),
	            ($2, 'Cronica de una Muerte',     '',      '',       '',       'cinematografica'),
	            ($3, 'Obra Huerfana',             '',      '',       '',       'unitario'),
	            ($4, 'Obra Sin IPI',              '',      '',       '',       'telenovela')`,
		obraCompleta, obraIncompleta, obraSinDeclaracion, obraSinIPI)

	// Suma exacta 100 con IPI en las dos partes: completa.
	ejecutar(`INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje) VALUES
	            ($1, $2, 'IPI-00000001', 60.0000),
	            ($1, $3, 'IPI-00000002', 40.0000)`,
		obraCompleta, titularAna, titularBeto)

	// Suma 60: incompleta. No se reparte nada de esta obra, se retiene el
	// total en reserva (R-04, RD 13.1.3).
	ejecutar(`INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje)
	          VALUES ($1, $2, 'IPI-00000001', 60.0000)`,
		obraIncompleta, titularAna)

	// Suma exacta 100 pero a una parte le falta el IPI. Es el caso que un
	// SUM(porcentaje) = 100 en SQL daria por bueno y el dominio no.
	ejecutar(`INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje) VALUES
	            ($1, $2, 'IPI-00000001', 60.0000),
	            ($1, $3, '',             40.0000)`,
		obraSinIPI, titularAna, titularBeto)

	// obraSinDeclaracion se queda sin filas en declaraciones a proposito.

	return &Store{pool: pool}, pool
}
