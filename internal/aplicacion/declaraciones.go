package aplicacion

import (
	"context"
	"fmt"
	"time"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Declaraciones son los casos de uso de la Declaracion de Obra: el ABM
// versionado que la #23 le debia a la #30.
//
// Un servicio con dos operaciones y no dos structs, por lo mismo que
// [Catalogo]: giran sobre el mismo agregado y comparten las mismas
// dependencias.
//
// No lleva un [BitacoraAuditoria] propio: el asiento de esta operacion es
// parte del mismo contrato atomico que [GestionDeclaraciones.Guardar], no un
// segundo puerto que este caso de uso orqueste por su cuenta -ver el
// comentario de ese puerto en puertos.go.
type Declaraciones struct {
	Gestion GestionDeclaraciones
	Reloj   Reloj
}

// GuardarSplits valida las partes que llegan y las guarda como una version
// nueva de la declaracion de la obra.
//
// Es la misma operacion tanto si es la primera declaracion de la obra como si
// es una edicion: [GestionDeclaraciones.Guardar] decide por si sola si cierra
// una version anterior o abre la primera. Separarlo en dos metodos de caso de
// uso duplicaria esta funcion entera para una diferencia que ya resuelve el
// adaptador.
func (d Declaraciones) GuardarSplits(ctx context.Context, obraID string, partes []repertorio.Parte, actorID string) (VersionDeclaracion, error) {
	decl, err := repertorio.NuevaDeclaracion(obraID, partes)
	if err != nil {
		return VersionDeclaracion{}, err
	}

	ahora := d.Reloj.Ahora()
	version, err := d.Gestion.Guardar(ctx, decl, ahora, actorID)
	if err != nil {
		return VersionDeclaracion{}, fmt.Errorf("guardar declaracion de la obra %q: %w", obraID, err)
	}

	return VersionDeclaracion{Version: version, VigenteDesde: ahora, Declaracion: decl}, nil
}

// Historial devuelve todas las versiones de la declaracion de una obra,
// ordenadas por version. Una obra sin ninguna declaracion aun devuelve una
// lista vacia, no un error: es el mismo criterio que [Catalogo.BuscarObras].
func (d Declaraciones) Historial(ctx context.Context, obraID string) ([]VersionDeclaracion, error) {
	historial, err := d.Gestion.Historial(ctx, obraID)
	if err != nil {
		return nil, fmt.Errorf("historial de declaraciones de la obra %q: %w", obraID, err)
	}
	return historial, nil
}

// VigenteEn resuelve que version de la declaracion regia en un instante dado.
// Es la pregunta que un reproceso necesita responder para reproducir un
// reparto pasado con el split que tenia entonces, no con el de hoy.
func (d Declaraciones) VigenteEn(ctx context.Context, obraID string, momento time.Time) (VersionDeclaracion, error) {
	vd, err := d.Gestion.VigenteEn(ctx, obraID, momento)
	if err != nil {
		return VersionDeclaracion{}, fmt.Errorf("declaracion vigente de la obra %q: %w", obraID, err)
	}
	return vd, nil
}

// AsientoDeclaracion es la forma del payload JSONB del asiento que
// [GestionDeclaraciones.Guardar] escribe. Exportada porque quien la
// serializa es el adaptador de infraestructura -es el unico que sabe en que
// transaccion asentar-, no este caso de uso; vive aqui y no como forma de red
// porque el asiento tampoco lo es: es el registro interno de la bitacora
// (ADR 0006), no una respuesta HTTP.
type AsientoDeclaracion struct {
	Version int                `json:"version"`
	Estado  string             `json:"estado"`
	Partes  []repertorio.Parte `json:"partes"`
}
