package aplicacion

import "testing"

func TestFormatoDeNombre(t *testing.T) {
	t.Parallel()

	casos := map[string]string{
		"parrilla.xlsx":      FormatoXLSX,
		"PARRILLA.XLSX":      FormatoXLSX,
		"macro.xlsm":         FormatoXLSX,
		"reporte.csv":        FormatoCSV,
		"reporte.json":       FormatoJSON,
		"padron.xls":         "", // formato binario viejo: excelize no lo lee
		"sin_extension":      "",
		"reporte.xlsx.zip":   "",
		"archivo.raro..":     "",
		"reportes/enero.csv": FormatoCSV,
	}
	for nombre, quiere := range casos {
		if got := FormatoDeNombre(nombre); got != quiere {
			t.Errorf("FormatoDeNombre(%q) = %q, se esperaba %q", nombre, got, quiere)
		}
	}
}
