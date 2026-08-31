package afiliacion

import "strings"

// Subtipo del vinculo con REDES SGC. No es cosmetica: gobierna derechos
// politicos y quien puede pedir un anticipo (`R-30`).
type Subtipo string

const (
	// Socio: vinculo societario. Titular originario. `RS 4.1`.
	Socio Subtipo = "socio"

	// TitularAdministrado: vinculo contractual. Incluye a quien opta por
	// esta figura, a quien perdio la calidad de socio, y a los
	// derechohabientes. `RS 4.2`.
	TitularAdministrado Subtipo = "administrado"
)

// Estado de la solicitud de admision. El Consejo Directivo es quien mueve
// la maquina (`RS 5.2`, `RS 5.3`).
type Estado string

const (
	EstadoPendiente Estado = "pendiente"
	EstadoAdmitido  Estado = "admitido"
	EstadoRechazado Estado = "rechazado"
)

// Afiliado es una solicitud de ingreso, y tras la admision es la fila del
// padron.
//
// Las claves de documento son referencias al almacen de objetos, no el
// binario: el dominio no lee ficheros. Vacias significan "no se adjunto".
type Afiliado struct {
	ID                 string
	Nombre             string
	Email              string
	DocumentoIdentidad string
	IPI                string
	Subtipo            Subtipo
	Estado             Estado
	PersonaNatural     bool
	PerteneceOtraSGC   bool
	ClaveRUT           string
	ClaveCertBancaria  string
	ClaveRenuncia      string
	TitularID          string
}

// ElegibleAnticipo es la regla R-30: solo un Socio admitido puede pedir
// un anticipo. Un Titular Administrado no, aunque este admitido, porque el
// anticipo se cubre con obra futura.
func (a Afiliado) ElegibleAnticipo() bool {
	return a.Estado == EstadoAdmitido && a.Subtipo == Socio
}

// ValidarSolicitud comprueba lo que tiene que estar bien ANTES de persistir.
//
// No mira el IPI: recogerlo es opcional en el alta. Lo exige [Afiliado.Admitir]
// para una persona natural, que es cuando la fila entra al padron de cobro.
func (a Afiliado) ValidarSolicitud() error {
	if strings.TrimSpace(a.Nombre) == "" {
		return ErrNombreObligatorio
	}
	if !correoValido(a.Email) {
		return ErrEmailInvalido
	}
	if strings.TrimSpace(a.DocumentoIdentidad) == "" {
		return ErrDocumentoObligatorio
	}
	if a.Subtipo != Socio && a.Subtipo != TitularAdministrado {
		return ErrSubtipoInvalido
	}
	if strings.TrimSpace(a.ClaveRUT) == "" || strings.TrimSpace(a.ClaveCertBancaria) == "" {
		return ErrDocumentosPago
	}
	return a.verificarExclusividad()
}

// verificarExclusividad es R-28. Pertenece a otra SGC del mismo genero es
// admisible SOLO con el documento de renuncia. Un booleano a solas no basta:
// hay que guardar la evidencia.
func (a Afiliado) verificarExclusividad() error {
	if a.PerteneceOtraSGC && strings.TrimSpace(a.ClaveRenuncia) == "" {
		return ErrExclusividad
	}
	return nil
}

// Admitir pasa de pendiente a admitido.
//
// Revalida la solicitud: admitir no puede ser un atajo que salte R-12 o
// R-28. Y para una persona natural exige IPI, que es el identificador del
// padron (`RD 3`).
func (a Afiliado) Admitir(titularID string) (Afiliado, error) {
	if a.Estado != EstadoPendiente {
		return Afiliado{}, ErrEstadoInvalido
	}
	if err := a.ValidarSolicitud(); err != nil {
		return Afiliado{}, err
	}
	if a.PersonaNatural && strings.TrimSpace(a.IPI) == "" {
		return Afiliado{}, ErrIPIObligatorio
	}
	if strings.TrimSpace(titularID) == "" {
		return Afiliado{}, ErrEstadoInvalido
	}
	a.Estado = EstadoAdmitido
	a.TitularID = titularID
	return a, nil
}

// Rechazar pasa de pendiente a rechazado. Una solicitud ya resuelta no
// se vuelve a resolver: el Consejo ya decidio.
func (a Afiliado) Rechazar() (Afiliado, error) {
	if a.Estado != EstadoPendiente {
		return Afiliado{}, ErrEstadoInvalido
	}
	a.Estado = EstadoRechazado
	return a, nil
}

func correoValido(email string) bool {
	email = strings.TrimSpace(email)
	arroba := strings.IndexByte(email, '@')
	if arroba <= 0 || arroba == len(email)-1 {
		return false
	}
	return strings.Contains(email[arroba+1:], ".")
}
