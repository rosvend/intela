package reparto

import (
	"fmt"
	"strings"
)

// ProcesoDeReparto es el agregado que recorre RD 13.5: Nacional e
// Internacional son dos maquinas de estado distintas, no una con bandera
// (ADR 0008) -- [secuenciaEtapas] las mantiene separadas.
type ProcesoDeReparto struct {
	ID            string
	Periodo       string
	Circuito      Circuito
	BolsaID       string
	SnapshotID    string
	Reglamento    string
	Etapa         Etapa
	Revision      int
	Firmas        []Firma
	RechazoMotivo string
}

// secuenciaEtapas fija el recorrido de cada circuito, verbatim RD 13.5. El
// internacional no valoriza por puntos (RD 7.4): sin importe_obra ni
// importe_titular. "Fees in Error" es solo suyo (R-16, RD 13.7).
func secuenciaEtapas(c Circuito) []Etapa {
	if c == Internacional {
		return []Etapa{
			EtapaRecaudo, EtapaDeducciones, EtapaLiquidacionParcial,
			EtapaVerificacion, EtapaLiquidacionFinal, EtapaPagoRegistro,
			EtapaFeesInError, EtapaAuditoria,
		}
	}
	return []Etapa{
		EtapaRecaudo, EtapaDeducciones, EtapaImporteObra, EtapaImporteTitular,
		EtapaLiquidacionParcial, EtapaVerificacion, EtapaLiquidacionFinal,
		EtapaPagoRegistro, EtapaAuditoria,
	}
}

// RolAcompuerta son los roles que firman una compuerta de RD 13.5.
//
// El reglamento nombra tres responsables (Contabilidad, Gerencia y
// Tesoreria) para Verificacion y Pago y Registro, pero el modelo de firmas
// hoy solo tiene dos roles (`firmas.rol`, PR #104 ya escrito contra ellos).
// Subir a los tres roles del texto verbatim es una migracion aparte,
// deliberadamente diferida -- ver ADR 0020.
type RolAcompuerta string

const (
	RolDistribucion RolAcompuerta = "distribucion"
	RolContabilidad RolAcompuerta = "contabilidad"
)

// etapasConCompuerta son las etapas entre corchetes de ADR 0008: no se
// avanza sin las dos firmas, y un rechazo aqui retrocede en vez de abortar.
func etapaEsCompuerta(e Etapa) bool {
	return e == EtapaVerificacion || e == EtapaPagoRegistro
}

// Firmar agrega una firma a la compuerta actual. Mismo invariante que
// [ReclamacionReserva.Avalar]: un rol no firma dos veces la misma revision,
// y un actor no cubre los dos roles.
func (p ProcesoDeReparto) Firmar(rol RolAcompuerta, actorID string) (ProcesoDeReparto, error) {
	if !etapaEsCompuerta(p.Etapa) {
		return p, fmt.Errorf("%w: %q no es una etapa con compuerta (RD 13.5)", ErrRepartoInvalido, p.Etapa)
	}
	if rol != RolDistribucion && rol != RolContabilidad {
		return p, fmt.Errorf("%w: rol de firma desconocido %q (RD 13.5)", ErrRepartoInvalido, rol)
	}
	for _, f := range p.Firmas {
		if f.SobreRev != p.Revision {
			continue
		}
		if f.Rol == string(rol) {
			return p, fmt.Errorf("%w: el rol %q ya firmo esta revision", ErrRepartoInvalido, rol)
		}
		if f.ActorID == actorID {
			return p, fmt.Errorf("%w: el actor %q ya cubre otro rol, no puede firmar los dos", ErrRepartoInvalido, actorID)
		}
	}
	firmas := make([]Firma, len(p.Firmas), len(p.Firmas)+1)
	copy(firmas, p.Firmas)
	p.Firmas = append(firmas, Firma{Rol: string(rol), ActorID: actorID, SobreRev: p.Revision})
	return p, nil
}

// firmasCompletas es true cuando los dos roles firmaron la revision actual.
func (p ProcesoDeReparto) firmasCompletas() bool {
	var tieneDistribucion, tieneContabilidad bool
	for _, f := range p.Firmas {
		if f.SobreRev != p.Revision {
			continue
		}
		switch RolAcompuerta(f.Rol) {
		case RolDistribucion:
			tieneDistribucion = true
		case RolContabilidad:
			tieneContabilidad = true
		}
	}
	return tieneDistribucion && tieneContabilidad
}

// AvanzarEtapa mueve el proceso a la siguiente etapa de su circuito. En una
// compuerta exige las dos firmas de la revision actual antes de dejar
// pasar; auditoria es terminal.
//
// `firmas` solo tiene clave (proceso_id, rol, revision), sin etapa: nada
// distingue una firma de Verificacion de una de Pago y Registro salvo la
// revision. Por eso salir de una compuerta sube la revision igual que un
// rechazo -- si no, las firmas de la primera compuerta bastarian tambien
// para la segunda.
func (p ProcesoDeReparto) AvanzarEtapa() (ProcesoDeReparto, error) {
	esCompuerta := etapaEsCompuerta(p.Etapa)
	if esCompuerta && !p.firmasCompletas() {
		return p, fmt.Errorf("%w: %q exige las firmas de distribucion y contabilidad antes de avanzar (RD 13.5)", ErrRepartoInvalido, p.Etapa)
	}
	secuencia := secuenciaEtapas(p.Circuito)
	idx := -1
	for i, e := range secuencia {
		if e == p.Etapa {
			idx = i
			break
		}
	}
	if idx < 0 || idx == len(secuencia)-1 {
		return p, fmt.Errorf("%w: %q es terminal para el circuito %q", ErrRepartoInvalido, p.Etapa, p.Circuito)
	}
	p.Etapa = secuencia[idx+1]
	if esCompuerta {
		p.Revision++
	}
	return p, nil
}

// RechazarGate retrocede una etapa (nunca a un estado terminal) y sube la
// revision, invalidando las firmas anteriores -- un rechazo retrocede,
// no aborta (ADR 0008).
func (p ProcesoDeReparto) RechazarGate(motivo string) (ProcesoDeReparto, error) {
	if !etapaEsCompuerta(p.Etapa) {
		return p, fmt.Errorf("%w: %q no es una etapa con compuerta (RD 13.5)", ErrRepartoInvalido, p.Etapa)
	}
	if strings.TrimSpace(motivo) == "" {
		return p, fmt.Errorf("%w: un rechazo exige motivo, para que la cifra siga siendo explicable", ErrRepartoInvalido)
	}
	secuencia := secuenciaEtapas(p.Circuito)
	idx := -1
	for i, e := range secuencia {
		if e == p.Etapa {
			idx = i
			break
		}
	}
	// idx > 0 siempre: una compuerta nunca es la primera etapa de su circuito.
	p.Etapa = secuencia[idx-1]
	p.Revision++
	p.RechazoMotivo = motivo
	p.Firmas = nil
	return p, nil
}

// AbrirProceso inicia una corrida en EtapaRecaudo, revision 1. El snapshot
// llega ya resuelto (ADR 0004/0005): abrir no lo resuelve, lo recibe.
func AbrirProceso(id, periodo string, circuito Circuito, bolsaID, snapshotID, reglamento string) (ProcesoDeReparto, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(periodo) == "" || strings.TrimSpace(bolsaID) == "" {
		return ProcesoDeReparto{}, fmt.Errorf("%w: id, periodo y bolsaID no pueden quedar vacios", ErrRepartoInvalido)
	}
	if circuito != Nacional && circuito != Internacional {
		return ProcesoDeReparto{}, fmt.Errorf("%w: circuito desconocido %q", ErrRepartoInvalido, circuito)
	}
	return ProcesoDeReparto{
		ID:         id,
		Periodo:    periodo,
		Circuito:   circuito,
		BolsaID:    bolsaID,
		SnapshotID: snapshotID,
		Reglamento: reglamento,
		Etapa:      EtapaRecaudo,
		Revision:   1,
	}, nil
}
