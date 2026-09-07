package postgres

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// primerAdmin es el usuario que provisiona una instalacion vacia.
var primerAdmin = aplicacion.Usuario{
	ID:     "usr-bootstrap",
	Email:  "bootstrap@redes.co",
	Nombre: "Bootstrap",
	Rol:    aplicacion.RolAdministrador,
}

// vacia devuelve un Store contra una base migrada y SIN usuarios.
//
// No usa sembrar(): esa helper inserta dos usuarios, que es justo la
// precondicion que estas pruebas necesitan que NO se cumpla.
func vacia(t *testing.T) *Store {
	t.Helper()
	return &Store{pool: testhelp.Pool(t)}
}

func TestCrearPrimerAdministradorEnUnaBaseVacia(t *testing.T) {
	s := vacia(t)

	if err := s.CrearPrimerAdministrador(t.Context(), primerAdmin, hashBcrypt); err != nil {
		t.Fatalf("CrearPrimerAdministrador: %v", err)
	}

	// Se comprueba por el mismo camino que usa el login, no con un SELECT
	// propio: lo que importa no es que haya una fila, es que esa fila sirva
	// para entrar.
	u, hash, err := s.UsuarioPorEmail(t.Context(), primerAdmin.Email)
	if err != nil {
		t.Fatalf("UsuarioPorEmail tras provisionar: %v", err)
	}
	if u.ID != primerAdmin.ID {
		t.Errorf("ID = %q, se esperaba %q", u.ID, primerAdmin.ID)
	}
	if u.Rol != aplicacion.RolAdministrador {
		t.Errorf("Rol = %q, se esperaba %q", u.Rol, aplicacion.RolAdministrador)
	}
	if hash != hashBcrypt {
		t.Errorf("hash = %q, se esperaba el que se paso", hash)
	}
	// titular_id NULL -> "" por el COALESCE de columnasUsuario.
	if u.TitularID != "" {
		t.Errorf("TitularID = %q, se esperaba vacio", u.TitularID)
	}
}

func TestCrearPrimerAdministradorSeNiegaSiYaHayAlguien(t *testing.T) {
	// sembrar() deja dos usuarios: la instalacion ya esta provisionada.
	s, pool := sembrar(t)

	err := s.CrearPrimerAdministrador(t.Context(), primerAdmin, hashBcrypt)
	if !errors.Is(err, aplicacion.ErrYaHayUsuarios) {
		t.Fatalf("err = %v, se esperaba ErrYaHayUsuarios", err)
	}

	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM usuarios WHERE id = $1`, primerAdmin.ID).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Errorf("filas insertadas = %d, se esperaba 0: el rechazo no debe escribir", n)
	}
}

// La invariante es "exactamente uno, nunca dos", y la unica forma de que sea
// cierta es que la comprobacion y la escritura sean la MISMA sentencia. Esta
// prueba fija el efecto: la segunda invocacion no crea una cuenta mas.
func TestCrearPrimerAdministradorEsDeUnaSolaVez(t *testing.T) {
	s := vacia(t)

	if err := s.CrearPrimerAdministrador(t.Context(), primerAdmin, hashBcrypt); err != nil {
		t.Fatalf("primera invocacion: %v", err)
	}

	// Segundo intento con OTRO id y OTRO email: si la guarda fuera la clave
	// primaria o el UNIQUE del email en vez del "solo si esta vacia", este
	// pasaria y la instalacion acabaria con dos administradores.
	otro := aplicacion.Usuario{
		ID:     "usr-segundo",
		Email:  "segundo@redes.co",
		Nombre: "Segundo",
		Rol:    aplicacion.RolAdministrador,
	}
	err := s.CrearPrimerAdministrador(t.Context(), otro, hashBcrypt)
	if !errors.Is(err, aplicacion.ErrYaHayUsuarios) {
		t.Fatalf("segunda invocacion: err = %v, se esperaba ErrYaHayUsuarios", err)
	}

	var total int
	if err := s.pool.QueryRow(t.Context(), `SELECT count(*) FROM usuarios`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 1 {
		t.Errorf("usuarios = %d, se esperaba 1", total)
	}
}

// Un titular tambien cuenta como "ya hay usuarios". La guarda mira la tabla
// entera y no el rol: si mirara solo administradores, una instalacion con
// titulares cargados aceptaria una cuenta de administrador nueva por esta via.
func TestCrearPrimerAdministradorCuentaCualquierRol(t *testing.T) {
	s := vacia(t)
	ctx := t.Context()

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
		 VALUES ('tit-x', 'Titular X', 'IPI-00000009', TRUE, 'socio')`); err != nil {
		t.Fatalf("sembrar titular: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash)
		 VALUES ('usr-x', 'x@redes.co', 'X', 'titular', 'tit-x', $1)`, hashBcrypt); err != nil {
		t.Fatalf("sembrar usuario titular: %v", err)
	}

	err := s.CrearPrimerAdministrador(ctx, primerAdmin, hashBcrypt)
	if !errors.Is(err, aplicacion.ErrYaHayUsuarios) {
		t.Fatalf("err = %v, se esperaba ErrYaHayUsuarios", err)
	}
}

// La asercion de compilacion del puerto. Va aqui y no en provision.go para que
// el fichero de produccion no cargue con una declaracion que solo existe para
// que el compilador avise.
var _ aplicacion.RepositorioProvisionInicial = (*Store)(nil)

// La invariante bajo CONCURRENCIA, que es donde la primera version fallaba.
//
// El WHERE NOT EXISTS por si solo no es exclusion mutua: bajo READ COMMITTED
// -- el nivel por defecto -- la subconsulta no ve las filas insertadas y aun no
// confirmadas por otra transaccion, asi que dos invocaciones simultaneas contra
// una tabla vacia ven las dos una tabla vacia y las dos insertan. Medido antes
// del arreglo: cuatro conexiones, dos ganadoras, dos filas.
//
// Conexiones INDEPENDIENTES y no goroutines sobre el mismo pool: lo que se
// prueba es la exclusion entre transacciones distintas, y un pool podria
// servirlas por turnos y esconder el fallo. La barrera las suelta a la vez.
//
// El escenario no es teorico: este comando se diseña para el reintento de un
// operador que no sabe si la primera invocacion llego, y una invocacion
// asincrona de Lambda reintenta sola.
func TestCrearPrimerAdministradorEsExclusivoBajoConcurrencia(t *testing.T) {
	dsn := testhelp.DSN(t)
	const n = 4

	stores := make([]*Store, n)
	for i := range stores {
		pool, err := pgxpool.New(t.Context(), dsn)
		if err != nil {
			t.Fatalf("abrir pool %d: %v", i, err)
		}
		t.Cleanup(pool.Close)
		// La conexion se establece AQUI, antes de la barrera. pgxpool.New es
		// perezoso: sin este Ping, el primer Exec de cada goroutine incluye el
		// saludo TCP y el arranque de la sesion, y eso las serializa lo
		// suficiente para que la carrera no se reproduzca. Con el pool en
		// frio, esta prueba pasaba INCLUSO SIN el lock.
		if err := pool.Ping(t.Context()); err != nil {
			t.Fatalf("calentar pool %d: %v", i, err)
		}
		stores[i] = &Store{pool: pool}
	}

	var (
		barrera   = make(chan struct{})
		wg        sync.WaitGroup
		mu        sync.Mutex
		ganadoras int
		yaHabia   int
		otros     []error
	)

	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := aplicacion.Usuario{
				ID:     fmt.Sprintf("usr-%d", i),
				Email:  fmt.Sprintf("admin%d@redes.co", i),
				Nombre: "Administrador",
				Rol:    aplicacion.RolAdministrador,
			}
			<-barrera
			err := stores[i].CrearPrimerAdministrador(t.Context(), u, hashBcrypt)

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ganadoras++
			case errors.Is(err, aplicacion.ErrYaHayUsuarios):
				yaHabia++
			default:
				otros = append(otros, err)
			}
		}(i)
	}
	close(barrera)
	wg.Wait()

	for _, err := range otros {
		t.Errorf("error inesperado: %v", err)
	}
	if ganadoras != 1 {
		t.Errorf("ganadoras = %d, se esperaba 1", ganadoras)
	}
	// Las perdedoras reciben el centinela, no un fallo de serializacion: la
	// operacion ya se hizo, y para quien reintenta eso no es un error.
	if yaHabia != n-1 {
		t.Errorf("con ErrYaHayUsuarios = %d, se esperaban %d", yaHabia, n-1)
	}

	var total int
	if err := stores[0].pool.QueryRow(t.Context(), `SELECT count(*) FROM usuarios`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 1 {
		t.Fatalf("usuarios = %d, se esperaba exactamente 1", total)
	}
}
