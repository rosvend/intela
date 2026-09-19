package reparto

import "github.com/shopspring/decimal"

// RepartirSuscripcionDespuesExclusion expone el orden "R-27 despues del
// split" solo a tests del mismo paquete. Produccion siempre excluye antes
// (ver repartirSuscripcion).
func RepartirSuscripcionDespuesExclusion(
	neto decimal.Decimal, usos []Uso, snap Snapshot,
) ([]LineaObra, []LineaGrupo, decimal.Decimal, decimal.Decimal, []ParteNoDistribuida, error) {
	return repartirSuscripcionOrden(neto, usos, snap, false)
}
