// Package exportacion adapta el puerto aplicacion.Exportador.
//
// excelize y maroto viven aqui, no en aplicacion: depguard deniega ambos
// paquetes dentro del nucleo (ADR 0002, ADR 0010).
package exportacion

import (
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
)

const (
	tipoPDF  = "application/pdf"
	tipoXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// Combinado satisface aplicacion.Exportador reuniendo los dos generadores.
type Combinado struct {
	XLSX GeneradorExcel
	Docs GeneradorPDF
}

var _ aplicacion.Exportador = Combinado{}

func (c Combinado) Excel(liq aplicacion.Liquidacion) (aplicacion.Archivo, error) {
	return c.XLSX.Generar(liq)
}

func (c Combinado) PDF(liq aplicacion.Liquidacion) (aplicacion.Archivo, error) {
	return c.Docs.Generar(liq)
}

func nombreArchivo(liq aplicacion.Liquidacion, ext string) string {
	if liq.Periodo == "" {
		return "liquidacion." + ext
	}
	return fmt.Sprintf("liquidacion-%s.%s", liq.Periodo, ext)
}

func etiquetaPeriodo(liq aplicacion.Liquidacion) string {
	if liq.Periodo == "" {
		return "todos"
	}
	return liq.Periodo
}
