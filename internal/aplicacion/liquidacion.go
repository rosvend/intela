package aplicacion

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
)

// periodoValido es el mismo patron que el CHECK de procesos.periodo.
var periodoValido = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2})?$`)

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
		p := liquidacion.Prorratear(f.Neto, f.ProcesoAdmin, f.ProcesoSocial, f.ProcesoReserva, f.ProcesoNeto)
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
	if !periodoValido.MatchString(periodo) {
		return ErrPeriodoInvalido
	}
	return nil
}
