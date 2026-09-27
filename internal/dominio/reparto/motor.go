package reparto

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Opciones controla variantes del pipeline que el proceso decide fuera.
type Opciones struct {
	// SinDeducciones salta R-06/R-07 (Fees in Error, R-16).
	SinDeducciones bool
	// SnapshotID se copia al resultado para trazabilidad.
	SnapshotID string
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
	if err := validarGrupos(usos, mod); err != nil {
		return Resultado{}, err
	}

	ded, err := aplicarDeducciones(bolsa, snap, opt.SinDeducciones)
	if err != nil {
		return Resultado{}, err
	}

	var (
		puntos        map[string]decimal.Decimal
		valorPunto    decimal.Decimal
		obras         []LineaObra
		residuo       decimal.Decimal
		noDistribuido decimal.Decimal
		partesNoDist  []ParteNoDistribuida
		porGrupo      []LineaGrupo
	)

	switch mod {
	case TV:
		puntos, err = puntosTV(usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo, noDistribuido, partesNoDist = asignarPorPuntos(ded.Neto, puntos)

	case Cine, Teatro:
		puntos, err = puntosCineTeatro(usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo, noDistribuido, partesNoDist = asignarPorPuntos(ded.Neto, puntos)

	case Transporte:
		puntos, err = puntosTransporte(usos)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo, noDistribuido, partesNoDist = asignarPorPuntos(ded.Neto, puntos)

	case OTT:
		puntos, err = puntosOTT(usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		obras, valorPunto, residuo, noDistribuido, partesNoDist = asignarPorPuntos(ded.Neto, puntos)

	case Suscripcion, Hotel:
		obras, porGrupo, residuo, noDistribuido, partesNoDist, err = repartirSuscripcion(ded.Neto, usos, snap)
		if err != nil {
			return Resultado{}, err
		}
		valorPunto = decimal.Zero

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
		Neto:                 ded.Neto,
		Admin:                ded.Admin,
		Social:               ded.Social,
		Reserva:              ded.Reserva,
		Retenido:             retenido,
		Residuo:              residuo,
		NoDistribuido:        noDistribuido,
		PartesNoDistribuidas: partesNoDist,
		ValorPunto:           valorPunto,
		PorGrupo:             porGrupo,
		Obras:                obras,
		Titulares:            titulares,
		SnapshotID:           opt.SnapshotID,
		Reglamento:           snap.Reglamento,
	}
	return res, nil
}

// validarGrupos exige Grupo tipado en suscripcion/hotel y lo rechaza en el resto.
func validarGrupos(usos []Uso, mod Modalidad) error {
	suscripcion := mod == Suscripcion || mod == Hotel
	for _, u := range usos {
		if suscripcion {
			if _, err := ParseGrupoCanal(string(u.Grupo)); err != nil {
				return fmt.Errorf("%w: obra %q: %v", ErrRepartoInvalido, u.ObraID, err)
			}
			continue
		}
		if u.Grupo != "" {
			return fmt.Errorf("%w: obra %q: grupo %q no aplica a modalidad %q",
				ErrRepartoInvalido, u.ObraID, u.Grupo, mod)
		}
	}
	return nil
}

// asignarPorPuntos convierte un mapa de puntos en lineas de obra.
// Si el peso total es cero, el neto entero va a NoDistribuido (no a Residuo).
func asignarPorPuntos(neto decimal.Decimal, puntos map[string]decimal.Decimal) (
	[]LineaObra, decimal.Decimal, decimal.Decimal, decimal.Decimal, []ParteNoDistribuida,
) {
	ids := obrasOrdenadas(puntos)
	totalPuntos := decimal.Zero
	for _, id := range ids {
		totalPuntos = totalPuntos.Add(puntos[id])
	}
	if totalPuntos.IsZero() || neto.IsZero() {
		obras := make([]LineaObra, 0, len(ids))
		for _, id := range ids {
			obras = append(obras, LineaObra{ObraID: id, Puntos: puntos[id].Round(precisionPuntos)})
		}
		if neto.IsPositive() {
			parte := ParteNoDistribuida{Motivo: MotivoPesoCero, Importe: neto}
			return obras, decimal.Zero, decimal.Zero, neto, []ParteNoDistribuida{parte}
		}
		return obras, decimal.Zero, decimal.Zero, decimal.Zero, nil
	}

	importes, residuo := repartirProporcional(neto, ids, puntos)
	valorPunto := neto.Div(totalPuntos).Round(precisionPuntos)
	obras := make([]LineaObra, 0, len(ids))
	for _, id := range ids {
		obras = append(obras, LineaObra{
			ObraID:  id,
			Puntos:  puntos[id].Round(precisionPuntos),
			Importe: importes[id],
		})
	}
	return obras, valorPunto, residuo, decimal.Zero, nil
}

// repartirSuscripcion: R-27 antes del split, split por grupos, 9.1.1 dentro
// de cada grupo con valor punto propio. excluirAntes queda como parametro
// interno para export_test; produccion siempre pasa true.
func repartirSuscripcion(neto decimal.Decimal, usos []Uso, snap Snapshot) (
	[]LineaObra, []LineaGrupo, decimal.Decimal, decimal.Decimal, []ParteNoDistribuida, error,
) {
	return repartirSuscripcionOrden(neto, usos, snap, true)
}

func repartirSuscripcionOrden(neto decimal.Decimal, usos []Uso, snap Snapshot, excluirAntes bool) (
	[]LineaObra, []LineaGrupo, decimal.Decimal, decimal.Decimal, []ParteNoDistribuida, error,
) {
	working := usos
	if excluirAntes {
		working = filtrarRepertorio(usos)
	}

	pcts := make(map[GrupoCanal]decimal.Decimal, 5)
	sumaPct := decimal.Zero
	for _, g := range GruposCanal() {
		p, err := pctGrupo(g, snap)
		if err != nil {
			return nil, nil, decimal.Zero, decimal.Zero, nil, err
		}
		pcts[g] = p
		sumaPct = sumaPct.Add(p)
	}
	if !sumaPct.Equal(decimal.NewFromInt(100)) {
		return nil, nil, decimal.Zero, decimal.Zero, nil, fmt.Errorf("%w: los porcentajes de grupo suman %s, se esperaba 100",
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

	porGrupoUsos := agruparPorGrupo(working)
	acum := make(map[string]LineaObra)
	residuo := residuoSplit
	noDist := decimal.Zero
	var partesNoDist []ParteNoDistribuida
	var lineasGrupo []LineaGrupo
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
		list := porGrupoUsos[g]
		if len(list) == 0 {
			if bolsaG.IsPositive() {
				noDist = noDist.Add(bolsaG)
				partesNoDist = append(partesNoDist, ParteNoDistribuida{
					Motivo: MotivoGrupoSinObras, Grupo: g, Importe: bolsaG,
				})
			}
			lineasGrupo = append(lineasGrupo, LineaGrupo{Grupo: g, Bolsa: bolsaG})
			continue
		}
		puntos, err := puntosTV(list, snap)
		if err != nil {
			return nil, nil, decimal.Zero, decimal.Zero, nil, err
		}
		obrasG, valorPuntoG, resG, noDistG, partesG := asignarPorPuntos(bolsaG, puntos)
		residuo = residuo.Add(resG)
		noDist = noDist.Add(noDistG)
		partesNoDist = append(partesNoDist, partesG...)

		totalPuntos := decimal.Zero
		for _, o := range obrasG {
			totalPuntos = totalPuntos.Add(o.Puntos)
		}
		lineasGrupo = append(lineasGrupo, LineaGrupo{
			Grupo:       g,
			Bolsa:       bolsaG,
			TotalPuntos: totalPuntos,
			ValorPunto:  valorPuntoG,
			Residuo:     resG,
		})

		for _, o := range obrasG {
			if fueraIDs[o.ObraID] {
				noDist = noDist.Add(o.Importe)
				partesNoDist = append(partesNoDist, ParteNoDistribuida{
					Motivo: MotivoExclusionR27, Grupo: g, ObraID: o.ObraID, Importe: o.Importe,
				})
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
	return obras, lineasGrupo, residuo, noDist, partesNoDist, nil
}

// AsignarPlataformaTerceros reparte una fraccion del bruto del usuario entre
// las obras efectivamente comunicadas en la plataforma, en las mismas
// proporciones que la distribucion padre (RD 9.7, parrafo final).
//
// Aplica deducciones legales y declaraciones (R-04, RD 11 / RD 3) sobre la
// bolsa derivada. obrasComunicadas es el censo de la plataforma: el padre
// solo presta proporciones, no su listado completo.
//
// La condicion "cuando dicha plataforma no corresponda a la principal
// ventana de difusion" no esta modelada aqui (queda abierta al cliente).
func AsignarPlataformaTerceros(
	parent []LineaObra,
	obrasComunicadas []string,
	brutoUsuario decimal.Decimal,
	circuito Circuito,
	snap Snapshot,
	decls []repertorio.Declaracion,
	opt Opciones,
) (Resultado, error) {
	if err := exigirPositivo("asignacion_terceros_pct", snap.AsignacionTercerosPct); err != nil {
		return Resultado{}, err
	}
	if len(parent) == 0 {
		return Resultado{}, fmt.Errorf("%w: sin lineas padre", ErrRepartoInvalido)
	}
	if len(obrasComunicadas) == 0 {
		return Resultado{}, fmt.Errorf("%w: sin obras comunicadas en la plataforma", ErrRepartoInvalido)
	}

	permitidas := make(map[string]bool, len(obrasComunicadas))
	for _, id := range obrasComunicadas {
		if id == "" {
			return Resultado{}, fmt.Errorf("%w: obra comunicada vacia", ErrRepartoInvalido)
		}
		permitidas[id] = true
	}

	pool := pctDe(brutoUsuario, snap.AsignacionTercerosPct)
	bolsaPool, err := recaudo.NuevaBolsa("_plataforma_terceros", "1970", circuito, pool)
	if err != nil {
		return Resultado{}, fmt.Errorf("%w: bolsa derivada: %v", ErrRepartoInvalido, err)
	}
	ded, err := aplicarDeducciones(bolsaPool, snap, opt.SinDeducciones)
	if err != nil {
		return Resultado{}, err
	}

	ids := make([]string, 0, len(parent))
	pesos := make(map[string]decimal.Decimal, len(parent))
	puntos := make(map[string]decimal.Decimal, len(parent))
	for _, o := range parent {
		if !permitidas[o.ObraID] {
			continue
		}
		ids = append(ids, o.ObraID)
		pesos[o.ObraID] = o.Importe
		puntos[o.ObraID] = o.Puntos
	}
	if len(ids) == 0 {
		return Resultado{}, fmt.Errorf("%w: ninguna linea padre esta en obras comunicadas", ErrRepartoInvalido)
	}
	sort.Strings(ids)

	obras, _, residuo, noDist, partesNoDist := asignarPorPuntosConPesos(ded.Neto, ids, pesos, puntos)
	titulares, retenido, obras := aplicarDeclaraciones(obras, indiceDeclaraciones(decls))
	repartido := sumaObrasNoRetenidas(obras)
	pagado := sumaTitulares(titulares)
	residuo = residuo.Add(repartido.Sub(pagado))

	return Resultado{
		Neto:                 ded.Neto,
		Admin:                ded.Admin,
		Social:               ded.Social,
		Reserva:              ded.Reserva,
		Retenido:             retenido,
		Residuo:              residuo,
		NoDistribuido:        noDist,
		PartesNoDistribuidas: partesNoDist,
		Obras:                obras,
		Titulares:            titulares,
		SnapshotID:           opt.SnapshotID,
		Reglamento:           snap.Reglamento,
	}, nil
}

// asignarPorPuntosConPesos reparte por pesos (importes padre) conservando
// los puntos informativos de cada obra.
func asignarPorPuntosConPesos(
	neto decimal.Decimal,
	ids []string,
	pesos, puntos map[string]decimal.Decimal,
) ([]LineaObra, decimal.Decimal, decimal.Decimal, decimal.Decimal, []ParteNoDistribuida) {
	total := decimal.Zero
	for _, id := range ids {
		if p := pesos[id]; p.GreaterThan(decimal.Zero) {
			total = total.Add(p)
		}
	}
	if total.IsZero() || neto.IsZero() {
		obras := make([]LineaObra, 0, len(ids))
		for _, id := range ids {
			obras = append(obras, LineaObra{ObraID: id, Puntos: puntos[id]})
		}
		if neto.IsPositive() {
			parte := ParteNoDistribuida{Motivo: MotivoPesoCero, Importe: neto}
			return obras, decimal.Zero, decimal.Zero, neto, []ParteNoDistribuida{parte}
		}
		return obras, decimal.Zero, decimal.Zero, decimal.Zero, nil
	}
	importes, residuo := repartirProporcional(neto, ids, pesos)
	obras := make([]LineaObra, 0, len(ids))
	for _, id := range ids {
		obras = append(obras, LineaObra{
			ObraID:  id,
			Puntos:  puntos[id],
			Importe: importes[id],
		})
	}
	return obras, decimal.Zero, residuo, decimal.Zero, nil
}
