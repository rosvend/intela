package aplicacion

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Tope de la cola ONI que se lee de una vez.
const (
	LimiteONIPorDefecto = 20
	LimiteONIMaximo     = 100
)

// FiltroONI acota la cola; un campo vacio no filtra.
type FiltroONI struct {
	Periodo string
	Fuente  string
	Limite  int
}

// ONIPendiente es un uso que la cascada no identifico (escalon ONI). Sin importes (R-18).
type ONIPendiente struct {
	UsoID          string
	Titulo         string
	TituloOriginal string
	Fuente         string
	Modalidad      string
	Periodo        string
	IDsFuente      string
	ReporteID      string
	DetectadaEn    time.Time
	Candidatos     int
}

// ColaONI es una pagina de la cola y el total pendiente bajo los mismos filtros.
type ColaONI struct {
	Pendientes int
	Obras      []ONIPendiente
}

// ConsultarONI lee la cola viva de obras no identificadas (#69); misma matriz que GET /identificacion/casos.
type ConsultarONI struct {
	Casos RepositorioCasosIdentificacion
}

// Ejecutar autoriza antes de leer: solo el administrador ve la cola entera de la sociedad.
func (c ConsultarONI) Ejecutar(ctx context.Context, actor Usuario, f FiltroONI) (ColaONI, error) {
	if actor.Rol != RolAdministrador {
		return ColaONI{}, ErrNoAutorizado
	}
	periodo := strings.TrimSpace(f.Periodo)
	if periodo != "" && !periodoRe.MatchString(periodo) {
		return ColaONI{}, ErrPeriodoInvalido
	}
	if f.Limite < 0 || f.Limite > LimiteONIMaximo {
		return ColaONI{}, fmt.Errorf("%w: el limite va de 1 a %d", ErrFiltroInvalido, LimiteONIMaximo)
	}
	if f.Limite == 0 {
		f.Limite = LimiteONIPorDefecto
	}
	pag, err := c.Casos.ListarCasosIdentificacion(ctx, ConsultaCasos{
		Escalones:  []string{identificacion.EscalonONI},
		Fuente:     strings.TrimSpace(f.Fuente),
		Periodo:    periodo,
		Paginacion: Paginacion{Limite: f.Limite},
	})
	if err != nil {
		return ColaONI{}, fmt.Errorf("listar ONI: %w", err)
	}
	cola := ColaONI{Pendientes: pag.Pendientes, Obras: make([]ONIPendiente, 0, len(pag.Casos))}
	for _, caso := range pag.Casos {
		cola.Obras = append(cola.Obras, ONIPendiente{
			UsoID: caso.UsoID, Titulo: caso.Titulo, TituloOriginal: caso.TituloOriginal, Fuente: caso.Fuente,
			Modalidad: caso.Modalidad, Periodo: caso.Periodo, IDsFuente: caso.IDsFuente, ReporteID: caso.ReporteID,
			DetectadaEn: caso.ReporteCreado, Candidatos: len(caso.Candidatos),
		})
	}
	return cola, nil
}
