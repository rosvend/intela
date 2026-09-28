package postgres

import (
	"context"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioResolucionIdentificacion = (*Store)(nil)

// PeriodoDeUso da el periodo del reporte de la fila.
//
// Sin filas, traducirError convierte pgx.ErrNoRows en aplicacion.ErrNoEncontrado,
// que es lo que el caso de uso traduce a 404.
func (s *Store) PeriodoDeUso(ctx context.Context, usoID string) (string, error) {
	var periodo string
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT r.periodo FROM usos u JOIN reportes r ON r.id = u.reporte_id WHERE u.id = $1`,
		usoID).Scan(&periodo)
	if err != nil {
		return "", traducirError(err, "periodo del uso %q", usoID)
	}
	return periodo, nil
}

// BloquearPeriodoDeUsos toma el cerrojo de aviso del periodo, el MISMO que usan
// la evaluacion de anomalias (#37), la ingesta y la valorizacion (#171).
//
// Es a proposito: `bloquearAlertasEn` y `claveCerrojoAlertas` son la unica
// cerradura de periodo que existe, y una resolucion manual que tomara otra
// clave se colaria entre la compuerta de anomalias y el calculo sin que ninguna
// de las dos la viera. Compartir la clave es lo que hace que resolver un caso
// de un periodo en reparto espere a que la corrida termine (D3).
func (s *Store) BloquearPeriodoDeUsos(ctx context.Context, periodo string) error {
	tx, hay := txDe(ctx)
	if !hay {
		return fmt.Errorf("bloquear el periodo %s: %w", periodo, errFueraDeUnidad)
	}
	return bloquearAlertasEn(ctx, tx, periodo)
}

// CasoParaResolver lee la fila bloqueandola y trae los candidatos que el motor
// propuso.
//
// FOR UPDATE OF u y no FOR UPDATE a secas: `reportes` no se bloquea, solo la
// fila de `usos` que se va a escribir. Sin el OF, Postgres bloquearia tambien
// la fila del reporte, y dos resoluciones de usos distintos del mismo reporte
// se serializarian sin necesidad.
//
// La proyeccion esta escrita a mano en vez de reusar [columnasUso] por dos
// razones: aquella va sin prefijo y en este JOIN `id` y `fuente` son ambiguos
// -las dos tablas las tienen-, y aqui hace falta ademas `r.periodo`. Si se
// anade una columna a `usos`, esta lista y [columnasUso] hay que moverlas
// juntas.
func (s *Store) CasoParaResolver(ctx context.Context, usoID string) (aplicacion.CasoParaResolver, error) {
	var (
		caso      aplicacion.CasoParaResolver
		modalidad string
		obraID    *string
	)
	err := s.ejecutorDe(ctx).QueryRow(ctx, sqlCasoParaResolver, usoID).Scan(
		&caso.Uso.ID, &caso.Uso.ReporteID, &caso.Uso.Fuente, &caso.Uso.Titulo, &caso.Uso.TituloOrig,
		&caso.Uso.IDsFuente, &obraID, &caso.Uso.Escalon, &caso.Uso.Evidencia, &caso.Uso.ONI,
		&modalidad, &caso.Uso.TipoObra, &caso.Uso.CanalID, &caso.Uso.Fecha, &caso.Uso.Hora,
		&caso.Uso.DuracionMin, &caso.Uso.Emisiones, &caso.Uso.Rating, &caso.Uso.Taquilla,
		&caso.Uso.Espectadores, &caso.Uso.Exhibiciones, &caso.Uso.Vistas, &caso.Uso.MinutosVistos,
		&caso.Uso.PB, &caso.Periodo)
	if err != nil {
		return aplicacion.CasoParaResolver{}, traducirError(err, "leer el caso del uso %q", usoID)
	}
	// obra_id viaja como puntero porque la columna es nullable mientras la obra
	// no este identificada; UsoPersistido la dice con "" (misma convencion que
	// columnasUso, que la envuelve en COALESCE).
	caso.Uso.ObraID = valorDe(obraID)
	caso.Uso.Modalidad = reparto.Modalidad(modalidad)

	// Los candidatos van DESPUES del bloqueo, dentro de la misma unidad: la
	// bandeja es la evidencia de lo que se propuso y tiene que ser la que se
	// leyo al decidir, no una que otra corrida reescribio entretanto.
	if caso.Candidatos, err = s.CandidatosDeUso(ctx, usoID); err != nil {
		return aplicacion.CasoParaResolver{}, err
	}
	return caso, nil
}

const sqlCasoParaResolver = `
SELECT u.id, u.reporte_id, u.fuente, u.titulo, u.titulo_original, u.ids_fuente,
       u.obra_id, u.escalon, u.evidencia, u.oni, COALESCE(u.modalidad, ''), u.tipo_obra,
       u.canal_id, u.fecha, u.hora, u.duracion_min, u.emisiones, u.rating, u.taquilla,
       u.espectadores, u.exhibiciones, u.vistas, u.minutos_vistos, u.pb,
       r.periodo
  FROM usos u
  JOIN reportes r ON r.id = u.reporte_id
 WHERE u.id = $1
   FOR UPDATE OF u`

// TituloDeObra da el titulo de catalogo, que el asiento guarda tal como estaba
// al decidir. ErrNoEncontrado si esa obra no esta: el caso de uso lo convierte
// en ErrObraInexistente (400) antes de escribir nada.
func (s *Store) TituloDeObra(ctx context.Context, obraID string) (string, error) {
	var titulo string
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT titulo FROM obras WHERE id = $1`, obraID).Scan(&titulo)
	if err != nil {
		return "", traducirError(err, "titulo de la obra %q", obraID)
	}
	return titulo, nil
}

// GuardarResolucionManual escribe la decision sobre la fila.
//
// obra_id = NULLIF($2, ”): el nucleo dice "sin obra" con la cadena vacia y la
// columna lo dice con NULL (misma convencion que GuardarMatch).
//
// El CASE de tipo_obra es el MISMO que GuardarMatch (#165/#169) y por la misma
// razon: con obra y el campo vacio, el tipo sale del catalogo. Un tipo que si
// trajo la fuente no se pisa.
//
// Los candidatos NO se borran: `candidatos_match` es la evidencia de lo que el
// motor propuso, y la lectura de la cola la sigue mostrando despues de resolver.
//
// WHERE escalon = 'oni' es defensa: la fila ya viene bloqueada por
// [Store.CasoParaResolver], asi que un RowsAffected de cero significa que algo
// se salio del guion. Devolver ErrCasoYaResuelto envuelto es mejor que un 200
// que afirme una escritura que no ocurrio.
func (s *Store) GuardarResolucionManual(ctx context.Context, r aplicacion.ResolucionManual) error {
	etiqueta, err := s.ejecutorDe(ctx).Exec(ctx,
		`UPDATE usos
		    SET obra_id = NULLIF($2, ''), escalon = $3, evidencia = $4, puntaje = $5,
		        oni = FALSE,
		        resuelto_por = $6, resuelto_en = $7, nota_resolucion = $8,
		        tipo_obra = CASE
		          WHEN $2 = '' OR tipo_obra <> '' THEN tipo_obra
		          ELSE COALESCE((SELECT tipo FROM obras WHERE id = $2), tipo_obra)
		        END
		  WHERE id = $1 AND escalon = 'oni'`,
		r.UsoID, r.Resultado.ObraID, r.Resultado.Escalon, r.Resultado.Evidencia, r.Resultado.Puntaje,
		r.ActorID, r.Cuando, r.Nota)
	if err != nil {
		return traducirError(err, "guardar la resolucion del uso %q", r.UsoID)
	}
	if etiqueta.RowsAffected() == 0 {
		return fmt.Errorf("guardar la resolucion del uso %q: ya no esta en escalon %q: %w",
			r.UsoID, identificacion.EscalonONI, identificacion.ErrCasoYaResuelto)
	}
	return nil
}

// CasoIdentificacionPorID lee un caso ya resuelto para devolverlo en la respuesta.
//
// Reusa la MISMA sentencia que la lista con el filtro por id: dos consultas
// hermanas divergen en cuanto alguien anade una columna a una sola, y el
// desajuste no se ve hasta que el JSON sale con un campo vacio.
func (s *Store) CasoIdentificacionPorID(ctx context.Context, usoID string) (aplicacion.CasoIdentificacion, error) {
	p, err := s.ListarCasosIdentificacion(ctx, aplicacion.ConsultaCasos{
		Escalones: []string{identificacion.EscalonONI, identificacion.EscalonManual, identificacion.EscalonDescartado},
		UsoID:     usoID,
		Paginacion: aplicacion.Paginacion{
			Limite: 1,
		},
	})
	if err != nil {
		return aplicacion.CasoIdentificacion{}, err
	}
	if len(p.Casos) == 0 {
		return aplicacion.CasoIdentificacion{}, fmt.Errorf("caso del uso %q: %w", usoID, aplicacion.ErrNoEncontrado)
	}
	return p.Casos[0], nil
}
