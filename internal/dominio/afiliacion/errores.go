package afiliacion

import "errors"

// Errores que el caso de uso distingue y el adaptador traduce a HTTP.
//
// El mensaje nombra la regla porque es lo que ve quien se queda fuera, y
// porque un "solicitud invalida" a secas no le dice que documento le falta.
var (
	// ErrNombreObligatorio: la solicitud no identifica a nadie.
	ErrNombreObligatorio = errors.New("el nombre es obligatorio")

	// ErrEmailInvalido: sin correo no hay forma de comunicar la decision
	// del Consejo (`RS 5.2.c`).
	ErrEmailInvalido = errors.New("el correo no es valido")

	// ErrDocumentoObligatorio: cedula o pasaporte, `RS 5.2` / `RS 5.3`.
	ErrDocumentoObligatorio = errors.New("el documento de identidad es obligatorio")

	// ErrSubtipoInvalido: el padron solo conoce Socio y Titular Administrado.
	ErrSubtipoInvalido = errors.New("el subtipo tiene que ser socio o administrado")

	// ErrDocumentosPago: RUT y certificacion bancaria, `R-12` / `RD 13.1.6`.
	// Su ausencia bloquea el cobro; aqui bloquea la solicitud porque el
	// asistente existe precisamente para no dejarlos para despues.
	ErrDocumentosPago = errors.New("r-12: el rut actualizado y la certificacion bancaria son obligatorios")

	// ErrExclusividad: pertenece a otra SGC del mismo genero y no hay
	// renuncia adjunta. `R-28`, `RS 4.1`, Decision Andina 351 Art. 45 k.
	ErrExclusividad = errors.New("r-28: no se acepta como afiliado a quien pertenezca a otra sociedad de gestion colectiva del mismo genero sin renuncia previa y expresa")

	// ErrEstadoInvalido: se intento admitir o rechazar una solicitud que
	// no esta pendiente.
	ErrEstadoInvalido = errors.New("la solicitud no esta pendiente de admision")

	// ErrIPIObligatorio: una persona natural entra al padron con IPI
	// (`RD 3`). Se permite omitirlo al solicitar; no al admitir.
	ErrIPIObligatorio = errors.New("el ipi es obligatorio para admitir a una persona natural")
)
