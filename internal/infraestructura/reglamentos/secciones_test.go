package reglamentos

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/aplicacion/herramientas"
)

const md9 = `---
reglamento: Reglamento de Distribucion
version: IX
seccion: 9. Metodología para la distribución
---

# 9. Metodología para la distribución

9.1. Televisión abierta y radiodifundida
Los recaudos provenientes de canales.

9.1.1. Fórmula para valorización de una obra
Total puntos por obra = Tipo de obra * Duración * Rating

10 El índice de medición que REDES SGC usa.
   1.     Una lista indentada
9.1. Una referencia hacia atras no abre seccion
`

const mdTarifas = `---
reglamento: Reglamento de Tarifas
version: VI
seccion: 3. Tarifas según la categoría de Usuario
---

# 3. Tarifas según la categoría de Usuario

Se definen seis tipos de usuario:

1. Televisión
`

const mdPresentacion = `---
reglamento: Reglamento de Distribucion
version: IX
seccion: Presentacion
---

# Presentacion

Sin numeral, no se puede citar.
`

func porCita(ss []aplicacion.SeccionReglamento) map[string]aplicacion.SeccionReglamento {
	m := map[string]aplicacion.SeccionReglamento{}
	for _, s := range ss {
		m[s.Cita] = s
	}
	return m
}

func TestLeerPartePorNumeralConSuCita(t *testing.T) {
	fsys := fstest.MapFS{
		"distribucion-v9/09-metodologia.md":  {Data: []byte(md9)},
		"distribucion-v9/00-presentacion.md": {Data: []byte(mdPresentacion)},
		"tarifas-v6/03-tarifas.md":           {Data: []byte(mdTarifas)},
		"README.md":                          {Data: []byte("# Reglamentos\n")},
	}

	ss, err := Leer(fsys)
	if err != nil {
		t.Fatal(err)
	}
	m := porCita(ss)
	if len(ss) != 3 {
		t.Fatalf("se esperaban RD 9.1, RD 9.1.1 y RT 3 (RD 9 solo agrupa); got %v", ss)
	}
	s := m["RD 9.1.1"]
	if s.Titulo != "Fórmula para valorización de una obra" || s.Reglamento != "Reglamento de Distribucion IX" {
		t.Errorf("RD 9.1.1 = %+v", s)
	}
	if !strings.Contains(s.Texto, "Total puntos por obra") || !strings.Contains(s.Texto, "referencia hacia atras") || !strings.Contains(s.Texto, "Una lista indentada") {
		t.Errorf("texto de RD 9.1.1 incompleto: %q", s.Texto)
	}
	if !strings.Contains(m["RD 9.1"].Texto, "Los recaudos provenientes") {
		t.Errorf("RD 9.1 = %+v", m["RD 9.1"])
	}
	if rt := m["RT 3"]; rt.Titulo != "Tarifas según la categoría de Usuario" || !strings.Contains(rt.Texto, "1. Televisión") {
		t.Errorf("RT 3 = %+v", rt)
	}
}

func TestLeerFallaConUnReglamentoDesconocido(t *testing.T) {
	fsys := fstest.MapFS{"x/01.md": {Data: []byte(strings.Replace(md9, "Reglamento de Distribucion", "Reglamento de Cocina", 1))}}

	if _, err := Leer(fsys); err == nil {
		t.Fatal("un reglamento sin abreviatura citable tiene que fallar")
	}
}

func TestLeerLosReglamentosReales(t *testing.T) {
	ss, err := Leer(os.DirFS("../../../docs/reglamentos"))
	if err != nil {
		t.Fatal(err)
	}
	m := porCita(ss)
	if len(m) != len(ss) {
		t.Fatalf("citas duplicadas: %d secciones, %d citas", len(ss), len(m))
	}
	// Los de la segunda linea tienen el encabezado indentado en el .md.
	for _, c := range []string{"RD 5.1", "RD 9.1.1", "RD 13.1.3", "RD 15", "RT 3.1", "RS 4.1", "RA 2.1",
		"RD 7.4", "RD 7.5", "RD 13.1", "RD 13.2", "RD 16.2", "RT 3.4.1", "RT 3.4.2", "RT 3.6", "RT 3.7"} {
		if s, ok := m[c]; !ok || strings.TrimSpace(s.Texto) == "" {
			t.Errorf("falta %s o esta vacia", c)
		}
	}
	if strings.Contains(m["RD 13.1.6"].Texto, "Documentos aportados por REDES SGC") {
		t.Errorf("RD 13.2 quedo pegado a RD 13.1.6: %q", m["RD 13.1.6"].Texto)
	}
}

func TestNingunaSeccionRealSeRecortaAlDevolverla(t *testing.T) {
	ss, err := Leer(os.DirFS("../../../docs/reglamentos"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ss {
		if n := utf8.RuneCountInString(s.Texto); n > herramientas.MaxRunasTextoSeccion {
			t.Errorf("%s tiene %d runas y buscar_reglamento la recortaria a %d", s.Cita, n, herramientas.MaxRunasTextoSeccion)
		}
	}
	if n := utf8.RuneCountInString(porCita(ss)["RD 13.5"].Texto); n < 6000 {
		t.Errorf("RD 13.5 tiene %d runas; la prueba necesita una seccion real larga", n)
	}
}
