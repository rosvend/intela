package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var todosLosCasos = []string{identificacion.EscalonONI, identificacion.EscalonManual}

// sembrarCasos deja, sobre sembrarIdentificacion, una cola con todos los escalones:
// u-1 oni con dos candidatos, u-2 manual, u-3 oni sin candidatos (rep-1, caracol, 2024),
// u-4 oni (rep-2, netflix, 2025-01) y u-5..u-7 que nunca son casos (excluido, difuso con
// candidatos viejos, alias).
func sembrarCasos(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar casos: %v\n%s", err, sql)
		}
	}
	ejecutar(`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes, creado)
	          VALUES ('rep-2', 'netflix', '2025-01', repeat('b', 64), 'reportes/rep-2.csv', 100, now() + interval '1 hour')`)
	ejecutar(`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
	          VALUES ('revisor-1', 'revisor@redes.test', 'Revisora Uno', 'administrador', $1)`, hashBcrypt)
	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{ID: "u-4", ReporteID: "rep-2", Fuente: "netflix", Titulo: "Serie netflix"})
	for _, id := range []string{"u-5", "u-6", "u-7"} {
		insertarUsoSQL(t, pool, aplicacion.UsoPersistido{ID: id, ReporteID: reporteUno, Fuente: "caracol", Titulo: "Otro " + id})
	}

	ejecutar(`UPDATE usos SET escalon = 'oni', evidencia = 'banda ambigua' WHERE id IN ('u-1', 'u-3', 'u-4')`)
	ejecutar(`INSERT INTO candidatos_match (uso_id, obra_id, puntaje, orden, titulo_consultado) VALUES
	          ('u-1', $1, 0.61, 0, 'titulo emitido'),
	          ('u-1', $2, 0.55, 1, 'titulo original'),
	          ('u-6', $1, 0.90, 0, 'viejo')`, obraIda, obraImdb)
	ejecutar(`UPDATE usos SET escalon = 'manual', obra_id = $1, oni = FALSE,
	                 resuelto_por = 'revisor-1', resuelto_en = '2025-02-03T09:00:00Z'
	           WHERE id = 'u-2'`, obraImdb)
	ejecutar(`UPDATE usos SET escalon = 'excluido', oni = FALSE WHERE id = 'u-5'`)
	ejecutar(`UPDATE usos SET escalon = 'difuso', obra_id = $1, oni = FALSE, puntaje = 0.9 WHERE id = 'u-6'`, obraIda)
	ejecutar(`UPDATE usos SET escalon = 'alias', obra_id = $1, oni = FALSE, puntaje = 1 WHERE id = 'u-7'`, obraImdb)
	return s, pool
}

func idsDeCasos(p aplicacion.PaginaCasos) []string {
	ids := make([]string, 0, len(p.Casos))
	for _, c := range p.Casos {
		ids = append(ids, c.UsoID)
	}
	return ids
}

func mismosIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func listarCasos(t *testing.T, s *Store, q aplicacion.ConsultaCasos) aplicacion.PaginaCasos {
	t.Helper()
	if q.Limite == 0 {
		q.Limite = aplicacion.LimiteObrasPorDefecto
	}
	p, err := s.ListarCasosIdentificacion(t.Context(), q)
	if err != nil {
		t.Fatalf("ListarCasosIdentificacion: %v", err)
	}
	return p
}

func TestCasosListaSoloONIYManualEnOrdenDeLlegada(t *testing.T) {
	s, _ := sembrarCasos(t)
	p := listarCasos(t, s, aplicacion.ConsultaCasos{Escalones: todosLosCasos})
	if quiere := []string{"u-1", "u-2", "u-3", "u-4"}; !mismosIDs(idsDeCasos(p), quiere) {
		t.Fatalf("ids = %v, quiere %v (excluido, difuso y alias nunca son casos)", idsDeCasos(p), quiere)
	}
	if p.Pendientes != 3 {
		t.Fatalf("pendientes = %d, quiere 3", p.Pendientes)
	}
}

func TestCasoPendienteTraeEntradaEvidenciaYCandidatosEnOrden(t *testing.T) {
	s, _ := sembrarCasos(t)
	c := listarCasos(t, s, aplicacion.ConsultaCasos{Escalones: todosLosCasos}).Casos[0]
	if c.UsoID != "u-1" || c.Escalon != identificacion.EscalonONI || c.Fuente != "caracol" ||
		c.ReporteID != reporteUno || c.Periodo != "2024" || c.Evidencia != "banda ambigua" || c.Modalidad != "tv" {
		t.Fatalf("caso = %+v", c)
	}
	if c.ReporteCreado.IsZero() || c.ResueltoEn != nil || c.ResueltoPor != nil || c.ObraAsignada != nil {
		t.Fatalf("un pendiente no tiene resolucion: %+v", c)
	}
	if len(c.Candidatos) != 2 {
		t.Fatalf("candidatos = %+v", c.Candidatos)
	}
	primero, segundo := c.Candidatos[0], c.Candidatos[1]
	if primero.ObraID != obraIda || primero.Puntaje.String() != "0.61" || primero.TituloConsultado != "titulo emitido" {
		t.Fatalf("primer candidato = %+v", primero)
	}
	if primero.Titulo != "Obra de catalogo "+obraIda || primero.Anio != 2000 || primero.Genero != "Drama" {
		t.Fatalf("el candidato no trae la ficha del catalogo: %+v", primero)
	}
	if segundo.ObraID != obraImdb {
		t.Fatalf("segundo candidato = %+v", segundo)
	}
}

func TestCasoBajoLaBandaTraeCandidatosVacios(t *testing.T) {
	s, _ := sembrarCasos(t)
	c := listarCasos(t, s, aplicacion.ConsultaCasos{Escalones: todosLosCasos}).Casos[2]
	if c.UsoID != "u-3" || len(c.Candidatos) != 0 {
		t.Fatalf("caso = %+v", c)
	}
}

func TestCasoManualTraeQuienCuandoYObra(t *testing.T) {
	s, _ := sembrarCasos(t)
	c := listarCasos(t, s, aplicacion.ConsultaCasos{Escalones: todosLosCasos}).Casos[1]
	if c.UsoID != "u-2" || c.Escalon != identificacion.EscalonManual {
		t.Fatalf("caso = %+v", c)
	}
	if c.ResueltoPor == nil || c.ResueltoPor.ID != "revisor-1" || c.ResueltoPor.Nombre != "Revisora Uno" {
		t.Fatalf("resuelto_por = %+v", c.ResueltoPor)
	}
	if c.ResueltoEn == nil || c.ResueltoEn.UTC().Format("2006-01-02T15:04") != "2025-02-03T09:00" {
		t.Fatalf("resuelto_en = %v", c.ResueltoEn)
	}
	if c.ObraAsignada == nil || c.ObraAsignada.ID != obraImdb || c.ObraAsignada.Titulo != "Obra de catalogo "+obraImdb {
		t.Fatalf("obra asignada = %+v", c.ObraAsignada)
	}
}

func TestCasosFiltranPorEscalonFuenteYPeriodo(t *testing.T) {
	s, _ := sembrarCasos(t)
	casos := []struct {
		nombre     string
		q          aplicacion.ConsultaCasos
		ids        []string
		pendientes int
	}{
		{"solo pendientes", aplicacion.ConsultaCasos{Escalones: []string{identificacion.EscalonONI}}, []string{"u-1", "u-3", "u-4"}, 3},
		{"solo asignados", aplicacion.ConsultaCasos{Escalones: []string{identificacion.EscalonManual}}, []string{"u-2"}, 3},
		{"fuente", aplicacion.ConsultaCasos{Escalones: todosLosCasos, Fuente: "netflix"}, []string{"u-4"}, 1},
		{"periodo", aplicacion.ConsultaCasos{Escalones: todosLosCasos, Periodo: "2024"}, []string{"u-1", "u-2", "u-3"}, 2},
		{"nada", aplicacion.ConsultaCasos{Escalones: todosLosCasos, Periodo: "2030"}, []string{}, 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p := listarCasos(t, s, c.q)
			if !mismosIDs(idsDeCasos(p), c.ids) || p.Pendientes != c.pendientes {
				t.Fatalf("ids = %v pendientes = %d, quiere %v %d", idsDeCasos(p), p.Pendientes, c.ids, c.pendientes)
			}
		})
	}
}

func TestCasosPaginanSinPerderElConteo(t *testing.T) {
	s, _ := sembrarCasos(t)
	p := listarCasos(t, s, aplicacion.ConsultaCasos{
		Escalones:  todosLosCasos,
		Paginacion: aplicacion.Paginacion{Limite: 2, Desplazamiento: 1},
	})
	if quiere := []string{"u-2", "u-3"}; !mismosIDs(idsDeCasos(p), quiere) || p.Pendientes != 3 {
		t.Fatalf("ids = %v pendientes = %d", idsDeCasos(p), p.Pendientes)
	}
}
