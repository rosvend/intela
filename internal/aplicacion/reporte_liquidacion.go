package aplicacion

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// ServicioLiquidacion consulta y exporta la liquidacion del titular que
// pregunta.
//
// El titularID NO viaja en la peticion: sale de la sesion. Un parametro
// titular_id dejaria que un titular pidiera la liquidacion de otro con solo
// conocerle el id (OE-6).
type ServicioLiquidacion struct {
	Repo       RepositorioReporteLiquidacion
	Exportador Exportador
}

// Consultar devuelve bruto, cada deduccion y neto por obra, filtrado por
// periodo si viene. Es la misma fuente que alimenta el export: el archivo
// no puede divergir del panel.
func (s ServicioLiquidacion) Consultar(ctx context.Context, actor Usuario, periodo string) (Liquidacion, error) {
	if err := exigirTitular(actor); err != nil {
		return Liquidacion{}, err
	}
	if err := validarPeriodo(periodo); err != nil {
		return Liquidacion{}, err
	}

	filas, err := s.Repo.FilasDeTitular(ctx, actor.TitularID, periodo)
	if err != nil {
		return Liquidacion{}, fmt.Errorf("liquidacion de titular: %w", err)
	}

	liq := Liquidacion{
		TitularID: actor.TitularID,
		Periodo:   periodo,
		Lineas:    make([]LineaLiquidacion, 0, len(filas)),
	}
	// Una sola corrida de ProrratearProceso por proceso: el vector de netos
	// es el mismo en cada fila del titular y recalcularlo seria O(n²).
	cache := map[string][]liquidacion.Linea{}
	for _, f := range filas {
		p, err := prorratearFila(f, cache)
		if err != nil {
			return Liquidacion{}, err
		}
		linea := LineaLiquidacion{
			Periodo: f.Periodo,
			ObraID:  f.ObraID,
			Titulo:  f.Titulo,
			Bruto:   p.Bruto,
			Admin:   p.Admin,
			Social:  p.Social,
			Reserva: p.Reserva,
			Neto:    p.Neto,
		}
		liq.Lineas = append(liq.Lineas, linea)
		liq.Totales.Bruto = liq.Totales.Bruto.Add(linea.Bruto)
		liq.Totales.Admin = liq.Totales.Admin.Add(linea.Admin)
		liq.Totales.Social = liq.Totales.Social.Add(linea.Social)
		liq.Totales.Reserva = liq.Totales.Reserva.Add(linea.Reserva)
		liq.Totales.Neto = liq.Totales.Neto.Add(linea.Neto)
	}
	return liq, nil
}

// prorratearFila usa [liquidacion.ProrratearProceso] cuando el repositorio
// trajo todos los netos del proceso (para que Σ deducciones cuadre con el
// proceso). Si no, [liquidacion.ProrratearLinea] con cubeta residual.
//
// Cuando Σ NetosProceso < ProcesoNeto (retenido, residuo o no distribuido),
// se agrega esa diferencia como partida residual —igual que [liquidacion.ProrratearLinea]—
// para que los titulares no carguen con deducciones ajenas. Si Σ supera el
// neto del proceso, la invariante del motor esta rota y se rechaza.
func prorratearFila(f FilaLiquidacion, cache map[string][]liquidacion.Linea) (liquidacion.Linea, error) {
	if len(f.NetosProceso) == 0 {
		return liquidacion.ProrratearLinea(f.Neto, f.ProcesoAdmin, f.ProcesoSocial, f.ProcesoReserva, f.ProcesoNeto), nil
	}
	if f.Indice < 0 || f.Indice >= len(f.NetosProceso) {
		return liquidacion.Linea{}, fmt.Errorf(
			"liquidacion: indice %d fuera de netos del proceso %s (%d netos)",
			f.Indice, f.ProcesoID, len(f.NetosProceso),
		)
	}

	lineas, ok := cache[f.ProcesoID]
	if !ok {
		pesos, err := pesosConCubetaResidual(f.NetosProceso, f.ProcesoNeto)
		if err != nil {
			return liquidacion.Linea{}, fmt.Errorf("liquidacion proceso %s: %w", f.ProcesoID, err)
		}
		lineas = liquidacion.ProrratearProceso(pesos, f.ProcesoAdmin, f.ProcesoSocial, f.ProcesoReserva)
		cache[f.ProcesoID] = lineas
	}
	return lineas[f.Indice], nil
}

// pesosConCubetaResidual copia los netos de titular y, si falta plato
// (retenido/residuo/no distribuido), agrega ProcesoNeto-Σ como ultima
// partida para que el mayor-resto no se lo coma los titulares.
func pesosConCubetaResidual(netos []decimal.Decimal, netoProc decimal.Decimal) ([]decimal.Decimal, error) {
	suma := decimal.Zero
	for _, n := range netos {
		suma = suma.Add(n)
	}
	if suma.GreaterThan(netoProc) {
		return nil, fmt.Errorf("suma de netos titulares %s > neto del proceso %s", suma, netoProc)
	}
	pesos := make([]decimal.Decimal, len(netos), len(netos)+1)
	copy(pesos, netos)
	if resto := netoProc.Sub(suma); resto.IsPositive() {
		pesos = append(pesos, resto)
	}
	return pesos, nil
}

// Exportar renderiza la misma liquidacion que Consultar. formato es pdf o
// xlsx; cualquier otro es ErrFormatoInvalido.
func (s ServicioLiquidacion) Exportar(ctx context.Context, actor Usuario, periodo, formato string) (Archivo, error) {
	liq, err := s.Consultar(ctx, actor, periodo)
	if err != nil {
		return Archivo{}, err
	}
	switch strings.ToLower(strings.TrimSpace(formato)) {
	case FormatoPDF:
		archivo, err := s.Exportador.PDF(liq)
		if err != nil {
			return Archivo{}, fmt.Errorf("generar pdf: %w", err)
		}
		return archivo, nil
	case FormatoXLSX:
		archivo, err := s.Exportador.Excel(liq)
		if err != nil {
			return Archivo{}, fmt.Errorf("generar excel: %w", err)
		}
		return archivo, nil
	default:
		return Archivo{}, ErrFormatoInvalido
	}
}

func exigirTitular(actor Usuario) error {
	if actor.Rol != RolTitular || actor.TitularID == "" {
		return ErrNoAutorizado
	}
	return nil
}

func validarPeriodo(periodo string) error {
	if periodo == "" {
		return nil
	}
	// El periodo lo juzga el dominio, no una copia de aqui: la regla -un ano,
	// o un ano y un mes que existe- es `recaudo.PeriodoValido`, la misma que
	// usan `ClaveTrabajo.Valida` y `prepararReporte`.
	if !recaudo.PeriodoValido(periodo) {
		return ErrPeriodoInvalido
	}
	return nil
}
