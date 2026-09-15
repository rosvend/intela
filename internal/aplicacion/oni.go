package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rosvend/intela/internal/dominio/oni"
)

// periodoRe es el mismo CHECK del esquema: YYYY o YYYY-MM. Duplicarlo aqui
// evita un viaje a la base para rechazar "enero" o "2026/01".
var periodoRe = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2})?$`)

// PublicarListadoONI toma la cola viva de un periodo, la congela como
// listado publico (R-18) y ancla la prescripcion de tres anos (R-19).
//
// Las direcciones no viajan en la peticion: son las de REDES SGC, y se
// inyectan desde entorno. Si faltan, no se publica: un listado sin
// direccion viola RD 13.8.4.3.
type PublicarListadoONI struct {
	ONI         RepositorioPublicacionONI
	Bitacora    BitacoraAuditoria
	Reloj       Reloj
	Tx          UnidadDeTrabajo
	Fisica      string
	Electronica string
}

// Ejecutar publica el listado del periodo. actorID queda en el asiento;
// puede ir vacio si quien dispara es un proceso, no una persona.
func (c PublicarListadoONI) Ejecutar(ctx context.Context, periodo, actorID string) (PublicacionONI, error) {
	periodo = strings.TrimSpace(periodo)
	if !periodoRe.MatchString(periodo) {
		return PublicacionONI{}, ErrPeriodoInvalido
	}
	if err := oni.ValidarMetadatos(periodo, c.Fisica, c.Electronica); err != nil {
		if errors.Is(err, oni.ErrDireccionAusente) {
			return PublicacionONI{}, ErrDireccionPublicacionAusente
		}
		return PublicacionONI{}, err
	}

	ahora := c.Reloj.Ahora()
	if _, err := oni.AnclarFecha("", ahora.UTC().Format(time.RFC3339)); err != nil {
		return PublicacionONI{}, err
	}

	pendientes, err := c.ONI.PendientesDePeriodo(ctx, periodo)
	if err != nil {
		return PublicacionONI{}, fmt.Errorf("listar ONI del periodo %q: %w", periodo, err)
	}

	obras := make([]oni.ProyeccionPublica, 0, len(pendientes))
	usoIDs := make([]string, 0, len(pendientes))
	for _, d := range pendientes {
		p, err := oni.Proyectar(d)
		if err != nil {
			return PublicacionONI{}, fmt.Errorf("proyectar uso %q: %w", d.ID, err)
		}
		obras = append(obras, p)
		usoIDs = append(usoIDs, p.ID)
	}

	pub := PublicacionONI{
		Periodo:              periodo,
		FechaProceso:         ahora,
		DireccionFisica:      strings.TrimSpace(c.Fisica),
		DireccionElectronica: strings.TrimSpace(c.Electronica),
		Obras:                obras,
	}

	err = c.Tx.Ejecutar(ctx, func(ctx context.Context) error {
		guardada, err := c.ONI.GuardarPublicacion(ctx, pub)
		if err != nil {
			return err
		}
		pub = guardada

		if err := c.ONI.AnclarPrescripcion(ctx, usoIDs, ahora); err != nil {
			return fmt.Errorf("anclar prescripcion: %w", err)
		}

		payload, err := json.Marshal(payloadPublicacion{
			Periodo:              pub.Periodo,
			FechaProceso:         pub.FechaProceso.UTC().Format(time.RFC3339),
			NObras:               len(pub.Obras),
			UsoIDs:               usoIDs,
			DireccionFisica:      pub.DireccionFisica,
			DireccionElectronica: pub.DireccionElectronica,
		})
		if err != nil {
			return fmt.Errorf("serializar asiento: %w", err)
		}

		return c.Bitacora.Asentar(ctx, Asiento{
			Hecho:   HechoListadoONIPublicado,
			RefTipo: RefTipoPublicacionONI,
			RefID:   pub.ID,
			ActorID: actorID,
			Payload: payload,
			Cuando:  ahora,
		})
	})
	if err != nil {
		return PublicacionONI{}, err
	}
	return pub, nil
}

// payloadPublicacion es lo que entra al asiento. No lleva montos: el
// asiento documenta QUE se publico, no cuanto vale cada fila. El monto
// retenido vive en otros asientos del reparto.
type payloadPublicacion struct {
	Periodo              string   `json:"periodo"`
	FechaProceso         string   `json:"fecha_proceso"`
	NObras               int      `json:"n_obras"`
	UsoIDs               []string `json:"uso_ids"`
	DireccionFisica      string   `json:"direccion_fisica"`
	DireccionElectronica string   `json:"direccion_electronica"`
}

// ConsultarListadoONI sirve el listado ya publicado. Sin autenticacion:
// RD 13.8.1 obliga a publicarlo en la web.
type ConsultarListadoONI struct {
	ONI RepositorioPublicacionONI
}

// Ejecutar devuelve la publicacion del periodo, o la mas reciente si
// periodo viene vacio.
func (c ConsultarListadoONI) Ejecutar(ctx context.Context, periodo string) (PublicacionONI, error) {
	periodo = strings.TrimSpace(periodo)
	if periodo == "" {
		return c.ONI.PublicacionVigente(ctx)
	}
	if !periodoRe.MatchString(periodo) {
		return PublicacionONI{}, ErrPeriodoInvalido
	}
	return c.ONI.PublicacionDePeriodo(ctx, periodo)
}
