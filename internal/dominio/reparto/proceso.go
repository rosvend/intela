package reparto

import "fmt"

type Circuito string

const (
	Nacional      Circuito = "nacional"
	Internacional Circuito = "internacional"
)

type Etapa string

const (
	EtapaRecaudo            Etapa = "recaudo"
	EtapaDeducciones        Etapa = "deducciones"
	EtapaImporteObra        Etapa = "importe_obra"
	EtapaImporteTitular     Etapa = "importe_titular"
	EtapaLiquidacionParcial Etapa = "liquidacion_parcial"
	EtapaVerificacion       Etapa = "verificacion"
	EtapaLiquidacionFinal   Etapa = "liquidacion_final"
	EtapaPagoRegistro       Etapa = "pago_registro"
	EtapaFeesInError        Etapa = "fees_in_error"
	EtapaAuditoria          Etapa = "auditoria"
)

type Firma struct {
	Rol      string
	ActorID  string
	SobreRev int
}

type Proceso struct {
	ID            string
	Circuito      Circuito
	Etapa         Etapa
	Periodo       string
	Revision      int
	Firmas        []Firma
	RechazoMotivo string
}

func Siguiente(c Circuito, e Etapa) (Etapa, bool) {
	var seq []Etapa
	if c == Internacional {
		seq = []Etapa{EtapaRecaudo, EtapaDeducciones, EtapaLiquidacionParcial, EtapaVerificacion, EtapaLiquidacionFinal, EtapaPagoRegistro, EtapaFeesInError, EtapaAuditoria}
	} else {
		seq = []Etapa{EtapaRecaudo, EtapaDeducciones, EtapaImporteObra, EtapaImporteTitular, EtapaLiquidacionParcial, EtapaVerificacion, EtapaLiquidacionFinal, EtapaPagoRegistro, EtapaAuditoria}
	}
	for i, x := range seq {
		if x == e && i+1 < len(seq) {
			return seq[i+1], true
		}
	}
	return e, false
}

func Anterior(c Circuito, e Etapa) Etapa {
	var seq []Etapa
	if c == Internacional {
		seq = []Etapa{EtapaRecaudo, EtapaDeducciones, EtapaLiquidacionParcial, EtapaVerificacion, EtapaLiquidacionFinal, EtapaPagoRegistro, EtapaFeesInError, EtapaAuditoria}
	} else {
		seq = []Etapa{EtapaRecaudo, EtapaDeducciones, EtapaImporteObra, EtapaImporteTitular, EtapaLiquidacionParcial, EtapaVerificacion, EtapaLiquidacionFinal, EtapaPagoRegistro, EtapaAuditoria}
	}
	for i, x := range seq {
		if x == e && i > 0 {
			return seq[i-1]
		}
	}
	return e
}

func Compuerta(e Etapa) bool {
	return e == EtapaVerificacion || e == EtapaPagoRegistro
}

func RolesRequeridos(e Etapa) []string {
	switch e {
	case EtapaVerificacion:
		return []string{"distribucion", "contabilidad"}
	case EtapaPagoRegistro:
		return []string{"distribucion", "contabilidad"}
	default:
		return nil
	}
}

func tieneRol(firmas []Firma, rol string, rev int) bool {
	for _, f := range firmas {
		if f.Rol == rol && f.SobreRev == rev {
			return true
		}
	}
	return false
}

func Avanzar(p Proceso, actorRol string) (Proceso, error) {
	if Compuerta(p.Etapa) {
		for _, rol := range RolesRequeridos(p.Etapa) {
			if !tieneRol(p.Firmas, rol, p.Revision) {
				return p, fmt.Errorf("compuerta %s exige firma de %s", p.Etapa, rol)
			}
		}
	}
	next, ok := Siguiente(p.Circuito, p.Etapa)
	if !ok {
		return p, fmt.Errorf("proceso en etapa terminal %s", p.Etapa)
	}
	p.Etapa = next
	return p, nil
}

func Firmar(p Proceso, rol, actorID string) (Proceso, error) {
	if !Compuerta(p.Etapa) {
		return p, fmt.Errorf("la etapa %s no es compuerta", p.Etapa)
	}
	okRol := false
	for _, r := range RolesRequeridos(p.Etapa) {
		if r == rol {
			okRol = true
		}
	}
	if !okRol {
		return p, fmt.Errorf("rol %s no firma esta compuerta", rol)
	}
	p.Firmas = append(p.Firmas, Firma{Rol: rol, ActorID: actorID, SobreRev: p.Revision})
	return p, nil
}

func Rechazar(p Proceso, motivo string) Proceso {
	p.Etapa = Anterior(p.Circuito, p.Etapa)
	p.Revision++
	p.RechazoMotivo = motivo
	return p
}
