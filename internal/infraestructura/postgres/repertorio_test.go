package postgres

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

func TestObraPorID(t *testing.T) {
	s, _ := sembrar(t)

	o, err := s.ObraPorID(t.Context(), obraCompleta)
	if err != nil {
		t.Fatalf("ObraPorID: %v", err)
	}
	if o.Titulo != "La Casa de las Dos Palmas" {
		t.Fatalf("Titulo = %q", o.Titulo)
	}
	if o.IDA != "IDA-1" || o.EIDR != "EIDR-1" || o.IMDB != "tt0001" {
		t.Fatalf("identificadores mal escaneados: %+v", o)
	}
	if o.Tipo != "serie" {
		t.Fatalf("Tipo = %q, se esperaba \"serie\"", o.Tipo)
	}
	if o.EstadoDecl != "completa" {
		t.Fatalf("EstadoDecl = %q, se esperaba \"completa\"", o.EstadoDecl)
	}
}

func TestObraPorIDNoEncontrada(t *testing.T) {
	s, _ := sembrar(t)

	_, err := s.ObraPorID(t.Context(), "obra-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// EstadoDecl no sale de ninguna columna: obras no la tiene. Sale de
// repertorio.Declaracion.Estado(), que es donde vive R-04 entero.
//
// Las tres obras incompletas fallan por motivos DISTINTOS -suma 60, cero
// partes, y suma 100 con un IPI vacio- y las tres tienen que dar "incompleta".
// Un SUM(porcentaje) = 100 en SQL daria la tercera por completa.
func TestListarObrasDerivaElEstadoDeLaDeclaracion(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.ListarObras(t.Context())
	if err != nil {
		t.Fatalf("ListarObras: %v", err)
	}
	if len(obras) != 4 {
		t.Fatalf("se esperaban 4 obras, llegaron %d", len(obras))
	}

	estados := map[string]string{}
	for _, o := range obras {
		estados[o.ID] = o.EstadoDecl
	}

	esperado := map[string]string{
		obraCompleta:       "completa",
		obraIncompleta:     "incompleta", // suma 60
		obraSinDeclaracion: "incompleta", // sin partes
		obraSinIPI:         "incompleta", // suma 100 pero falta un IPI
	}
	for id, quiero := range esperado {
		if estados[id] != quiero {
			t.Fatalf("obra %q: EstadoDecl = %q, se esperaba %q", id, estados[id], quiero)
		}
	}

	// ADR 0005: una lista sin orden explicito no es reproducible.
	for i := 1; i < len(obras); i++ {
		if obras[i-1].ID > obras[i].ID {
			t.Fatalf("las obras no vienen ordenadas por id: %q antes que %q",
				obras[i-1].ID, obras[i].ID)
		}
	}
}

// Una tabla vacia devuelve una lista vacia y NINGUN error. "No hay filas" solo
// es ErrNoEncontrado cuando se pidio una fila concreta.
func TestListarObrasSinObrasNoEsError(t *testing.T) {
	s, pool := sembrar(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM declaraciones; DELETE FROM obras`); err != nil {
		t.Fatalf("vaciar: %v", err)
	}

	obras, err := s.ListarObras(t.Context())
	if err != nil {
		t.Fatalf("una tabla vacia no es un error: %v", err)
	}
	if len(obras) != 0 {
		t.Fatalf("se esperaba lista vacia, llegaron %d", len(obras))
	}
}

func TestDeclaraciones(t *testing.T) {
	s, _ := sembrar(t)

	decls, err := s.Declaraciones(t.Context())
	if err != nil {
		t.Fatalf("Declaraciones: %v", err)
	}

	// obraSinDeclaracion no tiene partes, asi que no tiene entrada. Pedirla
	// devuelve la Declaracion cero, cuyo Estado() es "incompleta", que es la
	// respuesta correcta bajo R-04.
	if _, hay := decls[obraSinDeclaracion]; hay {
		t.Fatal("una obra sin partes no deberia tener entrada en el mapa")
	}
	if e := decls[obraSinDeclaracion].Estado(); e != "incompleta" {
		t.Fatalf("la Declaracion cero da Estado() = %q, se esperaba \"incompleta\"", e)
	}

	d := decls[obraCompleta]
	if d.ObraID != obraCompleta {
		t.Fatalf("ObraID = %q, se esperaba %q", d.ObraID, obraCompleta)
	}
	if !d.Completa() {
		t.Fatalf("60 + 40 con IPI tiene que ser completa: %+v", d.Partes)
	}
	if len(d.Partes) != 2 {
		t.Fatalf("se esperaban 2 partes, llegaron %d", len(d.Partes))
	}

	// Equal y no == ni String(): NUMERIC(8,4) llega como "60.0000" y
	// decimal compara exponentes en ==.
	if !d.Partes[0].Porcentaje.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("porcentaje = %s, se esperaba 60", d.Partes[0].Porcentaje)
	}
	if d.Partes[0].TitularID != titularAna || d.Partes[0].IPI != "IPI-00000001" {
		t.Fatalf("parte mal escaneada: %+v", d.Partes[0])
	}
}

// El caso que da nombre al invariante: una obra con declaracion_incompleta
// sobrevive el viaje de ida y vuelta y sigue diciendo "incompleta". No se
// reparte nada de ella, se retiene el total en reserva (R-04, RD 13.1.3).
func TestDeclaracionDeObraIncompletaRoundTrip(t *testing.T) {
	s, _ := sembrar(t)

	d, err := s.DeclaracionDeObra(t.Context(), obraIncompleta)
	if err != nil {
		t.Fatalf("DeclaracionDeObra: %v", err)
	}
	if d.Estado() != "incompleta" {
		t.Fatalf("Estado() = %q, se esperaba \"incompleta\"", d.Estado())
	}
	if len(d.Partes) != 1 {
		t.Fatalf("se esperaba 1 parte, llegaron %d", len(d.Partes))
	}
	if !d.Partes[0].Porcentaje.Equal(decimal.RequireFromString("60")) {
		t.Fatalf("porcentaje = %s, se esperaba 60", d.Partes[0].Porcentaje)
	}
}

func TestDeclaracionDeObraCompleta(t *testing.T) {
	s, _ := sembrar(t)

	d, err := s.DeclaracionDeObra(t.Context(), obraCompleta)
	if err != nil {
		t.Fatalf("DeclaracionDeObra: %v", err)
	}
	if !d.Completa() {
		t.Fatalf("se esperaba completa: %+v", d.Partes)
	}
	// Orden estable: dos corridas de la misma obra tienen que dar la misma
	// lista (ADR 0005).
	for i := 1; i < len(d.Partes); i++ {
		if d.Partes[i-1].TitularID > d.Partes[i].TitularID {
			t.Fatal("las partes no vienen ordenadas por titular_id")
		}
	}
}

// La distincion que importa: una obra QUE EXISTE sin partes es
// declaracion_incompleta -un estado valido del modelo-, no un 404. Solo una
// obra que no existe es ErrNoEncontrado.
func TestDeclaracionDeObraDistingueSinPartesDeObraInexistente(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	t.Run("obra sin partes es incompleta, no un error", func(t *testing.T) {
		d, err := s.DeclaracionDeObra(ctx, obraSinDeclaracion)
		if err != nil {
			t.Fatalf("una obra sin declaracion no es un error: %v", err)
		}
		if d.ObraID != obraSinDeclaracion {
			t.Fatalf("ObraID = %q, se esperaba %q", d.ObraID, obraSinDeclaracion)
		}
		if len(d.Partes) != 0 {
			t.Fatalf("se esperaban 0 partes, llegaron %d", len(d.Partes))
		}
		if d.Estado() != "incompleta" {
			t.Fatalf("Estado() = %q, se esperaba \"incompleta\"", d.Estado())
		}
	})

	t.Run("obra inexistente es ErrNoEncontrado", func(t *testing.T) {
		_, err := s.DeclaracionDeObra(ctx, "obra-que-no-existe")
		if !errors.Is(err, aplicacion.ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})
}

// El adaptador entrega el tipo del dominio, no una copia suya: quien lo reciba
// tiene el invariante disponible sin volver a implementarlo.
func TestDeclaracionDeObraDevuelveElTipoDelDominio(t *testing.T) {
	s, _ := sembrar(t)

	var d repertorio.Declaracion
	d, err := s.DeclaracionDeObra(t.Context(), obraSinIPI)
	if err != nil {
		t.Fatalf("DeclaracionDeObra: %v", err)
	}
	// Suma 100 exacto, pero una parte no trae IPI.
	if d.Completa() {
		t.Fatal("una parte sin IPI no puede dar una declaracion completa")
	}
}
