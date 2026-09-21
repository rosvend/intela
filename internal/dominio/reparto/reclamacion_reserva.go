package reparto

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// RolAvalReclamacion son los dos avales que RD 14.5.10-12 exige por escrito
// antes de ajustar y pagar una reclamacion contra la reserva. Cada numeral es
// un aval distinto: 14.5.11 (revisoria fiscal o auditoria interna, uno
// cualquiera de los dos) y 14.5.12 (distribucion y contabilidad).
type RolAvalReclamacion string

const (
	RolRevisoriaFiscalOAuditoriaInterna RolAvalReclamacion = "revisoria_fiscal_o_auditoria_interna"
	RolDistribucionYContabilidad        RolAvalReclamacion = "distribucion_y_contabilidad"
)

// AvalReclamacion es una firma sobre una reclamacion concreta.
type AvalReclamacion struct {
	Rol     RolAvalReclamacion
	ActorID string
}

// ReclamacionReserva es un reclamo de un titular contra la reserva de
// errores tecnicos de una corrida (RD 14.5). Solo pasa a pagable con los dos
// avales de RD 14.5.10-12, de roles distintos y actores distintos.
type ReclamacionReserva struct {
	ID              string
	TitularID       string
	ProcesoOrigenID string
	Detalle         string
	MontoSolicitado decimal.Decimal
	Avales          []AvalReclamacion
}

var (
	// ErrReclamacionAfiliacionPosterior: RD 14.5.5, no se atiende a quien no
	// estaba afiliado antes de que terminara el proceso del que sale la reserva.
	ErrReclamacionAfiliacionPosterior = errors.New("el titular no estaba afiliado antes del periodo del proceso (RD 14.5.5)")
	// ErrReclamacionErrorDeDeclaracion: RD 14.5.6, un error de la declaracion
	// lo resuelve quien la declaro, no la reserva.
	ErrReclamacionErrorDeDeclaracion = errors.New("no se atienden reclamaciones por errores de la declaracion (RD 14.5.6)")
)

// NuevaReclamacionReserva valida las dos exclusiones de elegibilidad de
// RD 14.5 antes de abrir el reclamo. afiliadoAntesDelPeriodo y
// esErrorDeDeclaracion los resuelve la capa de aplicacion (padron de
// afiliacion, motivo declarado): el dominio no consulta nada, solo decide.
func NuevaReclamacionReserva(
	id, titularID, procesoOrigenID, detalle string,
	montoSolicitado decimal.Decimal,
	afiliadoAntesDelPeriodo, esErrorDeDeclaracion bool,
) (ReclamacionReserva, error) {
	if !afiliadoAntesDelPeriodo {
		return ReclamacionReserva{}, ErrReclamacionAfiliacionPosterior
	}
	if esErrorDeDeclaracion {
		return ReclamacionReserva{}, ErrReclamacionErrorDeDeclaracion
	}
	// RD 14.3: cada reclamo se analiza y responde individualmente, por
	// escrito. Sin detalle no hay que responder.
	if strings.TrimSpace(detalle) == "" {
		return ReclamacionReserva{}, fmt.Errorf("%w: detalle vacio", ErrRepartoInvalido)
	}
	if err := exigirPositivo("monto_solicitado", montoSolicitado); err != nil {
		return ReclamacionReserva{}, err
	}
	return ReclamacionReserva{
		ID:              id,
		TitularID:       titularID,
		Detalle:         detalle,
		ProcesoOrigenID: procesoOrigenID,
		MontoSolicitado: montoSolicitado,
	}, nil
}

// Avalar agrega una firma. Mismo invariante que `firmas` (ADR sobre doble
// firma en el proceso): un actor no puede cubrir los dos roles, y un rol no
// avala dos veces.
func (r ReclamacionReserva) Avalar(rol RolAvalReclamacion, actorID string) (ReclamacionReserva, error) {
	if rol != RolRevisoriaFiscalOAuditoriaInterna && rol != RolDistribucionYContabilidad {
		return r, fmt.Errorf("%w: rol de aval desconocido %q (RD 14.5.10-12)", ErrRepartoInvalido, rol)
	}
	for _, a := range r.Avales {
		if a.Rol == rol {
			return r, fmt.Errorf("%w: el rol %q ya avalo esta reclamacion", ErrRepartoInvalido, rol)
		}
		if a.ActorID == actorID {
			return r, fmt.Errorf("%w: el actor %q ya cubre otro rol, no puede avalar los dos", ErrRepartoInvalido, actorID)
		}
	}
	avales := make([]AvalReclamacion, len(r.Avales), len(r.Avales)+1)
	copy(avales, r.Avales)
	r.Avales = append(avales, AvalReclamacion{Rol: rol, ActorID: actorID})
	return r, nil
}

// Pagable es true solo con los dos avales de RD 14.5.10-12.
func (r ReclamacionReserva) Pagable() bool {
	var tieneRevisoria, tieneDistribucion bool
	for _, a := range r.Avales {
		switch a.Rol {
		case RolRevisoriaFiscalOAuditoriaInterna:
			tieneRevisoria = true
		case RolDistribucionYContabilidad:
			tieneDistribucion = true
		}
	}
	return tieneRevisoria && tieneDistribucion
}
