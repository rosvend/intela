package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.RepositorioUsosDeReparto = (*Store)(nil)

// UsosDeCanal devuelve las filas canonicas que ponderan la bolsa de un canal.
//
// El periodo no esta en `usos`: vive en el reporte del que salio la fila, y va
// como subconsulta -- no como JOIN -- para poder reutilizar columnasUso tal
// cual. Con un JOIN habria que calificar la proyeccion con el alias de la
// tabla, y esa proyeccion se comparte justamente para que no diverja.
//
// `canal_id` no tiene clave foranea a `canales` (migracion 00011): conserva el
// identificador que declaro la fuente aunque el catalogo anual todavia no lo
// conozca. Por eso el filtro es por igualdad de texto y no por FK.
func (s *Store) UsosDeCanal(
	ctx context.Context, periodo, canalID string, anioClasificacion int,
) ([]aplicacion.UsoDeReparto, error) {
	usos, err := s.consultarUsos(ctx,
		`SELECT `+columnasUso+` FROM usos
		  WHERE reporte_id IN (SELECT id FROM reportes WHERE periodo = $1)
		    AND canal_id = $2
		  ORDER BY id`,
		"listar usos del periodo %q para el canal %q", periodo, canalID)
	if err != nil {
		return nil, err
	}
	if len(usos) == 0 {
		return nil, nil
	}

	// Una consulta y no una por fila: el canal es un parametro, asi que su
	// clasificacion es la misma para todo el lote.
	grupo, err := s.grupoDeCanal(ctx, canalID, anioClasificacion)
	if err != nil {
		return nil, err
	}

	filas := make([]aplicacion.UsoDeReparto, 0, len(usos))
	for _, u := range usos {
		filas = append(filas, aplicacion.UsoDeReparto{Uso: u, GrupoEfectivo: grupo})
	}
	return filas, nil
}

// grupoDeCanal resuelve la clasificacion de RD 9.5.4 de un ano concreto.
//
// Un canal sin fila para ese ano devuelve la cadena vacia y no un error: fuera
// de suscripcion el grupo no se usa, y dentro de ella quien falla ruidosamente
// es [reparto.ParseGrupoCanal] en el nucleo, con su error tipado.
func (s *Store) grupoDeCanal(ctx context.Context, canalID string, anio int) (string, error) {
	var grupo string
	err := s.pool.QueryRow(ctx,
		`SELECT grupo_efectivo FROM canales_clasificacion
		  WHERE canal_id = $1 AND anio_audiencia = $2`,
		canalID, anio).Scan(&grupo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", traducirError(err, "clasificacion del canal %q para %d", canalID, anio)
	}
	return grupo, nil
}
