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
	salud    Salud
	auth     Autenticacion
	catalogo Catalogo
	ingesta  Ingesta
	opts     Opciones
	log      *slog.Logger
}

// Casos agrupa los casos de uso que sirve el adaptador.
//
// Iban como parametros sueltos de [Nueva] mientras fueron dos. Con el tercero
// la lista deja de ser legible en la llamada -- tres interfaces seguidas se
// pueden cruzar sin que el compilador diga nada si dos comparten forma -- y
// pasan a campos con nombre. Opciones sigue aparte: eso es configuracion del
// entorno, esto son dependencias.
type Casos struct {
	Salud    Salud
	Auth     Autenticacion
	Catalogo Catalogo
	Ingesta  Ingesta
}

// Nueva construye el adaptador.
//
// Un caso de uso nil no es un fallo de arranque: su ruta responde 503. Ver
// [API.conIngesta]. Es lo que permite que un binario que todavia no cablea la
// boveda -- cmd/lambda, cuyo sistema de ficheros es de solo lectura -- siga
// sirviendo el resto de la API.
func Nueva(casos Casos, opts Opciones) *API {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &API{
		salud:    casos.Salud,
		auth:     casos.Auth,
		catalogo: casos.Catalogo,
		ingesta:  casos.Ingesta,
		opts:     opts,
		log:      log,
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
		protegido.Route("/admin", func(admin chi.Router) {
			admin.Use(requiereRol(aplicacion.RolAdministrador))
			admin.Get("/pipeline", superficieOK)
		})
		protegido.Route("/auditoria", func(audit chi.Router) {
			audit.Use(requiereRol(aplicacion.RolAuditor, aplicacion.RolAdministrador))
			audit.Get("/asientos", superficieOK)
		})

		// El catalogo maestro. Las cuatro rutas piden `administrador`,
		// lectura incluida: el catalogo es el cubo contra el que resuelve
		// todo el matching, y quien lo lee entero ve el repertorio completo
		// de la sociedad. Abrirlo a `auditor` -que tiene lectura de todo- o
		// recortarlo para `titular` con SoloPropiasObras (OE-6) son
		// decisiones de los issues que traigan esos paneles, no de este.
		protegido.Route("/obras", func(cat chi.Router) {
			cat.Use(requiereRol(aplicacion.RolAdministrador))
			cat.Get("/", a.buscarObras)
			cat.Post("/", a.registrarObra)
			cat.Get("/{id}", a.obraPorID)
			cat.Patch("/{id}", a.actualizarObra)
		})

		// La ingesta manual de reportes de uso. Pide `administrador` por lo
		// mismo que el catalogo: una entrega pondera el reparto de un periodo
		// entero, y el listado de cargas deja ver de que fuentes vive la
		// sociedad. Cuando entre el panel de operacion (#29), el rol que le
		// toque lo decide ese issue.
		protegido.Route("/reportes", func(rep chi.Router) {
			rep.Use(requiereRol(aplicacion.RolAdministrador))
			rep.Post("/", a.conIngesta(a.subirReporte))
			rep.Get("/", a.conIngesta(a.listarCargas))
		})
	})

	return r
}

// conIngesta responde 503 si el binario no cableo el caso de uso de ingesta.
//
// Sin esto, la ruta existe y el handler llama a una interfaz nil: el Recoverer
// lo convierte en un 500 sin cuerpo, que se lee como "el servidor esta roto"
// cuando lo que pasa es que a ESA instalacion le falta la boveda. 503 lo dice,
// y ademas es lo que un balanceador entiende.
func (a *API) conIngesta(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.ingesta == nil {
			escribirError(w, http.StatusServiceUnavailable,
				"la ingesta de reportes no esta configurada en esta instalacion")
			return
		}
		h(w, r)
	}
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
