package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Autenticacion es lo que la capa HTTP necesita del nucleo, declarado aqui.
//
// Se declara en el consumidor y no se importa el struct concreto, igual que
// [Salud]: asi este paquete depende de tres metodos y no del tipo que los
// implementa, y las pruebas de abajo pasan un doble sin levantar nada.
// aplicacion.Autenticacion la satisface sin nombrarla.
type Autenticacion interface {
	IniciarSesion(ctx context.Context, email, clave string) (aplicacion.Sesion, error)
	ResolverSesion(ctx context.Context, token string) (aplicacion.Usuario, error)
	CerrarSesion(ctx context.Context, token string) error
}

// claveContexto es un tipo propio y no exportado.
//
// Con una clave de tipo string, cualquier otro paquete que use el mismo texto
// lee o pisa este valor sin querer -y el compilador no dice nada-. Con un tipo
// no exportado, solo este paquete puede fabricar la clave.
type claveContexto int

const claveUsuario claveContexto = iota

// UsuarioDe devuelve el Usuario que [API.conSesion] dejo en el contexto.
//
// El segundo valor NO es decorativo: sin sesion, el primero es el Usuario cero
// -con Rol vacio-, y comparar ese rol contra una lista de permitidos es como
// se cuela una peticion sin autenticar. Quien llame comprueba el bool.
//
// Exportada porque es la unica forma que tiene un handler de saber quien
// pregunta, y es lo que consume el middleware de roles de #17.
func UsuarioDe(ctx context.Context) (aplicacion.Usuario, bool) {
	u, ok := ctx.Value(claveUsuario).(aplicacion.Usuario)
	return u, ok
}

// tokenBearer extrae el token de la cabecera Authorization.
//
// El esquema se compara sin distinguir mayusculas porque la RFC 7235 dice que
// no las distingue, y devolver 401 a un cliente que manda "bearer" seria una
// tarde entera de depuracion para encontrar una B.
//
// Solo Bearer: aceptar cualquier esquema dejaria pasar el resto de un
// "Basic dXNlcjpwYXNz" como si fuera un token.
func tokenBearer(r *http.Request) string {
	cabecera := r.Header.Get("Authorization")
	esquema, valor, hay := strings.Cut(cabecera, " ")
	if !hay || !strings.EqualFold(esquema, "Bearer") {
		return ""
	}
	return strings.TrimSpace(valor)
}

// conSesion resuelve el token a un Usuario y lo deja en el contexto.
//
// Distingue dos fallos que se parecen y no son lo mismo:
//
//   - ErrNoEncontrado -no hay token, o esta caducado o revocado- es 401.
//   - Cualquier otro error es 500. Si PostgreSQL esta caido, la sesion de
//     quien pregunta no es invalida: responderle 401 lo manda a
//     reautenticarse contra una base que no responde y esconde la averia.
func (a *API) conSesion(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenBearer(r)
		if token == "" {
			noAutenticado(w, "falta la cabecera Authorization con un token Bearer")
			return
		}

		usuario, err := a.auth.ResolverSesion(r.Context(), token)
		switch {
		case err == nil:
		case errors.Is(err, aplicacion.ErrNoEncontrado):
			noAutenticado(w, "sesion invalida o expirada")
			return
		default:
			// El token no entra en el log: acabaria en un fichero que no tiene
			// por que guardar credenciales.
			a.log.ErrorContext(r.Context(), "no se pudo resolver la sesion",
				slog.Any("error", err))
			escribirError(w, http.StatusInternalServerError, "no se pudo verificar la sesion")
			return
		}

		ctx := context.WithValue(r.Context(), claveUsuario, usuario)
		siguiente.ServeHTTP(w, r.WithContext(ctx))
	})
}

// noAutenticado responde 401 nombrando el esquema esperado.
//
// Sin WWW-Authenticate el cliente sabe que le hace falta credencial pero no
// de que tipo; la RFC 7235 la exige en un 401.
func noAutenticado(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="intela"`)
	escribirError(w, http.StatusUnauthorized, msg)
}

// ---------------------------------------------------------------------------
// Formas de red
//
// Los modelos de aplicacion no llevan etiquetas json a proposito: el nucleo no
// sabe que existe la serializacion. Estos structs son el contrato HTTP, y son
// los que tienen que cuadrar con los schemas de api/openapi.yaml. Renombrar un
// campo del nucleo no cambia lo que ve un cliente.

type credenciales struct {
	Email string `json:"email"`
	Clave string `json:"clave"`
}

type usuarioJSON struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Nombre    string `json:"nombre"`
	Rol       string `json:"rol"`
	TitularID string `json:"titular_id"`
}

type sesionJSON struct {
	Token   string      `json:"token"`
	Expira  string      `json:"expira"`
	Usuario usuarioJSON `json:"usuario"`
}

func aUsuarioJSON(u aplicacion.Usuario) usuarioJSON {
	return usuarioJSON{
		ID:        u.ID,
		Email:     u.Email,
		Nombre:    u.Nombre,
		Rol:       string(u.Rol),
		TitularID: u.TitularID,
	}
}

// ---------------------------------------------------------------------------
// Handlers

// iniciarSesion cambia unas credenciales por un token.
//
// Es la unica ruta que se llama sin sesion, y por eso el contrato la marca con
// `security: []`.
func (a *API) iniciarSesion(w http.ResponseWriter, r *http.Request) {
	var cred credenciales
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con email y clave")
		return
	}
	// 400 y no 401: no es que las credenciales sean malas, es que no llegaron.
	// Un 401 aqui haria pensar en un usuario mal escrito y no en una peticion
	// mal formada.
	if cred.Email == "" || cred.Clave == "" {
		escribirError(w, http.StatusBadRequest, "email y clave son obligatorios")
		return
	}

	sesion, err := a.auth.IniciarSesion(r.Context(), cred.Email, cred.Clave)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrCredenciales):
		// El mensaje no dice cual de los dos factores fallo: distinguirlos
		// reabre por el cuerpo el canal lateral que el caso de uso cierra por
		// el reloj. Se registra el intento -sin la clave- para el issue de
		// rate-limiting.
		a.log.WarnContext(r.Context(), "intento de inicio de sesion fallido",
			slog.String("email", cred.Email))
		noAutenticado(w, "credenciales invalidas")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al iniciar sesion", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo iniciar la sesion")
		return
	}

	escribirJSON(w, http.StatusOK, sesionJSON{
		Token:   sesion.Token,
		Expira:  sesion.Expira.Format(time.RFC3339),
		Usuario: aUsuarioJSON(sesion.Usuario),
	})
}

// sesionActual dice quien es el portador del token.
//
// Existe para que el frontend pinte la sesion sin guardar el usuario en el
// navegador, y es la primera ruta protegida del sistema: la que prueba de
// punta a punta que conSesion hace su trabajo.
func (a *API) sesionActual(w http.ResponseWriter, r *http.Request) {
	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		// Inalcanzable detras de conSesion. Si alguien monta este handler sin
		// el middleware, esto es lo que evita servir el Usuario cero.
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	escribirJSON(w, http.StatusOK, aUsuarioJSON(usuario))
}

// cerrarSesion revoca el token presentado.
//
// 204 sin cuerpo: no hay nada que contar, y el logout es idempotente.
func (a *API) cerrarSesion(w http.ResponseWriter, r *http.Request) {
	token := tokenBearer(r)
	if err := a.auth.CerrarSesion(r.Context(), token); err != nil {
		a.log.ErrorContext(r.Context(), "fallo al cerrar sesion", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo cerrar la sesion")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
