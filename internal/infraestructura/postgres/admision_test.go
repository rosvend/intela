package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

func solicitudDePrueba(id string) afiliacion.Afiliado {
	return afiliacion.Afiliado{
		ID:                 id,
		Nombre:             "Nueva Escritora",
		Email:              "nueva@redes.co",
		DocumentoIdentidad: "99887766",
		IPI:                "IPI-00000999",
		Subtipo:            afiliacion.Socio,
		Estado:             afiliacion.EstadoPendiente,
		PersonaNatural:     true,
		ClaveRUT:           "afiliaciones/" + id + "/rut",
		ClaveCertBancaria:  "afiliaciones/" + id + "/banco",
	}
}

func TestGuardarYLeerSolicitud(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	a := solicitudDePrueba("afil-nueva")

	if err := s.GuardarSolicitud(ctx, a); err != nil {
		t.Fatalf("GuardarSolicitud: %v", err)
	}

	got, err := s.SolicitudPorID(ctx, a.ID)
	if err != nil {
		t.Fatalf("SolicitudPorID: %v", err)
	}
	if got.Estado != afiliacion.EstadoPendiente {
		t.Fatalf("Estado = %q", got.Estado)
	}
	if got.Email != a.Email || got.Subtipo != a.Subtipo {
		t.Fatalf("got = %+v", got)
	}
	if got.ClaveRUT != a.ClaveRUT || got.ClaveCertBancaria != a.ClaveCertBancaria {
		t.Fatal("las claves de documento no sobrevivieron al round-trip")
	}
}

func TestGuardarSolicitudDuplicadaEsConflicto(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	a := solicitudDePrueba("afil-1")
	if err := s.GuardarSolicitud(ctx, a); err != nil {
		t.Fatalf("primera: %v", err)
	}

	otra := a
	otra.ID = "afil-2"
	if err := s.GuardarSolicitud(ctx, otra); !errors.Is(err, aplicacion.ErrConflicto) {
		t.Fatalf("se esperaba ErrConflicto, se obtuvo %v", err)
	}
}

func TestExclusividadSinRenunciaLaBaseLaRechaza(t *testing.T) {
	s, _ := sembrar(t)
	a := solicitudDePrueba("afil-sgc")
	a.PerteneceOtraSGC = true

	err := s.GuardarSolicitud(t.Context(), a)
	if err == nil {
		t.Fatal("el CHECK de exclusividad tenia que rechazar la fila")
	}
}

func TestAdmitirSolicitudCreaElTitularYCambiaEstado(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	a := solicitudDePrueba("afil-ok")
	if err := s.GuardarSolicitud(ctx, a); err != nil {
		t.Fatalf("GuardarSolicitud: %v", err)
	}

	admitida, err := a.Admitir("tit-nueva")
	if err != nil {
		t.Fatalf("Admitir: %v", err)
	}
	if err := s.AdmitirSolicitud(ctx, admitida); err != nil {
		t.Fatalf("AdmitirSolicitud: %v", err)
	}

	got, err := s.SolicitudPorID(ctx, a.ID)
	if err != nil {
		t.Fatalf("SolicitudPorID: %v", err)
	}
	if got.Estado != afiliacion.EstadoAdmitido {
		t.Fatalf("Estado = %q", got.Estado)
	}
	if got.TitularID != "tit-nueva" {
		t.Fatalf("TitularID = %q", got.TitularID)
	}

	var clase string
	if err := s.pool.QueryRow(ctx,
		`SELECT clase FROM titulares WHERE id = $1`, "tit-nueva").Scan(&clase); err != nil {
		t.Fatalf("el titular no quedo en el padron: %v", err)
	}
	if clase != "socio" {
		t.Fatalf("clase = %q, el subtipo tiene que llegar al padron", clase)
	}
}

func TestSolicitudInexistenteEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.SolicitudPorID(t.Context(), "afil-nadie")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}
