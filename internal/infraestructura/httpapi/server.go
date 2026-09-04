// Package httpapi es el adaptador de entrada HTTP.
//
// # La regla de dependencia
//
// La direccion es httpapi -> aplicacion -> puertos <- postgres. NUNCA
// httpapi -> postgres.
//
// Este tipo NO tiene un campo con el puerto de persistencia, y es deliberado.
// Cuando lo tuvo, trece handlers consultaban la base directamente saltandose
// la capa de aplicacion: la autenticacion, las liquidaciones de un titular
// -dinero-, los parametros normativos en crudo y la lectura de la bitacora
// entre ellos. Cada una de esas aristas se salta la autorizacion, el asiento
// en bitacora y los limites de transaccion.
//
// Cada lectura tiene que tener su caso de uso, aunque al principio muchos
// sean de una linea. Es lo que hace que la autorizacion y el asiento tengan
// donde vivir.
//
// depguard no puede vigilar esto: internal/infraestructura/ esta excluido de
// la regla, y con razon, porque los adaptadores importan infraestructura por
// definicion. Esta frontera se sostiene en revision.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Salud responde si las dependencias del proceso estan vivas.
type Salud interface {
	Ping(ctx context.Context) error
}

// Opciones del servidor.
type Opciones struct {
	// OrigenesPermitidos para CORS. Vacio deshabilita CORS.
	//
	// Nunca "*": esta API autoriza movimientos de dinero. El comodin junto
	// con Authorization en Allow-Headers permite que cualquier origen la
	// llame con un token robado.
	OrigenesPermitidos []string
	Log                *slog.Logger
}

// API es el adaptador. Los casos de uso se inyectan de uno en uno segun
// entren sus PRs.
type API struct {
	salud       Salud
	auth        Autenticacion
	listadoONI  LecturaONI
	publicarONI EscrituraONI
	opts        Opciones
	log         *slog.Logger
}

// Casos de uso que el adaptador expone. Van juntos porque Nueva ya no puede
// seguir creciendo parametro a parametro: el comentario de abajo lo pedia
// a partir del tercero.
type Casos struct {
	ListadoONI  LecturaONI
	PublicarONI EscrituraONI
}

// Nueva construye el adaptador.
//
// Los casos de uso van en Casos y no dentro de Opciones porque son
// dependencias, no configuracion: Opciones se rellena desde el entorno, y
// esto se cablea en cmd/api.
func Nueva(salud Salud, auth Autenticacion, casos Casos, opts Opciones) *API {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &API{
		salud:       salud,
		auth:        auth,
		listadoONI:  casos.ListadoONI,
		publicarONI: casos.PublicarONI,
		opts:        opts,
		log:         log,
	}
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(a.cors)

	// Los handlers por defecto de chi responden en text/plain. Si no se
	// sustituyen, un 404 o un 405 salen con un content-type distinto al del
	// resto de la API y cualquier cliente que parsee JSON se atraganta justo
	// en el caso de error.
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		escribirError(w, http.StatusNotFound, "ruta no encontrada")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido")
	})

	// Las sondas van sin sesion: el orquestador no tiene credenciales, y una
	// sonda que exigiera token reiniciaria el contenedor en bucle.
	r.Get("/health", a.health)
	r.Get("/ready", a.ready)

	// R-18: el listado ONI se publica en la web, sin autenticacion.
	// security: [] en el contrato. Montarlo detras de conSesion violaría
	// RD 13.8.1 y no arrancaria el reloj de R-19 para quien no tenga cuenta.
	r.Get("/publico/oni", a.obtenerListadoONI)

	// El login es la unica ruta de /auth/session que se llama sin token; las
	// otras dos van detras del middleware. Se agrupan con r.Group para que la
	// diferencia se vea de un vistazo: quien anada una ruta protegida la mete
	// en el grupo y no tiene que acordarse de nada.
	r.Post("/auth/session", a.iniciarSesion)
	r.Group(func(protegido chi.Router) {
		protegido.Use(a.conSesion)
		protegido.Get("/auth/session", a.sesionActual)
		protegido.Delete("/auth/session", a.cerrarSesion)

		// Los grupos de rol van DENTRO de conSesion: sin sesion la
		// respuesta es 401, no 403. La matriz Rol -> capacidad esta en
		// docs/architecture/roles.md; quien anada un endpoint lo mete
		// en el grupo que le corresponde y no escribe el chequeo a mano.

		protegido.Group(func(roles chi.Router) {
			roles.Use(a.conRoles(aplicacion.RolAdministrador, aplicacion.RolDistribucion))
			roles.Post("/oni/publicaciones", a.crearPublicacionONI)
		})

		protegido.Route("/admin", func(admin chi.Router) {
			admin.Use(requiereRol(aplicacion.RolAdministrador))
			admin.Get("/pipeline", superficieOK)
		})

		protegido.Route("/auditoria", func(audit chi.Router) {
			audit.Use(requiereRol(aplicacion.RolAuditor, aplicacion.RolAdministrador))
			audit.Get("/asientos", superficieOK)
		})
	})

	return r
}

// health dice que el proceso esta vivo. No toca la base: si lo hiciera, una
// caida de Postgres reiniciaria los contenedores en bucle.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]string{"estado": "ok"})
}

// ready dice que el proceso puede atender trafico, dependencias incluidas.
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if a.salud != nil {
		if err := a.salud.Ping(ctx); err != nil {
			a.log.WarnContext(ctx, "dependencia no lista", slog.Any("error", err))
			escribirError(w, http.StatusServiceUnavailable, "base de datos no disponible")
			return
		}
	}
	escribirJSON(w, http.StatusOK, map[string]string{"estado": "listo"})
}

// cors responde solo a los origenes de la lista blanca, que viene de entorno
// y es distinta en desarrollo y en produccion.
func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origen := r.Header.Get("Origin")
		if origen != "" && slices.Contains(a.opts.OrigenesPermitidos, origen) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origen)
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Max-Age", "600")
			// El origen entra en la respuesta, asi que las caches
			// intermedias tienen que variar por el.
			h.Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// escribirJSON fija el content-type ANTES del codigo de estado. Al reves no
// tiene efecto.
func escribirJSON(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(v)
}

// escribirError devuelve JSON con content-type de JSON.
//
// http.Error fuerza text/plain, asi que usarlo para escribir un cuerpo JSON
// deja todas las respuestas de error con el content-type equivocado.
func escribirError(w http.ResponseWriter, codigo int, msg string) {
	escribirJSON(w, codigo, map[string]string{"error": msg})
}
