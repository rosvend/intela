package semilla

import (
	"bytes"
	"maps"
	"slices"
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

// TestParametrosSinteticosVanEtiquetados comprueba el conjunto ENTERO, no solo
// Wa/Wb/Wc.
//
// La version anterior miraba tres claves por su nombre, asi que los otros
// cuatro valores inventados podian llevar el nombre de un organo de gobierno
// -"Asamblea General", "Consejo Directivo"- y la prueba pasaba. El reglamento
// fija TECHOS ("hasta 20%", "hasta 10%", "hasta 5%"): escribir el techo como
// si fuera la tarifa que la Asamblea resolvio inventa una resolucion. La
// consulta con la que un auditor pregunta "ensename solo lo que aprobo un
// organo" es `WHERE reglamento <> 'RD-IX-seed-sintetico'`, y tiene que
// devolver unicamente lo que este publicado de verdad.
func TestParametrosSinteticosVanEtiquetados(t *testing.T) {
	sinteticosEsperados := map[string]bool{
		// Techos del reglamento, no tasas aprobadas (R-06, R-07).
		"deduccion.administrativa": true,
		"deduccion.social":         true,
		"reserva.errores_tecnicos": true,
		// Umbral de ingenieria: un ADR no es un reglamento (ADR 0007).
		"matching.umbral": true,
		// No publicados (RD 9.7, ADR 0004).
		"ott.wa": true, "ott.wb": true, "ott.wc": true,
	}

	sinteticos := map[string]bool{}
	for _, p := range Construir().Parametros {
		if p.VigenteDesde == "" || p.Organo == "" || p.Reglamento == "" {
			t.Fatalf("%s sin procedencia: no es un parametro, es una constante disfrazada (ADR 0004)", p.Clave)
		}
		// Las dos columnas van juntas o no dicen nada: un reglamento sintetico
		// con un organo de gobierno sigue citando una resolucion inexistente.
		if (p.Reglamento == ReglamentoSintetico) != (p.Organo == OrganoSintetico) {
			t.Fatalf("%s: reglamento=%q con organo=%q, las dos columnas van juntas",
				p.Clave, p.Reglamento, p.Organo)
		}
		if p.Reglamento == ReglamentoSintetico {
			sinteticos[p.Clave] = true
		}
	}

	if !maps.Equal(sinteticos, sinteticosEsperados) {
		t.Fatalf("parametros sinteticos = %v, se esperaban %v",
			slices.Sorted(maps.Keys(sinteticos)), slices.Sorted(maps.Keys(sinteticosEsperados)))
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

func TestObrasTraenGeneroYAnio(t *testing.T) {
	for _, o := range Construir().Obras {
		if o.Genero == "" || o.Anio <= 0 {
			t.Fatalf("obra %s: genero=%q anio=%d (00002 exige ambos)", o.ID, o.Genero, o.Anio)
		}
	}
}

func TestTitularesNaturalesTienenIPI(t *testing.T) {
	for _, tit := range Construir().Titulares {
		if tit.PersonaNatural && tit.IPI == "" {
			t.Fatalf("titular %s es persona natural sin IPI (R-01 / CHECK titular_natural_tiene_ipi)", tit.ID)
		}
	}
}

// TestObrasDelDatasetSonObrasValidas es la misma comprobacion que hace el seed
// al sembrar, pero sin Docker: entra en `go test -short`, asi que una obra sin
// coautores se cae en el bucle rapido y no varias capas mas tarde, leyendo el
// catalogo contra una base de verdad.
func TestObrasDelDatasetSonObrasValidas(t *testing.T) {
	for _, o := range Construir().Obras {
		obra, err := repertorio.NuevaObra(o.ID, repertorio.Metadatos{
			Titulo:    o.Titulo,
			Genero:    o.Genero,
			Anio:      o.Anio,
			Tipo:      o.Tipo,
			IDA:       o.IDA,
			EIDR:      o.EIDR,
			IMDB:      o.IMDB,
			Coautores: o.Coautores,
		})
		if err != nil {
			t.Fatalf("obra %s: %v", o.ID, err)
		}
		// El IPI de cada coautor tiene que ser el del padron: el catalogo se
		// busca por IPI (`RD 3`), y uno que no case es una obra que no
		// encuentra a su autor.
		for _, c := range obra.Coautores() {
			var enPadron bool
			for _, tit := range Construir().Titulares {
				if tit.IPI == c.IPI {
					enPadron = true
					break
				}
			}
			if !enPadron {
				t.Fatalf("obra %s: el coautor %q trae el IPI %q, que no esta en el padron",
					o.ID, c.Nombre, c.IPI)
			}
		}
	}
}
