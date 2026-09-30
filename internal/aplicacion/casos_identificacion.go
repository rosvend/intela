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
	EstadoCasoPendiente  = "pendiente"
	EstadoCasoAsignado   = "asignado"
	EstadoCasoDescartado = "descartado"
)

// escalonesDeEstado es la unica traduccion estado <-> escalon; el adaptador no la repite.
var escalonesDeEstado = map[string]string{
	EstadoCasoPendiente:  identificacion.EscalonONI,
	EstadoCasoAsignado:   identificacion.EscalonManual,
	EstadoCasoDescartado: identificacion.EscalonDescartado,
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
	// UsoID lee UN caso concreto: la respuesta de una resolucion reusa la
	// misma sentencia que la lista, para que las dos no puedan divergir.
	UsoID string
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

	// Nota es la justificacion de la decision manual (#175, D5). Vacia en un
	// caso pendiente, y obligatoria en uno asignado o descartado.
	Nota string

	// Sugerencia es la propuesta del rankeador (#53). No resuelve el caso:
	// la persona la confirma o elige otra.
	Sugerencia *SugerenciaCaso
}

// PaginaCasos es una pagina de la cola y el total de pendientes bajo los mismos filtros de fuente y periodo.
type PaginaCasos struct {
	Casos      []CasoIdentificacion
	Pendientes int
}

// CasosIdentificacion sirve la bandeja de identificacion manual (#39).
//
// Ejemplos y Rankeador son opcionales en la lectura: sin ellos la bandeja
// sigue listando y la sugerencia queda en "ninguna". La escritura de una
// resolucion si los exige, porque ahi es donde el ejemplo tiene que quedar
// guardado (#53).
type CasosIdentificacion struct {
	Repo      RepositorioCasosIdentificacion
	Ejemplos  RepositorioEjemplosResolucion
	Rankeador PuertoRankeadorDeResoluciones
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
	casos, err = c.conSugerencias(ctx, casos)
	if err != nil {
		return PaginaCasos{}, err
	}
	pag.Casos = casos
	return pag, nil
}

// conSugerencias adjunta la propuesta de cada caso.
//
// Un pendiente se rankea con el historial de su titulo, sin contarse a si
// mismo: todavia nadie lo resolvio. Uno ya resuelto muestra la sugerencia que
// quedo guardada al decidir, no una recalculada con historia posterior.
//
// El estado del caso no se toca. Una sugerencia de asignar deja el caso
// pendiente: rankear no es resolver (ADR 0007).
func (c CasosIdentificacion) conSugerencias(ctx context.Context, casos []CasoIdentificacion) ([]CasoIdentificacion, error) {
	if c.Ejemplos == nil || c.Rankeador == nil {
		for i := range casos {
			casos[i].Sugerencia = sugerenciaNinguna()
		}
		return casos, nil
	}

	var claves []string
	var resueltos []string
	vistos := map[string]bool{}
	for _, caso := range casos {
		if caso.Estado == EstadoCasoPendiente {
			clave := identificacion.ClaveDeTitulo(caso.Titulo, caso.TituloOriginal)
			if clave != "" && !vistos[clave] {
				vistos[clave] = true
				claves = append(claves, clave)
			}
			continue
		}
		resueltos = append(resueltos, caso.UsoID)
	}

	var historia []identificacion.EjemploEtiquetado
	var err error
	if len(claves) > 0 {
		historia, err = c.Ejemplos.HistoriaPorClaves(ctx, claves)
		if err != nil {
			return nil, err
		}
	}
	guardados := map[string]EjemploGuardado{}
	if len(resueltos) > 0 {
		guardados, err = c.Ejemplos.EjemplosDe(ctx, resueltos)
		if err != nil {
			return nil, err
		}
	}

	for i, caso := range casos {
		if caso.Estado != EstadoCasoPendiente {
			g, hay := guardados[caso.UsoID]
			if !hay {
				casos[i].Sugerencia = sugerenciaNinguna()
				continue
			}
			aceptada := g.Aceptada
			casos[i].Sugerencia = &SugerenciaCaso{
				Decision:  g.SugerenciaDecision,
				ObraID:    g.SugerenciaObraID,
				Titulo:    g.Titulo,
				Confianza: g.Confianza,
				Motivo:    g.Motivo,
				Orden:     g.Orden,
				Aceptada:  &aceptada,
			}
			if casos[i].Sugerencia.Orden == nil {
				casos[i].Sugerencia.Orden = []string{}
			}
			if casos[i].Sugerencia.Titulo == "" {
				casos[i].Sugerencia.Titulo = tituloDeCandidato(caso.Candidatos, g.SugerenciaObraID)
			}
			continue
		}
		clave := identificacion.ClaveDeTitulo(caso.Titulo, caso.TituloOriginal)
		sug := c.Rankeador.Rankear(identificacion.PedidoTriage{
			Clave:      clave,
			Candidatos: candidatosDeRanking(caso.Candidatos),
			Historia:   historia,
		})
		casos[i].Sugerencia = sugerenciaVisible(sug, caso.Candidatos, nil)
	}
	return casos, nil
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
		q.Escalones = []string{identificacion.EscalonONI, identificacion.EscalonManual, identificacion.EscalonDescartado}
		return q, nil
	}
	escalon, ok := escalonesDeEstado[estado]
	if !ok {
		return ConsultaCasos{}, fmt.Errorf("%w: estado %q, se esperaba %q, %q o %q",
			ErrFiltroCasosInvalido, estado, EstadoCasoPendiente, EstadoCasoAsignado, EstadoCasoDescartado)
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
	case identificacion.EscalonDescartado:
		// Igual que manual: un descarte tambien va firmado y con fecha, o no
		// es un descarte (ADR 0006, D1).
		if caso.ResueltoEn == nil {
			return CasoIdentificacion{}, fmt.Errorf("caso %s: escalon descartado sin resuelto_en", caso.UsoID)
		}
		caso.Estado = EstadoCasoDescartado
		caso.UltimaActualizacion = *caso.ResueltoEn
	default:
		return CasoIdentificacion{}, fmt.Errorf("caso %s: escalon %q no es un caso de la cola manual", caso.UsoID, caso.Escalon)
	}
	if caso.Candidatos == nil {
		caso.Candidatos = []CandidatoCaso{}
	}
	return caso, nil
}
