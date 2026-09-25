package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

type procesosFalso struct {
	vista aplicacion.ProcesoVista
	lista []aplicacion.ProcesoVista
	err   error

	idRecibido       string
	periodoRecibido  string
	circuitoRecibido reparto.Circuito
	bolsaIDRecibida  string
	rolRecibido      reparto.RolAcompuerta
	actorRecibido    string
	motivoRecibido   string
}

func (p *procesosFalso) IniciarProceso(_ context.Context, id, periodo string, circuito reparto.Circuito, bolsaID string) (aplicacion.ProcesoVista, error) {
	p.idRecibido, p.periodoRecibido, p.circuitoRecibido, p.bolsaIDRecibida = id, periodo, circuito, bolsaID
	return p.vista, p.err
}
func (p *procesosFalso) AvanzarEtapa(_ context.Context, id string) (aplicacion.ProcesoVista, error) {
	p.idRecibido = id
	return p.vista, p.err
}
func (p *procesosFalso) Firmar(_ context.Context, id string, rol reparto.RolAcompuerta, actorID string) (aplicacion.ProcesoVista, error) {
	p.idRecibido, p.rolRecibido, p.actorRecibido = id, rol, actorID
	return p.vista, p.err
}
func (p *procesosFalso) RechazarGate(_ context.Context, id, motivo string) (aplicacion.ProcesoVista, error) {
	p.idRecibido, p.motivoRecibido = id, motivo
	return p.vista, p.err
}
func (p *procesosFalso) ConsultarEstadoProceso(_ context.Context, id string) (aplicacion.ProcesoVista, error) {
	p.idRecibido = id
	return p.vista, p.err
}
func (p *procesosFalso) ListarProcesos(context.Context) ([]aplicacion.ProcesoVista, error) {
	return p.lista, p.err
}

func procesoVistaDeEjemplo() aplicacion.ProcesoVista {
	return aplicacion.ProcesoVista{
		ID:         "proc-1",
		Circuito:   reparto.Nacional,
		Etapa:      reparto.EtapaVerificacion,
		Periodo:    "2026-01",
		BolsaID:    "bolsa-1",
		SnapshotID: "snap-1",
		Reglamento: "IX",
		Revision:   1,
		Firmas:     []reparto.Firma{{Rol: "distribucion", ActorID: "usr-dist", SobreRev: 1}},
	}
}

const cuerpoAbrirProceso = `{
  "id": "proc-1",
  "periodo": "2026-01",
  "circuito": "nacional",
  "bolsa_id": "bolsa-1"
}`

func servidorConProcesos(t *testing.T, rol aplicacion.Rol, procesos *procesosFalso) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-" + string(rol), Rol: rol}}
	return Nueva(Casos{Auth: auth, Procesos: procesos}, Opciones{}).Router()
}

func TestProcesosExigeRolQuePuedaOperarElPipelineOFirmar(t *testing.T) {
	// titular queda fuera de todo el modulo: OE-6 solo le da las obras donde
	// participa, no el flujo de aprobaciones del recaudo entero.
	peticiones := []struct{ metodo, ruta, cuerpo string }{
		{http.MethodGet, "/procesos", ""},
		{http.MethodGet, "/procesos/proc-1", ""},
		{http.MethodPost, "/procesos", cuerpoAbrirProceso},
		{http.MethodPost, "/procesos/proc-1/avanzar", ""},
		{http.MethodPost, "/procesos/proc-1/firmar", ""},
		{http.MethodPost, "/procesos/proc-1/rechazar", `{"motivo":"faltan soportes"}`},
	}
	h := servidorConProcesos(t, aplicacion.RolTitular, &procesosFalso{})
	for _, p := range peticiones {
		t.Run(p.metodo+" "+p.ruta, func(t *testing.T) {
			rec := pedir(t, h, p.metodo, p.ruta, p.cuerpo, "tok")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestProcesosDistribucionYContabilidadLeenYFirmanPeroNoAbrenNiAvanzan(t *testing.T) {
	// distribucion y contabilidad son las dos firmas de la compuerta
	// (roles.md), no quien opera el pipeline -eso es administrador.
	for _, rol := range []aplicacion.Rol{aplicacion.RolDistribucion, aplicacion.RolContabilidad} {
		t.Run(string(rol), func(t *testing.T) {
			h := servidorConProcesos(t, rol, &procesosFalso{vista: procesoVistaDeEjemplo()})

			if rec := pedir(t, h, http.MethodGet, "/procesos", "", "tok"); rec.Code != http.StatusOK {
				t.Fatalf("GET /procesos = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
			}
			if rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/firmar", "", "tok"); rec.Code != http.StatusOK {
				t.Fatalf("POST firmar = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
			}
			if rec := pedir(t, h, http.MethodPost, "/procesos", cuerpoAbrirProceso, "tok"); rec.Code != http.StatusForbidden {
				t.Fatalf("POST /procesos = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
			}
			if rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/avanzar", "", "tok"); rec.Code != http.StatusForbidden {
				t.Fatalf("POST avanzar = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestAbrirProcesoDevuelve201YLocation(t *testing.T) {
	falso := &procesosFalso{vista: procesoVistaDeEjemplo()}
	h := servidorConProcesos(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, "/procesos", cuerpoAbrirProceso, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/procesos/proc-1" {
		t.Fatalf("Location = %q", loc)
	}
	if falso.periodoRecibido != "2026-01" || falso.circuitoRecibido != reparto.Nacional || falso.bolsaIDRecibida != "bolsa-1" {
		t.Fatalf("datos recibidos: periodo=%q circuito=%q bolsa=%q", falso.periodoRecibido, falso.circuitoRecibido, falso.bolsaIDRecibida)
	}

	var cuerpo procesoJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es un proceso: %v", err)
	}
	if cuerpo.ID != "proc-1" || cuerpo.Etapa != string(reparto.EtapaVerificacion) {
		t.Fatalf("cuerpo = %+v", cuerpo)
	}
}

func TestFirmarUsaElRolYElActorDeLaSesionNoDelCuerpo(t *testing.T) {
	// La firma no puede salir de un campo del cuerpo: si pudiera, un actor de
	// contabilidad podria firmar "como distribucion" y la doble firma de
	// RD 13.5 dejaria de ser un control de dos personas.
	falso := &procesosFalso{vista: procesoVistaDeEjemplo()}
	h := servidorConProcesos(t, aplicacion.RolContabilidad, falso)

	rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/firmar", `{"rol":"distribucion","actor_id":"otro"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.rolRecibido != reparto.RolContabilidad {
		t.Fatalf("rol recibido = %q, se esperaba el de la sesion (contabilidad), no el del cuerpo", falso.rolRecibido)
	}
	if falso.actorRecibido != "usr-contabilidad" {
		t.Fatalf("actor recibido = %q, se esperaba el de la sesion", falso.actorRecibido)
	}
}

func TestRechazarGateEnviaElMotivo(t *testing.T) {
	falso := &procesosFalso{vista: procesoVistaDeEjemplo()}
	h := servidorConProcesos(t, aplicacion.RolDistribucion, falso)

	rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/rechazar", `{"motivo":"faltan soportes"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.motivoRecibido != "faltan soportes" {
		t.Fatalf("motivo recibido = %q", falso.motivoRecibido)
	}
}

func TestProcesoPorIDNoEncontradoEs404(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolAuditor, &procesosFalso{err: aplicacion.ErrNoEncontrado})
	rec := pedir(t, h, http.MethodGet, "/procesos/proc-que-no-existe", "", "tok")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestAvanzarEtapaViolacionDeDominioEs409(t *testing.T) {
	// La compuerta sin las dos firmas es un estado invalido, no un dato mal
	// formado: el cuerpo de la peticion es correcto, lo que no cuadra es el
	// estado del proceso contra RD 13.5.
	h := servidorConProcesos(t, aplicacion.RolAdministrador, &procesosFalso{err: reparto.ErrRepartoInvalido})
	rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/avanzar", "", "tok")
	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// TestAbrirProcesoDatoMalFormadoEs400 y TestRechazarGateMotivoVacioEs400
// reproducen el hallazgo #6 de la revision de #159: un dato de entrada mal
// formado -campo vacio, motivo en blanco- no es un conflicto de estado ni
// un fallo del servidor, es un 400.
func TestAbrirProcesoDatoMalFormadoEs400(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolAdministrador, &procesosFalso{err: reparto.ErrProcesoInvalido})
	rec := pedir(t, h, http.MethodPost, "/procesos", cuerpoAbrirProceso, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestRechazarGateMotivoVacioEs400(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolDistribucion, &procesosFalso{err: reparto.ErrProcesoInvalido})
	rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/rechazar", `{"motivo":""}`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// TestAbrirProcesoBolsaNoCoincideEs409 y TestAbrirProcesoIDReutilizadoEs409
// son el mismo hallazgo #2/#3: el dato en si esta bien formado, lo que
// conflictua es contra otro recurso (la bolsa) o contra un proceso que ya
// existe con otros datos.
func TestAbrirProcesoBolsaNoCoincideEs409(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolAdministrador, &procesosFalso{err: aplicacion.ErrProcesoBolsaNoCoincide})
	rec := pedir(t, h, http.MethodPost, "/procesos", cuerpoAbrirProceso, "tok")
	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestAbrirProcesoIDReutilizadoEs409(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolAdministrador, &procesosFalso{err: aplicacion.ErrProcesoIDReutilizado})
	rec := pedir(t, h, http.MethodPost, "/procesos", cuerpoAbrirProceso, "tok")
	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// TestAvanzarEtapaConflictoDeConcurrenciaEs409 es el hallazgo #4: otra
// transicion cambio la fila entre que se leyo y que se escribio. Tambien es
// 409 -conflicto contra el estado actual-, pero el cliente tiene que poder
// distinguirlo (releer y reintentar) de un ErrRepartoInvalido real, asi que
// el mensaje lo dice.
func TestAvanzarEtapaConflictoDeConcurrenciaEs409(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolAdministrador, &procesosFalso{err: aplicacion.ErrProcesoConflictoDeConcurrencia})
	rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/avanzar", "", "tok")
	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestAvanzarEtapaConCriticasAbiertasEs409(t *testing.T) {
	err := fmt.Errorf("avanzar etapa de %q: %w: 2 en %q", "proc-1", aplicacion.ErrAnomaliasCriticasAbiertas, "2026-01")
	h := servidorConProcesos(t, aplicacion.RolAdministrador, &procesosFalso{err: err})
	rec := pedir(t, h, http.MethodPost, "/procesos/proc-1/avanzar", "", "tok")
	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestListarProcesosSinFilasDevuelveListaVaciaNoNull(t *testing.T) {
	h := servidorConProcesos(t, aplicacion.RolAuditor, &procesosFalso{})
	rec := pedir(t, h, http.MethodGet, "/procesos", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Fatalf("cuerpo = %q, se esperaba una lista vacia []", body)
	}
}
