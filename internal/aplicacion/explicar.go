package aplicacion

import (
	"context"

	"github.com/shopspring/decimal"
)

// Explicacion es el linaje de una cifra. Lo produce ExplicarCifra y lo
// consume el panel del titular (y el de auditoria) sin recomputar.
//
// Contesta las preguntas del ADR 0006 que el titular necesita para
// entender un monto: corrida, reporte de origen, obra + escalon + puntaje,
// regla + snapshot, split, y el paso de bruto a neto. El bruto SOLO vive
// aqui; el panel no lo enseña como cifra de cabecera (OE-6).
type Explicacion struct {
	Ref         string
	TitularID   string
	Neto        decimal.Decimal
	Bruto       decimal.Decimal
	Corrida     CorridaLinaje
	Reporte     ReporteLinaje
	Obra        ObraLinaje
	Regla       ReglaLinaje
	Split       SplitLinaje
	Deducciones []Deduccion
}

// CorridaLinaje es la corrida que produjo la cifra.
type CorridaLinaje struct {
	ProcesoID string
	Periodo   string
	Circuito  string
}

// ReporteLinaje es el archivo crudo que pondero la bolsa, no "el de Caracol".
type ReporteLinaje struct {
	ID     string
	Fuente string
	SHA256 string
}

// ObraLinaje es como se reconocio la obra: escalon de la cascada y puntaje.
type ObraLinaje struct {
	ID      string
	Titulo  string
	Escalon string
	Puntaje decimal.Decimal
}

// ReglaLinaje es el snapshot normativo congelado al abrir el proceso.
type ReglaLinaje struct {
	SnapshotID string
	Reglamento string
}

// SplitLinaje es la declaracion de obra con la que se partio el importe.
type SplitLinaje struct {
	TitularID  string
	IPI        string
	Porcentaje decimal.Decimal
	Version    int
}

// Deduccion es un recorte legal aplicado sobre el bruto del titular.
type Deduccion struct {
	Concepto   string
	Porcentaje decimal.Decimal
	Monto      decimal.Decimal
}

// ExplicarCifra recorre el linaje persistido de una linea de titular.
//
// Que un titular vea el origen de su dinero y que un auditor lo verifique
// son la misma consulta con distinto alcance (ADR 0006). El alcance lo
// decide SoloPropiasObras: el titular solo ve cifras suyas; el personal
// privilegiado ve cualquiera.
type ExplicarCifra struct {
	Repo RepositorioExplicacion
}

// Explicar devuelve el linaje de ref. Carga primero y autoriza despues:
// una cifra ajena existente es 403, no 404. El issue lo pide asi, y es
// lo que el panel enseña cuando alguien fuerza una ref que no es suya.
func (e ExplicarCifra) Explicar(ctx context.Context, actor Usuario, ref string) (Explicacion, error) {
	procesoID, obraID, titularID, err := ParsearRef(ref)
	if err != nil {
		return Explicacion{}, err
	}

	x, err := e.Repo.PorLinea(ctx, procesoID, obraID, titularID)
	if err != nil {
		return Explicacion{}, err
	}

	if !SoloPropiasObras(actor, []string{x.TitularID}) {
		return Explicacion{}, ErrNoAutorizado
	}
	return x, nil
}
