package exportacion

import (
	"fmt"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

// GeneradorPDF renderiza la liquidacion con maroto. Las cifras van
// embebidas en el documento: no hay enlaces al panel, y el archivo se
// puede abrir sin red (OE-6).
type GeneradorPDF struct{}

func (GeneradorPDF) Generar(liq aplicacion.Liquidacion) (aplicacion.Archivo, error) {
	cfg := config.NewBuilder().
		WithOrientation(orientation.Horizontal).
		WithPageNumber().
		Build()
	m := maroto.New(cfg)

	m.AddRow(12, text.NewCol(12, "Liquidacion de pago", props.Text{
		Size:  14,
		Style: fontstyle.Bold,
	}))
	m.AddRow(8, text.NewCol(12, "Periodo: "+etiquetaPeriodo(liq), props.Text{Size: 10}))
	m.AddRow(4, col.New(12))

	m.AddRow(8, celdasPDF([]string{"Periodo", "Obra", "Bruto", "Admin", "Social", "Reserva", "Neto"}, true)...)
	for _, linea := range liq.Lineas {
		m.AddRow(7, celdasPDF([]string{
			linea.Periodo,
			linea.Titulo,
			dinero(linea.Bruto),
			dinero(linea.Admin),
			dinero(linea.Social),
			dinero(linea.Reserva),
			dinero(linea.Neto),
		}, false)...)
	}
	m.AddRow(8, celdasPDF([]string{
		"Totales",
		"",
		dinero(liq.Totales.Bruto),
		dinero(liq.Totales.Admin),
		dinero(liq.Totales.Social),
		dinero(liq.Totales.Reserva),
		dinero(liq.Totales.Neto),
	}, true)...)

	doc, err := m.Generate()
	if err != nil {
		return aplicacion.Archivo{}, fmt.Errorf("generar pdf: %w", err)
	}
	return aplicacion.Archivo{
		Nombre:    nombreArchivo(liq, "pdf"),
		TipoMIME:  tipoPDF,
		Contenido: doc.GetBytes(),
	}, nil
}

func celdasPDF(valores []string, negrita bool) []core.Col {
	anchos := []int{2, 3, 2, 1, 1, 1, 2}
	estilo := props.Text{Size: 8, Top: 1}
	if negrita {
		estilo.Style = fontstyle.Bold
	}
	out := make([]core.Col, 0, len(valores))
	for i, v := range valores {
		e := estilo
		if i >= 2 {
			e.Align = align.Right
		}
		out = append(out, text.NewCol(anchos[i], v, e))
	}
	return out
}

func dinero(d decimal.Decimal) string {
	return d.StringFixed(2)
}
