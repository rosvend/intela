package ingesta

import (
	"slices"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// La clave logica de un registro esta escrita DOS VECES y en dos idiomas, y
// tiene que ser la misma:
//
//   - aqui, en `Mapa.ClaveRegistro`, con los nombres de COLUMNA del archivo
//     del cliente (`ID_Ficha`, `Fecha`). Es lo unico que sirve mientras la
//     fila todavia es una fila de un .xlsx;
//   - en `aplicacion.clavesDeRegistro`, con el vocabulario de ids_fuente del
//     ADR 0018 (`id_ficha`, `fecha`). Es lo unico que sirve cuando la fila ya
//     esta en `usos` y las columnas del archivo no existen.
//
// No se pueden fundir: son dos representaciones de la misma decision en dos
// momentos distintos del dato. Lo que si se puede es impedir que se separen, y
// eso es esta prueba: traduce cada ClaveRegistro al vocabulario canonico y
// exige que coincida componente a componente. Si alguien anade una columna a
// la clave de Caracol y no la anade alla, esto falla -- que es justo la deriva
// que el ADR 0018 existe para impedir, y que de otro modo se notaria como
// "duplicados que no se detectan", sin que nada fallara.
func TestClaveDeRegistroCuadraConLosMapasDeIngesta(t *testing.T) {
	for _, m := range MapasDelCliente() {
		t.Run(m.Fuente, func(t *testing.T) {
			canonica, err := claveCanonicaDe(m)
			if err != nil {
				t.Fatalf("traducir la clave de %q: %v", m.Fuente, err)
			}
			declarada := aplicacion.ComponentesDeClaveDeRegistro(m.Fuente)
			if !slices.Equal(canonica, declarada) {
				t.Fatalf(
					"la clave de registro de %q es %v segun el mapa de columnas y %v segun aplicacion.ComponentesDeClaveDeRegistro",
					m.Fuente, canonica, declarada)
			}
		})
	}
}

// Y al reves: ninguna fuente perfilada puede quedarse sin clave canonica
// declarada. Sin esto, borrar una entrada de `clavesDeRegistro` dejaria la
// prueba de arriba comparando dos listas vacias.
func TestTodaFuentePerfiladaTieneClaveCanonica(t *testing.T) {
	for _, m := range MapasDelCliente() {
		if len(m.ClaveRegistro) == 0 {
			t.Fatalf("la fuente %q no declara ClaveRegistro en su mapa", m.Fuente)
		}
		if len(aplicacion.ComponentesDeClaveDeRegistro(m.Fuente)) == 0 {
			t.Fatalf("la fuente %q no tiene clave canonica en aplicacion", m.Fuente)
		}
	}
}

// claveCanonicaDe traduce las columnas del archivo a los componentes del
// contrato: una columna de ids_fuente aporta su ClaveID, y `fecha` y `hora`
// aportan su nombre canonico. Cualquier otro campo no tiene traduccion y es un
// fallo de la prueba, no un caso que ignorar.
func claveCanonicaDe(m Mapa) ([]string, error) {
	porNombre := make(map[string]Columna, len(m.Columnas))
	for _, c := range m.Columnas {
		porNombre[c.Nombre] = c
	}

	out := make([]string, 0, len(m.ClaveRegistro))
	for _, nombre := range m.ClaveRegistro {
		c, hay := porNombre[nombre]
		if !hay {
			return nil, errClaveSinColumna{fuente: m.Fuente, columna: nombre}
		}
		switch c.Campo {
		case CampoIDsFuente:
			out = append(out, c.ClaveID)
		case CampoFecha:
			out = append(out, aplicacion.ComponenteFecha)
		case CampoHora:
			out = append(out, aplicacion.ComponenteHora)
		default:
			return nil, errCampoSinTraduccion{fuente: m.Fuente, campo: string(c.Campo)}
		}
	}
	return out, nil
}

type errClaveSinColumna struct{ fuente, columna string }

func (e errClaveSinColumna) Error() string {
	return "la clave de registro de " + e.fuente + " nombra la columna " + e.columna +
		", que el mapa no declara"
}

type errCampoSinTraduccion struct{ fuente, campo string }

func (e errCampoSinTraduccion) Error() string {
	return "el campo " + e.campo + " de " + e.fuente +
		" entra en la clave de registro y no tiene componente canonico en aplicacion.ClaveDeRegistro"
}
