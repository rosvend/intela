package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// Recaudo es lo que la capa HTTP necesita del nucleo para el lado del ingreso,
// declarado aqui igual que [Catalogo] y [Declaraciones].
//
// aplicacion.Recaudo la satisface sin nombrarla, y las pruebas pasan un doble
// sin levantar nada.
type Recaudo interface {
	Registrar(ctx context.Context, b aplicacion.BolsaPersistida, actorID string) (aplicacion.BolsaPersistida, error)
	Listar(ctx context.Context, periodo string) ([]aplicacion.BolsaPersistida, error)
	PorID(ctx context.Context, id string) (aplicacion.BolsaPersistida, error)
	RegistrarUsuario(ctx context.Context, u recaudo.Usuario, actorID string) (recaudo.Usuario, error)
	ListarUsuarios(ctx context.Context) ([]recaudo.Usuario, error)
}

// ---------------------------------------------------------------------------
// Formas de red

// bolsaJSON es una bolsa tal como viaja por la red.
//
// Bruto es decimal.Decimal y NO float64. Es la unica decision de este fichero
// que no es cosmetica: con float64, un importe con centavos que en binario no
// es exacto entra al nucleo alterado, y esto es dinero de terceros que el
// ADR 0005 obliga a tratar con aritmetica decimal exacta. shopspring/decimal
// desserializa desde un numero JSON sin pasar por coma flotante.
//
// Convenio, Tarifa y Factura son PROCEDENCIA -la pregunta 1 del ADR 0006, de
// donde salio este dinero-, no insumos de calculo: bajo P-08 Intela recibe el
// importe ya cobrado y no liquida tarifas. Los tres son opcionales porque
// `T-11` dice que la tarifa publicada rige CUANDO NO HAY convenio.
type bolsaJSON struct {
	ID        string          `json:"id"`
	UsuarioID string          `json:"usuario_id"`
	Periodo   string          `json:"periodo"`
	Circuito  string          `json:"circuito"`
	Bruto     decimal.Decimal `json:"bruto"`
	Convenio  string          `json:"convenio"`
	Tarifa    string          `json:"tarifa"`
	Factura   string          `json:"factura"`
}

// usuarioRecaudoJSON es el pagador: un canal, una sala, un hotel, una OTT.
//
// No es el usuarioJSON de la sesion. Ese es la CUENTA que inicia sesion; este
// es quien paga el recaudo, y por eso su tabla es `usuarios_recaudo`.
type usuarioRecaudoJSON struct {
	ID        string `json:"id"`
	Nombre    string `json:"nombre"`
	NIT       string `json:"nit"`
	Categoria string `json:"categoria"`
}

func (b bolsaJSON) aDominio() aplicacion.BolsaPersistida {
	return aplicacion.BolsaPersistida{
		ID:        b.ID,
		UsuarioID: b.UsuarioID,
		Periodo:   b.Periodo,
		Circuito:  recaudo.Circuito(b.Circuito),
		Bruto:     b.Bruto,
		Convenio:  b.Convenio,
		Tarifa:    b.Tarifa,
		Factura:   b.Factura,
	}
}

func aBolsaJSON(b aplicacion.BolsaPersistida) bolsaJSON {
	return bolsaJSON{
		ID:        b.ID,
		UsuarioID: b.UsuarioID,
		Periodo:   b.Periodo,
		Circuito:  string(b.Circuito),
		Bruto:     b.Bruto,
		Convenio:  b.Convenio,
		Tarifa:    b.Tarifa,
		Factura:   b.Factura,
	}
}

func aUsuarioRecaudoJSON(u recaudo.Usuario) usuarioRecaudoJSON {
	d := u.Datos()
	return usuarioRecaudoJSON{
		ID:        u.ID(),
		Nombre:    d.Nombre,
		NIT:       d.NIT,
		Categoria: string(d.Categoria),
	}
}

// ---------------------------------------------------------------------------
// Handlers

// maxCuerpoRecaudo acota el cuerpo. Las dos altas de este fichero son objetos
// de tamano fijo, asi que 64 KiB sobra de largo; el limite esta para que un
// cuerpo enorme no se lea entero antes de rechazarlo.
const maxCuerpoRecaudo = 64 << 10

// registrarRecaudo da de alta lo cobrado a un usuario en un periodo.
//
// No calcula la tarifa (P-08): el importe llega ya cobrado. El identificador lo
// trae el cuerpo -sale del reporte de recaudo de REDES, no de aqui-, que es lo
// que hace del duplicado un 409 en vez de una segunda bolsa inventada.
func (a *API) registrarRecaudo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoRecaudo)

	var cuerpo bolsaJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con la bolsa")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	bolsa, err := a.recaudo.Registrar(r.Context(), cuerpo.aDominio(), usuario.ID)
	switch {
	case err == nil:
	case errors.Is(err, recaudo.ErrBolsaInvalida):
		// 400 y no 422: los datos llegaron, no forman una bolsa, y el mensaje
		// del dominio dice cual campo falla.
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrUsuarioRecaudoInexistente):
		// 400 y no 404: lo que no existe es un dato DENTRO del cuerpo, no el
		// recurso de la URL.
		escribirError(w, http.StatusBadRequest, aplicacion.ErrUsuarioRecaudoInexistente.Error())
		return
	case errors.Is(err, aplicacion.ErrBolsaDuplicada):
		escribirError(w, http.StatusConflict, aplicacion.ErrBolsaDuplicada.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al registrar recaudo", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo registrar el recaudo")
		return
	}

	w.Header().Set("Location", "/bolsas/"+bolsa.ID)
	escribirJSON(w, http.StatusCreated, aBolsaJSON(bolsa))
}

// listarBolsas sirve las bolsas, con o sin filtro de periodo.
func (a *API) listarBolsas(w http.ResponseWriter, r *http.Request) {
	bolsas, err := a.recaudo.Listar(r.Context(), r.URL.Query().Get("periodo"))
	switch {
	case err == nil:
	case errors.Is(err, recaudo.ErrBolsaInvalida):
		// Un ?periodo=enero se rechaza en vez de ignorarse: ignorado devolveria
		// el listado entero y quien pregunta lo leeria como "no hay ninguna de
		// ese periodo".
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al listar bolsas", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudieron consultar las bolsas")
		return
	}

	// make y no var: sin coincidencias tiene que salir [] y no null, o
	// cualquier cliente que itere la respuesta revienta.
	cuerpo := make([]bolsaJSON, 0, len(bolsas))
	for _, b := range bolsas {
		cuerpo = append(cuerpo, aBolsaJSON(b))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

func (a *API) bolsaPorID(w http.ResponseWriter, r *http.Request) {
	bolsa, err := a.recaudo.PorID(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "esa bolsa no existe")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al leer una bolsa", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar la bolsa")
		return
	}
	escribirJSON(w, http.StatusOK, aBolsaJSON(bolsa))
}

// registrarUsuarioRecaudo da de alta un pagador.
//
// Tiene que existir ANTES de su primera bolsa: la clave foranea lo exige, y por
// eso el alta es un endpoint y no un efecto secundario de registrar recaudo. Un
// `usuario_id` mal escrito crearia si no un pagador fantasma al que el reparto
// atribuiria dinero que alguien pago de verdad.
func (a *API) registrarUsuarioRecaudo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoRecaudo)

	var cuerpo usuarioRecaudoJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con el usuario")
		return
	}

	sesion, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	// La validacion es del dominio: [recaudo.NuevoUsuario] es la unica puerta,
	// asi que el handler no repite ninguna regla.
	nuevo, err := recaudo.NuevoUsuario(cuerpo.ID, recaudo.Datos{
		Nombre:    cuerpo.Nombre,
		NIT:       cuerpo.NIT,
		Categoria: recaudo.CategoriaUsuario(cuerpo.Categoria),
	})
	if err != nil {
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	}

	usuario, err := a.recaudo.RegistrarUsuario(r.Context(), nuevo, sesion.ID)
	switch {
	case err == nil:
	case errors.Is(err, recaudo.ErrUsuarioInvalido):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrUsuarioDeRecaudoDuplicado):
		escribirError(w, http.StatusConflict, aplicacion.ErrUsuarioDeRecaudoDuplicado.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al dar de alta un usuario de recaudo", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo dar de alta el usuario")
		return
	}

	// Sin cabecera Location, a diferencia del alta de una bolsa: no hay
	// `GET /recaudo/usuarios/{id}` que apuntar. Un Location hacia una ruta que
	// no existe es peor que ninguno -- el cliente que lo siga recibe un 404 y no
	// tiene forma de saber si el alta funciono-. El pagador recien creado sale
	// en el cuerpo y en `GET /recaudo/usuarios`; cuando algun panel necesite la
	// lectura individual, entra con su ruta y su Location.
	escribirJSON(w, http.StatusCreated, aUsuarioRecaudoJSON(usuario))
}

func (a *API) listarUsuariosRecaudo(w http.ResponseWriter, r *http.Request) {
	usuarios, err := a.recaudo.ListarUsuarios(r.Context())
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al listar usuarios de recaudo", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudieron consultar los usuarios")
		return
	}

	cuerpo := make([]usuarioRecaudoJSON, 0, len(usuarios))
	for _, u := range usuarios {
		cuerpo = append(cuerpo, aUsuarioRecaudoJSON(u))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}
