package exportacion

import (
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/rosvend/intela/internal/aplicacion"
)

const hojaLiquidacion = "Liquidacion"

// GeneradorExcel renderiza la liquidacion con excelize. Las cifras van
// embebidas: no hay vinculos a la API, y el total es una formula SUM sobre
// las filas (OE-6 pide Excel, no CSV, porque las formulas importan).
type GeneradorExcel struct{}

func (GeneradorExcel) Generar(liq aplicacion.Liquidacion) (aplicacion.Archivo, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	if err := f.SetSheetName("Sheet1", hojaLiquidacion); err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("renombrar hoja: %w", err)
	}

	negrita, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	if err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("estilo titulo: %w", err)
	}
	encabezado, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("estilo encabezado: %w", err)
	}
	fmtDinero := "#,##0.00"
	dinero, err := f.NewStyle(&excelize.Style{CustomNumFmt: &fmtDinero})
	if err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("estilo dinero: %w", err)
	}
	dineroNegrita, err := f.NewStyle(&excelize.Style{
		Font:         &excelize.Font{Bold: true},
		CustomNumFmt: &fmtDinero,
	})
	if err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("estilo total: %w", err)
	}

	if err := f.SetCellValue(hojaLiquidacion, "A1", "Liquidacion de pago"); err != nil {
		return aplicacion.Archivo{}, err
	}
	if err := f.SetCellStyle(hojaLiquidacion, "A1", "A1", negrita); err != nil {
		return aplicacion.Archivo{}, err
	}
	if err := f.SetCellValue(hojaLiquidacion, "A2", "Periodo: "+etiquetaPeriodo(liq)); err != nil {
		return aplicacion.Archivo{}, err
	}

	encabezados := []string{"Periodo", "Obra", "Bruto", "Admin", "Social", "Reserva", "Neto"}
	for i, h := range encabezados {
		celda, err := excelize.CoordinatesToCellName(i+1, 4)
		if err != nil {
			return aplicacion.Archivo{}, err
		}
		if err := f.SetCellValue(hojaLiquidacion, celda, h); err != nil {
			return aplicacion.Archivo{}, err
		}
	}
	if err := f.SetCellStyle(hojaLiquidacion, "A4", "G4", encabezado); err != nil {
		return aplicacion.Archivo{}, err
	}

	for i, linea := range liq.Lineas {
		fila := 5 + i
		valores := []any{
			linea.Periodo,
			linea.Titulo,
			aFloat(linea.Bruto),
			aFloat(linea.Admin),
			aFloat(linea.Social),
			aFloat(linea.Reserva),
			aFloat(linea.Neto),
		}
		for col, v := range valores {
			celda, err := excelize.CoordinatesToCellName(col+1, fila)
			if err != nil {
				return aplicacion.Archivo{}, err
			}
			if err := f.SetCellValue(hojaLiquidacion, celda, v); err != nil {
				return aplicacion.Archivo{}, err
			}
		}
		if err := f.SetCellStyle(hojaLiquidacion, cell(3, fila), cell(7, fila), dinero); err != nil {
			return aplicacion.Archivo{}, err
		}
	}

	filaTotales := 5 + len(liq.Lineas)
	if err := f.SetCellValue(hojaLiquidacion, cell(1, filaTotales), "Totales"); err != nil {
		return aplicacion.Archivo{}, err
	}
	if err := f.SetCellStyle(hojaLiquidacion, cell(1, filaTotales), cell(1, filaTotales), encabezado); err != nil {
		return aplicacion.Archivo{}, err
	}

	if len(liq.Lineas) == 0 {
		for col := 3; col <= 7; col++ {
			if err := f.SetCellValue(hojaLiquidacion, cell(col, filaTotales), 0.0); err != nil {
				return aplicacion.Archivo{}, err
			}
		}
	} else {
		primera := 5
		ultima := 4 + len(liq.Lineas)
		for col := 3; col <= 7; col++ {
			letra := string(rune('A' + col - 1))
			formula := fmt.Sprintf("SUM(%s%d:%s%d)", letra, primera, letra, ultima)
			if err := f.SetCellFormula(hojaLiquidacion, cell(col, filaTotales), formula); err != nil {
				return aplicacion.Archivo{}, err
			}
		}
	}
	if err := f.SetCellStyle(hojaLiquidacion, cell(3, filaTotales), cell(7, filaTotales), dineroNegrita); err != nil {
		return aplicacion.Archivo{}, err
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("serializar xlsx: %w", err)
	}
	return aplicacion.Archivo{
		Nombre:    nombreArchivo(liq, "xlsx"),
		TipoMIME:  tipoXLSX,
		Contenido: buf.Bytes(),
	}, nil
}

func aFloat(d decimal.Decimal) float64 {
	v, _ := d.Round(2).Float64()
	return v
}

func cell(col, fila int) string {
	nombre, err := excelize.CoordinatesToCellName(col, fila)
	if err != nil {
		return fmt.Sprintf("A%d", fila)
	}
	return nombre
}
