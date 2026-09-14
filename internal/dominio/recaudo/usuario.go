package recaudo

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrUsuarioInvalido es el unico centinela que emite [NuevoUsuario].
//
// Se llama igual que aplicacion.ErrUsuarioInvalido y no es el mismo: aquel es
// la CUENTA que inicia sesion, este es el PAGADOR. Son dos cosas distintas que
// el reglamento llama con la misma palabra -ver el comentario de [Usuario]- y
// el paquete es lo que las separa.
var ErrUsuarioInvalido = errors.New("usuario de recaudo invalido")

// CategoriaUsuario clasifica al pagador segun el Reglamento de Tarifas.
//
// El conjunto esta cerrado, y lo que compra al cerrarlo no es la tarifa -bajo
// P-08 Intela no factura- sino saber QUE FORMULA de reparto aplica aguas
// abajo: `docs/dominio/formulas.md` valoriza television por puntos (9.1.1),
// cine y teatro proporcional a taquilla (9.2, 9.3), OTT por la ecuacion de
// 9.7 y hoteles por remision a 9.5 (9.6). Con un texto libre, una bolsa entra
// sin que nadie sepa como ponderarla.
//
// Los diez primeros valores son las diez categorias de la tabla resumen
// `RT 4`, que es la operativa: `RT 3` anuncia seis tipos de usuario y a
// continuacion enumera siete, y la discrepancia esta en el documento fuente.
type CategoriaUsuario string

const (
	TVAbierta             CategoriaUsuario = "tv_abierta"             // RT 3.1.1, T-01
	TVCerrada             CategoriaUsuario = "tv_cerrada"             // RT 3.1.2, T-01
	Cine                  CategoriaUsuario = "cine"                   // RT 3.2, T-02
	TransporteAereo       CategoriaUsuario = "transporte_aereo"       // RT 3.3, T-03
	TransporteTerrestre   CategoriaUsuario = "transporte_terrestre"   // RT 3.4.1, T-04
	TransporteFluvial     CategoriaUsuario = "transporte_fluvial"     // RT 3.4.2, T-05
	Hotel                 CategoriaUsuario = "hotel"                  // RT 3.5, T-06
	Salud                 CategoriaUsuario = "salud"                  // RT 3.5, T-07
	MediosDigitales       CategoriaUsuario = "medios_digitales"       // RT 3.6, T-08
	OtrosEstablecimientos CategoriaUsuario = "otros_establecimientos" // RT 3.7, T-09

	// SinClasificar es el hueco declarado, no una categoria del reglamento.
	//
	// Existe porque la migracion 00009 tiene que dar de alta los pagadores que
	// las bolsas ya citaban antes de que hubiera tabla, y la categoria de esos
	// no se puede deducir de una fila de `bolsas`. Poner `otros_establecimientos`
	// habria sido inventar una clasificacion con consecuencias de calculo; el
	// ADR 0004 pide justo lo contrario -que el hueco se vea-.
	SinClasificar CategoriaUsuario = "sin_clasificar"
)

// CategoriasUsuario devuelve las categorias validas, en el orden en que las
// declara el CHECK de la migracion.
func CategoriasUsuario() []CategoriaUsuario {
	return []CategoriaUsuario{
		TVAbierta, TVCerrada, Cine,
		TransporteAereo, TransporteTerrestre, TransporteFluvial,
		Hotel, Salud, MediosDigitales, OtrosEstablecimientos,
		SinClasificar,
	}
}

// Datos son los campos mutables de un [Usuario].
//
// No hay tarifa pactada ni vigencia aqui, y no es un olvido: bajo P-08 Intela
// recibe lo cobrado y no liquida, asi que el convenio no es un insumo de
// calculo sino la PROCEDENCIA de una bolsa concreta, y viaja en la bolsa
// -ADR 0006, pregunta 1: de donde salio este dinero-. Si algun dia hubiera que
// modelar un convenio con vigencia, no cabria en este paquete: depguard
// deniega `time` en internal/dominio y una vigencia son dos instantes.
type Datos struct {
	Nombre string
	// NIT del pagador. Opcional a proposito: el circuito internacional lo
	// alimentan sociedades hermanas que no tienen NIT colombiano.
	NIT       string
	Categoria CategoriaUsuario
}

// Usuario es quien explota el repertorio y PAGA por ello: un canal, una sala
// de cine, un hotel, una plataforma OTT, una empresa de transporte (glosario,
// `RT 2`).
//
// No confundir con aplicacion.Usuario, que es la cuenta que inicia sesion en
// Intela. La colision viene del lenguaje del reglamento, que llama "usuario"
// al que usa las obras; por eso la tabla se llama `usuarios_recaudo` y no
// `usuarios`, que ya la ocupa la autenticacion.
//
// Recaudo es el unico modulo que lo conoce (ADR 0003): aguas abajo solo
// circula una [Bolsa], que lleva el `UsuarioID` y ni la categoria ni el NIT.
type Usuario struct {
	id    string
	datos Datos
}

// NuevoUsuario valida un pagador antes de que llegue a persistirse.
//
// El id NO lo genera este tipo, por lo mismo que repertorio.NuevaObra: lo
// asigna quien da de alta al usuario, y el duplicado lo rechaza la clave
// primaria en vez de acunar uno propio y dejar dos filas para el mismo canal.
func NuevoUsuario(id string, d Datos) (Usuario, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Usuario{}, fmt.Errorf("%w: falta el identificador", ErrUsuarioInvalido)
	}
	d, err := normalizar(d)
	if err != nil {
		return Usuario{}, err
	}
	return Usuario{id: id, datos: d}, nil
}

// ID es el identificador inmutable del usuario.
func (u Usuario) ID() string { return u.id }

// Datos devuelve los campos mutables. No lleva rebanadas, asi que la copia de
// valor basta para que nadie reescriba por dentro lo que el constructor
// valido.
func (u Usuario) Datos() Datos { return u.datos }

// ConDatos devuelve una copia con otros datos, validados por la misma puerta.
func (u Usuario) ConDatos(d Datos) (Usuario, error) {
	d, err := normalizar(d)
	if err != nil {
		return Usuario{}, err
	}
	return Usuario{id: u.id, datos: d}, nil
}

// normalizar recorta y valida. Es la unica puerta: [NuevoUsuario] y
// [Usuario.ConDatos] pasan las dos por aqui, asi que un usuario corregido
// cumple lo mismo que uno recien creado.
func normalizar(d Datos) (Datos, error) {
	d.Nombre = strings.TrimSpace(d.Nombre)
	d.NIT = strings.TrimSpace(d.NIT)

	if d.Nombre == "" {
		return Datos{}, fmt.Errorf("%w: falta el nombre", ErrUsuarioInvalido)
	}
	if !slices.Contains(CategoriasUsuario(), d.Categoria) {
		return Datos{}, fmt.Errorf("%w: categoria %q, se esperaba una de %v",
			ErrUsuarioInvalido, d.Categoria, CategoriasUsuario())
	}
	return d, nil
}
