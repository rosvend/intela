package aplicacion

import (
	"context"
	"fmt"
	"strings"

	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// Recaudo son los casos de uso del lado del ingreso: dar de alta a los
// pagadores y registrar lo que cada uno pago en un periodo.
//
// # Registra, no factura
//
// Bajo P-08 Intela RECIBE el importe ya cobrado. No calcula lo que cada
// usuario debe, no aplica la tabla `T-01` a `T-11` y no emite cuenta de cobro.
// Por eso aqui no hay ni `Tarifar` ni `Facturar`: la unica escritura de dinero
// es [Recaudo.Registrar], y lo que recibe es una cifra que ya se cobro.
//
// # Lo que este servicio NO hace
//
// No reparte. Lo unico que sale de aqui aguas abajo es la bolsa; la categoria
// del pagador, su NIT y el convenio que lo origina no bajan (ADR 0003). Si
// bajaran, alguien acabaria calculando un importe por obra a partir del
// precio, que es el modelo equivocado -- el dinero no llega por fila.
type Recaudo struct {
	Bolsas  RepositorioRecaudo
	Gestion GestionRecaudo
	Reloj   Reloj
}

// Registrar da de alta lo cobrado a un usuario en un periodo.
//
// La validacion es del dominio: [recaudo.NuevaBolsa] es la unica puerta, asi
// que no hay forma de que llegue al adaptador una bolsa con un periodo que no
// es un periodo, un circuito que no existe, o un importe negativo o con mas
// centavos de los que cabe en la columna.
//
// Devuelve ErrBolsaDuplicada si ya hay una bolsa de ese usuario, periodo y
// circuito, y ErrUsuarioRecaudoInexistente si cita un pagador que no esta.
func (r Recaudo) Registrar(ctx context.Context, b BolsaPersistida, actorID string) (BolsaPersistida, error) {
	// El id no lo valida el dominio -- al motor de reparto la bolsa le llega
	// como valor y no necesita saber de que fila salio-, pero sin el no se
	// puede referenciar desde `procesos` ni desde un asiento de la bitacora.
	// El centinela es el del dominio igualmente: lo que falla es que esto no
	// llega a ser una bolsa, y quien llama no gana nada distinguiendo cual de
	// los campos falta con un error aparte.
	b.ID = strings.TrimSpace(b.ID)
	if b.ID == "" {
		return BolsaPersistida{}, fmt.Errorf("%w: falta el identificador de la bolsa",
			recaudo.ErrBolsaInvalida)
	}
	// El id lo trae el reporte de recaudo de REDES y termina formando la ruta
	// `/bolsas/{id}`. Uno con barra o con delimitador de URL deja la fila
	// ESCRITA Y NO LEIBLE: el router casa un solo segmento, asi que
	// `GET /bolsas/reporte/2025` responde 404 y esa bolsa no se puede consultar
	// por id nunca mas. Rechazarlo aqui es lo unico que lo impide -- la validacion
	// no puede vivir en el dominio, que no sabe que la bolsa se sirve por HTTP.
	if strings.ContainsAny(b.ID, "/?#%") {
		return BolsaPersistida{}, fmt.Errorf(
			"%w: el identificador %q no puede llevar / ? # ni %%, porque la bolsa se lee por /bolsas/{id}",
			recaudo.ErrBolsaInvalida, b.ID)
	}

	bolsa, err := recaudo.NuevaBolsa(b.UsuarioID, b.Periodo, b.Circuito, b.Bruto)
	if err != nil {
		return BolsaPersistida{}, err
	}

	// Lo normalizado por el dominio es lo que se escribe: si se escribiera la
	// entrada cruda, el `usuario_id` con espacios no casaria con la clave
	// foranea y el UNIQUE dejaria pasar dos bolsas del mismo periodo.
	b.UsuarioID, b.Periodo, b.Circuito, b.Bruto = bolsa.UsuarioID, bolsa.Periodo, bolsa.Circuito, bolsa.Bruto
	b.Convenio = strings.TrimSpace(b.Convenio)
	b.Tarifa = strings.TrimSpace(b.Tarifa)
	b.Factura = strings.TrimSpace(b.Factura)

	if err := r.Gestion.RegistrarBolsa(ctx, b, r.Reloj.Ahora(), actorID); err != nil {
		// Sin envolver: quien llama distingue ErrBolsaDuplicada y
		// ErrUsuarioRecaudoInexistente con errors.Is, y el adaptador ya le
		// puso su contexto.
		return BolsaPersistida{}, err
	}
	return b, nil
}

// Listar devuelve las bolsas, opcionalmente las de un periodo.
//
// Un periodo vacio devuelve todas, que es el listado. Un periodo mal formado
// se RECHAZA en vez de ignorarse: ignorado devolveria el listado entero y
// quien pregunta lo leeria como "no hay ninguna de ese periodo". Es el mismo
// criterio que [Catalogo.BuscarObras] aplica a `anio`.
func (r Recaudo) Listar(ctx context.Context, periodo string) ([]BolsaPersistida, error) {
	if strings.TrimSpace(periodo) == "" {
		bolsas, err := r.Bolsas.ListarBolsas(ctx)
		if err != nil {
			return nil, fmt.Errorf("listar bolsas: %w", err)
		}
		return bolsas, nil
	}

	// Valida con el MISMO validador que [recaudo.NuevaBolsa] y no con una copia
	// del patron en este paquete, que usaba `[0-9]{2}` para el mes: con esa
	// copia, `?periodo=2025-13` pasaba el filtro y devolvia una lista vacia con
	// 200, que se lee como "ese mes no tuvo recaudo" en vez de "ese mes no
	// existe". Esa copia ya no existe: hoy lo que este paquete comprueba es
	// `recaudo.PeriodoValido`, la misma regla del constructor.
	periodo, err := recaudo.ValidarPeriodo(periodo)
	if err != nil {
		return nil, err
	}

	bolsas, err := r.Bolsas.BolsasDePeriodo(ctx, periodo)
	if err != nil {
		return nil, fmt.Errorf("listar bolsas del periodo %q: %w", periodo, err)
	}
	return bolsas, nil
}

// PorID devuelve una bolsa, o ErrNoEncontrado.
func (r Recaudo) PorID(ctx context.Context, id string) (BolsaPersistida, error) {
	b, err := r.Bolsas.BolsaPorID(ctx, id)
	if err != nil {
		return BolsaPersistida{}, fmt.Errorf("bolsa %q: %w", id, err)
	}
	return b, nil
}

// RegistrarUsuario da de alta a un pagador.
//
// Revalida por el constructor del dominio en vez de confiar en que quien llama
// lo construyera bien: [recaudo.Usuario] tiene un valor cero -- sin id y sin
// categoria -- que compila, y escribirlo dejaria que el CHECK de la tabla
// devolviera un 500 generico donde lo que hay es un dato mal formado.
func (r Recaudo) RegistrarUsuario(ctx context.Context, u recaudo.Usuario, actorID string) (recaudo.Usuario, error) {
	usuario, err := recaudo.NuevoUsuario(u.ID(), u.Datos())
	if err != nil {
		return recaudo.Usuario{}, err
	}
	if err := r.Gestion.RegistrarUsuario(ctx, usuario, r.Reloj.Ahora(), actorID); err != nil {
		return recaudo.Usuario{}, err
	}
	return usuario, nil
}

// ListarUsuarios devuelve los pagadores dados de alta.
func (r Recaudo) ListarUsuarios(ctx context.Context) ([]recaudo.Usuario, error) {
	usuarios, err := r.Bolsas.ListarUsuarios(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar usuarios de recaudo: %w", err)
	}
	return usuarios, nil
}
