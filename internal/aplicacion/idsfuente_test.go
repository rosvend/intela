package aplicacion

import (
	"maps"
	"testing"
)

// Contrato de ids_fuente (ADR 0018): lo que escribe EscribirIDsFuente es
// exactamente lo que lee LeerIDsFuente. Es la prueba que ata el lado de la
// ingesta con el de la cascada.
func TestIDsFuenteIdaYVuelta(t *testing.T) {
	texto, err := EscribirIDsFuente(
		IDFuente{Clave: ClaveNetflixID, Valor: "81003997"},
		IDFuente{Clave: ClaveShowID, Valor: " 80141259 "},
		IDFuente{Clave: ClaveSeriesID, Valor: "81004793"},
		IDFuente{Clave: ClaveEIDR, Valor: ""}, // columna sin dato: se omite
	)
	if err != nil {
		t.Fatalf("EscribirIDsFuente: %v", err)
	}

	// Ordenado por clave y sin la vacia: el mismo texto para la misma fila.
	const quiero = "netflix_id=81003997\nseries_id=81004793\nshow_id=80141259"
	if texto != quiero {
		t.Fatalf("texto = %q, se esperaba %q", texto, quiero)
	}

	leido := LeerIDsFuente(texto)
	esperado := map[string]string{
		ClaveNetflixID: "81003997",
		ClaveShowID:    "80141259",
		ClaveSeriesID:  "81004793",
	}
	if !maps.Equal(leido, esperado) {
		t.Fatalf("LeerIDsFuente = %v, se esperaba %v", leido, esperado)
	}
}

func TestEscribirIDsFuenteRechazaLoQueElLectorIgnoraria(t *testing.T) {
	casos := map[string][]IDFuente{
		"clave fuera del contrato": {{Clave: "ID_Ficha", Valor: "871732"}},
		"clave vacia":              {{Clave: "", Valor: "871732"}},
		"clave repetida":           {{Clave: ClaveIDFicha, Valor: "1"}, {Clave: ClaveIDFicha, Valor: "2"}},
		"valor con salto de linea": {{Clave: ClaveIDFicha, Valor: "1\nimdb=tt1"}},
		"valor con igual":          {{Clave: ClaveIDFicha, Valor: "a=b"}},
	}
	for nombre, ids := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := EscribirIDsFuente(ids...); err == nil {
				t.Fatal("se esperaba un error")
			}
		})
	}
}

func TestEscribirIDsFuenteSinIDsEsVacio(t *testing.T) {
	texto, err := EscribirIDsFuente(IDFuente{Clave: ClaveIMDB, Valor: "  "})
	if err != nil {
		t.Fatalf("EscribirIDsFuente: %v", err)
	}
	if texto != "" {
		t.Fatalf("texto = %q, se esperaba vacio", texto)
	}
}

func TestLeerIDsFuenteEsEstricto(t *testing.T) {
	leido := LeerIDsFuente("871732\nID_Ficha=1\nfoo=bar\nid_ficha=\n=2\nimdb = tt1 ")
	esperado := map[string]string{ClaveIMDB: "tt1"}
	if !maps.Equal(leido, esperado) {
		t.Fatalf("LeerIDsFuente = %v, se esperaba %v", leido, esperado)
	}
}
