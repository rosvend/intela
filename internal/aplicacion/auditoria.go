package aplicacion

import (
	"context"
	"fmt"
	"strings"
)

// Auditoria es la lectura de la bitacora para el Portal de Auditoria (OE-7).
//
// Existe aunque hoy sean operaciones de una linea, por lo mismo que
// [Titulares]: cada lectura tiene que tener su caso de uso, y es aqui donde
// viven el defecto de la paginacion y, mas adelante, el alcance por rol si la
// auditoria deja de ser lectura de todo.
//
// La autorizacion -solo `auditor` (Revisor Fiscal) y `administrador`- vive en
// la ruta HTTP (`requiereRol`), no aqui: estos metodos no reciben actor, igual
// que el resto de lecturas del nucleo.
//
// No lleva [BitacoraAuditoria] para ESCRIBIR: leer la bitacora no es un hecho
// que haya que asentar. El ADR 0006 pide asiento para lo que mueve dinero o
// cambia una cifra, no para cada consulta.
type Auditoria struct {
	Bitacora BitacoraAuditoria
}

// Asientos devuelve una pagina de la bitacora en orden de timeline: lo mas
// reciente primero. Sin filtros de servidor, a proposito: los filtros de la
// vista (tipo, fecha, actor) se aplican en el cliente sobre la pagina.
// El defecto de la paginacion vive aqui -no en cada adaptador- para que
// cualquier [BitacoraAuditoria] lo herede y se pueda comprobar sin levantar
// Postgres; es la misma forma que [Catalogo.BuscarObras].
func (a Auditoria) Asientos(ctx context.Context, pag Paginacion) ([]Asiento, error) {
	pag = pag.ConDefecto()
	asientos, err := a.Bitacora.ListarAsientos(ctx, pag)
	if err != nil {
		return nil, fmt.Errorf("leer la bitacora: %w", err)
	}
	return asientos, nil
}

// HistorialDeObra devuelve los asientos que referencian a una obra, en orden
// de cadena: del mas antiguo al mas nuevo, que es el orden en que ocurrieron
// los hechos y el que espera ExplicarCifra para reconstruir.
//
// Una obra sin asientos devuelve una lista vacia, no un 404: mismo criterio
// que el historial de declaraciones -"sin hechos" no es "la obra no existe",
// y distinguir las dos cosas pediria leer el catalogo, que el auditor no
// puede ver (es solo de `administrador`).
func (a Auditoria) HistorialDeObra(ctx context.Context, obraID string) ([]Asiento, error) {
	obraID = strings.TrimSpace(obraID)
	asientos, err := a.Bitacora.De(ctx, "obra", obraID)
	if err != nil {
		return nil, fmt.Errorf("historial de la obra %q: %w", obraID, err)
	}
	return asientos, nil
}
