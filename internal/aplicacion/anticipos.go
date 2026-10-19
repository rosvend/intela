package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/anticipos"
)

// RefAnticipo es el tipo de referencia de los asientos de este modulo. Con el
// id del anticipo, [BitacoraAuditoria.De] devuelve su historia completa:
// solicitud, decision del Consejo y cada descuento, en orden (ADR 0006).
const RefAnticipo = "anticipo"

// Hechos que el modulo de anticipos asienta. Son SUYOS: el descuento de R-33 se
// asienta aqui con la referencia del anticipo, y la orden de pago lo muestra en
// su propio asiento de emision como un renglon del desglose. Ninguno escribe en
// la trazabilidad del otro (ADR 0003, ADR 0006).
const (
	// HechoAnticipoSolicitado lleva el calculo del cupo: es el "estado de
	// cuenta unico y calculo del cupo" que RA 3.2.2.d pide adjuntar.
	HechoAnticipoSolicitado = "anticipo.solicitado"
	// HechoAnticipoAprobado lleva la ubicacion del soporte (RA 3.2.5) y el
	// cupo recalculado en la fecha de la aprobacion.
	HechoAnticipoAprobado = "anticipo.aprobado"
	// HechoAnticipoRechazado lleva la justificacion literal (RA 3.2.3).
	HechoAnticipoRechazado = "anticipo.rechazado"
	// HechoAnticipoGirado es el peso que SALE: la transferencia al socio
	// (RA 3.2.4) con la ubicacion de su registro (RA 3.3). Desde aqui R-33
	// puede descontar.
	HechoAnticipoGirado = "anticipo.girado"
	// HechoAnticipoDescontado es R-33: cuanto se recupero, de que orden y que
	// saldo quedo.
	HechoAnticipoDescontado = "anticipo.descontado"
)

const prefijoAnticipo = "ant-"

// Anticipos es el modulo de anticipos a socios (R-30..R-33).
//
// El rol `administrador` representa aqui al Consejo Directivo, igual que en
// [Admision]: es quien aprueba o niega cada anticipo uno por uno (RA 3.2.3).
// La delegacion en el Gerente del paragrafo de RA 3.2.3 no esta modelada.
// `contabilidad` registra el giro (RA 3.2.4, RA 3.3), y tiene que ser otra
// persona que quien aprobo: el dinero no sale con una sola firma (ADR 0027).
//
// # Por que una unidad de trabajo
//
// Pedir, aprobar y negar son dos escrituras -la fila y su asiento- que son un
// solo hecho (ADR 0006), por la misma razon que en [Catalogo]. Recuperar NO
// abre la suya: corre dentro de la de [Liquidaciones.GenerarLiquidacion],
// porque el descuento, el saldo y la orden que lo muestra son un solo hecho.
type Anticipos struct {
	Repo     RepositorioAnticipos
	SMMLV    FuenteSMMLV
	IDs      GeneradorTokens
	Reloj    Reloj
	Bitacora BitacoraAuditoria
	Unidad   UnidadDeTrabajo
}

var _ RecuperacionAnticipos = Anticipos{}

// SolicitudAnticipo es lo que el socio manda (RA 3.2.1): el monto y la
// justificacion -destino de los recursos y necesidad-. El titular no viaja
// aqui: sale de la sesion.
type SolicitudAnticipo struct {
	Monto         decimal.Decimal
	Justificacion string
}

// Solicitar registra la solicitud del socio autenticado.
//
// Solo un titular con fila en el padron, y solo para si mismo: RA 3.1.f, "La
// Solicitud tendra que ser hecha directamente por el Socio ... No se admitiran
// solicitudes de anticipos a traves de ningun tercero". Ni el Consejo la pide
// en su nombre. El apoderado por fuerza mayor del mismo numeral no esta
// modelado.
//
// R-30, R-31 y R-32 se aplican YA al pedir: quien no tiene cupo, o tiene un
// anticipo sin saldar, recibe el motivo en el acto en vez de dejar una
// solicitud que el Consejo tendria que negar.
func (s Anticipos) Solicitar(ctx context.Context, actor Usuario, in SolicitudAnticipo) (AnticipoRegistrado, error) {
	if actor.Rol != RolTitular || strings.TrimSpace(actor.TitularID) == "" {
		return AnticipoRegistrado{}, ErrNoAutorizado
	}
	if err := s.cableadoParaDecidir(); err != nil {
		return AnticipoRegistrado{}, err
	}
	token, err := s.IDs.Generar()
	if err != nil {
		return AnticipoRegistrado{}, fmt.Errorf("generar id de anticipo: %w", err)
	}
	ahora := s.Reloj.Ahora().UTC()

	var registrado AnticipoRegistrado
	err = s.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		sit, cupo, err := s.situacion(ctx, actor.TitularID, ahora)
		if err != nil {
			return err
		}
		a, err := anticipos.Solicitar(anticipos.Solicitud{
			ID:            prefijoAnticipo + token,
			TitularID:     actor.TitularID,
			Monto:         in.Monto,
			Justificacion: in.Justificacion,
		}, sit)
		if err != nil {
			return err
		}
		if err := s.Repo.Registrar(ctx, a, ahora); err != nil {
			return fmt.Errorf("registrar el anticipo de %s: %w", a.TitularID, err)
		}
		p := asientoDeAnticipo(a, "")
		p.Justificacion = a.Justificacion
		p.Cupo = &cupo
		if err := s.asentar(ctx, HechoAnticipoSolicitado, a.ID, actor.ID, p); err != nil {
			return err
		}
		registrado = AnticipoRegistrado{Anticipo: a, Solicitado: ahora}
		return nil
	})
	if err != nil {
		return AnticipoRegistrado{}, err
	}
	return registrado, nil
}

// Aprobar es la decision del Consejo Directivo (RA 3.2.3) sobre una solicitud
// en tramite. soporte es la ubicacion del acta y la resolucion en el
// repositorio electronico (RA 3.2.5).
//
// Las reglas se vuelven a aplicar con la situacion de HOY, no la del dia en que
// se pidio: lo que el reglamento prohibe es desembolsar a quien tenga embargos
// reportados (RA 3.1.e) o mas de lo que permite el cupo, y entre pedir y
// aprobar puede cambiar cualquiera de las dos cosas.
func (s Anticipos) Aprobar(ctx context.Context, actor Usuario, id, soporte string) (AnticipoRegistrado, error) {
	return s.resolver(ctx, actor, id, HechoAnticipoAprobado,
		func(ctx context.Context, actual anticipos.Anticipo, ahora time.Time) (anticipos.Anticipo, *CupoAsentado, error) {
			sit, cupo, err := s.situacion(ctx, actual.TitularID, ahora)
			if err != nil {
				return anticipos.Anticipo{}, nil, err
			}
			aprobado, err := actual.Aprobar(soporte, sit)
			return aprobado, &cupo, err
		})
}

// Rechazar es la negacion del Consejo Directivo, con su justificacion literal
// (RA 3.2.3). Negado, el socio puede volver a pedir: un rechazo no es un
// anticipo vigente.
func (s Anticipos) Rechazar(ctx context.Context, actor Usuario, id, motivo string) (AnticipoRegistrado, error) {
	return s.resolver(ctx, actor, id, HechoAnticipoRechazado,
		func(_ context.Context, actual anticipos.Anticipo, _ time.Time) (anticipos.Anticipo, *CupoAsentado, error) {
			rechazado, err := actual.Rechazar(motivo)
			return rechazado, nil, err
		})
}

// Girar registra la transferencia al socio de un anticipo aprobado (RA 3.2.4).
// soporte es la ubicacion del registro de la transferencia (RA 3.3).
//
// Solo `contabilidad`, y nunca quien aprobo: la aprobacion del Consejo y el
// registro del giro son las dos firmas de la salida de dinero (ADR 0027). El
// embargo se vuelve a mirar con la fecha del giro, porque RA 3.1.e prohibe
// desembolsar, no aprobar.
func (s Anticipos) Girar(ctx context.Context, actor Usuario, id, soporte string) (AnticipoRegistrado, error) {
	if actor.Rol != RolContabilidad {
		return AnticipoRegistrado{}, ErrNoAutorizado
	}
	if err := s.cableadoParaDecidir(); err != nil {
		return AnticipoRegistrado{}, err
	}
	ahora := s.Reloj.Ahora().UTC()

	var girado AnticipoRegistrado
	err := s.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		actual, err := s.Repo.PorID(ctx, id)
		if err != nil {
			return fmt.Errorf("anticipo %q: %w", id, err)
		}
		if actual.ResueltoPor == actor.ID {
			return fmt.Errorf("%w: quien aprobo el anticipo %q no registra su giro", ErrNoAutorizado, id)
		}
		embargo, err := s.Repo.EmbargoVigente(ctx, actual.Anticipo.TitularID, ahora)
		if err != nil {
			return fmt.Errorf("embargos de %s: %w", actual.Anticipo.TitularID, err)
		}
		nuevo, err := actual.Anticipo.Girar(soporte, embargo)
		if err != nil {
			return err
		}
		if err := s.Repo.RegistrarGiro(ctx, nuevo, ahora, actor.ID); err != nil {
			return fmt.Errorf("registrar el giro del anticipo %q: %w", id, err)
		}
		p := asientoDeAnticipo(nuevo, actual.Anticipo.Estado)
		p.SoporteGiro = nuevo.SoporteGiro
		if err := s.asentar(ctx, HechoAnticipoGirado, nuevo.ID, actor.ID, p); err != nil {
			return err
		}
		girado = actual
		girado.Anticipo = nuevo
		girado.Girado = ahora
		girado.GiradoPor = actor.ID
		return nil
	})
	if err != nil {
		return AnticipoRegistrado{}, err
	}
	return girado, nil
}

// decision es lo que distingue aprobar de rechazar: el resto -quien decide, la
// unidad, la escritura condicional y el asiento- es comun.
type decision func(ctx context.Context, actual anticipos.Anticipo, ahora time.Time) (anticipos.Anticipo, *CupoAsentado, error)

func (s Anticipos) resolver(ctx context.Context, actor Usuario, id, hecho string, decidir decision) (AnticipoRegistrado, error) {
	if actor.Rol != RolAdministrador {
		return AnticipoRegistrado{}, ErrNoAutorizado
	}
	if err := s.cableadoParaDecidir(); err != nil {
		return AnticipoRegistrado{}, err
	}
	ahora := s.Reloj.Ahora().UTC()

	var resuelto AnticipoRegistrado
	err := s.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		actual, err := s.Repo.PorID(ctx, id)
		if err != nil {
			return fmt.Errorf("anticipo %q: %w", id, err)
		}
		nuevo, cupo, err := decidir(ctx, actual.Anticipo, ahora)
		if err != nil {
			return err
		}
		// Condicional en el adaptador: si otra decision gano la carrera,
		// esta no escribe y su asiento se revierte con la unidad.
		if err := s.Repo.Resolver(ctx, nuevo, ahora, actor.ID); err != nil {
			return fmt.Errorf("resolver el anticipo %q: %w", id, err)
		}
		p := asientoDeAnticipo(nuevo, actual.Anticipo.Estado)
		p.Soporte = nuevo.Soporte
		p.MotivoRechazo = nuevo.MotivoRechazo
		p.Cupo = cupo
		if err := s.asentar(ctx, hecho, nuevo.ID, actor.ID, p); err != nil {
			return err
		}
		resuelto = AnticipoRegistrado{
			Anticipo:    nuevo,
			Solicitado:  actual.Solicitado,
			Resuelto:    ahora,
			ResueltoPor: actor.ID,
		}
		return nil
	})
	if err != nil {
		return AnticipoRegistrado{}, err
	}
	return resuelto, nil
}

// Listar es GET /anticipos. El staff ve los de todos -el Consejo decide,
// contabilidad gira (RA 3.2.4), distribucion liquida y el auditor lee todo-; un
// titular ve solo los suyos, recortados por el TitularID de la sesion (OE-6).
//
// estado, si no va vacio, deja solo los de ese estado: lo que el Consejo mira
// es lo que sigue `solicitado`. Un estado que no existe es
// [anticipos.ErrSolicitudInvalida], no una lista vacia que parezca "no hay".
//
// La justificacion es la necesidad personal del socio (RA 3.2.1.d y e, RA
// 2.3) y solo la ven quienes deciden, giran o auditan, ademas del propio
// socio. `distribucion` liquida y no la necesita: le llega vacia.
func (s Anticipos) Listar(ctx context.Context, actor Usuario, estado string) ([]AnticipoRegistrado, error) {
	if s.Repo == nil {
		return nil, errors.New("anticipos mal cableados: falta RepositorioAnticipos")
	}
	filtro, err := anticipos.ParseEstado(estado)
	if err != nil {
		return nil, err
	}
	var lista []AnticipoRegistrado
	switch {
	case esStaff(actor.Rol):
		if lista, err = s.Repo.Listar(ctx); err != nil {
			return nil, fmt.Errorf("listar anticipos: %w", err)
		}
	case actor.Rol == RolTitular && strings.TrimSpace(actor.TitularID) != "":
		if lista, err = s.Repo.DeTitular(ctx, actor.TitularID); err != nil {
			return nil, fmt.Errorf("anticipos de %s: %w", actor.TitularID, err)
		}
	default:
		return nil, ErrNoAutorizado
	}
	out := make([]AnticipoRegistrado, 0, len(lista))
	for _, r := range lista {
		if filtro != "" && r.Anticipo.Estado != filtro {
			continue
		}
		if actor.Rol == RolDistribucion {
			r.Anticipo.Justificacion = ""
		}
		out = append(out, r)
	}
	return out, nil
}

// Recuperar es R-33 (RA 2.1): descuenta del importe que la orden ordenID iba a
// girar al titular lo que quede de su anticipo aprobado, y devuelve cuanto.
//
// Corre DENTRO de la unidad de la emision de [Liquidaciones]: lee el anticipo
// con su fila bloqueada, escribe el saldo y asienta el descuento con ese ctx,
// de modo que si la emision se revierte el descuento tampoco existio. Sin
// anticipo aprobado, o sin nada que girar, devuelve cero y no escribe nada: es
// el camino de toda orden de quien no tiene anticipo.
//
// El asiento lo produce el sistema, no una persona: [actorSistema], igual que
// la emision de la orden.
func (s Anticipos) Recuperar(ctx context.Context, titularID, ordenID string, disponible decimal.Decimal) (decimal.Decimal, error) {
	if err := s.cableadoParaRecuperar(); err != nil {
		return decimal.Zero, err
	}
	actual, hay, err := s.Repo.PorDescontar(ctx, titularID)
	if err != nil {
		return decimal.Zero, fmt.Errorf("anticipo por descontar de %s: %w", titularID, err)
	}
	if !hay {
		return decimal.Zero, nil
	}
	descuento, despues := actual.Recuperar(disponible)
	if !descuento.IsPositive() {
		return decimal.Zero, nil
	}
	if err := s.Repo.Descontar(ctx, despues, actual.Saldo); err != nil {
		return decimal.Zero, fmt.Errorf("descontar el anticipo %s: %w", actual.ID, err)
	}
	p := asientoDeAnticipo(despues, actual.Estado)
	p.OrdenID = ordenID
	p.Descontado = descuento.StringFixed(2)
	p.SaldoAnterior = actual.Saldo.StringFixed(2)
	if err := s.asentar(ctx, HechoAnticipoDescontado, despues.ID, actorSistema, p); err != nil {
		return decimal.Zero, err
	}
	return descuento, nil
}

// situacion arma lo que R-30..R-32 miran del titular en la fecha ahora.
//
// La ventana de RA 3.1.g son los [anticipos.VentanaAnios] anteriores a ahora, y
// el SMMLV el vigente ahora (ADR 0027). Devuelve tambien el calculo del cupo
// tal como se asienta.
func (s Anticipos) situacion(ctx context.Context, titularID string, ahora time.Time) (anticipos.Situacion, CupoAsentado, error) {
	clase, err := s.Repo.ClaseDe(ctx, titularID)
	if err != nil {
		return anticipos.Situacion{}, CupoAsentado{}, fmt.Errorf("clase del titular %s: %w", titularID, err)
	}
	embargo, err := s.Repo.EmbargoVigente(ctx, titularID, ahora)
	if err != nil {
		return anticipos.Situacion{}, CupoAsentado{}, fmt.Errorf("embargos de %s: %w", titularID, err)
	}
	registrados, err := s.Repo.DeTitular(ctx, titularID)
	if err != nil {
		return anticipos.Situacion{}, CupoAsentado{}, fmt.Errorf("anticipos de %s: %w", titularID, err)
	}
	// AddDate normaliza: desde un 29 de febrero, dos anos atras no hay 29 y
	// cae en el 1 de marzo, no en el 28 de febrero. Como la ventana es
	// abierta por abajo, ese dia la base pierde las ordenes enviadas el 1 de
	// marzo de hace dos anos. Queda anotado y no corregido: la lectura de
	// "los ultimos dos anos" en un bisiesto no la fija el reglamento.
	desde := ahora.AddDate(-anticipos.VentanaAnios, 0, 0)
	derechos, err := s.Repo.DerechosLiquidados(ctx, titularID, desde, ahora)
	if err != nil {
		return anticipos.Situacion{}, CupoAsentado{}, fmt.Errorf("derechos liquidados de %s: %w", titularID, err)
	}
	smmlv, err := s.SMMLV.SMMLVVigente(ctx, ahora)
	if err != nil {
		return anticipos.Situacion{}, CupoAsentado{}, fmt.Errorf("smmlv: %w", err)
	}
	// Un SMMLV no positivo no es un tope de cero: es un parametro que no esta
	// (ADR 0004). Con cero, nadie tendria cupo y nada diria por que.
	if !smmlv.IsPositive() {
		return anticipos.Situacion{}, CupoAsentado{}, fmt.Errorf("%w: %s", ErrParametroAusente, claveSMMLV)
	}

	previos := make([]anticipos.Anticipo, 0, len(registrados))
	for _, r := range registrados {
		previos = append(previos, r.Anticipo)
	}
	sit := anticipos.Situacion{
		Clase:            clase,
		EmbargoReportado: embargo,
		Anticipos:        previos,
		Derechos:         derechos,
		SMMLV:            smmlv,
	}
	cupo := CupoAsentado{
		Derechos: derechos.StringFixed(2),
		Desde:    diaCivil(desde),
		Hasta:    diaCivil(ahora),
		SMMLV:    smmlv.StringFixed(2),
		Tope:     anticipos.Tope(derechos, smmlv).StringFixed(2),
	}
	return sit, cupo, nil
}

// asentar escribe un asiento del modulo. El error no se descarta: un hecho sin
// asiento no esta hecho (ADR 0006), y devolverlo es lo que revierte la unidad.
func (s Anticipos) asentar(ctx context.Context, hecho, id, actorID string, p AsientoAnticipo) error {
	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("serializar el asiento del anticipo %q: %w", id, err)
	}
	if err := s.Bitacora.Asentar(ctx, Asiento{
		Hecho:   hecho,
		RefTipo: RefAnticipo,
		RefID:   id,
		ActorID: actorID,
		Payload: payload,
		Cuando:  s.Reloj.Ahora(),
	}); err != nil {
		return fmt.Errorf("asentar %q sobre el anticipo %q: %w", hecho, id, err)
	}
	return nil
}

// cableadoParaRecuperar comprueba lo que usa el descuento. Falla con un mensaje
// que nombra lo que falta, no con un nil pointer dentro de la transaccion de
// la emision; y fallar es lo correcto, porque una liquidacion que siguiera sin
// poder descontar giraria entero lo que el socio debe (R-33).
func (s Anticipos) cableadoParaRecuperar() error {
	switch {
	case s.Repo == nil:
		return errors.New("anticipos mal cableados: falta RepositorioAnticipos")
	case s.Reloj == nil:
		return errors.New("anticipos mal cableados: falta Reloj")
	case s.Bitacora == nil:
		return errors.New("anticipos mal cableados: falta BitacoraAuditoria")
	}
	return nil
}

// cableadoParaDecidir anade lo que solo usan pedir, aprobar y negar.
func (s Anticipos) cableadoParaDecidir() error {
	if err := s.cableadoParaRecuperar(); err != nil {
		return err
	}
	switch {
	case s.SMMLV == nil:
		return errors.New("anticipos mal cableados: falta FuenteSMMLV")
	case s.IDs == nil:
		return errors.New("anticipos mal cableados: falta GeneradorTokens")
	case s.Unidad == nil:
		return errors.New("anticipos mal cableados: falta UnidadDeTrabajo")
	}
	return nil
}

// AsientoAnticipo es el payload JSONB de los asientos del modulo. Tipo propio
// y no [anticipos.Anticipo] serializado, por lo mismo que [AsientoOrden]: el
// libro se conserva diez anos y tiene que seguir siendo legible por una
// persona. Los montos van como cadena (ADR 0010).
type AsientoAnticipo struct {
	TitularID string `json:"titular_id"`
	Monto     string `json:"monto"`
	Saldo     string `json:"saldo"`
	Estado    string `json:"estado"`

	// EstadoAnterior va vacio en la solicitud -no habia nada antes-.
	EstadoAnterior string `json:"estado_anterior,omitempty"`

	Justificacion string `json:"justificacion,omitempty"`
	Soporte       string `json:"soporte,omitempty"`
	MotivoRechazo string `json:"motivo_rechazo,omitempty"`
	SoporteGiro   string `json:"soporte_giro,omitempty"`

	// Cupo va en la solicitud y en la aprobacion: es el calculo con el que se
	// decidio, en la fecha de cada una.
	Cupo *CupoAsentado `json:"cupo,omitempty"`

	// OrdenID, Descontado y SaldoAnterior van en el descuento de R-33.
	OrdenID       string `json:"orden_id,omitempty"`
	Descontado    string `json:"descontado,omitempty"`
	SaldoAnterior string `json:"saldo_anterior,omitempty"`
}

// CupoAsentado es el calculo de R-31 tal como se decidio: la base de RA 3.1.g
// con su ventana, el SMMLV de RA 3.1.h y el tope que salio de los dos.
type CupoAsentado struct {
	Derechos string `json:"derechos"`
	Desde    string `json:"desde"`
	Hasta    string `json:"hasta"`
	SMMLV    string `json:"smmlv"`
	Tope     string `json:"tope"`
}

func asientoDeAnticipo(a anticipos.Anticipo, anterior anticipos.Estado) AsientoAnticipo {
	return AsientoAnticipo{
		TitularID:      a.TitularID,
		Monto:          a.Monto.StringFixed(2),
		Saldo:          a.Saldo.StringFixed(2),
		Estado:         string(a.Estado),
		EstadoAnterior: string(anterior),
	}
}
