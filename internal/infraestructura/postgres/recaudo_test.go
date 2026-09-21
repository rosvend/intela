package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

const (
	pagadorCaracol = "caracol"
	pagadorNetflix = "netflix"
)

// sembrarRecaudo deja dos pagadores dados de alta. Las bolsas las crea cada
// prueba: lo que se prueba aqui es justamente la escritura.
func sembrarRecaudo(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	s, pool := sembrar(t)
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("sembrar recaudo (%s): %v", sql, err)
		}
	}
	ejecutar(`INSERT INTO usuarios_recaudo (id, nombre, nit, categoria) VALUES
	            ($1, 'Caracol Television S.A.', '860025674-2', 'tv_abierta'),
	            ($2, 'Netflix Colombia',        '',            'medios_digitales')`,
		pagadorCaracol, pagadorNetflix)

	return s, pool
}

func bolsa(usuario, periodo string, c recaudo.Circuito, bruto string) aplicacion.BolsaPersistida {
	return aplicacion.BolsaPersistida{
		ID:        "bolsa-" + usuario + "-" + periodo + "-" + string(c),
		UsuarioID: usuario,
		Periodo:   periodo,
		Circuito:  c,
		Bruto:     decimal.RequireFromString(bruto),
		Convenio:  "conv-" + usuario,
		Tarifa:    "RT-VI-2026",
		Factura:   "FE-1024",
	}
}

func ahoraDePrueba() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func TestRegistrarBolsaYLeerla(t *testing.T) {
	s, _ := sembrarRecaudo(t)
	b := bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00")

	if err := s.RegistrarBolsa(t.Context(), b, ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarBolsa: %v", err)
	}

	leida, err := s.BolsaPorID(t.Context(), b.ID)
	if err != nil {
		t.Fatalf("BolsaPorID: %v", err)
	}
	if leida.UsuarioID != b.UsuarioID || leida.Periodo != b.Periodo || leida.Circuito != b.Circuito {
		t.Fatalf("bolsa leida = %+v, se esperaba %+v", leida, b)
	}
	// Igualdad decimal, no de cadena: NUMERIC(18,2) devuelve "1000000.00" y el
	// dominio pudo recibir "1000000".
	if !leida.Bruto.Equal(b.Bruto) {
		t.Fatalf("bruto = %s, se esperaba %s", leida.Bruto, b.Bruto)
	}
	// La procedencia es lo que responde la pregunta 1 del ADR 0006. Si no
	// vuelve de la base, la cifra no es explicable hasta su origen.
	if leida.Convenio != b.Convenio || leida.Tarifa != b.Tarifa || leida.Factura != b.Factura {
		t.Fatalf("procedencia = %q/%q/%q, se esperaba %q/%q/%q",
			leida.Convenio, leida.Tarifa, leida.Factura, b.Convenio, b.Tarifa, b.Factura)
	}
}

func TestRegistrarBolsaDejaAsientoEnLaMismaTransaccion(t *testing.T) {
	s, pool := sembrarRecaudo(t)
	b := bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00")
	ahora := ahoraDePrueba()

	if err := s.RegistrarBolsa(t.Context(), b, ahora, usuarioAdmin); err != nil {
		t.Fatalf("RegistrarBolsa: %v", err)
	}

	var hecho, refTipo, refID, actor string
	err := pool.QueryRow(t.Context(),
		`SELECT hecho, ref_tipo, ref_id, actor_id FROM asientos WHERE ref_id = $1`, b.ID).
		Scan(&hecho, &refTipo, &refID, &actor)
	if err != nil {
		t.Fatalf("leer el asiento: %v", err)
	}
	if hecho != "recaudo.registrado" || refTipo != "bolsa" || actor != usuarioAdmin {
		t.Fatalf("asiento = %q/%q/%q, no describe el alta de la bolsa", hecho, refTipo, actor)
	}
}

func TestRegistrarBolsaDuplicadaEsCentinela(t *testing.T) {
	s, _ := sembrarRecaudo(t)
	b := bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00")

	if err := s.RegistrarBolsa(t.Context(), b, ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("primer alta: %v", err)
	}

	// Mismo usuario, periodo y circuito con OTRO id: el duplicado que importa
	// no es el de la clave primaria sino el del negocio. Cargar dos veces el
	// reporte de recaudo de un periodo repartiria ese dinero dos veces.
	repetida := b
	repetida.ID = "bolsa-otra-id"
	repetida.Bruto = decimal.RequireFromString("999.00")

	err := s.RegistrarBolsa(t.Context(), repetida, ahoraDePrueba(), usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrBolsaDuplicada) {
		t.Fatalf("err = %v, se esperaba ErrBolsaDuplicada", err)
	}
}

func TestNacionalEInternacionalDelMismoPeriodoConviven(t *testing.T) {
	// RD 10.3 / R-35: son dos circuitos separados, no una etiqueta. El UNIQUE
	// tiene que dejar pasar los dos del mismo usuario y periodo.
	s, _ := sembrarRecaudo(t)

	if err := s.RegistrarBolsa(t.Context(), bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00"), ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("bolsa nacional: %v", err)
	}
	if err := s.RegistrarBolsa(t.Context(), bolsa(pagadorCaracol, "2025-01", recaudo.Internacional, "200000.00"), ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("bolsa internacional: %v", err)
	}

	bolsas, err := s.BolsasDePeriodo(t.Context(), "2025-01")
	if err != nil {
		t.Fatalf("BolsasDePeriodo: %v", err)
	}
	if len(bolsas) != 2 {
		t.Fatalf("bolsas = %d, se esperaban 2 (una por circuito)", len(bolsas))
	}
}

func TestRegistrarBolsaDeUsuarioInexistenteEsCentinela(t *testing.T) {
	// Sin la FK, un typo en `usuario_id` crea un pagador fantasma y el reparto
	// atribuye a un canal que no existe dinero que alguien pago de verdad.
	s, _ := sembrarRecaudo(t)
	b := bolsa("carcaol", "2025-01", recaudo.Nacional, "1000000.00")

	err := s.RegistrarBolsa(t.Context(), b, ahoraDePrueba(), usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrUsuarioRecaudoInexistente) {
		t.Fatalf("err = %v, se esperaba ErrUsuarioRecaudoInexistente", err)
	}
}

func TestBolsaPorIDInexistenteEsNoEncontrado(t *testing.T) {
	s, _ := sembrarRecaudo(t)
	_, err := s.BolsaPorID(t.Context(), "bolsa-que-no-esta")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

func TestListarBolsasVieneOrdenadoYCompleto(t *testing.T) {
	s, _ := sembrarRecaudo(t)

	// A proposito en orden inverso al alfabetico: el ADR 0005 exige que una
	// corrida se reproduzca bit a bit, y PostgreSQL no promete ningun orden.
	for _, b := range []aplicacion.BolsaPersistida{
		bolsa(pagadorNetflix, "2025-02", recaudo.Nacional, "500000.00"),
		bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00"),
	} {
		if err := s.RegistrarBolsa(t.Context(), b, ahoraDePrueba(), usuarioAdmin); err != nil {
			t.Fatalf("RegistrarBolsa: %v", err)
		}
	}

	bolsas, err := s.ListarBolsas(t.Context())
	if err != nil {
		t.Fatalf("ListarBolsas: %v", err)
	}
	if len(bolsas) != 2 {
		t.Fatalf("bolsas = %d, se esperaban 2", len(bolsas))
	}
	if bolsas[0].ID > bolsas[1].ID {
		t.Fatalf("sin orden explicito: %q antes que %q", bolsas[0].ID, bolsas[1].ID)
	}
}

func TestBolsasDePeriodoNoDevuelveOtrosPeriodos(t *testing.T) {
	s, _ := sembrarRecaudo(t)

	if err := s.RegistrarBolsa(t.Context(), bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00"), ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarBolsa: %v", err)
	}
	if err := s.RegistrarBolsa(t.Context(), bolsa(pagadorNetflix, "2025-02", recaudo.Nacional, "500000.00"), ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarBolsa: %v", err)
	}

	bolsas, err := s.BolsasDePeriodo(t.Context(), "2025-01")
	if err != nil {
		t.Fatalf("BolsasDePeriodo: %v", err)
	}
	if len(bolsas) != 1 || bolsas[0].UsuarioID != pagadorCaracol {
		t.Fatalf("bolsas = %+v, se esperaba solo la de %s", bolsas, pagadorCaracol)
	}
}

func TestBolsasDePeriodoSinCoincidenciasDevuelveVacio(t *testing.T) {
	s, _ := sembrarRecaudo(t)
	bolsas, err := s.BolsasDePeriodo(t.Context(), "1999")
	if err != nil {
		t.Fatalf("BolsasDePeriodo: %v", err)
	}
	if len(bolsas) != 0 {
		t.Fatalf("bolsas = %+v, se esperaba vacio y no un error", bolsas)
	}
}

func TestRegistrarUsuarioYListarlos(t *testing.T) {
	s, _ := sembrarRecaudo(t)

	u, err := recaudo.NuevoUsuario("procinal", recaudo.Datos{
		Nombre:    "Procinal S.A.",
		NIT:       "800123456-1",
		Categoria: recaudo.Cine,
	})
	if err != nil {
		t.Fatalf("NuevoUsuario: %v", err)
	}
	if err := s.RegistrarUsuario(t.Context(), u, ahoraDePrueba(), usuarioAdmin); err != nil {
		t.Fatalf("RegistrarUsuario: %v", err)
	}

	usuarios, err := s.ListarUsuarios(t.Context())
	if err != nil {
		t.Fatalf("ListarUsuarios: %v", err)
	}
	if len(usuarios) != 3 {
		t.Fatalf("usuarios = %d, se esperaban 3", len(usuarios))
	}
	// Se reconstruye con el mismo constructor del dominio que lo creo, asi que
	// lo que vuelve de la base cumple las mismas invariantes.
	var visto bool
	for _, x := range usuarios {
		if x.ID() == "procinal" {
			visto = true
			if x.Datos().Categoria != recaudo.Cine || x.Datos().NIT != "800123456-1" {
				t.Fatalf("usuario leido = %+v", x.Datos())
			}
		}
	}
	if !visto {
		t.Fatal("el usuario dado de alta no esta en el listado")
	}
}

func TestRegistrarUsuarioDuplicadoNoEsUn500Generico(t *testing.T) {
	s, _ := sembrarRecaudo(t)

	u, err := recaudo.NuevoUsuario(pagadorCaracol, recaudo.Datos{
		Nombre:    "Caracol otra vez",
		Categoria: recaudo.TVAbierta,
	})
	if err != nil {
		t.Fatalf("NuevoUsuario: %v", err)
	}
	if err := s.RegistrarUsuario(t.Context(), u, ahoraDePrueba(), usuarioAdmin); !errors.Is(err, aplicacion.ErrUsuarioDeRecaudoDuplicado) {
		t.Fatalf("err = %v, se esperaba ErrUsuarioDeRecaudoDuplicado", err)
	}
}

func TestElEsquemaRechazaLoQueElDominioRechaza(t *testing.T) {
	// El dominio y el CHECK tienen que cerrar el MISMO conjunto. Si alguien
	// anade una categoria o un circuito al enum de Go sin tocar la migracion,
	// la prueba de dominio sigue verde y la insercion revienta en produccion;
	// esta es la que empareja las dos orillas.
	_, pool := sembrarRecaudo(t)

	casos := []struct {
		nombre string
		sql    string
		args   []any
	}{
		{
			"categoria fuera del enum",
			`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('x', 'X', 'radio')`,
			nil,
		},
		{
			"nombre en blanco",
			`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('x', '   ', 'cine')`,
			nil,
		},
		{
			"circuito fuera del enum",
			`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
			 VALUES ('b', $1, '2025-01', 'regional', 1000)`,
			[]any{pagadorCaracol},
		},
		{
			"bruto negativo",
			`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
			 VALUES ('b', $1, '2025-01', 'nacional', -1)`,
			[]any{pagadorCaracol},
		},
		{
			"periodo mal formado",
			`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
			 VALUES ('b', $1, 'enero', 'nacional', 1000)`,
			[]any{pagadorCaracol},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), c.sql, c.args...); err == nil {
				t.Fatal("el esquema acepto una fila que el dominio rechaza")
			}
		})
	}
}

func TestTodasLasCategoriasDelDominioCabenEnElCheck(t *testing.T) {
	// La otra direccion del emparejamiento: una categoria valida para el
	// dominio que el CHECK rechace deja un alta imposible de hacer.
	_, pool := sembrarRecaudo(t)

	for i, c := range recaudo.CategoriasUsuario() {
		t.Run(string(c), func(t *testing.T) {
			_, err := pool.Exec(t.Context(),
				`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ($1, 'X', $2)`,
				decimal.NewFromInt(int64(i)).String(), string(c))
			if err != nil {
				t.Fatalf("el CHECK rechaza la categoria %q del dominio: %v", c, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegistrarBolsa DENTRO de la unidad de otro puerto (ronda 2 de la revision
// de PR #134: "los metodos que van por s.pool se escapan de la unidad").
//
// RegistrarBolsa no es del catalogo -es el primer puerto NO relacionado con
// #91 que se prueba anidado a proposito-. Antes de esta ronda abria SIEMPRE
// su propia transaccion con [Store.EnTransaccion] (`s.pool.Begin` directo,
// sin mirar el contexto): confirmaba SOLA, sin importar como terminara la
// unidad de quien la llamara, y ademas pedia una conexion propia del pool
// para hacerlo. Las dos pruebas de abajo prueban esas dos mitades por
// separado. Corridas contra esa version anterior, LAS DOS fallan -la primera
// porque la bolsa sobrevive a un rollback que no debia sobrevivir, la
// segunda porque se queda esperando una conexion que nunca se libera y
// revienta con el timeout del contexto-.

// Si la unidad de fuera falla DESPUES de que RegistrarBolsa ya escribio, ni
// la bolsa ni su asiento pueden quedar en la base: son commits de la MISMA
// transaccion o no son ninguno.
func TestRegistrarBolsaDentroDeUnaUnidadRevierteConLaDeFuera(t *testing.T) {
	s, _ := sembrarRecaudo(t)
	ctx := t.Context()
	b := bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00")
	fallo := errors.New("el caso de uso de fuera se arrepintio")

	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		if err := s.RegistrarBolsa(ctx, b, ahoraDePrueba(), usuarioAdmin); err != nil {
			return err
		}
		return fallo
	})
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error de la unidad de fuera, se obtuvo %v", err)
	}

	if _, err := s.BolsaPorID(ctx, b.ID); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("la bolsa sobrevivio al rollback de la unidad de fuera: %v", err)
	}
	var asientos int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM asientos WHERE ref_id = $1`, b.ID).Scan(&asientos); err != nil {
		t.Fatalf("contar asientos: %v", err)
	}
	if asientos != 0 {
		t.Fatalf("el asiento de la bolsa sobrevivio al rollback: %d asientos", asientos)
	}
}

// La otra mitad del mismo defecto: no solo que la escritura anidada tenia
// que confirmar aparte, sino que ademas pedia una SEGUNDA conexion del pool
// para hacerlo. Se prueba con un pool de UNA sola conexion: la unidad de
// fuera ya se queda con la unica que hay, asi que si RegistrarBolsa pidiera
// la suya se quedaria esperando una conexion que jamas se libera -porque
// quien la tiene esta, en el mismo goroutine, esperando a que ESTA llamada
// termine-. Es un auto-interbloqueo de un solo goroutine, mas facil de forzar
// que uno entre dos goroutines y por eso mas determinista: no depende de
// ganar ninguna carrera, un pool de tamano 1 lo fuerza siempre.
//
// El contexto lleva un timeout de 5 s para que, contra la version que no
// reutilizaba la conexion, la prueba falle limpio en vez de colgarse.
func TestRegistrarBolsaDentroDeUnaUnidadNoPideSegundaConexion(t *testing.T) {
	_, pool := sembrarRecaudo(t)

	cfg := pool.Config().Copy()
	cfg.MaxConns = 1
	unaConexion, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("abrir pool de una conexion: %v", err)
	}
	defer unaConexion.Close()
	s1 := Nuevo(unaConexion)

	ctx, cancelar := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelar()

	b := bolsa(pagadorCaracol, "2025-01", recaudo.Nacional, "1000000.00")
	err = s1.EnUnidad(ctx, func(ctx context.Context) error {
		return s1.RegistrarBolsa(ctx, b, ahoraDePrueba(), usuarioAdmin)
	})
	if err != nil {
		t.Fatalf("RegistrarBolsa anidado con pool de una conexion: %v", err)
	}

	leida, err := s1.BolsaPorID(t.Context(), b.ID)
	if err != nil {
		t.Fatalf("BolsaPorID: %v", err)
	}
	if leida.ID != b.ID {
		t.Fatalf("bolsa leida = %+v, se esperaba %q", leida, b.ID)
	}
}
