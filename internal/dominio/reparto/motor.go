package reparto

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Opciones controla variantes del pipeline que el proceso decide fuera.
type Opciones struct {
	// SinDeducciones salta R-06/R-07 (Fees in Error, R-16).
	SinDeducciones bool
	// SnapshotID se copia al resultado para trazabilidad.
	SnapshotID string
	// OmitirExclusionAntesDeSplit es solo para pruebas: cuando es true, R-27
	// se aplica despues del split por grupos. El reglamento (y el cero-valor
	// de esta struct) excluye ANTES del split.
	OmitirExclusionAntesDeSplit bool
}

// Reparto valoriza los usos de un periodo y reparte la bolsa neta.
//
// Funcion pura: sin E/S, sin reloj, sin aleatoriedad (ADR 0005). Los
// recorridos van sobre claves ordenadas. El residuo de redondeo es un campo
// explicito de [Resultado], no se absorbe en la ultima linea.
//
// Todos los usos de una llamada deben compartir la misma modalidad.
func Reparto(bolsa Bolsa, usos []Uso, snap Snapshot, decls []repertorio.Declaracion, opt Opciones) (Resultado, error) {
	if len(usos) == 0 {
		return Resultado{}, fmt.Errorf("%w: no hay usos", ErrRepartoInvalido)
	}
	mod := usos[0].Modalidad
	if _, err := ParseModalidad(string(mod)); err != nil {
		return Resultado{}, err
	}
	for _, u := range usos[1:] {
		if u.Modalidad != mod {
			return Resultado{}, fmt.Errorf("%w: modalidades mezcladas %q y %q", ErrRepartoInvalido, mod, u.Modalidad)
		}
		if _, err := ParseModalidad(string(u.Modalidad)); err != nil {
			return Resultado{}, err
		}
	}

	ded, err := aplicarDeducciones(bolsa, snap, opt.SinDeducciones)
	if err != nil {
		return Resultado{}, err
	}

	var (
		puntos     map[string]decimal.Decimal
		valorPunto decimal.Decimal
		obras      []LineaObra
		residuo    decimal.Decimal
	)

	switch mod {
	case TV:
		puntos, err = puntosTV(usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo = asignarPorPuntos(ded.Neto, puntos)

	case Cine, Teatro:
		puntos, err = puntosCineTeatro(usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo = asignarPorPuntos(ded.Neto, puntos)

	case Transporte:
		puntos = puntosTransporte(usos)
		obras, valorPunto, residuo = asignarPorPuntos(ded.Neto, puntos)

	case OTT:
		puntos, err = puntosOTT(usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo = asignarPorPuntos(ded.Neto, puntos)

	case Suscripcion, Hotel:
		obras, valorPunto, residuo, err = repartirSuscripcion(ded.Neto, usos, snap, !opt.OmitirExclusionAntesDeSplit)
		if err != nil {
			return Resultado{}, err
		}

	default:
		return Resultado{}, fmt.Errorf("%w: %q", ErrModalidadDesconocida, mod)
	}

	titulares, retenido, obras := aplicarDeclaraciones(obras, indiceDeclaraciones(decls))

	// Residuo de titulares: lo repartido a obras no retenidas menos lo que
	// quedo en lineas de titular tras el redondeo por porcentaje.
	repartido := sumaObrasNoRetenidas(obras)
	pagado := sumaTitulares(titulares)
	residuoTit := repartido.Sub(pagado)
	residuo = residuo.Add(residuoTit)

	res := Resultado{
		Neto:       ded.Neto,
		Admin:      ded.Admin,
		Social:     ded.Social,
		Reserva:    ded.Reserva,
		Retenido:   retenido,
		Residuo:    residuo,
		ValorPunto: valorPunto,
		Obras:      obras,
		Titulares:  titulares,
		SnapshotID: opt.SnapshotID,
		Reglamento: snap.Reglamento,
	}
	return res, nil
}

// asignarPorPuntos convierte un mapa de puntos en lineas de obra.
func asignarPorPuntos(neto decimal.Decimal, puntos map[string]decimal.Decimal) ([]LineaObra, decimal.Decimal, decimal.Decimal) {
	ids := obrasOrdenadas(puntos)
	importes, residuo := repartirProporcional(neto, ids, puntos)
	totalPuntos := decimal.Zero
	for _, id := range ids {
		totalPuntos = totalPuntos.Add(puntos[id])
	}
	valorPunto := decimal.Zero
	if totalPuntos.GreaterThan(decimal.Zero) {
		valorPunto = neto.Div(totalPuntos).Round(precisionPuntos)
	}
	obras := make([]LineaObra, 0, len(ids))
	for _, id := range ids {
		obras = append(obras, LineaObra{
			ObraID:  id,
			Puntos:  puntos[id].Round(precisionPuntos),
			Importe: importes[id],
		})
	}
	return obras, valorPunto, residuo
}

// repartirSuscripcion: R-27 (opcionalmente antes), split por grupos, 9.1.1
// dentro de cada grupo con valor punto propio.
func repartirSuscripcion(neto decimal.Decimal, usos []Uso, snap Snapshot, excluirAntes bool) ([]LineaObra, decimal.Decimal, decimal.Decimal, error) {
	working := usos
	if excluirAntes {
		working = filtrarRepertorio(usos)
	}

	// Porcentajes de los cinco grupos desde el snapshot (nunca literales).
	pcts := make(map[GrupoCanal]decimal.Decimal, 5)
	sumaPct := decimal.Zero
	for _, g := range GruposCanal() {
		p, err := pctGrupo(g, snap)
		if err != nil {
			return nil, decimal.Zero, decimal.Zero, err
		}
		pcts[g] = p
		sumaPct = sumaPct.Add(p)
	}
	if !sumaPct.Equal(decimal.NewFromInt(100)) {
		return nil, decimal.Zero, decimal.Zero, fmt.Errorf("%w: los porcentajes de grupo suman %s, se esperaba 100",
			ErrRepartoInvalido, sumaPct.String())
	}

	bolsasGrupo := make(map[GrupoCanal]decimal.Decimal, 5)
	claves := make([]string, 0, 5)
	pesos := make(map[string]decimal.Decimal, 5)
	for _, g := range GruposCanal() {
		claves = append(claves, string(g))
		pesos[string(g)] = pcts[g]
	}
	partes, residuoSplit := repartirProporcional(neto, claves, pesos)
	for _, g := range GruposCanal() {
		bolsasGrupo[g] = partes[string(g)]
	}

	porGrupo := agruparPorGrupo(working)
	acum := make(map[string]LineaObra)
	residuo := residuoSplit
	fueraIDs := make(map[string]bool)
	if !excluirAntes {
		for _, u := range usos {
			if u.FueraDeRepertorio {
				fueraIDs[u.ObraID] = true
			}
		}
	}

	for _, g := range GruposCanal() {
		bolsaG := bolsasGrupo[g]
		list := porGrupo[g]
		if len(list) == 0 {
			residuo = residuo.Add(bolsaG)
			continue
		}
		puntos, err := puntosTV(list, snap)
		if err != nil {
			return nil, decimal.Zero, decimal.Zero, err
		}
		obrasG, _, resG := asignarPorPuntos(bolsaG, puntos)
		residuo = residuo.Add(resG)
		for _, o := range obrasG {
			if fueraIDs[o.ObraID] {
				// Exclusion a posteriori: la parte ya asignada no se paga.
				residuo = residuo.Add(o.Importe)
				continue
			}
			if prev, ok := acum[o.ObraID]; ok {
				prev.Puntos = prev.Puntos.Add(o.Puntos)
				prev.Importe = prev.Importe.Add(o.Importe)
				acum[o.ObraID] = prev
			} else {
				acum[o.ObraID] = o
			}
		}
	}

	ids := make([]string, 0, len(acum))
	for id := range acum {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	obras := make([]LineaObra, 0, len(ids))
	for _, id := range ids {
		obras = append(obras, acum[id])
	}
	return obras, decimal.Zero, residuo, nil
}

// AsignarPlataformaTerceros reparte una fraccion del bruto del usuario en las
// mismas proporciones que la distribucion padre (RD 9.7, parrafo final).
// No valoriza por puntos. El porcentaje sale del snapshot.
func AsignarPlataformaTerceros(parent []LineaObra, brutoUsuario decimal.Decimal, snap Snapshot) (Resultado, error) {
	if err := exigirPositivo("asignacion_terceros_pct", snap.AsignacionTercerosPct); err != nil {
		return Resultado{}, err
	}
	if len(parent) == 0 {
		return Resultado{}, fmt.Errorf("%w: sin lineas padre", ErrRepartoInvalido)
	}

	pool := pctDe(brutoUsuario, snap.AsignacionTercerosPct)
	ids := make([]string, 0, len(parent))
	pesos := make(map[string]decimal.Decimal, len(parent))
	puntos := make(map[string]decimal.Decimal, len(parent))
	for _, o := range parent {
		ids = append(ids, o.ObraID)
		pesos[o.ObraID] = o.Importe
		puntos[o.ObraID] = o.Puntos
	}
	sort.Strings(ids)
	importes, residuo := repartirProporcional(pool, ids, pesos)

	obras := make([]LineaObra, 0, len(ids))
	for _, id := range ids {
		obras = append(obras, LineaObra{
			ObraID:  id,
			Puntos:  puntos[id],
			Importe: importes[id],
		})
	}
	return Resultado{
		Neto:       pool,
		Residuo:    residuo,
		Obras:      obras,
		Reglamento: snap.Reglamento,
	}, nil
}
