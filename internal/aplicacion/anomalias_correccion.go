package aplicacion

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/rosvend/intela/internal/dominio/anomalias"
)

// Hechos de la correccion de una anomalia critica (#164). Van sobre el
// registro que cambio -- el uso o la entrega --, no sobre la alerta: el
// historial de una fila tiene que contar por que dejo de ponderar sin pasar
// por la bandeja. El cierre de la alerta tiene su propio asiento
// ([HechoAlertaResuelta]).
const (
	HechoUsoExcluidoPorDuplicado     = "correccion.uso_excluido"
	HechoEntregaExcluidaPorDuplicado = "correccion.entrega_excluida"
	HechoTipoObraAsignado            = "correccion.tipo_obra_asignado"
)

// RefReporte es el tipo de referencia de un asiento que apunta a una entrega,
// el mismo `reporte` que usa `alertas.ref_tipo`.
const RefReporte = "reporte"

// CierreDeAlerta es la escritura del cierre de una alerta: quien, con que rol,
// por que y que se hizo con el dato.
type CierreDeAlerta struct {
	ActorID        string
	ActorRol       string
	Nota           string
	Accion         string
	AccionObjetivo string
	Cuando         time.Time
}

// CorreccionDeDatos es lo que el cierre de una critica escribe sobre el dato
// (#164). Se declara aqui, junto a quien la consume, como [LecturaDeEntregas]:
// son tres escrituras concretas sobre `usos` y `reportes`, y ningun otro caso
// de uso tiene por que poder hacerlas.
//
// Las tres son CONDICIONALES al estado que el caso de uso valido: si la fila o
// la entrega cambiaron entre la lectura y la escritura -- la cascada no toma el
// cerrojo de periodo --, devuelven [anomalias.ErrAccionNoAplica] y la unidad
// revierte el cierre con ellas.
//
// *postgres.Store la satisface.
type CorreccionDeDatos interface {
	// ExcluirUsoDuplicado pasa la fila a escalon 'duplicado' (sin obra, sin
	// ONI, firmada y con nota) si sigue en el escalon y con la obra que se leyeron.
	ExcluirUsoDuplicado(ctx context.Context, e ExclusionDeUso) error

	// ExcluirEntregaDuplicada marca la entrega como excluida y pasa a
	// 'duplicado' todas sus filas que sigan en juego ([anomalias.SigueEnJuego]).
	// Devuelve lo que cambio, para el asiento.
	ExcluirEntregaDuplicada(ctx context.Context, e ExclusionDeEntrega) (EntregaExcluida, error)

	// AsignarTipoObraAUso pone el tipo de obra a una fila identificada que no lo
	// tiene, y devuelve la obra de la fila. Una fila que ya tiene tipo, o que ya
	// no tiene obra, no se toca.
	AsignarTipoObraAUso(ctx context.Context, usoID, tipoObra string) (obraID string, err error)
}

// ExclusionDeUso es la escritura de una fila excluida por duplicada.
type ExclusionDeUso struct {
	UsoID string
	// EscalonPrevio y ObraPrevia son los que se validaron: la escritura es condicional a ellos.
	EscalonPrevio string
	ObraPrevia    string
	ActorID       string
	Nota          string
	// Evidencia es la que queda en la fila: por que alerta salio y que tenia antes.
	Evidencia string
	Cuando    time.Time
}

// ExclusionDeEntrega es la escritura de una entrega excluida por duplicada.
type ExclusionDeEntrega struct {
	ReporteID string
	ActorID   string
	Nota      string
	// Evidencia es el prefijo que queda en cada fila; el adaptador le anade lo
	// que esa fila tenia antes (escalon y obra).
	Evidencia string
	Cuando    time.Time
}

// EntregaExcluida es lo que cambio al excluir una entrega.
type EntregaExcluida struct {
	// Usos es cuantas filas pasaron a 'duplicado'.
	Usos int
	// PorEscalon cuenta esas filas por el escalon que tenian antes.
	PorEscalon map[string]int
	// Obras son las obras que esas filas ponderaban, ordenadas y sin repetir.
	Obras []string
}

// corregir aplica la correccion ya validada en forma y devuelve el asiento que
// la registra, sin actor ni instante (los pone quien lo escribe). nil si la
// accion no toca el dato: sin accion (no critica) o aceptada tal cual.
func (a Anomalias) corregir(ctx context.Context, alerta Alerta, c anomalias.Correccion,
	actorID, rol, nota string, ahora time.Time) (*Asiento, error) {

	switch c.Accion {
	case anomalias.AccionExcluirUso:
		return a.excluirUso(ctx, alerta, c.Objetivo, actorID, rol, nota, ahora)
	case anomalias.AccionExcluirEntrega:
		return a.excluirEntrega(ctx, alerta, c.Objetivo, actorID, rol, nota, ahora)
	case anomalias.AccionAsignarTipoObra:
		return a.asignarTipoObra(ctx, alerta, c.Objetivo, rol, nota)
	default:
		return nil, nil
	}
}

// excluirUso valida contra la foto del periodo y excluye la fila objetivo.
func (a Anomalias) excluirUso(ctx context.Context, alerta Alerta, usoID, actorID, rol, nota string,
	ahora time.Time) (*Asiento, error) {

	usos, err := a.Entregas.UsosDePeriodo(ctx, alerta.Periodo)
	if err != nil {
		return nil, fmt.Errorf("usos del periodo %q: %w", alerta.Periodo, err)
	}
	foto := make([]anomalias.Uso, 0, len(usos))
	for _, u := range usos {
		foto = append(foto, usoDeDominio(u))
	}
	objetivo, err := anomalias.ValidarExclusionDeUso(foto, alerta.RefID, usoID)
	if err != nil {
		return nil, fmt.Errorf("resolver la alerta %q: %w", alerta.ID, err)
	}

	evidencia := fmt.Sprintf("duplicado: excluida al resolver la alerta %s (registro %s de %q); antes %s",
		alerta.ID, objetivo.ClaveRegistro, objetivo.Fuente, estadoPrevio(objetivo.Escalon, objetivo.ObraID))
	if err := a.Correcciones.ExcluirUsoDuplicado(ctx, ExclusionDeUso{
		UsoID: objetivo.ID, EscalonPrevio: objetivo.Escalon, ObraPrevia: objetivo.ObraID,
		ActorID: actorID, Nota: nota, Evidencia: evidencia, Cuando: ahora,
	}); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(struct {
		AlertaID        string `json:"alerta_id"`
		Periodo         string `json:"periodo"`
		UsoID           string `json:"uso_id"`
		UsoDeLaAlerta   string `json:"uso_de_la_alerta"`
		ReporteID       string `json:"reporte_id"`
		Fuente          string `json:"fuente"`
		ClaveRegistro   string `json:"clave_registro"`
		EscalonAnterior string `json:"escalon_anterior"`
		ObraAnterior    string `json:"obra_anterior,omitempty"`
		Nota            string `json:"nota"`
		ActorRol        string `json:"actor_rol,omitempty"`
	}{
		AlertaID: alerta.ID, Periodo: alerta.Periodo, UsoID: objetivo.ID, UsoDeLaAlerta: alerta.RefID,
		ReporteID: objetivo.ReporteID, Fuente: objetivo.Fuente, ClaveRegistro: objetivo.ClaveRegistro,
		EscalonAnterior: objetivo.Escalon, ObraAnterior: objetivo.ObraID, Nota: nota, ActorRol: rol,
	})
	if err != nil {
		return nil, fmt.Errorf("serializar la exclusion del uso %q: %w", objetivo.ID, err)
	}
	return &Asiento{Hecho: HechoUsoExcluidoPorDuplicado, RefTipo: RefUso, RefID: objetivo.ID, Payload: payload}, nil
}

// excluirEntrega valida contra las entregas conocidas y excluye la entrega objetivo entera.
func (a Anomalias) excluirEntrega(ctx context.Context, alerta Alerta, reporteID, actorID, rol, nota string,
	ahora time.Time) (*Asiento, error) {

	cargas, err := a.Entregas.EntregasRecibidas(ctx)
	if err != nil {
		return nil, fmt.Errorf("entregas recibidas: %w", err)
	}
	foto := make([]anomalias.Entrega, 0, len(cargas))
	for _, c := range cargas {
		foto = append(foto, entregaDeDominio(c))
	}
	objetivo, err := anomalias.ValidarExclusionDeEntrega(foto, alerta.Periodo, alerta.RefID, reporteID)
	if err != nil {
		return nil, fmt.Errorf("resolver la alerta %q: %w", alerta.ID, err)
	}

	hecho, err := a.Correcciones.ExcluirEntregaDuplicada(ctx, ExclusionDeEntrega{
		ReporteID: objetivo.ID, ActorID: actorID, Nota: nota, Cuando: ahora,
		Evidencia: fmt.Sprintf("duplicado: entrega %s excluida al resolver la alerta %s (sha256 %s)",
			objetivo.ID, alerta.ID, objetivo.SHA256),
	})
	if err != nil {
		return nil, err
	}
	obras := hecho.Obras
	if obras == nil {
		obras = []string{}
	}
	slices.Sort(obras)

	payload, err := json.Marshal(struct {
		AlertaID          string         `json:"alerta_id"`
		Periodo           string         `json:"periodo"`
		ReporteID         string         `json:"reporte_id"`
		EntregaDeLaAlerta string         `json:"entrega_de_la_alerta"`
		Fuente            string         `json:"fuente"`
		SHA256            string         `json:"sha256"`
		UsosExcluidos     int            `json:"usos_excluidos"`
		PorEscalon        map[string]int `json:"por_escalon_anterior"`
		ObrasAfectadas    []string       `json:"obras_afectadas"`
		Nota              string         `json:"nota"`
		ActorRol          string         `json:"actor_rol,omitempty"`
	}{
		AlertaID: alerta.ID, Periodo: alerta.Periodo, ReporteID: objetivo.ID, EntregaDeLaAlerta: alerta.RefID,
		Fuente: objetivo.Fuente, SHA256: objetivo.SHA256, UsosExcluidos: hecho.Usos,
		PorEscalon: hecho.PorEscalon, ObrasAfectadas: obras, Nota: nota, ActorRol: rol,
	})
	if err != nil {
		return nil, fmt.Errorf("serializar la exclusion de la entrega %q: %w", objetivo.ID, err)
	}
	return &Asiento{Hecho: HechoEntregaExcluidaPorDuplicado, RefTipo: RefReporte, RefID: objetivo.ID, Payload: payload}, nil
}

// asignarTipoObra pone el tipo a la fila de la alerta.
func (a Anomalias) asignarTipoObra(ctx context.Context, alerta Alerta, tipoObra, rol, nota string) (*Asiento, error) {
	obraID, err := a.Correcciones.AsignarTipoObraAUso(ctx, alerta.RefID, tipoObra)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		AlertaID string `json:"alerta_id"`
		Periodo  string `json:"periodo"`
		UsoID    string `json:"uso_id"`
		ObraID   string `json:"obra_id"`
		TipoObra string `json:"tipo_obra"`
		Nota     string `json:"nota"`
		ActorRol string `json:"actor_rol,omitempty"`
	}{
		AlertaID: alerta.ID, Periodo: alerta.Periodo, UsoID: alerta.RefID, ObraID: obraID,
		TipoObra: tipoObra, Nota: nota, ActorRol: rol,
	})
	if err != nil {
		return nil, fmt.Errorf("serializar la asignacion de tipo del uso %q: %w", alerta.RefID, err)
	}
	return &Asiento{Hecho: HechoTipoObraAsignado, RefTipo: RefUso, RefID: alerta.RefID, Payload: payload}, nil
}

// estadoPrevio narra el escalon y la obra que tenia una fila, para su evidencia.
func estadoPrevio(escalon, obraID string) string {
	if obraID == "" {
		return fmt.Sprintf("escalon %s sin obra", escalon)
	}
	return fmt.Sprintf("escalon %s con obra %s", escalon, obraID)
}
