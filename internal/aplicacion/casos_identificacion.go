package aplicacion

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// Estados de un caso de la cola manual. "descartado" llega con la escritura de #175.
const (
	EstadoCasoPendiente = "pendiente"
	EstadoCasoAsignado  = "asignado"
)

// escalonesDeEstado es la unica traduccion estado <-> escalon; el adaptador no la repite.
var escalonesDeEstado = map[string]string{
	EstadoCasoPendiente: identificacion.EscalonONI,
	EstadoCasoAsignado:  identificacion.EscalonManual,
}

// FiltroCasos es lo que pide quien lee la cola; un campo vacio no filtra.
type FiltroCasos struct {
	Estado  string
	Fuente  string
	Periodo string
	Paginacion
}

// ConsultaCasos es el filtro ya traducido a escalones para el repositorio.
type ConsultaCasos struct {
	Escalones []string
	Fuente    string
	Periodo   string
	Paginacion
}

// CandidatoCaso es una obra de la banda ambigua, en el orden de candidatos_match.
type CandidatoCaso struct {
	ObraID           string
	Titulo           string
	Anio             int
	Genero           string
	Puntaje          decimal.Decimal
	TituloConsultado string
}

// ObraAsignada es la obra que una persona le dio al caso.
type ObraAsignada struct {
	ID     string
	Titulo string
}

// Resolutor es quien resolvio el caso, con su nombre para mostrar.
type Resolutor struct {
	ID     string
	Nombre string
}

// CasoIdentificacion es un uso que la cascada no resolvio. Sin importes ni medidas de ponderacion (ADR 0007, R-18).
type CasoIdentificacion struct {
	UsoID               string
	Titulo              string
	TituloOriginal      string
	Fuente              string
	Modalidad           string
	ReporteID           string
	Periodo             string
	IDsFuente           string
	Evidencia           string
	Escalon             string
	Estado              string
	Candidatos          []CandidatoCaso
	ObraAsignada        *ObraAsignada
	ResueltoPor         *Resolutor
	ResueltoEn          *time.Time
	ReporteCreado       time.Time
	UltimaActualizacion time.Time
}

// PaginaCasos es una pagina de la cola y el total de pendientes bajo los mismos filtros de fuente y periodo.
type PaginaCasos struct {
	Casos      []CasoIdentificacion
	Pendientes int
}

// CasosIdentificacion sirve la bandeja de identificacion manual (#39).
type CasosIdentificacion struct {
	Repo RepositorioCasosIdentificacion
}

// Listar valida el filtro, lo traduce a escalones y completa estado y ultima actualizacion de cada caso.
func (c CasosIdentificacion) Listar(ctx context.Context, f FiltroCasos) (PaginaCasos, error) {
	q, err := consultaDe(f)
	if err != nil {
		return PaginaCasos{}, err
	}
	pag, err := c.Repo.ListarCasosIdentificacion(ctx, q)
	if err != nil {
		return PaginaCasos{}, err
	}
	casos := make([]CasoIdentificacion, 0, len(pag.Casos))
	for _, caso := range pag.Casos {
		completo, err := completarCaso(caso)
		if err != nil {
			return PaginaCasos{}, err
		}
		casos = append(casos, completo)
	}
	pag.Casos = casos
	return pag, nil
}

func consultaDe(f FiltroCasos) (ConsultaCasos, error) {
	q := ConsultaCasos{
		Fuente:     strings.TrimSpace(f.Fuente),
		Periodo:    strings.TrimSpace(f.Periodo),
		Paginacion: f.ConDefecto(),
	}
	if q.Periodo != "" && !recaudo.PeriodoValido(q.Periodo) {
		return ConsultaCasos{}, fmt.Errorf("%w: periodo %q, se esperaba AAAA o AAAA-MM con un mes entre 01 y 12", ErrFiltroCasosInvalido, q.Periodo)
	}
	estado := strings.TrimSpace(f.Estado)
	if estado == "" {
		q.Escalones = []string{identificacion.EscalonONI, identificacion.EscalonManual}
		return q, nil
	}
	escalon, ok := escalonesDeEstado[estado]
	if !ok {
		return ConsultaCasos{}, fmt.Errorf("%w: estado %q, se esperaba %q o %q", ErrFiltroCasosInvalido, estado, EstadoCasoPendiente, EstadoCasoAsignado)
	}
	q.Escalones = []string{escalon}
	return q, nil
}

func completarCaso(caso CasoIdentificacion) (CasoIdentificacion, error) {
	switch caso.Escalon {
	case identificacion.EscalonONI:
		caso.Estado = EstadoCasoPendiente
		caso.UltimaActualizacion = caso.ReporteCreado
	case identificacion.EscalonManual:
		if caso.ResueltoEn == nil {
			return CasoIdentificacion{}, fmt.Errorf("caso %s: escalon manual sin resuelto_en", caso.UsoID)
		}
		caso.Estado = EstadoCasoAsignado
		caso.UltimaActualizacion = *caso.ResueltoEn
	default:
		return CasoIdentificacion{}, fmt.Errorf("caso %s: escalon %q no es un caso de la cola manual", caso.UsoID, caso.Escalon)
	}
	if caso.Candidatos == nil {
		caso.Candidatos = []CandidatoCaso{}
	}
	return caso, nil
}
