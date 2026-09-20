package afiliacion

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrTitularInvalido es el unico centinela que emite la construccion de un
// [Titular].
//
// Uno solo y no cinco, por lo mismo que repertorio.ErrObraInvalida: quien
// llama necesita distinguir "esa fila del padron no forma un titular" de "la
// base no responde", y el detalle de QUE falta va en el texto envuelto, que es
// lo que lee una persona.
var ErrTitularInvalido = errors.New("titular invalido")

// Clase es el tipo de afiliado. Socio y Titular Administrado no son lo mismo:
// la distincion decide quien vota (capitulo 4 del reglamento de socios).
//
// Es un conjunto cerrado porque la columna `titulares.clase` lleva el mismo
// CHECK. Se valida aqui ademas de en el esquema a proposito: un INSERT que
// revienta por un CHECK dice "violacion de restriccion", no "la clase
// 'aspirante' no existe en el reglamento".
//
// No decide quien cobra. Eso es R-01 y lo dice
// [Titular.PuedeRecibirReparto], que mira la persona natural: un administrado
// cobra igual que un socio.
type Clase string

const (
	ClaseSocio        Clase = "socio"
	ClaseAdministrado Clase = "administrado"
)

// Clases devuelve las clases validas, en el orden en que las declara el CHECK
// de la migracion.
func Clases() []Clase { return []Clase{ClaseSocio, ClaseAdministrado} }

// Titular es una entrada del padron: quien figura ante la sociedad.
//
// # Estar en el padron no es poder cobrar
//
// Las dos cosas se confunden, y la diferencia es R-01 (`RD 4.5`): una
// productora esta en el padron y NO puede recibir reparto. Por eso el padron
// se lee entero, persona natural y todo -quien edita un reparto tiene que
// poder ver que el titular del padron al que no se le ofrece una parte existe
// y por que-, y por eso lo que decide el pago tiene nombre propio en
// [Titular.PuedeRecibirReparto] en vez de ser un booleano suelto.
//
// # Que no vive aqui
//
// Ni un porcentaje ni un saldo. Lo que un titular declara sobre una obra sale
// de la Declaracion de Obra (`R-02`, `R-03`) y lo que cobra sale del reparto;
// el padron es identidad.
//
// Tampoco el email, aunque la tabla lo tenga: es dato de contacto y ninguna
// regla de este paquete lo lee. Entra el dia que haya un caso de uso que lo
// consuma, no antes.
type Titular struct {
	id             string
	nombre         string
	ipi            string
	personaNatural bool
	clase          Clase
}

// NuevoTitular construye una entrada del padron o falla.
//
// No hay forma de obtener un Titular invalido: el constructor es el unico
// camino. Y las comprobaciones son EXACTAMENTE las del esquema -nombre no
// vacio, clase dentro del conjunto, e IPI no vacio cuando es persona
// natural-, ni una mas: este constructor tambien reconstruye lo que se lee de
// `titulares`, asi que un invariante mas estricto que el CHECK convertiria una
// fila legitima del padron en una lectura fallida.
//
// Las cadenas se recortan antes de validar y se guardan recortadas, igual que
// en repertorio.NuevaObra: asi el dominio y el btrim del esquema opinan lo
// mismo sobre lo que esta vacio.
//
// El identificador lo trae quien llama y no se genera aqui: es el del padron
// de REDES, que se asigna fuera de este sistema.
func NuevoTitular(id, nombre, ipi string, personaNatural bool, clase Clase) (Titular, error) {
	id = strings.TrimSpace(id)
	nombre = strings.TrimSpace(nombre)
	ipi = strings.TrimSpace(ipi)

	if id == "" {
		return Titular{}, fmt.Errorf("%w: falta el identificador", ErrTitularInvalido)
	}
	if nombre == "" {
		return Titular{}, fmt.Errorf("%w: falta el nombre", ErrTitularInvalido)
	}
	if !slices.Contains(Clases(), clase) {
		return Titular{}, fmt.Errorf("%w: clase %q, se esperaba una de %v",
			ErrTitularInvalido, clase, Clases())
	}
	// La misma condicion que el CHECK `titular_natural_tiene_ipi`: una
	// persona natural sin IPI es un nombre suelto que no resuelve a nadie
	// fuera de esta sociedad, y el IPI es lo unico que cruza (`RD 3`). A una
	// persona JURIDICA si se le admite el vacio, que es lo que hace el CHECK:
	// una productora sin IPI esta en el padron y no cobra reparto.
	if personaNatural && ipi == "" {
		return Titular{}, fmt.Errorf("%w: %q es persona natural y no tiene IPI",
			ErrTitularInvalido, nombre)
	}
	return Titular{
		id:             id,
		nombre:         nombre,
		ipi:            ipi,
		personaNatural: personaNatural,
		clase:          clase,
	}, nil
}

// ID es el identificador de la fila del padron. Lo referencian
// `declaraciones`, `usuarios` y las ordenes de pago, y no cambia.
func (t Titular) ID() string { return t.id }

// Nombre es el nombre de la persona o de la empresa, para mostrar.
func (t Titular) Nombre() string { return t.nombre }

// IPI es el identificador de autores de la CISAC (`RD 3`). Cadena vacia en un
// titular que no sea persona natural y no lo tenga.
func (t Titular) IPI() string { return t.ipi }

// PersonaNatural dice si el titular es una persona fisica.
func (t Titular) PersonaNatural() bool { return t.personaNatural }

// Clase es socio o administrado.
func (t Titular) Clase() Clase { return t.clase }

// PuedeRecibirReparto es R-01 / `RD 4.5`: solo un escritor persona natural
// recibe una orden de pago.
//
// Hoy devuelve el campo tal cual, y aun asi lleva el nombre de la regla. Quien
// consume el padron pregunta por R-01, no por una columna: el editor de
// reparto, que no puede ofrecer a una productora como parte, y la comprobacion
// de escritura de la declaracion tienen que estar preguntando lo mismo, y una
// condicion escrita dos veces es una condicion que algun dia discrepa. El
// nombre de la regla es ademas el del trigger que la impone en la base
// -`exigir_persona_natural`-, y que las dos mitades se llamen igual es lo que
// hace evidente que son la misma regla.
//
// La ultima barrera no es esta: es el trigger, que se dispara antes de que el
// dinero salga y tambien contra lo que entre por SQL crudo. Esto es la mitad
// del nucleo, que es lo que permite decirselo a quien edita ANTES de guardar.
func (t Titular) PuedeRecibirReparto() bool { return t.personaNatural }
