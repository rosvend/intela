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
	salud         Salud
	auth          Autenticacion
	catalogo      Catalogo
	padron        Padron
	ingesta       Ingesta
	declaraciones Declaraciones
	recaudo       Recaudo
	liq           Liquidaciones
	procesos      Procesos
	cola          ColaRevision
	auditoria     Auditoria
	opts          Opciones
	log           *slog.Logger
}

// Casos agrupa los casos de uso que sirve el adaptador.
//
// Iban como parametros sueltos de [Nueva] mientras fueron dos. Con el tercero
// la lista deja de ser legible en la llamada -- tres interfaces seguidas se
// pueden cruzar sin que el compilador diga nada si dos comparten forma -- y
// pasan a campos con nombre. Opciones sigue aparte: eso es configuracion del
// entorno, esto son dependencias.
type Casos struct {
	Salud         Salud
	Auth          Autenticacion
	Catalogo      Catalogo
	Padron        Padron
	Ingesta       Ingesta
	Declaraciones Declaraciones
	Recaudo       Recaudo
	Liquidaciones Liquidaciones
	Procesos      Procesos
	Cola          ColaRevision
	Auditoria     Auditoria
}

// ColaRevision lista lo que espera ojo humano: filas que no se pudieron
// normalizar, y mas adelante las anomalias del #37. Se declara en el
// consumidor, igual que [Catalogo].
type ColaRevision interface {
	ListarRevision(ctx context.Context) ([]aplicacion.ItemRevision, error)
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
		salud:         casos.Salud,
		auth:          casos.Auth,
		catalogo:      casos.Catalogo,
		padron:        casos.Padron,
		ingesta:       casos.Ingesta,
		declaraciones: casos.Declaraciones,
		recaudo:       casos.Recaudo,
		liq:           casos.Liquidaciones,
		procesos:      casos.Procesos,
		cola:          casos.Cola,
		auditoria:     casos.Auditoria,
		opts:          opts,
		log:           log,
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
			admin.Get("/cola-revision", a.listarColaRevision)
		})
		protegido.Route("/auditoria", func(audit chi.Router) {
			audit.Use(requiereRol(aplicacion.RolAuditor, aplicacion.RolAdministrador))
			audit.Get("/asientos", a.listarAsientos)
			audit.Get("/obra/{id}", a.historialDeObra)
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

			// El editor de splits de la #30. Mismo rol que el resto del
			// catalogo: es la misma superficie -quien edita una declaracion
			// ve el repertorio entero-.
			cat.Post("/{id}/declaracion", a.declararObra)
			cat.Put("/{id}/declaracion", a.editarDeclaracion)
			cat.Get("/{id}/declaracion/historial", a.historialDeclaracion)
		})

		// El padron de titulares, que es lo que llena el selector de partes
		// del editor de splits (#30). Mismo rol que el catalogo -quien edita
		// una declaracion ve el repertorio entero, y el padron es la otra
		// mitad de esa superficie-.
		//
		// Se sirve SIN recortar las personas juridicas: R-01 (`RD 4.5`) deja
		// fuera del reparto a todo el que no sea persona natural, y quien
		// edita tiene que poder ver que el titular que busca existe en el
		// padron y que no se le ofrece por esa regla.
		protegido.Route("/titulares", func(pad chi.Router) {
			pad.Use(requiereRol(aplicacion.RolAdministrador))
			pad.Get("/", a.buscarTitulares)
		})

		// El lado del ingreso (#27). Entra dinero, asi que escribe
		// `contabilidad` -- que es quien factura (roles.md, `RD 13.5`)-- y
		// `administrador`. Ni `distribucion` ni `auditor` registran recaudo:
		// distribucion es la OTRA firma de las compuertas y auditor no opera
		// el pipeline.
		protegido.Route("/recaudo", func(rec chi.Router) {
			rec.Use(requiereRol(aplicacion.RolContabilidad, aplicacion.RolAdministrador))
			rec.Post("/", a.registrarRecaudo)
			rec.Get("/usuarios", a.listarUsuariosRecaudo)
			rec.Post("/usuarios", a.registrarUsuarioRecaudo)
		})

		// Las bolsas se leen desde mas sitios de los que se escriben:
		// `distribucion` necesita la bolsa para correr el reparto y `auditor`
		// tiene lectura de todo. Sigue fuera `titular`, que solo ve las obras
		// donde participa (OE-6) y no el ingreso de la sociedad.
		protegido.Route("/bolsas", func(bol chi.Router) {
			bol.Use(requiereRol(
				aplicacion.RolContabilidad, aplicacion.RolAdministrador,
				aplicacion.RolDistribucion, aplicacion.RolAuditor,
			))
			bol.Get("/", a.listarBolsas)
			bol.Get("/{id}", a.bolsaPorID)
		})
		protegido.Group(func(titular chi.Router) {
			titular.Use(requiereRol(aplicacion.RolTitular))
			titular.Get("/mis-liquidaciones", a.consultarLiquidaciones)
			titular.Get("/mis-liquidaciones/export", a.exportarLiquidaciones)
		})

		// El flujo de aprobaciones de RD 13.5 (#34). Tres grupos, no uno,
		// porque no comparten roles: administrador OPERA el pipeline (abre y
		// avanza etapas), y distribucion/contabilidad son las dos firmas de
		// sus compuertas (firmar, rechazar) -- la MISMA separacion que ya
		// aplica a /recaudo y /bolsas, y por la misma razon: quien co-firma
		// la salida del dinero no debe ser quien opera el pipeline que la
		// prepara. La lectura la comparten los tres, mas auditor.
		protegido.Route("/procesos", func(proc chi.Router) {
			proc.Group(func(lectura chi.Router) {
				lectura.Use(requiereRol(
					aplicacion.RolAdministrador, aplicacion.RolDistribucion,
					aplicacion.RolContabilidad, aplicacion.RolAuditor,
				))
				lectura.Get("/", a.listarProcesos)
				lectura.Get("/{id}", a.procesoPorID)
			})
			proc.Group(func(pipeline chi.Router) {
				pipeline.Use(requiereRol(aplicacion.RolAdministrador))
				pipeline.Post("/", a.abrirProceso)
				pipeline.Post("/{id}/avanzar", a.avanzarEtapaProceso)
			})
			proc.Group(func(compuerta chi.Router) {
				compuerta.Use(requiereRol(aplicacion.RolDistribucion, aplicacion.RolContabilidad))
				compuerta.Post("/{id}/firmar", a.firmarProceso)
				compuerta.Post("/{id}/rechazar", a.rechazarGateProceso)
			})
		})

		// La ingesta manual de reportes de uso. Pide `administrador` por lo
		// mismo que el catalogo: una entrega pondera el reparto de un periodo
		// entero, y el listado de cargas deja ver de que fuentes vive la
		// sociedad. La pantalla de ingesta de #29 lo confirmo: es solo de
		// administrador, y el log de rechazos de una carga va en el mismo
		// grupo porque es la misma pantalla.
		protegido.Route("/reportes", func(rep chi.Router) {
			rep.Use(requiereRol(aplicacion.RolAdministrador))
			rep.Post("/", a.conIngesta(a.subirReporte))
			rep.Get("/", a.conIngesta(a.listarCargas))
			rep.Get("/{id}/rechazos", a.conIngesta(a.listarRechazosDeCarga))
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
			h.Set("Access-Control-Expose-Headers", "Content-Disposition")
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
