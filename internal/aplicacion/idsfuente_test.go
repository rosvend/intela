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

func TestEsClaveIDsFuente(t *testing.T) {
	if !EsClaveIDsFuente(ClaveShowID) || EsClaveIDsFuente("ID_Ficha") || EsClaveIDsFuente("") {
		t.Fatal("la lista cerrada tiene que aceptar las constantes y rechazar el encabezado del archivo")
	}
}

func TestLeerIDsFuenteEsEstricto(t *testing.T) {
	leido := LeerIDsFuente("871732\nID_Ficha=1\nfoo=bar\nid_ficha=\n=2\nimdb = tt1 ")
	esperado := map[string]string{ClaveIMDB: "tt1"}
	if !maps.Equal(leido, esperado) {
		t.Fatalf("LeerIDsFuente = %v, se esperaba %v", leido, esperado)
	}
}

// ---------------------------------------------------------------------------
// La clave logica de registro (ADR 0018 llevado a la fila persistida)

func TestClaveDeRegistroPorFuente(t *testing.T) {
	casos := []struct {
		nombre                   string
		fuente, ids, fecha, hora string
		quiero                   string
	}{
		{
			// La granularidad de Caracol es la EMISION: sin la fecha y la hora,
			// 30 emisiones legitimas del mismo programa serian duplicados.
			nombre: "caracol: id_ficha mas fecha y hora",
			fuente: "caracol", ids: "id_ficha=871732\nimdb=tt01", fecha: "2025-01-02", hora: "20:00:00",
			quiero: "id_ficha=871732|fecha=2025-01-02|hora=20:00:00",
		},
		{
			// `netflix_id` es de EPISODIO y es unico en las 49 filas reales.
			// `show_id` se repite: sirve para aprender alias, no para deduplicar.
			nombre: "netflix: solo el id de episodio",
			fuente: "netflix", ids: "netflix_id=81003997\nseries_id=81004793\nshow_id=80141259",
			quiero: "netflix_id=81003997",
		},
		{
			nombre: "cine: el id de pelicula",
			fuente: "cine", ids: "id_pelicula=p-1",
			quiero: "id_pelicula=p-1",
		},
		{
			// Una fuente sin clave declarada no se deduplica: es "todavia no se
			// sabe que identifica un registro suyo", no "todas sus filas son
			// iguales".
			nombre: "fuente desconocida: vacia",
			fuente: "hotel-x", ids: "id_ficha=1",
			quiero: "",
		},
		{
			// Una clave a la que le falta una pieza no identifica el mismo
			// registro que una completa: compararlas marcaria como duplicadas
			// dos emisiones distintas del mismo programa.
			nombre: "caracol sin hora: vacia",
			fuente: "caracol", ids: "id_ficha=871732", fecha: "2025-01-02",
			quiero: "",
		},
		{
			nombre: "caracol sin id_ficha: vacia",
			fuente: "caracol", fecha: "2025-01-02", hora: "20:00:00",
			quiero: "",
		},
		{
			// ids_fuente fuera de contrato lo ignora LeerIDsFuente, y sin la
			// clave la fila no se puede comparar.
			nombre: "ids_fuente fuera de contrato: vacia",
			fuente: "netflix", ids: "871732",
			quiero: "",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := ClaveDeRegistro(c.fuente, c.ids, c.fecha, c.hora)
			if got != c.quiero {
				t.Fatalf("ClaveDeRegistro = %q, se esperaba %q", got, c.quiero)
			}
		})
	}
}

// Dos filas que son el mismo hecho dan la misma clave, y dos que no, no. Es la
// propiedad entera del detector de duplicado por registro del #37.
func TestClaveDeRegistroDistingueElMismoHechoDeDosHechos(t *testing.T) {
	mismo := ClaveDeRegistro("caracol", "id_ficha=7", "2025-01-02", "20:00:00")
	otra := ClaveDeRegistro("caracol", "id_ficha=7", "2025-01-02", "22:00:00")
	if mismo == "" || otra == "" {
		t.Fatalf("alguna clave salio vacia: %q y %q", mismo, otra)
	}
	if mismo == otra {
		t.Fatalf("dos emisiones a distinta hora dieron la misma clave %q", mismo)
	}
	if repetida := ClaveDeRegistro("caracol", "id_ficha=7", "2025-01-02", "20:00:00"); repetida != mismo {
		t.Fatalf("la misma fila dio %q y %q", mismo, repetida)
	}
}

// ComponentesDeClaveDeRegistro devuelve una copia: el mapa es estado de
// paquete y quien lo reciba no debe poder reordenarlo para todos los demas.
func TestComponentesDeClaveDeRegistroDevuelveUnaCopia(t *testing.T) {
	primera := ComponentesDeClaveDeRegistro("caracol")
	if len(primera) != 3 {
		t.Fatalf("componentes de caracol = %v", primera)
	}
	primera[0] = "pisado"
	if segunda := ComponentesDeClaveDeRegistro("caracol"); segunda[0] != ClaveIDFicha {
		t.Fatalf("el mapa quedo mutado: %v", segunda)
	}
	if ComponentesDeClaveDeRegistro("no-existe") != nil {
		t.Fatal("una fuente desconocida devolvio componentes")
	}
}

// `fecha` y `hora` no son claves de ids_fuente y no pueden colarse en
// EscribirIDsFuente: si algun dia lo fueran, el separador `|` de la clave
// logica dejaria de distinguir un componente de otro.
func TestLosComponentesCanonicosNoSonClavesDeIDsFuente(t *testing.T) {
	for _, c := range []string{ComponenteFecha, ComponenteHora} {
		if EsClaveIDsFuente(c) {
			t.Fatalf("%q es a la vez componente canonico y clave de ids_fuente", c)
		}
	}
}
