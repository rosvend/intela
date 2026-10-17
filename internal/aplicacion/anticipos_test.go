package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
	"github.com/rosvend/intela/internal/dominio/anticipos"
)

var _ RepositorioAnticipos = (*repoAnticiposMemoria)(nil)

// repoAnticiposMemoria es el doble de [RepositorioAnticipos].
//
// Emula las dos garantias que el contrato del puerto pone en la base: el indice
// unico de vigentes (Registrar falla con ErrAnticipoVigente si ya hay otro en
// tramite o aprobado) y la condicion de Resolver (solo desde `solicitado`). Un
// doble que no las emulara dejaria pasar un caso de uso que confia en ellas sin
// comprobar nada. Que la base las haga cumplir de verdad se prueba contra
// Postgres en postgres/anticipos_test.go.
//
// Tambien emula la vigencia de un embargo: embargos guarda, por titular, el dia
// en que se reporto uno que nadie ha levantado, y EmbargoVigente lo compara
// con el dia que recibe, en dias civiles como la consulta real. Un doble que
// ignorara el dia daria por bueno un caso de uso que preguntara por ayer.
type repoAnticiposMemoria struct {
	clases    map[string]afiliacion.Clase
	embargos  map[string]time.Time
	derechos  decimal.Decimal
	anticipos []AnticipoRegistrado

	desdeRecibido, hastaRecibido time.Time
	diaEmbargoRecibido           time.Time
	resueltoPor                  string
	bloqueosPorDescontar         int
	escrituras                   int
}

func (r *repoAnticiposMemoria) ClaseDe(_ context.Context, titularID string) (afiliacion.Clase, error) {
	c, hay := r.clases[titularID]
	if !hay {
		return "", ErrNoEncontrado
	}
	return c, nil
}

func (r *repoAnticiposMemoria) EmbargoVigente(_ context.Context, titularID string, dia time.Time) (bool, error) {
	r.diaEmbargoRecibido = dia
	reportado, hay := r.embargos[titularID]
	return hay && diaCivil(reportado) <= diaCivil(dia), nil
}

func (r *repoAnticiposMemoria) DerechosLiquidados(_ context.Context, _ string, desde, hasta time.Time) (decimal.Decimal, error) {
	r.desdeRecibido, r.hastaRecibido = desde, hasta
	return r.derechos, nil
}

func (r *repoAnticiposMemoria) DeTitular(_ context.Context, titularID string) ([]AnticipoRegistrado, error) {
	out := []AnticipoRegistrado{}
	for _, a := range r.anticipos {
		if a.Anticipo.TitularID == titularID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *repoAnticiposMemoria) Listar(context.Context) ([]AnticipoRegistrado, error) {
	return append([]AnticipoRegistrado{}, r.anticipos...), nil
}

func (r *repoAnticiposMemoria) PorID(_ context.Context, id string) (AnticipoRegistrado, error) {
	for _, a := range r.anticipos {
		if a.Anticipo.ID == id {
			return a, nil
		}
	}
	return AnticipoRegistrado{}, ErrNoEncontrado
}

func (r *repoAnticiposMemoria) Registrar(_ context.Context, a anticipos.Anticipo, cuando time.Time) error {
	for _, otro := range r.anticipos {
		if otro.Anticipo.TitularID == a.TitularID &&
			(otro.Anticipo.Estado == anticipos.EstadoSolicitado || otro.Anticipo.Estado == anticipos.EstadoAprobado) {
			return anticipos.ErrAnticipoVigente
		}
	}
	r.escrituras++
	r.anticipos = append(r.anticipos, AnticipoRegistrado{Anticipo: a, Solicitado: cuando})
	return nil
}

func (r *repoAnticiposMemoria) Resolver(_ context.Context, a anticipos.Anticipo, cuando time.Time, actorID string) error {
	for i, otro := range r.anticipos {
		if otro.Anticipo.ID != a.ID {
			continue
		}
		if otro.Anticipo.Estado != anticipos.EstadoSolicitado {
			return anticipos.ErrEstadoInvalido
		}
		r.escrituras++
		r.resueltoPor = actorID
		r.anticipos[i].Anticipo = a
		r.anticipos[i].Resuelto = cuando
		r.anticipos[i].ResueltoPor = actorID
		return nil
	}
	return ErrNoEncontrado
}

func (r *repoAnticiposMemoria) PorDescontar(_ context.Context, titularID string) (anticipos.Anticipo, bool, error) {
	r.bloqueosPorDescontar++
	for _, a := range r.anticipos {
		if a.Anticipo.TitularID == titularID && a.Anticipo.Estado == anticipos.EstadoAprobado {
			return a.Anticipo, true, nil
		}
	}
	return anticipos.Anticipo{}, false, nil
}

func (r *repoAnticiposMemoria) Descontar(_ context.Context, a anticipos.Anticipo, saldoAnterior decimal.Decimal) error {
	for i, otro := range r.anticipos {
		if otro.Anticipo.ID == a.ID && otro.Anticipo.Estado == anticipos.EstadoAprobado &&
			otro.Anticipo.Saldo.Equal(saldoAnterior) {
			r.escrituras++
			r.anticipos[i].Anticipo = a
			return nil
		}
	}
	return anticipos.ErrEstadoInvalido
}

type smmlvFijo struct {
	valor decimal.Decimal
	err   error
}

func (s smmlvFijo) SMMLVVigente(context.Context, time.Time) (decimal.Decimal, error) {
	return s.valor, s.err
}

var (
	instanteAnticipo = time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)

	socioAna         = Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}
	consejo          = Usuario{ID: "usr-consejo", Rol: RolAdministrador}
	tesoreria        = Usuario{ID: "usr-conta", Rol: RolContabilidad}
	administradoBeto = Usuario{ID: "usr-beto", Rol: RolTitular, TitularID: "tit-beto"}
	montoValid       = SolicitudAnticipo{Monto: liqDec("1000000"), Justificacion: "vivienda afectada por inundacion (RA 2.3.a)"}
)

type entornoAnticipos struct {
	repo   *repoAnticiposMemoria
	libro  *bitacoraFalsa
	unidad *unidadFalsa
	svc    Anticipos
}

// montarAnticipos deja a Ana como socio con 4.000.000 en derechos de la
// ventana (tope 1.000.000) y a Beto como titular administrado.
func montarAnticipos() *entornoAnticipos {
	e := &entornoAnticipos{
		repo: &repoAnticiposMemoria{
			clases: map[string]afiliacion.Clase{
				"tit-ana":  afiliacion.ClaseSocio,
				"tit-beto": afiliacion.ClaseAdministrado,
			},
			embargos: map[string]time.Time{},
			derechos: liqDec("4000000"),
		},
		libro:  &bitacoraFalsa{},
		unidad: &unidadFalsa{},
	}
	e.svc = Anticipos{
		Repo:     e.repo,
		SMMLV:    smmlvFijo{valor: liqDec("1300000")},
		IDs:      tokensFijos{valor: "abc"},
		Reloj:    relojFijo{instante: instanteAnticipo},
		Bitacora: e.libro,
		Unidad:   e.unidad,
	}
	return e
}

func payloadAnticipo(t *testing.T, a Asiento) AsientoAnticipo {
	t.Helper()
	var p AsientoAnticipo
	if err := json.Unmarshal(a.Payload, &p); err != nil {
		t.Fatalf("payload del asiento %q: %v", a.Hecho, err)
	}
	return p
}

func TestSolicitarAnticipoRegistraYAsientaElCupo(t *testing.T) {
	e := montarAnticipos()

	got, err := e.svc.Solicitar(context.Background(), socioAna, montoValid)
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}
	if got.Anticipo.ID != "ant-abc" || got.Anticipo.TitularID != "tit-ana" {
		t.Fatalf("identidad = %q/%q; el titular sale de la sesion", got.Anticipo.ID, got.Anticipo.TitularID)
	}
	if got.Anticipo.Estado != anticipos.EstadoSolicitado || !got.Solicitado.Equal(instanteAnticipo) {
		t.Fatalf("registrado = %+v", got)
	}
	if len(e.repo.anticipos) != 1 {
		t.Fatalf("se registraron %d anticipos", len(e.repo.anticipos))
	}
	if !e.unidad.confirmo {
		t.Fatal("la solicitud y su asiento son un solo hecho: van en una unidad")
	}

	if len(e.libro.asientos) != 1 {
		t.Fatalf("asientos = %d, se esperaba 1 (ADR 0006)", len(e.libro.asientos))
	}
	a := e.libro.asientos[0]
	if a.Hecho != HechoAnticipoSolicitado || a.RefTipo != RefAnticipo || a.RefID != "ant-abc" {
		t.Fatalf("asiento = %+v", a)
	}
	// La solicitud es una decision de una persona: la del socio.
	if a.ActorID != "usr-ana" {
		t.Fatalf("actor = %q", a.ActorID)
	}
	// El asiento es el "estado de cuenta unico y calculo del cupo" de
	// RA 3.2.2.d: de donde sale el tope y con que cifras.
	p := payloadAnticipo(t, a)
	if p.Cupo == nil {
		t.Fatal("el asiento de la solicitud tiene que llevar el calculo del cupo")
	}
	if p.Cupo.Tope != "1000000.00" || p.Cupo.Derechos != "4000000.00" || p.Cupo.SMMLV != "1300000.00" {
		t.Fatalf("cupo = %+v", *p.Cupo)
	}
	if p.Cupo.Desde != "2024-06-15" || p.Cupo.Hasta != "2026-06-15" {
		t.Fatalf("ventana = %s..%s, se esperaban los ultimos dos anos", p.Cupo.Desde, p.Cupo.Hasta)
	}
	if p.Monto != "1000000.00" || p.Estado != string(anticipos.EstadoSolicitado) {
		t.Fatalf("payload = %+v", p)
	}
}

// TestSolicitarAnticipoMideLaVentanaDeDosAnios: la base de RA 3.1.g se lee
// sobre "los ultimos dos anos" contados desde el reloj, no desde el ano civil.
func TestSolicitarAnticipoMideLaVentanaDeDosAnios(t *testing.T) {
	e := montarAnticipos()
	if _, err := e.svc.Solicitar(context.Background(), socioAna, montoValid); err != nil {
		t.Fatalf("Solicitar: %v", err)
	}
	if !e.repo.hastaRecibido.Equal(instanteAnticipo) {
		t.Fatalf("hasta = %s, se esperaba el instante del reloj", e.repo.hastaRecibido)
	}
	if !e.repo.desdeRecibido.Equal(instanteAnticipo.AddDate(-2, 0, 0)) {
		t.Fatalf("desde = %s, se esperaban dos anos antes del reloj", e.repo.desdeRecibido)
	}
	// El embargo se pregunta por el dia del reloj (RA 3.1.e), no por otro.
	if !e.repo.diaEmbargoRecibido.Equal(instanteAnticipo) {
		t.Fatalf("embargo consultado para %s, se esperaba el instante del reloj", e.repo.diaEmbargoRecibido)
	}
}

// TestSolicitarAnticipoAplicaLasReglas recorre R-30, R-31 y R-32 desde el caso
// de uso: cada rechazo llega con su centinela y no deja nada escrito.
func TestSolicitarAnticipoAplicaLasReglas(t *testing.T) {
	casos := []struct {
		nombre  string
		actor   Usuario
		monto   string
		ajustar func(*entornoAnticipos)
		quiero  error
	}{
		{nombre: "un titular administrado no pide (R-30)", actor: administradoBeto, monto: "1000", quiero: anticipos.ErrNoEsSocio},
		{nombre: "por encima del tope (R-31)", actor: socioAna, monto: "1000000.01", quiero: anticipos.ErrSuperaTope},
		{
			// Reportado el mismo dia de la solicitud: RA 3.1.e mira el dia de
			// hoy, no el de ayer.
			nombre: "con embargo reportado (R-32)", actor: socioAna, monto: "1000", quiero: anticipos.ErrEmbargoReportado,
			ajustar: func(e *entornoAnticipos) { e.repo.embargos["tit-ana"] = instanteAnticipo },
		},
		{
			nombre: "con una solicitud en tramite (R-32)", actor: socioAna, monto: "1000", quiero: anticipos.ErrAnticipoVigente,
			ajustar: func(e *entornoAnticipos) {
				e.repo.anticipos = []AnticipoRegistrado{{Anticipo: anticipos.Anticipo{
					ID: "ant-0", TitularID: "tit-ana", Monto: liqDec("10"), Estado: anticipos.EstadoSolicitado,
				}}}
			},
		},
		{
			nombre: "con saldo pendiente (R-32)", actor: socioAna, monto: "1000", quiero: anticipos.ErrSaldoPendiente,
			ajustar: func(e *entornoAnticipos) {
				e.repo.anticipos = []AnticipoRegistrado{{Anticipo: anticipos.Anticipo{
					ID: "ant-0", TitularID: "tit-ana", Monto: liqDec("10"), Saldo: liqDec("5"), Estado: anticipos.EstadoAprobado,
				}}}
			},
		},
		{
			nombre: "un titular que no figura en el padron", actor: Usuario{ID: "u", Rol: RolTitular, TitularID: "tit-nadie"},
			monto: "1000", quiero: ErrNoEncontrado,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e := montarAnticipos()
			if c.ajustar != nil {
				c.ajustar(e)
			}
			antes := len(e.repo.anticipos)
			_, err := e.svc.Solicitar(context.Background(), c.actor,
				SolicitudAnticipo{Monto: liqDec(c.monto), Justificacion: "x"})
			if !errors.Is(err, c.quiero) {
				t.Fatalf("err = %v, se esperaba %v", err, c.quiero)
			}
			if len(e.repo.anticipos) != antes || e.repo.escrituras != 0 {
				t.Fatal("un rechazo no deja la solicitud registrada")
			}
			if len(e.libro.asientos) != 0 {
				t.Fatal("un rechazo no deja asiento de solicitud")
			}
		})
	}
}

// TestSolicitarAnticipoLoPideElPropioSocio es RA 3.1.f: "La Solicitud tendra
// que ser hecha directamente por el Socio". Ni el Consejo ni otro rol la piden
// en su nombre, y el titular sale de la sesion, no del cuerpo.
func TestSolicitarAnticipoLoPideElPropioSocio(t *testing.T) {
	for _, actor := range []Usuario{
		consejo,
		tesoreria,
		{ID: "usr-x", Rol: RolTitular}, // titular sin fila del padron en la sesion
		{},
	} {
		e := montarAnticipos()
		if _, err := e.svc.Solicitar(context.Background(), actor, montoValid); !errors.Is(err, ErrNoAutorizado) {
			t.Fatalf("actor %+v: err = %v, se esperaba ErrNoAutorizado", actor, err)
		}
		if len(e.repo.anticipos) != 0 {
			t.Fatalf("actor %+v dejo una solicitud", actor)
		}
	}
}

// TestSolicitarAnticipoSinSMMLVFalla es el ADR 0004: sin SMMLV vigente no hay
// tope de RA 3.1.h, y un cero lo convertiria en "nadie tiene cupo" sin decir
// por que.
func TestSolicitarAnticipoSinSMMLVFalla(t *testing.T) {
	for nombre, fuente := range map[string]smmlvFijo{
		"ausente": {err: ErrParametroAusente},
		"cero":    {valor: decimal.Zero},
	} {
		t.Run(nombre, func(t *testing.T) {
			e := montarAnticipos()
			e.svc.SMMLV = fuente
			_, err := e.svc.Solicitar(context.Background(), socioAna, montoValid)
			if !errors.Is(err, ErrParametroAusente) {
				t.Fatalf("err = %v, se esperaba ErrParametroAusente", err)
			}
			if len(e.repo.anticipos) != 0 {
				t.Fatal("sin SMMLV no se registra nada")
			}
		})
	}
}

func solicitadoDeAna(t *testing.T, e *entornoAnticipos) AnticipoRegistrado {
	t.Helper()
	a, err := e.svc.Solicitar(context.Background(), socioAna, montoValid)
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}
	e.libro.asientos = nil
	e.repo.escrituras = 0
	return a
}

func TestAprobarAnticipo(t *testing.T) {
	e := montarAnticipos()
	pedido := solicitadoDeAna(t, e)
	e.svc.Reloj = relojFijo{instante: instanteAnticipo.Add(48 * time.Hour)}

	got, err := e.svc.Aprobar(context.Background(), consejo, pedido.Anticipo.ID, "acta 12, resolucion 34")
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	if got.Anticipo.Estado != anticipos.EstadoAprobado || !got.Anticipo.Saldo.Equal(liqDec("1000000")) {
		t.Fatalf("aprobado = %+v", got.Anticipo)
	}
	if got.ResueltoPor != "usr-consejo" || !got.Resuelto.Equal(instanteAnticipo.Add(48*time.Hour)) {
		t.Fatalf("resolucion = %q en %s", got.ResueltoPor, got.Resuelto)
	}
	if !got.Solicitado.Equal(instanteAnticipo) {
		t.Fatalf("solicitado = %s, la fecha de la solicitud no cambia al aprobar", got.Solicitado)
	}
	if e.repo.resueltoPor != "usr-consejo" {
		t.Fatalf("el repositorio recibio el actor %q", e.repo.resueltoPor)
	}

	if len(e.libro.asientos) != 1 {
		t.Fatalf("asientos = %d", len(e.libro.asientos))
	}
	a := e.libro.asientos[0]
	if a.Hecho != HechoAnticipoAprobado || a.ActorID != "usr-consejo" || a.RefID != pedido.Anticipo.ID {
		t.Fatalf("asiento = %+v", a)
	}
	p := payloadAnticipo(t, a)
	if p.Soporte != "acta 12, resolucion 34" || p.Saldo != "1000000.00" || p.EstadoAnterior != "solicitado" {
		t.Fatalf("payload = %+v", p)
	}
	// La aprobacion recalcula el cupo en SU fecha: es el que se otorga.
	if p.Cupo == nil || p.Cupo.Hasta != "2026-06-17" {
		t.Fatalf("cupo de la aprobacion = %+v", p.Cupo)
	}
}

// TestAprobarAnticipoLoDecideElConsejo: solo `administrador`, que representa al
// Consejo Directivo (RA 3.2.3, docs/architecture/roles.md).
func TestAprobarAnticipoLoDecideElConsejo(t *testing.T) {
	for _, actor := range []Usuario{socioAna, tesoreria, {ID: "usr-d", Rol: RolDistribucion}, {}} {
		e := montarAnticipos()
		pedido := solicitadoDeAna(t, e)
		if _, err := e.svc.Aprobar(context.Background(), actor, pedido.Anticipo.ID, "acta"); !errors.Is(err, ErrNoAutorizado) {
			t.Fatalf("actor %+v: err = %v", actor, err)
		}
		if _, err := e.svc.Rechazar(context.Background(), actor, pedido.Anticipo.ID, "no"); !errors.Is(err, ErrNoAutorizado) {
			t.Fatalf("actor %+v rechazando: err = %v", actor, err)
		}
		if e.repo.escrituras != 0 || len(e.libro.asientos) != 0 {
			t.Fatalf("actor %+v escribio", actor)
		}
	}
}

// TestAprobarAnticipoRevalidaEnLaFechaDeAprobacion: un embargo que llega entre
// la solicitud y la aprobacion impide el desembolso (RA 3.1.e). Se reporta el
// mismo dia en que el Consejo decide: es el dia de la aprobacion el que cuenta.
func TestAprobarAnticipoRevalidaEnLaFechaDeAprobacion(t *testing.T) {
	e := montarAnticipos()
	pedido := solicitadoDeAna(t, e)
	decide := instanteAnticipo.Add(48 * time.Hour)
	e.svc.Reloj = relojFijo{instante: decide}
	e.repo.embargos["tit-ana"] = decide

	_, err := e.svc.Aprobar(context.Background(), consejo, pedido.Anticipo.ID, "acta 12")
	if !errors.Is(err, anticipos.ErrEmbargoReportado) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.anticipos[0].Anticipo.Estado != anticipos.EstadoSolicitado || e.repo.escrituras != 0 {
		t.Fatal("una aprobacion rechazada no cambia el anticipo")
	}
}

func TestAprobarAnticipoInexistente(t *testing.T) {
	e := montarAnticipos()
	if _, err := e.svc.Aprobar(context.Background(), consejo, "ant-nada", "acta"); !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("err = %v", err)
	}
}

// TestAprobarAnticipoDosVecesNoDuplica: la segunda decision llega con el
// anticipo ya resuelto y no escribe nada mas.
func TestAprobarAnticipoDosVecesNoDuplica(t *testing.T) {
	e := montarAnticipos()
	pedido := solicitadoDeAna(t, e)
	if _, err := e.svc.Aprobar(context.Background(), consejo, pedido.Anticipo.ID, "acta 12"); err != nil {
		t.Fatalf("primera aprobacion: %v", err)
	}
	if _, err := e.svc.Aprobar(context.Background(), consejo, pedido.Anticipo.ID, "acta 13"); !errors.Is(err, anticipos.ErrEstadoInvalido) {
		t.Fatalf("segunda aprobacion: err = %v", err)
	}
	if len(e.libro.asientos) != 1 {
		t.Fatalf("asientos = %d, la segunda no asienta", len(e.libro.asientos))
	}
}

func TestRechazarAnticipo(t *testing.T) {
	e := montarAnticipos()
	pedido := solicitadoDeAna(t, e)

	got, err := e.svc.Rechazar(context.Background(), consejo, pedido.Anticipo.ID, "no demuestra la necesidad (RA 2.3)")
	if err != nil {
		t.Fatalf("Rechazar: %v", err)
	}
	if got.Anticipo.Estado != anticipos.EstadoRechazado || got.Anticipo.MotivoRechazo == "" {
		t.Fatalf("rechazado = %+v", got.Anticipo)
	}
	if len(e.libro.asientos) != 1 || e.libro.asientos[0].Hecho != HechoAnticipoRechazado {
		t.Fatalf("asientos = %+v", e.libro.asientos)
	}
	if p := payloadAnticipo(t, e.libro.asientos[0]); p.MotivoRechazo != "no demuestra la necesidad (RA 2.3)" {
		t.Fatalf("payload = %+v", p)
	}

	// Rechazado, el socio puede volver a pedir: un rechazo no lo deja atrapado.
	e.svc.IDs = tokensFijos{valor: "def"}
	if _, err := e.svc.Solicitar(context.Background(), socioAna, montoValid); err != nil {
		t.Fatalf("pedir de nuevo tras un rechazo: %v", err)
	}
}

func TestRechazarAnticipoSinMotivo(t *testing.T) {
	e := montarAnticipos()
	pedido := solicitadoDeAna(t, e)
	if _, err := e.svc.Rechazar(context.Background(), consejo, pedido.Anticipo.ID, "  "); !errors.Is(err, anticipos.ErrResolucionIncompleta) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.escrituras != 0 {
		t.Fatal("un rechazo sin motivo no se escribe")
	}
}

// TestListarAnticipos: el staff ve todos; el titular, solo los suyos (OE-6),
// recortado por el TitularID de la sesion.
func TestListarAnticipos(t *testing.T) {
	e := montarAnticipos()
	e.repo.anticipos = []AnticipoRegistrado{
		{Anticipo: anticipos.Anticipo{ID: "ant-1", TitularID: "tit-ana", Estado: anticipos.EstadoSolicitado}},
		{Anticipo: anticipos.Anticipo{ID: "ant-2", TitularID: "tit-otro", Estado: anticipos.EstadoRechazado}},
	}
	for _, staff := range []Usuario{consejo, tesoreria, {ID: "d", Rol: RolDistribucion}, {ID: "a", Rol: RolAuditor}} {
		got, err := e.svc.Listar(context.Background(), staff)
		if err != nil || len(got) != 2 {
			t.Fatalf("%s: %d anticipos, err %v", staff.Rol, len(got), err)
		}
	}
	got, err := e.svc.Listar(context.Background(), socioAna)
	if err != nil || len(got) != 1 || got[0].Anticipo.ID != "ant-1" {
		t.Fatalf("titular: %+v, err %v", got, err)
	}
	for _, nadie := range []Usuario{{}, {ID: "x", Rol: RolTitular}} {
		if _, err := e.svc.Listar(context.Background(), nadie); !errors.Is(err, ErrNoAutorizado) {
			t.Fatalf("%+v: err = %v", nadie, err)
		}
	}
}

func aprobadoDeAna(t *testing.T, e *entornoAnticipos) AnticipoRegistrado {
	t.Helper()
	pedido := solicitadoDeAna(t, e)
	a, err := e.svc.Aprobar(context.Background(), consejo, pedido.Anticipo.ID, "acta 12")
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	e.libro.asientos = nil
	e.repo.escrituras = 0
	return a
}

// TestRecuperarDescuentaYAsienta es la mitad de R-33 que vive en anticipos:
// el saldo baja, queda escrito, y el asiento dice de que orden salio.
func TestRecuperarDescuentaYAsienta(t *testing.T) {
	e := montarAnticipos()
	a := aprobadoDeAna(t, e)

	descuento, err := e.svc.Recuperar(context.Background(), "tit-ana", "liq-2026-nacional-tit-ana", liqDec("400000"))
	if err != nil {
		t.Fatalf("Recuperar: %v", err)
	}
	if !descuento.Equal(liqDec("400000")) {
		t.Fatalf("descuento = %s", descuento)
	}
	quedo, _ := e.repo.PorID(context.Background(), a.Anticipo.ID)
	if !quedo.Anticipo.Saldo.Equal(liqDec("600000")) || quedo.Anticipo.Estado != anticipos.EstadoAprobado {
		t.Fatalf("anticipo = %+v", quedo.Anticipo)
	}
	if e.repo.bloqueosPorDescontar != 1 {
		t.Fatalf("PorDescontar se pidio %d veces; el saldo se lee con la fila bloqueada", e.repo.bloqueosPorDescontar)
	}
	if len(e.libro.asientos) != 1 {
		t.Fatalf("asientos = %d", len(e.libro.asientos))
	}
	as := e.libro.asientos[0]
	// Lo produce la liquidacion, no una persona: actor del sistema.
	if as.Hecho != HechoAnticipoDescontado || as.RefID != a.Anticipo.ID || as.ActorID != actorSistema {
		t.Fatalf("asiento = %+v", as)
	}
	p := payloadAnticipo(t, as)
	if p.OrdenID != "liq-2026-nacional-tit-ana" || p.Descontado != "400000.00" ||
		p.SaldoAnterior != "1000000.00" || p.Saldo != "600000.00" {
		t.Fatalf("payload = %+v", p)
	}

	// La siguiente liquidacion salda el resto y no mas.
	descuento, err = e.svc.Recuperar(context.Background(), "tit-ana", "liq-2027-nacional-tit-ana", liqDec("900000"))
	if err != nil || !descuento.Equal(liqDec("600000")) {
		t.Fatalf("segundo descuento = %s, err %v", descuento, err)
	}
	quedo, _ = e.repo.PorID(context.Background(), a.Anticipo.ID)
	if !quedo.Anticipo.Saldo.IsZero() || quedo.Anticipo.Estado != anticipos.EstadoDescontado {
		t.Fatalf("anticipo saldado = %+v", quedo.Anticipo)
	}

	// Saldado, la tercera liquidacion se paga entera.
	e.libro.asientos = nil
	descuento, err = e.svc.Recuperar(context.Background(), "tit-ana", "liq-2028-nacional-tit-ana", liqDec("900000"))
	if err != nil || !descuento.IsZero() {
		t.Fatalf("tercer descuento = %s, err %v", descuento, err)
	}
	if len(e.libro.asientos) != 0 {
		t.Fatal("sin descuento no hay asiento")
	}
}

// TestRecuperarSinAnticipoNoEscribe: sin anticipo aprobado, o sin nada que
// girar, el descuento es cero y no se toca nada. Es el camino por el que pasa
// toda orden de pago de quien no tiene anticipo.
func TestRecuperarSinAnticipoNoEscribe(t *testing.T) {
	e := montarAnticipos()
	descuento, err := e.svc.Recuperar(context.Background(), "tit-ana", "liq-1", liqDec("500"))
	if err != nil || !descuento.IsZero() {
		t.Fatalf("sin anticipo: descuento %s, err %v", descuento, err)
	}

	aprobadoDeAna(t, e)
	descuento, err = e.svc.Recuperar(context.Background(), "tit-ana", "liq-1", decimal.Zero)
	if err != nil || !descuento.IsZero() {
		t.Fatalf("neto cero: descuento %s, err %v", descuento, err)
	}
	if e.repo.escrituras != 0 || len(e.libro.asientos) != 0 {
		t.Fatal("un descuento cero no escribe ni asienta")
	}
}

func TestRecuperarSiElAsientoFallaDevuelveElError(t *testing.T) {
	e := montarAnticipos()
	aprobadoDeAna(t, e)
	e.libro.err = errors.New("bitacora caida")
	if _, err := e.svc.Recuperar(context.Background(), "tit-ana", "liq-1", liqDec("10")); err == nil {
		t.Fatal("un descuento sin asiento no esta hecho (ADR 0006): el error tiene que subir para revertir la emision")
	}
}

func TestAnticiposExigeEstarCableado(t *testing.T) {
	if _, err := (Anticipos{}).Solicitar(context.Background(), socioAna, montoValid); err == nil {
		t.Fatal("sin puertos no se puede solicitar")
	}
	if _, err := (Anticipos{}).Recuperar(context.Background(), "tit-ana", "liq-1", liqDec("1")); err == nil {
		t.Fatal("sin puertos no se puede recuperar: la liquidacion no puede seguir como si no hubiera anticipo")
	}
}
