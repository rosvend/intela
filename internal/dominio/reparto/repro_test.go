package reparto_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// serializarResultado es la forma canonica de un Resultado para comparar dos
// corridas: JSON con las cifras en texto. `decimal.Decimal` se serializa como
// numero exacto -nunca coma flotante-, y los recorridos del motor van sobre
// secuencias ordenadas, asi que dos corridas iguales dan los mismos bytes.
//
// Vive aqui y no en el paquete porque es instrumentacion de la prueba, no
// parte del motor: el motor no sabe que existe la serializacion.
func serializarResultado(r reparto.Resultado) ([]byte, error) {
	return json.Marshal(r)
}

func TestSerializarResultadoEsEstable(t *testing.T) {
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica", CanalID: "z",
			DuracionMin: d("70"), Emisiones: 1, Rating: d("4.5")},
	}
	decls := []repertorio.Declaracion{declCompleta("x", "tx", "IPI-X")}

	primero, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	segundo, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}

	a, err := serializarResultado(primero)
	if err != nil {
		t.Fatal(err)
	}
	b, err := serializarResultado(segundo)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("la misma corrida serializo distinto:\n%s\n%s", a, b)
	}
}

// TestReparto_ReejecucionEsIdentica es la comparacion que pide el ADR 0005:
// correr dos veces los insumos dorados del Canal Z (RD 9.1.1) tiene que dar
// bit a bit lo mismo. Corre en CI en la etapa dedicada (repro-reparto), y
// tambien dentro del `go test ./...` normal: el determinismo no es una
// propiedad que se compruebe solo a veces.
//
// Se comparan los bytes serializados Y la igualdad profunda: los bytes
// prueban lo observable -lo que se pagaria-, y el DeepEqual lo interno -un
// campo sin etiqueta json que divergiera no cambiaria los bytes y si el
// comportamiento futuro.
func TestReparto_ReejecucionEsIdentica(t *testing.T) {
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica", CanalID: "z",
			DuracionMin: d("70"), Emisiones: 1, Rating: d("4.5")},
		{ObraID: "y", Modalidad: reparto.TV, TipoObra: "serie", CanalID: "z",
			DuracionMin: d("48"), Emisiones: 10, Rating: d("9")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}

	primero, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	segundo, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(primero, segundo) {
		t.Fatalf("la reejecucion difiere:\n%+v\n%+v", primero, segundo)
	}
	a, err := serializarResultado(primero)
	if err != nil {
		t.Fatal(err)
	}
	b, err := serializarResultado(segundo)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("la reejecucion serializa distinto:\n%s\n%s", a, b)
	}
	cierra(t, primero)
}
