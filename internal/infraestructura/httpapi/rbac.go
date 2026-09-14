package httpapi

import (
	"net/http"
	"slices"

	"github.com/rosvend/intela/internal/aplicacion"
)

// requiereRol es la fabrica de middleware de autorizacion.
//
// Lee el Usuario que [API.conSesion] dejo en el contexto y responde 403 JSON
// si el rol no esta en el conjunto permitido. Va DETRAS de conSesion: sin
// sesion la respuesta es 401, no 403. Un 403 a quien no se identifico le
// diria que la ruta existe y que el problema es el rol.
//
// El segundo valor de [UsuarioDe] no se ignora. El Usuario cero trae Rol
// vacio, y comparar ese vacio contra la lista de permitidos es exactamente
// como se cuela una peticion sin autenticar. Si este middleware se monta
// sin conSesion, la respuesta es 401, no un pase.
//
// Vive en el grupo de rutas, no en cada handler: olvidar un chequeo en un
// handler es el fallo que el issue de security-review (#47) quiere poder
// auditar en un solo sitio.
func requiereRol(roles ...aplicacion.Rol) func(http.Handler) http.Handler {
	return func(siguiente http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			usuario, hay := UsuarioDe(r.Context())
			if !hay {
				noAutenticado(w, "sesion invalida o expirada")
				return
			}
			if !slices.Contains(roles, usuario.Rol) {
				escribirError(w, http.StatusForbidden, aplicacion.ErrNoAutorizado.Error())
				return
			}
			siguiente.ServeHTTP(w, r)
		})
	}
}

// superficieOK responde 204. No hay payload porque los casos de uso de
// estas superficies aterrizan en PRs posteriores: hoy el grupo existe para
// que el rol se haga cumplir en un solo sitio.
func superficieOK(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
