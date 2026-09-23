package aplicacion

import (
	"context"
	"fmt"
	"strings"

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
	Repo       RepositorioLiquidacion
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

	filas, err := s.Repo.DeTitular(ctx, actor.TitularID, periodo)
	if err != nil {
		return Liquidacion{}, fmt.Errorf("liquidacion de titular: %w", err)
	}

	liq := Liquidacion{
		TitularID: actor.TitularID,
		Periodo:   periodo,
		Lineas:    make([]LineaLiquidacion, 0, len(filas)),
	}
	for _, f := range filas {
		p := prorratearFila(f)
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
// proceso). Si no, [liquidacion.Prorratear] con cubeta residual.
func prorratearFila(f FilaLiquidacion) liquidacion.Linea {
	if len(f.NetosProceso) == 0 {
		return liquidacion.Prorratear(f.Neto, f.ProcesoAdmin, f.ProcesoSocial, f.ProcesoReserva, f.ProcesoNeto)
	}
	lineas := liquidacion.ProrratearProceso(f.NetosProceso, f.ProcesoAdmin, f.ProcesoSocial, f.ProcesoReserva)
	if f.Indice < 0 || f.Indice >= len(lineas) {
		return liquidacion.Prorratear(f.Neto, f.ProcesoAdmin, f.ProcesoSocial, f.ProcesoReserva, f.ProcesoNeto)
	}
	return lineas[f.Indice]
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
