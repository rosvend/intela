package aplicacion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
)

// Hecho y referencia que escribe el modulo de recaudo al registrar una bolsa.
const (
	HechoRecaudoRegistrado = "recaudo.registrado"
	RefBolsa               = "bolsa"
)

// Explicacion es el linaje de una cifra, leido solo de la bitacora (ADR 0006).
type Explicacion struct {
	Ref            string
	TitularID      string
	Neto           decimal.Decimal
	Bruto          decimal.Decimal
	Retenida       bool
	Motivo         string
	Corrida        CorridaLinaje
	Bolsa          BolsaLinaje
	Reporte        ReporteAsentado
	Reportes       []ReporteAsentado
	Obra           ObraLinaje
	Identificacion []IdentificacionDeUso
	Regla          ReglaLinaje
	Split          *SplitLinaje
	Deducciones    []DeduccionLinaje
	Firmas         []FirmaLinaje
	// Faltantes nombra los eslabones accesorios sin asiento; la cadena del dinero nunca falta.
	Faltantes []string
}

// CorridaLinaje es la corrida que produjo la cifra.
type CorridaLinaje struct {
	ProcesoID string
	Periodo   string
	Circuito  string
}

// BolsaLinaje es la bolsa repartida y, si se asento, su recaudo de origen.
type BolsaLinaje struct {
	ID        string
	UsuarioID string
	Bruto     decimal.Decimal
	Recaudo   *RecaudoLinaje
}

// RecaudoLinaje es como se cobro la bolsa (RT).
type RecaudoLinaje struct {
	Convenio string `json:"convenio"`
	Tarifa   string `json:"tarifa"`
	Factura  string `json:"factura"`
}

// ObraLinaje resume la obra por su eslabon mas debil de identificacion.
type ObraLinaje struct {
	ID      string
	Titulo  string
	Escalon string
	Puntaje string
}

// ReglaLinaje es el snapshot normativo y el reglamento de la corrida.
type ReglaLinaje struct {
	SnapshotID string
	Reglamento string
}

// SplitLinaje es la parte declarada del titular, con la version de la declaracion usada.
type SplitLinaje struct {
	TitularID  string
	IPI        string
	Porcentaje decimal.Decimal
	Version    *int
}

// DeduccionLinaje es la parte de una deduccion legal que corresponde a la cifra.
type DeduccionLinaje struct {
	Concepto   string
	Porcentaje decimal.Decimal
	Monto      decimal.Decimal
}

// FirmaLinaje es una firma de compuerta de la corrida.
type FirmaLinaje struct {
	Rol           string
	ActorID       string
	SobreRevision int
	Etapa         string
	Cuando        time.Time
}

// ExplicarCifra recorre la bitacora; titular y auditor hacen la misma consulta con distinto alcance.
type ExplicarCifra struct {
	Bitacora BitacoraAuditoria
}

// FormarRef arma la ref de una linea de titular, o de la obra si titularID va vacio.
func FormarRef(procesoID, obraID, titularID string) string {
	if titularID == "" {
		return procesoID + ":" + obraID
	}
	return procesoID + ":" + obraID + ":" + titularID
}

// ParsearRef acepta proceso:obra o proceso:obra:titular; cualquier otra forma no existe.
func ParsearRef(ref string) (procesoID, obraID, titularID string, err error) {
	partes := strings.Split(ref, ":")
	if len(partes) < 2 || len(partes) > 3 {
		return "", "", "", fmt.Errorf("ref %q: %w", ref, ErrNoEncontrado)
	}
	for _, p := range partes {
		if strings.TrimSpace(p) == "" {
			return "", "", "", fmt.Errorf("ref %q: %w", ref, ErrNoEncontrado)
		}
	}
	if len(partes) == 3 {
		titularID = partes[2]
	}
	return partes[0], partes[1], titularID, nil
}

// Explicar carga primero y autoriza despues: una cifra ajena que existe es 403, no 404.
func (e ExplicarCifra) Explicar(ctx context.Context, actor Usuario, ref string) (Explicacion, error) {
	procesoID, obraID, titularID, err := ParsearRef(ref)
	if err != nil {
		return Explicacion{}, err
	}

	delProceso, err := e.Bitacora.De(ctx, RefProceso, procesoID)
	if err != nil {
		return Explicacion{}, fmt.Errorf("explicar %q: %w", ref, err)
	}
	var corrida AsientoValorizacion
	if !ultimo(delProceso, HechoRepartoValorizado, &corrida, func() bool { return true }) {
		return Explicacion{}, fmt.Errorf("explicar %q: corrida sin valorizacion asentada: %w", ref, ErrNoEncontrado)
	}

	deLaObra, err := e.Bitacora.De(ctx, RefObra, obraID)
	if err != nil {
		return Explicacion{}, fmt.Errorf("explicar %q: %w", ref, err)
	}
	var obra AsientoObraValorizada
	if !ultimo(deLaObra, HechoRepartoObraValorizada, &obra, func() bool { return obra.ProcesoID == procesoID }) {
		return Explicacion{}, fmt.Errorf("explicar %q: obra sin valorizacion en la corrida: %w", ref, ErrNoEncontrado)
	}

	x := Explicacion{
		Ref:            ref,
		Retenida:       obra.Retenida,
		Motivo:         obra.Motivo,
		Corrida:        CorridaLinaje{ProcesoID: corrida.ProcesoID, Periodo: corrida.Periodo, Circuito: corrida.Circuito},
		Regla:          ReglaLinaje{SnapshotID: corrida.SnapshotID, Reglamento: corrida.Reglamento},
		Identificacion: obra.Usos,
		Faltantes:      []string{},
	}
	cifra, err := decimal.NewFromString(obra.Importe)
	if err != nil {
		return Explicacion{}, fmt.Errorf("explicar %q: importe de obra: %w", ref, ErrLinajeIncompleto)
	}

	titulares := make([]string, 0, len(obra.Titulares))
	for _, t := range obra.Titulares {
		titulares = append(titulares, t.TitularID)
	}
	if titularID != "" {
		linea, ok := lineaDe(obra.Titulares, titularID)
		if !ok {
			return Explicacion{}, fmt.Errorf("explicar %q: titular sin linea en la obra: %w", ref, ErrNoEncontrado)
		}
		titulares = []string{titularID}
		x.TitularID = titularID
		if x.Split, cifra, err = splitDe(linea, obra.Declaracion); err != nil {
			return Explicacion{}, fmt.Errorf("explicar %q: %w", ref, err)
		}
	}
	if !SoloPropiasObras(actor, titulares) {
		return Explicacion{}, ErrNoAutorizado
	}

	if x.Bruto, x.Deducciones, err = brutoYDeducciones(cifra, corrida, obraID, titularID); err != nil {
		return Explicacion{}, fmt.Errorf("explicar %q: %w", ref, err)
	}
	x.Neto = cifra
	x.Obra, x.Faltantes = obraLinaje(obraID, obra.Usos, deLaObra, x.Faltantes)
	x.Reportes = reportesDeLaObra(obra.Usos, corrida.Reportes)
	x.Reporte = reportePrincipal(obra.Usos, x.Reportes)
	x.Firmas = firmasDe(delProceso)
	x.Bolsa, x.Faltantes, err = e.bolsaDe(ctx, corrida.Bolsa, x.Faltantes)
	if err != nil {
		return Explicacion{}, fmt.Errorf("explicar %q: %w", ref, err)
	}
	return x, nil
}

// ultimo decodifica en destino el asiento mas reciente del hecho que cumple acepta.
func ultimo(asientos []Asiento, hecho string, destino any, acepta func() bool) bool {
	for i := len(asientos) - 1; i >= 0; i-- {
		if asientos[i].Hecho != hecho {
			continue
		}
		if err := json.Unmarshal(asientos[i].Payload, destino); err == nil && acepta() {
			return true
		}
	}
	return false
}

func lineaDe(titulares []TitularAsentado, titularID string) (TitularAsentado, bool) {
	for _, t := range titulares {
		if t.TitularID == titularID {
			return t, true
		}
	}
	return TitularAsentado{}, false
}

func splitDe(t TitularAsentado, decl *DeclaracionAsentada) (*SplitLinaje, decimal.Decimal, error) {
	porcentaje, err := decimal.NewFromString(t.Porcentaje)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("porcentaje de %q: %w", t.TitularID, ErrLinajeIncompleto)
	}
	importe, err := decimal.NewFromString(t.Importe)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("importe de %q: %w", t.TitularID, ErrLinajeIncompleto)
	}
	s := &SplitLinaje{TitularID: t.TitularID, IPI: t.IPI, Porcentaje: porcentaje}
	// La version de la linea es la que esa corrida guardo. El asiento de la
	// obra solo cubre corridas anteriores a #183, que no la tenian por linea.
	switch {
	case t.DeclaracionVersion != nil:
		s.Version = copiarVersion(t.DeclaracionVersion)
	case decl != nil:
		// Asientos anteriores a #183: la version vivia solo en la obra.
		v := decl.Version
		s.Version = &v
	}
	return s, importe, nil
}

// brutoYDeducciones prorratea las deducciones de la corrida sobre la cifra.
//
// Con el vector de netos del asiento usa el mismo mayor-resto que el export
// ([prorratearFila]). Sin vector —asientos anteriores a ese campo— cae en
// [liquidacion.ProrratearLinea]. Una cifra en cero o un neto de proceso que
// no es positivo no inventa porcentajes de deduccion.
func brutoYDeducciones(cifra decimal.Decimal, c AsientoValorizacion, obraID, titularID string) (decimal.Decimal, []DeduccionLinaje, error) {
	neto, err := decimal.NewFromString(c.Neto)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("neto de la corrida: %w", ErrLinajeIncompleto)
	}
	if !neto.IsPositive() || cifra.IsZero() {
		return cifra, []DeduccionLinaje{}, nil
	}
	montos := make(map[string]decimal.Decimal, len(c.Deducciones))
	tasas := make(map[string]decimal.Decimal, len(c.Deducciones))
	for _, d := range c.Deducciones {
		m, errM := decimal.NewFromString(d.Monto)
		p, errP := decimal.NewFromString(d.Porcentaje)
		if errM != nil || errP != nil {
			return decimal.Zero, nil, fmt.Errorf("deduccion %q: %w", d.Concepto, ErrLinajeIncompleto)
		}
		montos[d.Concepto], tasas[d.Concepto] = m, p
	}
	admin := montos[liquidacion.ConceptoAdministracion]
	social := montos[liquidacion.ConceptoSocial]
	reserva := montos[liquidacion.ConceptoReserva]

	var linea liquidacion.Linea
	if titularID != "" && len(c.Netos) > 0 {
		netos := make([]decimal.Decimal, len(c.Netos))
		indice := -1
		for i, n := range c.Netos {
			v, err := decimal.NewFromString(n.Importe)
			if err != nil {
				return decimal.Zero, nil, fmt.Errorf("neto de %s/%s: %w", n.ObraID, n.TitularID, ErrLinajeIncompleto)
			}
			netos[i] = v
			if n.ObraID == obraID && n.TitularID == titularID {
				indice = i
			}
		}
		if indice < 0 {
			return decimal.Zero, nil, fmt.Errorf("la cifra no esta en el vector de netos: %w", ErrLinajeIncompleto)
		}
		linea, err = prorratearFila(FilaLiquidacion{
			ObraID: obraID, Neto: cifra, ProcesoID: c.ProcesoID,
			ProcesoAdmin: admin, ProcesoSocial: social, ProcesoReserva: reserva, ProcesoNeto: neto,
			NetosProceso: netos, Indice: indice,
		}, map[string][]liquidacion.Linea{})
		if err != nil {
			return decimal.Zero, nil, err
		}
	} else {
		linea = liquidacion.ProrratearLinea(cifra, admin, social, reserva, neto)
	}
	if linea.Bruto.IsZero() && linea.Admin.IsZero() && linea.Social.IsZero() && linea.Reserva.IsZero() {
		return cifra, []DeduccionLinaje{}, nil
	}
	return linea.Bruto, []DeduccionLinaje{
		{Concepto: liquidacion.ConceptoAdministracion, Porcentaje: tasas[liquidacion.ConceptoAdministracion], Monto: linea.Admin},
		{Concepto: liquidacion.ConceptoSocial, Porcentaje: tasas[liquidacion.ConceptoSocial], Monto: linea.Social},
		{Concepto: liquidacion.ConceptoReserva, Porcentaje: tasas[liquidacion.ConceptoReserva], Monto: linea.Reserva},
	}, nil
}

// rangoDeCerteza ordena del eslabon mas debil al mas exacto (ADR 0007).
var rangoDeCerteza = map[string]int{
	identificacion.EscalonManual:   0,
	identificacion.EscalonDifuso:   1,
	identificacion.EscalonIDGlobal: 2,
	identificacion.EscalonAlias:    3,
}

// usoMasDebil es el uso de identificacion menos cierta; el resumen de la obra no presume mas certeza.
func usoMasDebil(usos []IdentificacionDeUso) (IdentificacionDeUso, bool) {
	if len(usos) == 0 {
		return IdentificacionDeUso{}, false
	}
	debil := usos[0]
	for _, u := range usos[1:] {
		ru, rd := rangoDeCerteza[u.Escalon], rangoDeCerteza[debil.Escalon]
		if ru < rd || (ru == rd && menorPuntaje(u.Puntaje, debil.Puntaje)) {
			debil = u
		}
	}
	return debil, true
}

// menorPuntaje compara como decimal; un puntaje ausente nunca es menor.
func menorPuntaje(a, b string) bool {
	pa, errA := decimal.NewFromString(a)
	if errA != nil {
		return false
	}
	pb, errB := decimal.NewFromString(b)
	return errB != nil || pa.LessThan(pb)
}

// obraLinaje resume la obra. El titulo sale de la ultima correccion o del alta; faltantes nombra el alta si no esta asentada.
func obraLinaje(obraID string, usos []IdentificacionDeUso, deLaObra []Asiento, faltantes []string) (ObraLinaje, []string) {
	o := ObraLinaje{ID: obraID}
	if u, ok := usoMasDebil(usos); ok {
		o.Escalon, o.Puntaje = u.Escalon, u.Puntaje
	}
	var alta, correccion AsientoObra
	hayAlta := ultimo(deLaObra, HechoObraRegistrada, &alta, func() bool { return true })
	switch {
	case ultimo(deLaObra, HechoObraCorregida, &correccion, func() bool { return true }):
		o.Titulo = correccion.Despues.Titulo
	case hayAlta:
		o.Titulo = alta.Despues.Titulo
	}
	if !hayAlta {
		faltantes = append(faltantes, HechoObraRegistrada)
	}
	return o, faltantes
}

func reportesDeLaObra(usos []IdentificacionDeUso, deLaCorrida []ReporteAsentado) []ReporteAsentado {
	usados := make(map[string]bool, len(usos))
	for _, u := range usos {
		usados[u.ReporteID] = true
	}
	out := make([]ReporteAsentado, 0, len(usados))
	for _, r := range deLaCorrida {
		if usados[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

func reportePrincipal(usos []IdentificacionDeUso, reportes []ReporteAsentado) ReporteAsentado {
	if u, ok := usoMasDebil(usos); ok {
		for _, r := range reportes {
			if r.ID == u.ReporteID {
				return r
			}
		}
	}
	if len(reportes) > 0 {
		return reportes[0]
	}
	return ReporteAsentado{}
}

func firmasDe(delProceso []Asiento) []FirmaLinaje {
	firmas := make([]FirmaLinaje, 0, 4)
	for _, a := range delProceso {
		if a.Hecho != HechoFirmaRegistrada {
			continue
		}
		var p AsientoProceso
		if err := json.Unmarshal(a.Payload, &p); err != nil || p.Firma == nil {
			continue
		}
		firmas = append(firmas, FirmaLinaje{
			Rol: p.Firma.Rol, ActorID: p.Firma.ActorID, SobreRevision: p.Firma.SobreRevision,
			Etapa: p.Etapa, Cuando: a.Cuando,
		})
	}
	return firmas
}

func (e ExplicarCifra) bolsaDe(ctx context.Context, b BolsaAsentada, faltantes []string) (BolsaLinaje, []string, error) {
	bruto, err := decimal.NewFromString(b.Bruto)
	if err != nil {
		return BolsaLinaje{}, nil, fmt.Errorf("bruto de la bolsa: %w", ErrLinajeIncompleto)
	}
	out := BolsaLinaje{ID: b.ID, UsuarioID: b.UsuarioID, Bruto: bruto}
	asientos, err := e.Bitacora.De(ctx, RefBolsa, b.ID)
	if err != nil {
		return BolsaLinaje{}, nil, err
	}
	var r RecaudoLinaje
	if ultimo(asientos, HechoRecaudoRegistrado, &r, func() bool { return true }) {
		out.Recaudo = &r
		return out, faltantes, nil
	}
	return out, append(faltantes, HechoRecaudoRegistrado), nil
}
