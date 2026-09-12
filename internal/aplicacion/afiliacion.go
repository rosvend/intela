package aplicacion

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

const (
	prefijoAfiliacion  = "afil-"
	prefijoTitular     = "tit-"
	tamanoMaxDocumento = 5 << 20
	largoMinClave      = 8
	largoMaxClave      = 72
)

// Admision es el alta de un titular: la solicitud que lo deja pendiente
// y la decision del Consejo que lo admite al padron.
//
// El rol administrador representa aqui al Consejo Directivo (`RS 5.2`):
// el andamiaje no tiene todavia un rol propio, y anadir uno exigiria
// cambiar el CHECK de usuarios.rol, que es de otro issue.
type Admision struct {
	Solicitudes RepositorioAdmision
	Objetos     AlmacenObjetos
	IDs         GeneradorTokens
	Claves      Hasher
}

// Solicitar valida la solicitud, guarda RUT y certificacion bancaria en
// el almacen de objetos, y deja al titular en estado pendiente.
//
// La clave se hashea aqui y se persiste con la solicitud: al admitir, el
// adaptador crea la fila de `usuarios` con ese hash. Sin ella el padron
// tendria un titular que no puede entrar.
func (s Admision) Solicitar(ctx context.Context, in SolicitudAfiliacion) (vista AfiliacionVista, err error) {
	if err := documentosDePago(in.RUT, in.CertBancaria); err != nil {
		return AfiliacionVista{}, err
	}
	if in.PerteneceOtraSGC {
		if len(in.Renuncia) == 0 {
			return AfiliacionVista{}, afiliacion.ErrExclusividad
		}
		if err := documentoAceptado(in.Renuncia); err != nil {
			return AfiliacionVista{}, err
		}
	} else if len(in.Renuncia) > 0 {
		if err := documentoAceptado(in.Renuncia); err != nil {
			return AfiliacionVista{}, err
		}
	}

	claveHash, err := s.hashDeClave(in.Clave)
	if err != nil {
		return AfiliacionVista{}, err
	}

	token, err := s.IDs.Generar()
	if err != nil {
		return AfiliacionVista{}, fmt.Errorf("generar id de solicitud: %w", err)
	}
	id := prefijoAfiliacion + token

	a := afiliacion.Afiliado{
		ID:                 id,
		Nombre:             strings.TrimSpace(in.Nombre),
		Email:              strings.ToLower(strings.TrimSpace(in.Email)),
		DocumentoIdentidad: strings.TrimSpace(in.DocumentoIdentidad),
		IPI:                strings.TrimSpace(in.IPI),
		Subtipo:            afiliacion.Subtipo(strings.TrimSpace(in.Subtipo)),
		Estado:             afiliacion.EstadoPendiente,
		PersonaNatural:     true, // este asistente da de alta escritores (R-01)
		PerteneceOtraSGC:   in.PerteneceOtraSGC,
		ClaveRUT:           claveDocumento(id, "rut"),
		ClaveCertBancaria:  claveDocumento(id, "banco"),
	}
	if len(in.Renuncia) > 0 {
		a.ClaveRenuncia = claveDocumento(id, "renuncia")
	}

	if err := a.ValidarSolicitud(); err != nil {
		return AfiliacionVista{}, err
	}

	var escritos []string
	defer func() {
		if err == nil {
			return
		}
		for _, clave := range escritos {
			_ = s.Objetos.Borrar(ctx, clave)
		}
	}()

	if err = s.Objetos.Poner(ctx, a.ClaveRUT, in.RUT); err != nil {
		return AfiliacionVista{}, fmt.Errorf("guardar rut: %w", err)
	}
	escritos = append(escritos, a.ClaveRUT)

	if err = s.Objetos.Poner(ctx, a.ClaveCertBancaria, in.CertBancaria); err != nil {
		return AfiliacionVista{}, fmt.Errorf("guardar certificacion bancaria: %w", err)
	}
	escritos = append(escritos, a.ClaveCertBancaria)

	if a.ClaveRenuncia != "" {
		if err = s.Objetos.Poner(ctx, a.ClaveRenuncia, in.Renuncia); err != nil {
			return AfiliacionVista{}, fmt.Errorf("guardar renuncia: %w", err)
		}
		escritos = append(escritos, a.ClaveRenuncia)
	}

	if err = s.Solicitudes.GuardarSolicitud(ctx, a, claveHash); err != nil {
		return AfiliacionVista{}, fmt.Errorf("guardar solicitud: %w", err)
	}
	return vistaDe(a), nil
}

// CompletarIPI rellena el identificador que el alta permite omitir.
//
// Va sin actor: quien solicita todavia no tiene sesion, y el identificador
// de la solicitud es un token opaco de 256 bits. Es el "despues" que el
// asistente promete, y sin el una persona natural queda pendiente para
// siempre porque Admitir exige IPI.
func (s Admision) CompletarIPI(ctx context.Context, id, ipi string) (AfiliacionVista, error) {
	actual, err := s.Solicitudes.SolicitudPorID(ctx, id)
	if err != nil {
		return AfiliacionVista{}, err
	}
	actualizada, err := actual.CompletarIPI(ipi)
	if err != nil {
		return AfiliacionVista{}, err
	}
	if err := s.Solicitudes.ActualizarPendiente(ctx, actualizada); err != nil {
		return AfiliacionVista{}, fmt.Errorf("completar ipi: %w", err)
	}
	return vistaDe(actualizada), nil
}

// Aprobar admite una solicitud pendiente. Solo el administrador (Consejo
// Directivo) puede hacerlo. Crea la fila del padron y, en la misma
// transaccion del adaptador, la cuenta con la que el titular entra.
// A partir de ahi el subtipo gobierna el anticipo (`R-30`).
func (s Admision) Aprobar(ctx context.Context, actor Usuario, id string) (AfiliacionVista, error) {
	if actor.Rol != RolAdministrador {
		return AfiliacionVista{}, ErrNoAutorizado
	}

	actual, err := s.Solicitudes.SolicitudPorID(ctx, id)
	if err != nil {
		return AfiliacionVista{}, err
	}

	admitido, err := actual.Admitir(prefijoTitular + strings.TrimPrefix(actual.ID, prefijoAfiliacion))
	if err != nil {
		return AfiliacionVista{}, err
	}

	if err := s.Solicitudes.AdmitirSolicitud(ctx, admitido); err != nil {
		return AfiliacionVista{}, fmt.Errorf("admitir solicitud: %w", err)
	}
	return vistaDe(admitido), nil
}

// Rechazar cierra una solicitud pendiente. Solo el Consejo. Libera el
// correo (y el IPI) para que el aspirante pueda volver a presentarse.
func (s Admision) Rechazar(ctx context.Context, actor Usuario, id string) (AfiliacionVista, error) {
	if actor.Rol != RolAdministrador {
		return AfiliacionVista{}, ErrNoAutorizado
	}

	actual, err := s.Solicitudes.SolicitudPorID(ctx, id)
	if err != nil {
		return AfiliacionVista{}, err
	}
	rechazada, err := actual.Rechazar()
	if err != nil {
		return AfiliacionVista{}, err
	}
	if err := s.Solicitudes.ActualizarPendiente(ctx, rechazada); err != nil {
		return AfiliacionVista{}, fmt.Errorf("rechazar solicitud: %w", err)
	}
	return vistaDe(rechazada), nil
}

func (s Admision) hashDeClave(clave string) (string, error) {
	if len(clave) < largoMinClave || len(clave) > largoMaxClave {
		return "", ErrClaveInvalida
	}
	if s.Claves == nil {
		return "", fmt.Errorf("%w: falta el Hasher", ErrClaveInvalida)
	}
	hash, err := s.Claves.Hash(clave)
	if err != nil {
		return "", fmt.Errorf("hashear clave: %w", err)
	}
	return hash, nil
}

func vistaDe(a afiliacion.Afiliado) AfiliacionVista {
	return AfiliacionVista{
		ID:                 a.ID,
		Nombre:             a.Nombre,
		Email:              a.Email,
		DocumentoIdentidad: a.DocumentoIdentidad,
		IPI:                a.IPI,
		Subtipo:            string(a.Subtipo),
		Estado:             string(a.Estado),
		ElegibleAnticipo:   a.ElegibleAnticipo(),
		TieneRUT:           a.ClaveRUT != "",
		TieneCertBancaria:  a.ClaveCertBancaria != "",
		TieneRenuncia:      a.ClaveRenuncia != "",
		TitularID:          a.TitularID,
	}
}

func claveDocumento(id, clase string) string {
	return "afiliaciones/" + id + "/" + clase
}

func documentosDePago(rut, banco []byte) error {
	if len(rut) == 0 || len(banco) == 0 {
		return afiliacion.ErrDocumentosPago
	}
	if err := documentoAceptado(rut); err != nil {
		return err
	}
	if err := documentoAceptado(banco); err != nil {
		return err
	}
	return nil
}

func documentoAceptado(b []byte) error {
	if len(b) == 0 || len(b) > tamanoMaxDocumento {
		return ErrDocumentoInvalido
	}
	if bytes.HasPrefix(b, []byte("%PDF")) {
		return nil
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return nil
	}
	if bytes.HasPrefix(b, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return nil
	}
	return ErrDocumentoInvalido
}
