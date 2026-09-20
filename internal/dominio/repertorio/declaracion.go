package repertorio

import (
	"errors"
	"fmt"
	"strings"

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

// precisionPorcentaje es la escala de la columna `declaraciones.porcentaje`
// (NUMERIC(8,4), migracion 00001): 4 decimales.
//
// NuevaDeclaracion RECHAZA -no redondea- un porcentaje con mas precision que
// esta. Redondear en el dominio exigiria que el modo de redondeo de
// shopspring/decimal coincida bit a bit con el de Postgres al truncar a la
// escala de la columna, y una diferencia de un solo caso limite (un empate a
// la mitad del ultimo decimal) dejaria al dominio validando una suma que la
// base termina guardando distinta. Rechazar de entrada evita esa dependencia
// por completo: lo que este validador acepta ya cabe exacto en la columna,
// sin que Postgres tenga que redondear nada.
//
// Sin esto, un porcentaje positivo por debajo de la mitad del ultimo decimal
// -0.00004, por ejemplo- pasaba esta validacion (es > 0 en la precision
// arbitraria de Go) y Postgres lo redondeaba a 0.0000 al escribir: el UNICO
// lugar donde eso se notaba era el CHECK (porcentaje > 0) de la tabla, que
// sale como un error generico y no como el ErrDeclaracionInvalida que
// corresponde a un dato mal formado. Y en la otra direccion: dos partes que
// suman un poco MENOS de 100 (declaracion_incompleta valida, R-04) podian
// redondear cada una por separado hasta sumar exactamente 100 al guardarse
// -una declaracion que nunca llego a completa segun el dominio, pasando a
// "completa" para el motor de reparto sin que nadie la haya validado asi.
const precisionPorcentaje = 4

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
//
// # Forma canonica
//
// `TitularID` e `IPI` se recortan aqui, sobre una copia: el padron guarda esos
// mismos campos recortados ([afiliacion.NuevoTitular]) y esta puerta es quien
// decide que es una parte valida, asi que es donde se fija la forma canonica.
// Quien recibe la [Declaracion] devuelta -la comprobacion de R-01, la
// conciliacion del IPI, lo que se persiste- trabaja con lo ya recortado y no
// vuelve a decidirlo. Un id de solo espacios cae en "no trae titular", y
// `" tit-a "` y `"tit-a"` son el mismo titular repetido.
//
// `ObraID` NO se recorta: viene del path de la ruta y recortarlo cambiaria que
// obra se busca, y con ello que 404 se devuelve.
func NuevaDeclaracion(obraID string, partes []Parte) (Declaracion, error) {
	if obraID == "" {
		return Declaracion{}, fmt.Errorf("%w: falta el identificador de la obra", ErrDeclaracionInvalida)
	}
	if len(partes) == 0 {
		return Declaracion{}, fmt.Errorf("%w: no trae ninguna parte", ErrDeclaracionInvalida)
	}

	normalizadas := make([]Parte, len(partes))
	vistos := make(map[string]bool, len(partes))
	suma := decimal.Zero
	for i, p := range partes {
		p.TitularID = strings.TrimSpace(p.TitularID)
		p.IPI = strings.TrimSpace(p.IPI)
		normalizadas[i] = p

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
		if !p.Porcentaje.Equal(p.Porcentaje.Round(precisionPorcentaje)) {
			return Declaracion{}, fmt.Errorf("%w: el porcentaje del titular %q admite hasta %d decimales",
				ErrDeclaracionInvalida, p.TitularID, precisionPorcentaje)
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

	return Declaracion{ObraID: obraID, Partes: normalizadas}, nil
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
