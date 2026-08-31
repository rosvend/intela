package semilla

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

func TestConstruirEsDeterminista(t *testing.T) {
	a, b := Construir(), Construir()
	if a.Periodo != b.Periodo {
		t.Fatalf("Periodo: %q vs %q", a.Periodo, b.Periodo)
	}
	if len(a.Obras) != len(b.Obras) || a.Obras[0].ID != b.Obras[0].ID {
		t.Fatal("las obras no coinciden entre dos Construir()")
	}
	if !bytes.Equal(a.Reportes[0].Bytes, b.Reportes[0].Bytes) {
		t.Fatal("los bytes del reporte TV no son estables")
	}
}

func TestDeclaracionesCubrenLosTresCasos(t *testing.T) {
	d := Construir()
	estados := map[string]string{}
	coautores := map[string]int{}
	for _, decl := range d.Declaraciones {
		estados[decl.ObraID] = decl.Estado()
		coautores[decl.ObraID] = len(decl.Partes)
	}

	if estados[ObraCine] != "completa" {
		t.Fatalf("obra cine: Estado() = %q, se esperaba completa", estados[ObraCine])
	}
	if estados[ObraSerie] != "incompleta" {
		t.Fatalf("obra serie: Estado() = %q, se esperaba incompleta", estados[ObraSerie])
	}
	if coautores[ObraUnitario] < 3 {
		t.Fatalf("obra unitario: %d partes, se esperaban al menos 3 coautores", coautores[ObraUnitario])
	}
	if estados[ObraUnitario] != "completa" {
		t.Fatalf("obra unitario (coautores): Estado() = %q, se esperaba completa", estados[ObraUnitario])
	}

	// El dominio, no un SUM en el test: 40+35+25 tiene que ser 100 con IPI.
	decl := repertorio.Declaracion{}
	for _, x := range d.Declaraciones {
		if x.ObraID == ObraUnitario {
			decl = x
			break
		}
	}
	if !decl.Completa() {
		t.Fatal("la declaracion multi-coautor tenia que sumar 100 con IPI en cada parte")
	}
}

func TestUsosCubrenTVCineOTTYPonderacion(t *testing.T) {
	d := Construir()

	modalidad := map[reparto.Modalidad]int{}
	tiposTV := map[string]bool{}
	var ottConVistas bool

	for _, r := range d.Reportes {
		if len(r.Bytes) == 0 {
			t.Fatalf("reporte %q sin bytes crudos", r.Fuente)
		}
		for _, u := range r.Usos {
			modalidad[u.Modalidad]++
			if u.Modalidad == reparto.TV {
				tiposTV[u.TipoObra] = true
			}
			if u.Modalidad == reparto.OTT && u.Vistas.GreaterThan(decimal.Zero) {
				ottConVistas = true
			}
			if u.ONI {
				t.Fatalf("uso %q marcado ONI: el seed identifica a proposito para las demos", u.Titulo)
			}
			if u.ObraID == "" {
				t.Fatalf("uso %q sin obra_id", u.Titulo)
			}
		}
	}

	for _, m := range []reparto.Modalidad{reparto.TV, reparto.Cine, reparto.OTT} {
		if modalidad[m] == 0 {
			t.Fatalf("no hay filas de uso para %s", m)
		}
	}
	for _, tipo := range []string{"cinematografica", "unitario", "serie", "sketches"} {
		if !tiposTV[tipo] {
			t.Fatalf("la parrilla TV no ejercita tipo %q (ponderacion RD 9.1.1)", tipo)
		}
	}
	if !ottConVistas {
		t.Fatal("las filas OTT tienen que traer V (vistas) poblado")
	}
}

func TestParametrosSinteticosVanEtiquetados(t *testing.T) {
	d := Construir()

	sinteticos := map[string]bool{}
	for _, p := range d.Parametros {
		if p.Clave == "ott.wa" || p.Clave == "ott.wb" || p.Clave == "ott.wc" {
			if p.Reglamento != ReglamentoSintetico || p.Organo != OrganoSintetico {
				t.Fatalf("%s: reglamento=%q organo=%q, se esperaba %s / %s",
					p.Clave, p.Reglamento, p.Organo, ReglamentoSintetico, OrganoSintetico)
			}
			sinteticos[p.Clave] = true
		}
		if p.VigenteDesde == "" || p.Organo == "" || p.Reglamento == "" {
			t.Fatalf("%s sin procedencia: no es un parametro, es una constante disfrazada (ADR 0004)", p.Clave)
		}
	}
	if len(sinteticos) != 3 {
		t.Fatalf("faltan coeficientes OTT sinteticos: %v", sinteticos)
	}
}

func TestBolsasPorUsuarioPeriodoCircuito(t *testing.T) {
	d := Construir()
	if len(d.Bolsas) == 0 {
		t.Fatal("no hay bolsas")
	}
	visto := map[string]bool{}
	var nacional, internacional bool
	for _, b := range d.Bolsas {
		clave := b.UsuarioID + "/" + b.Periodo + "/" + string(b.Circuito)
		if visto[clave] {
			t.Fatalf("bolsa duplicada para %s", clave)
		}
		visto[clave] = true
		if b.Periodo != Periodo {
			t.Fatalf("bolsa %s periodo %q, se esperaba %s", b.ID, b.Periodo, Periodo)
		}
		if b.Circuito == reparto.Nacional {
			nacional = true
		}
		if b.Circuito == reparto.Internacional {
			internacional = true
		}
		if !b.Bruto.GreaterThan(decimal.Zero) {
			t.Fatalf("bolsa %s con bruto %s", b.ID, b.Bruto)
		}
	}
	if !nacional || !internacional {
		t.Fatal("hace falta al menos una bolsa nacional y una internacional")
	}
}

func TestTitularesNaturalesTienenIPI(t *testing.T) {
	for _, tit := range Construir().Titulares {
		if tit.PersonaNatural && tit.IPI == "" {
			t.Fatalf("titular %s es persona natural sin IPI (R-01 / CHECK titular_natural_tiene_ipi)", tit.ID)
		}
	}
}
