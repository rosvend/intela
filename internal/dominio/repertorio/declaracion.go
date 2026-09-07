package repertorio

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrDeclaracionInvalida es el unico centinela que emite [NuevaDeclaracion].
//
// Distinto de "incompleta": una declaracion cuya suma da menos de 100 es un
// ESTADO valido del negocio (R-04, `RD 13.1.3`) y [NuevaDeclaracion] la acepta
// sin protestar. Lo que este error marca es que lo que llego no llega siquiera
// a ser una declaracion -falta un IPI, un porcentaje no es positivo, un
// titular aparece dos veces, o la suma se PASA de 100, que no tiene lectura de
// negocio posible-.
var ErrDeclaracionInvalida = errors.New("declaracion invalida")

type Parte struct {
	TitularID  string
	IPI        string
	Porcentaje decimal.Decimal
}

type Declaracion struct {
	ObraID string
	Partes []Parte
}

// NuevaDeclaracion valida un conjunto de partes antes de que llegue a
// persistirse.
//
// No es lo mismo que [Declaracion.Completa]: esa lectura clasifica una
// declaracion YA GUARDADA como completa o incompleta, partes invalidas
// incluidas, porque una fila que ya esta en la base no se puede rechazar
// retroactivamente. Esto es la puerta de ENTRADA -lo que valida un alta o una
// edicion antes de escribir nada- y por eso es mas estricta: una parte sin IPI
// o con un porcentaje que no es positivo no es "declaracion incompleta", es un
// dato mal formado que no debe llegar a guardarse.
func NuevaDeclaracion(obraID string, partes []Parte) (Declaracion, error) {
	if obraID == "" {
		return Declaracion{}, fmt.Errorf("%w: falta el identificador de la obra", ErrDeclaracionInvalida)
	}
	if len(partes) == 0 {
		return Declaracion{}, fmt.Errorf("%w: no trae ninguna parte", ErrDeclaracionInvalida)
	}

	vistos := make(map[string]bool, len(partes))
	suma := decimal.Zero
	for _, p := range partes {
		if p.TitularID == "" {
			return Declaracion{}, fmt.Errorf("%w: una parte no trae titular", ErrDeclaracionInvalida)
		}
		if vistos[p.TitularID] {
			return Declaracion{}, fmt.Errorf("%w: el titular %q aparece dos veces", ErrDeclaracionInvalida, p.TitularID)
		}
		vistos[p.TitularID] = true

		if p.IPI == "" {
			return Declaracion{}, fmt.Errorf("%w: al titular %q le falta el IPI", ErrDeclaracionInvalida, p.TitularID)
		}
		if p.Porcentaje.LessThanOrEqual(decimal.Zero) {
			return Declaracion{}, fmt.Errorf("%w: el porcentaje del titular %q tiene que ser positivo", ErrDeclaracionInvalida, p.TitularID)
		}
		suma = suma.Add(p.Porcentaje)
	}

	// Sumar mas de 100 no tiene lectura de negocio: R-04 solo distingue "suma
	// exacta" de "suma menor", nunca "suma mayor". Una suma menor SI pasa: es
	// la declaracion_incompleta valida que Estado() ya sabe reconocer.
	if suma.GreaterThan(decimal.NewFromInt(100)) {
		return Declaracion{}, fmt.Errorf("%w: la suma de porcentajes es %s%%, no puede superar 100",
			ErrDeclaracionInvalida, suma.StringFixed(4))
	}

	return Declaracion{ObraID: obraID, Partes: partes}, nil
}

func (d Declaracion) Completa() bool {
	if len(d.Partes) == 0 {
		return false
	}
	suma := decimal.Zero
	for _, p := range d.Partes {
		if p.IPI == "" {
			return false
		}
		// Cada parte tiene que ser positiva por si sola. Sin esto, 150 y -50
		// suman 100 y una declaracion imposible pasa por "completa", que es
		// justo la puerta que R-04 (RD 13.1.3) cierra: si lo declarado no
		// suma 100%, no se reparte nada de esa obra.
		if p.Porcentaje.LessThanOrEqual(decimal.Zero) {
			return false
		}
		suma = suma.Add(p.Porcentaje)
	}
	return suma.Equal(decimal.NewFromInt(100))
}

func (d Declaracion) Estado() string {
	if d.Completa() {
		return "completa"
	}
	return "incompleta"
}
