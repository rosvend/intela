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
}

// Solicitar valida la solicitud, guarda RUT y certificacion bancaria en
// el almacen de objetos, y deja al titular en estado pendiente.
func (s Admision) Solicitar(ctx context.Context, in SolicitudAfiliacion) (AfiliacionVista, error) {
	if err := documentosDePago(in.RUT, in.CertBancaria); err != nil {
		return AfiliacionVista{}, err
	}
	if in.PerteneceOtraSGC {
		if err := documentoAceptado(in.Renuncia); err != nil {
			return AfiliacionVista{}, afiliacion.ErrExclusividad
		}
	} else if len(in.Renuncia) > 0 {
		if err := documentoAceptado(in.Renuncia); err != nil {
			return AfiliacionVista{}, err
		}
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

	if err := s.Objetos.Poner(ctx, a.ClaveRUT, in.RUT); err != nil {
		return AfiliacionVista{}, fmt.Errorf("guardar rut: %w", err)
	}
	if err := s.Objetos.Poner(ctx, a.ClaveCertBancaria, in.CertBancaria); err != nil {
		return AfiliacionVista{}, fmt.Errorf("guardar certificacion bancaria: %w", err)
	}
	if a.ClaveRenuncia != "" {
		if err := s.Objetos.Poner(ctx, a.ClaveRenuncia, in.Renuncia); err != nil {
			return AfiliacionVista{}, fmt.Errorf("guardar renuncia: %w", err)
		}
	}

	if err := s.Solicitudes.GuardarSolicitud(ctx, a); err != nil {
		return AfiliacionVista{}, fmt.Errorf("guardar solicitud: %w", err)
	}
	return vistaDe(a), nil
}

// Aprobar admite una solicitud pendiente. Solo el administrador (Consejo
// Directivo) puede hacerlo. Crea la fila del padron: a partir de ahi el
// subtipo gobierna el anticipo (`R-30`).
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
	if err := documentoAceptado(rut); err != nil {
		return afiliacion.ErrDocumentosPago
	}
	if err := documentoAceptado(banco); err != nil {
		return afiliacion.ErrDocumentosPago
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
