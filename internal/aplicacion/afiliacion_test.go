package aplicacion

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

func pdfPrueba() []byte {
	return []byte("%PDF-1.4\n%intela-prueba\n")
}

type repoAdmisionMemoria struct {
	porID map[string]afiliacion.Afiliado
	err   error
}

func nuevoRepoAdmision() *repoAdmisionMemoria {
	return &repoAdmisionMemoria{porID: map[string]afiliacion.Afiliado{}}
}

func (r *repoAdmisionMemoria) GuardarSolicitud(_ context.Context, a afiliacion.Afiliado) error {
	if r.err != nil {
		return r.err
	}
	r.porID[a.ID] = a
	return nil
}

func (r *repoAdmisionMemoria) SolicitudPorID(_ context.Context, id string) (afiliacion.Afiliado, error) {
	if r.err != nil {
		return afiliacion.Afiliado{}, r.err
	}
	a, hay := r.porID[id]
	if !hay {
		return afiliacion.Afiliado{}, ErrNoEncontrado
	}
	return a, nil
}

func (r *repoAdmisionMemoria) AdmitirSolicitud(_ context.Context, a afiliacion.Afiliado) error {
	if r.err != nil {
		return r.err
	}
	r.porID[a.ID] = a
	return nil
}

type objetosMemoria struct {
	guardados map[string][]byte
}

func nuevosObjetos() *objetosMemoria {
	return &objetosMemoria{guardados: map[string][]byte{}}
}

func (o *objetosMemoria) Poner(_ context.Context, clave string, datos []byte) error {
	o.guardados[clave] = bytes.Clone(datos)
	return nil
}

func (o *objetosMemoria) Obtener(_ context.Context, clave string) ([]byte, error) {
	b, hay := o.guardados[clave]
	if !hay {
		return nil, ErrNoEncontrado
	}
	return b, nil
}

func nuevaAdmision(repo *repoAdmisionMemoria, obj *objetosMemoria) Admision {
	return Admision{
		Solicitudes: repo,
		Objetos:     obj,
		IDs:         tokensFijos{valor: "id-fijo"},
	}
}

func solicitudCompleta() SolicitudAfiliacion {
	return SolicitudAfiliacion{
		Nombre:             "Ana Escritora",
		Email:              "Ana@Redes.Co",
		DocumentoIdentidad: "12345678",
		IPI:                "IPI-00000001",
		Subtipo:            "socio",
		RUT:                pdfPrueba(),
		CertBancaria:       pdfPrueba(),
	}
}

func TestSolicitarDejaLaSolicitudPendienteYGuardaDocumentos(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()

	vista, err := nuevaAdmision(repo, obj).Solicitar(context.Background(), solicitudCompleta())
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}

	if vista.Estado != string(afiliacion.EstadoPendiente) {
		t.Fatalf("Estado = %q, se esperaba pendiente", vista.Estado)
	}
	if vista.ID != "afil-id-fijo" {
		t.Fatalf("ID = %q", vista.ID)
	}
	if vista.Email != "ana@redes.co" {
		t.Fatalf("Email = %q, se esperaba en minusculas", vista.Email)
	}
	if vista.ElegibleAnticipo {
		t.Fatal("un pendiente no pide anticipo")
	}
	if !vista.TieneRUT || !vista.TieneCertBancaria {
		t.Fatal("RUT y certificacion bancaria tienen que quedar registrados")
	}

	if _, hay := obj.guardados["afiliaciones/afil-id-fijo/rut"]; !hay {
		t.Fatal("el RUT no quedo en el almacen")
	}
	if _, hay := obj.guardados["afiliaciones/afil-id-fijo/banco"]; !hay {
		t.Fatal("la certificacion bancaria no quedo en el almacen")
	}

	guardada, hay := repo.porID[vista.ID]
	if !hay {
		t.Fatal("la solicitud no quedo persistida")
	}
	if guardada.Estado != afiliacion.EstadoPendiente {
		t.Fatalf("estado persistido = %q", guardada.Estado)
	}
}

func TestSolicitarExigeRUTYCertificacion(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	s := nuevaAdmision(repo, obj)

	sinRUT := solicitudCompleta()
	sinRUT.RUT = nil
	if _, err := s.Solicitar(context.Background(), sinRUT); !errors.Is(err, afiliacion.ErrDocumentosPago) {
		t.Fatalf("sin RUT: se esperaba ErrDocumentosPago, se obtuvo %v", err)
	}

	sinBanco := solicitudCompleta()
	sinBanco.CertBancaria = nil
	if _, err := s.Solicitar(context.Background(), sinBanco); !errors.Is(err, afiliacion.ErrDocumentosPago) {
		t.Fatalf("sin banco: se esperaba ErrDocumentosPago, se obtuvo %v", err)
	}

	if len(repo.porID) != 0 {
		t.Fatal("no se persiste una solicitud sin documentos de cobro")
	}
	if len(obj.guardados) != 0 {
		t.Fatal("no se guardan documentos de una solicitud invalida")
	}
}

func TestSolicitarBloqueaExclusividadSinRenuncia(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	in := solicitudCompleta()
	in.PerteneceOtraSGC = true

	_, err := nuevaAdmision(repo, obj).Solicitar(context.Background(), in)
	if !errors.Is(err, afiliacion.ErrExclusividad) {
		t.Fatalf("se esperaba ErrExclusividad, se obtuvo %v", err)
	}
	if len(repo.porID) != 0 {
		t.Fatal("un conflicto de exclusividad no deja fila")
	}
}

func TestSolicitarAceptaExclusividadConRenuncia(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	in := solicitudCompleta()
	in.PerteneceOtraSGC = true
	in.Renuncia = pdfPrueba()

	vista, err := nuevaAdmision(repo, obj).Solicitar(context.Background(), in)
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}
	if !vista.TieneRenuncia {
		t.Fatal("la renuncia tiene que quedar registrada")
	}
	if _, hay := obj.guardados["afiliaciones/afil-id-fijo/renuncia"]; !hay {
		t.Fatal("la renuncia no quedo en el almacen")
	}
}

func TestSolicitarPermiteOmitirIPI(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	in := solicitudCompleta()
	in.IPI = ""

	vista, err := nuevaAdmision(repo, obj).Solicitar(context.Background(), in)
	if err != nil {
		t.Fatalf("el IPI es opcional al solicitar: %v", err)
	}
	if vista.IPI != "" {
		t.Fatalf("IPI = %q", vista.IPI)
	}
}

func TestAprobarAdmiteYHabilitaAnticipoAlSocio(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	s := nuevaAdmision(repo, obj)

	vista, err := s.Solicitar(context.Background(), solicitudCompleta())
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}

	actor := Usuario{ID: "usr-admin", Rol: RolAdministrador}
	admitida, err := s.Aprobar(context.Background(), actor, vista.ID)
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	if admitida.Estado != string(afiliacion.EstadoAdmitido) {
		t.Fatalf("Estado = %q", admitida.Estado)
	}
	if !admitida.ElegibleAnticipo {
		t.Fatal("un socio admitido tiene que ser elegible para anticipo")
	}
	if !strings.HasPrefix(admitida.TitularID, "tit-") {
		t.Fatalf("TitularID = %q, se esperaba prefijo tit-", admitida.TitularID)
	}
}

func TestAprobarNoHabilitaAnticipoAlAdministrado(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	s := nuevaAdmision(repo, obj)
	in := solicitudCompleta()
	in.Subtipo = "administrado"

	vista, err := s.Solicitar(context.Background(), in)
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}

	admitida, err := s.Aprobar(context.Background(), Usuario{Rol: RolAdministrador}, vista.ID)
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	if admitida.ElegibleAnticipo {
		t.Fatal("un titular administrado no pide anticipo")
	}
}

func TestAprobarExigeRolDeConsejo(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	s := nuevaAdmision(repo, obj)
	vista, err := s.Solicitar(context.Background(), solicitudCompleta())
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}

	_, err = s.Aprobar(context.Background(), Usuario{Rol: RolTitular}, vista.ID)
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("se esperaba ErrNoAutorizado, se obtuvo %v", err)
	}
}

func TestAprobarSinIPIFallaParaPersonaNatural(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	s := nuevaAdmision(repo, obj)
	in := solicitudCompleta()
	in.IPI = ""

	vista, err := s.Solicitar(context.Background(), in)
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}
	_, err = s.Aprobar(context.Background(), Usuario{Rol: RolAdministrador}, vista.ID)
	if !errors.Is(err, afiliacion.ErrIPIObligatorio) {
		t.Fatalf("se esperaba ErrIPIObligatorio, se obtuvo %v", err)
	}
}

func TestSolicitarRechazaUnAdjuntoQueNoEsDocumento(t *testing.T) {
	repo := nuevoRepoAdmision()
	obj := nuevosObjetos()
	in := solicitudCompleta()
	in.RUT = []byte("esto no es un pdf")

	_, err := nuevaAdmision(repo, obj).Solicitar(context.Background(), in)
	if !errors.Is(err, afiliacion.ErrDocumentosPago) {
		t.Fatalf("se esperaba ErrDocumentosPago, se obtuvo %v", err)
	}
}
