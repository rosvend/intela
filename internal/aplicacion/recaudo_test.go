package aplicacion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// gestionRecaudoFalsa cuenta lo que recibe. Los contadores son lo que permite
// comprobar que una bolsa que el dominio rechaza no llega NUNCA al puerto.
type gestionRecaudoFalsa struct {
	bolsas        []BolsaPersistida
	usuarios      []recaudo.Usuario
	ahoraRecibido time.Time
	actorRecibido string
	err           error
}

func (g *gestionRecaudoFalsa) RegistrarBolsa(_ context.Context, b BolsaPersistida, ahora time.Time, actorID string) error {
	g.ahoraRecibido, g.actorRecibido = ahora, actorID
	if g.err != nil {
		return g.err
	}
	g.bolsas = append(g.bolsas, b)
	return nil
}

func (g *gestionRecaudoFalsa) RegistrarUsuario(_ context.Context, u recaudo.Usuario, ahora time.Time, actorID string) error {
	g.ahoraRecibido, g.actorRecibido = ahora, actorID
	if g.err != nil {
		return g.err
	}
	g.usuarios = append(g.usuarios, u)
	return nil
}

type repoRecaudoFalso struct {
	bolsas          []BolsaPersistida
	usuarios        []recaudo.Usuario
	periodoRecibido string
	llamadasListar  int
	err             error
}

func (r *repoRecaudoFalso) ListarBolsas(context.Context) ([]BolsaPersistida, error) {
	r.llamadasListar++
	return r.bolsas, r.err
}

func (r *repoRecaudoFalso) BolsasDePeriodo(_ context.Context, periodo string) ([]BolsaPersistida, error) {
	r.llamadasListar++
	r.periodoRecibido = periodo
	return r.bolsas, r.err
}

func (r *repoRecaudoFalso) BolsaPorID(context.Context, string) (BolsaPersistida, error) {
	if r.err != nil {
		return BolsaPersistida{}, r.err
	}
	return r.bolsas[0], nil
}

func (r *repoRecaudoFalso) ListarUsuarios(context.Context) ([]recaudo.Usuario, error) {
	return r.usuarios, r.err
}

func bolsaDePrueba() BolsaPersistida {
	return BolsaPersistida{
		ID:        "bolsa-caracol-2025-01-nacional",
		UsuarioID: "caracol",
		Periodo:   "2025-01",
		Circuito:  recaudo.Nacional,
		Bruto:     decimal.RequireFromString("1000000.00"),
		Convenio:  "conv-caracol-2025",
		Factura:   "FE-1024",
	}
}

const instante = "2026-09-13T10:00:00Z"

func relojDePrueba(t *testing.T) relojFijo {
	t.Helper()
	i, err := time.Parse(time.RFC3339, instante)
	if err != nil {
		t.Fatalf("parsear instante: %v", err)
	}
	return relojFijo{instante: i}
}

func TestRegistrarPasaLaBolsaYElInstanteDelReloj(t *testing.T) {
	gestion := &gestionRecaudoFalsa{}
	r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

	b, err := r.Registrar(t.Context(), bolsaDePrueba(), "usr-contabilidad")
	if err != nil {
		t.Fatalf("Registrar: %v", err)
	}
	if len(gestion.bolsas) != 1 {
		t.Fatalf("bolsas escritas = %d, se esperaba 1", len(gestion.bolsas))
	}
	if b.ID != "bolsa-caracol-2025-01-nacional" || !b.Bruto.Equal(decimal.RequireFromString("1000000.00")) {
		t.Fatalf("bolsa devuelta = %+v", b)
	}
	// El instante entra por el puerto Reloj y no por time.Now(): sin eso, una
	// corrida no se reproduce (ADR 0005) y el asiento no se puede fechar en
	// una prueba.
	if !gestion.ahoraRecibido.Equal(relojDePrueba(t).Ahora()) {
		t.Fatalf("ahora = %v, se esperaba el del reloj inyectado", gestion.ahoraRecibido)
	}
	if gestion.actorRecibido != "usr-contabilidad" {
		t.Fatalf("actor = %q, se esperaba usr-contabilidad", gestion.actorRecibido)
	}
}

func TestRegistrarNormalizaAntesDeEscribir(t *testing.T) {
	gestion := &gestionRecaudoFalsa{}
	r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

	entrada := bolsaDePrueba()
	entrada.UsuarioID = "  caracol  "
	entrada.Periodo = " 2025-01 "

	b, err := r.Registrar(t.Context(), entrada, "usr-contabilidad")
	if err != nil {
		t.Fatalf("Registrar: %v", err)
	}
	if b.UsuarioID != "caracol" || b.Periodo != "2025-01" {
		t.Fatalf("bolsa = %+v, se esperaba normalizada por el dominio", b)
	}
	if gestion.bolsas[0].UsuarioID != "caracol" {
		t.Fatalf("al puerto llego %q sin normalizar", gestion.bolsas[0].UsuarioID)
	}
}

func TestRegistrarUnaBolsaInvalidaNoTocaElPuerto(t *testing.T) {
	casos := map[string]func(*BolsaPersistida){
		"sin id":               func(b *BolsaPersistida) { b.ID = "" },
		"sin usuario":          func(b *BolsaPersistida) { b.UsuarioID = "" },
		"periodo mal formado":  func(b *BolsaPersistida) { b.Periodo = "enero" },
		"circuito inventado":   func(b *BolsaPersistida) { b.Circuito = recaudo.Circuito("regional") },
		"bruto negativo":       func(b *BolsaPersistida) { b.Bruto = decimal.RequireFromString("-1") },
		"bruto con centesimas": func(b *BolsaPersistida) { b.Bruto = decimal.RequireFromString("100.555") },
	}

	for nombre, romper := range casos {
		t.Run(nombre, func(t *testing.T) {
			gestion := &gestionRecaudoFalsa{}
			r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

			entrada := bolsaDePrueba()
			romper(&entrada)

			if _, err := r.Registrar(t.Context(), entrada, "usr-contabilidad"); err == nil {
				t.Fatal("se esperaba error")
			}
			if len(gestion.bolsas) != 0 {
				t.Fatal("se intento escribir una bolsa que el dominio rechaza")
			}
		})
	}
}

func TestRegistrarSinIDEsErrBolsaInvalida(t *testing.T) {
	// El id no lo valida el dominio -reparto no lo necesita- pero sin el la
	// bolsa no se puede referenciar desde `procesos` ni desde un asiento.
	r := Recaudo{Gestion: &gestionRecaudoFalsa{}, Reloj: relojDePrueba(t)}
	entrada := bolsaDePrueba()
	entrada.ID = "   "

	_, err := r.Registrar(t.Context(), entrada, "usr-contabilidad")
	if !errors.Is(err, recaudo.ErrBolsaInvalida) {
		t.Fatalf("err = %v, se esperaba ErrBolsaInvalida", err)
	}
}

func TestRegistrarRechazaUnIDQueNoCabeEnUnaRuta(t *testing.T) {
	// El id viene del reporte de recaudo de REDES y termina en `/bolsas/{id}`.
	// Uno con barra deja la fila ESCRITA Y NO LEIBLE: el router solo casa un
	// segmento, asi que GET /bolsas/reporte/2025 da 404 y la bolsa no se puede
	// consultar por id nunca mas.
	casos := map[string]string{
		"con barra":        "reporte/2025",
		"con barra final":  "bolsa-1/",
		"con interrogante": "bolsa?1",
		"con almohadilla":  "bolsa#1",
		"solo una barra":   "/",
	}

	for nombre, id := range casos {
		t.Run(nombre, func(t *testing.T) {
			gestion := &gestionRecaudoFalsa{}
			r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

			entrada := bolsaDePrueba()
			entrada.ID = id

			if _, err := r.Registrar(t.Context(), entrada, "usr-contabilidad"); !errors.Is(err, recaudo.ErrBolsaInvalida) {
				t.Fatalf("err = %v, se esperaba ErrBolsaInvalida", err)
			}
			if len(gestion.bolsas) != 0 {
				t.Fatal("se escribio una bolsa que no se va a poder leer por id")
			}
		})
	}
}

func TestRegistrarDevuelveElCentinelaDelPuertoSinTaparlo(t *testing.T) {
	// Quien llama lo distingue con errors.Is para responder 409 en vez de 500.
	gestion := &gestionRecaudoFalsa{err: ErrBolsaDuplicada}
	r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

	_, err := r.Registrar(t.Context(), bolsaDePrueba(), "usr-contabilidad")
	if !errors.Is(err, ErrBolsaDuplicada) {
		t.Fatalf("err = %v, se esperaba ErrBolsaDuplicada", err)
	}
}

func TestListarSinPeriodoPideTodasLasBolsas(t *testing.T) {
	repo := &repoRecaudoFalso{bolsas: []BolsaPersistida{bolsaDePrueba()}}
	r := Recaudo{Bolsas: repo}

	bolsas, err := r.Listar(t.Context(), "")
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(bolsas) != 1 {
		t.Fatalf("bolsas = %d, se esperaba 1", len(bolsas))
	}
	if repo.periodoRecibido != "" {
		t.Fatalf("se filtro por el periodo %q sin que nadie lo pidiera", repo.periodoRecibido)
	}
}

func TestListarConPeriodoFiltra(t *testing.T) {
	repo := &repoRecaudoFalso{bolsas: []BolsaPersistida{bolsaDePrueba()}}
	r := Recaudo{Bolsas: repo}

	if _, err := r.Listar(t.Context(), " 2025-01 "); err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if repo.periodoRecibido != "2025-01" {
		t.Fatalf("periodo = %q, se esperaba 2025-01 recortado", repo.periodoRecibido)
	}
}

func TestListarConPeriodoMalFormadoNoTocaElPuerto(t *testing.T) {
	// Ignorarlo devolveria el listado entero, y quien pregunta lo leeria como
	// "no hay ninguna de ese periodo". Es el mismo criterio que BuscarObras
	// aplica a `anio`.
	//
	// Los meses imposibles son el caso que de verdad enganaba: `2025-13` pasaba
	// el patron de esta capa -que usa [0-9]{2}- y devolvia una lista VACIA con
	// 200, que se lee como "ese mes no tuvo recaudo" en vez de "ese mes no
	// existe". Por eso la validacion es ahora la del dominio, la misma que
	// aplica NuevaBolsa, y no una copia de este paquete.
	casos := []string{"enero de 2025", "2025-1", "2025-01-15", "2025-00", "2025-13", "2025-99"}

	for _, periodo := range casos {
		t.Run(periodo, func(t *testing.T) {
			repo := &repoRecaudoFalso{}
			r := Recaudo{Bolsas: repo}

			if _, err := r.Listar(t.Context(), periodo); !errors.Is(err, recaudo.ErrBolsaInvalida) {
				t.Fatalf("err = %v, se esperaba ErrBolsaInvalida", err)
			}
			if repo.llamadasListar != 0 {
				t.Fatal("se consulto la base con un periodo que no es un periodo")
			}
		})
	}
}

func TestListarAceptaLosPeriodosQueAceptaElDominio(t *testing.T) {
	// La otra direccion: el filtro no puede ser mas estricto que el
	// constructor, o habria bolsas que se pueden escribir y no consultar.
	for _, periodo := range []string{"2025", "2025-01", "2025-12"} {
		t.Run(periodo, func(t *testing.T) {
			repo := &repoRecaudoFalso{}
			r := Recaudo{Bolsas: repo}

			if _, err := r.Listar(t.Context(), periodo); err != nil {
				t.Fatalf("Listar(%q): %v", periodo, err)
			}
			if repo.periodoRecibido != periodo {
				t.Fatalf("periodo recibido = %q, se esperaba %q", repo.periodoRecibido, periodo)
			}
		})
	}
}

func TestRegistrarUsuarioValidaAntesDeEscribir(t *testing.T) {
	gestion := &gestionRecaudoFalsa{}
	r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

	u, err := recaudo.NuevoUsuario("caracol", recaudo.Datos{
		Nombre:    "Caracol Television S.A.",
		Categoria: recaudo.TVAbierta,
	})
	if err != nil {
		t.Fatalf("NuevoUsuario: %v", err)
	}

	if _, err := r.RegistrarUsuario(t.Context(), u, "usr-admin"); err != nil {
		t.Fatalf("RegistrarUsuario: %v", err)
	}
	if len(gestion.usuarios) != 1 || gestion.usuarios[0].ID() != "caracol" {
		t.Fatalf("usuarios escritos = %+v", gestion.usuarios)
	}
	if gestion.actorRecibido != "usr-admin" {
		t.Fatalf("actor = %q, se esperaba usr-admin", gestion.actorRecibido)
	}
}

func TestRegistrarUsuarioSinConstruirEsInvalido(t *testing.T) {
	// El cero de recaudo.Usuario no tiene id ni categoria: dejarlo pasar
	// escribiria una fila que el CHECK de la tabla rechaza, y el error saldria
	// como un 500 generico en vez de como un dato mal formado.
	gestion := &gestionRecaudoFalsa{}
	r := Recaudo{Gestion: gestion, Reloj: relojDePrueba(t)}

	_, err := r.RegistrarUsuario(t.Context(), recaudo.Usuario{}, "usr-admin")
	if !errors.Is(err, recaudo.ErrUsuarioInvalido) {
		t.Fatalf("err = %v, se esperaba ErrUsuarioInvalido", err)
	}
	if len(gestion.usuarios) != 0 {
		t.Fatal("se intento escribir un usuario que el dominio rechaza")
	}
}
