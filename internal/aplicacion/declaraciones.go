package aplicacion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Declaraciones son los casos de uso de la Declaracion de Obra: el ABM
// versionado que la #23 le debia a la #30.
//
// Un servicio con tres operaciones y no tres structs, por lo mismo que
// [Catalogo]: giran sobre el mismo agregado y comparten las mismas
// dependencias.
type Declaraciones struct {
	Gestion  GestionDeclaraciones
	Bitacora BitacoraAuditoria
	Reloj    Reloj
}

// GuardarSplits valida las partes que llegan y las guarda como una version
// nueva de la declaracion de la obra.
//
// Es la misma operacion tanto si es la primera declaracion de la obra como si
// es una edicion: [GestionDeclaraciones.Guardar] decide por si sola si cierra
// una version anterior o abre la primera. Separarlo en dos metodos de caso de
// uso duplicaria esta funcion entera para una diferencia que ya resuelve el
// adaptador.
//
// El asiento de auditoria es parte de la definicion de hecho de esta
// operacion (ADR 0006): si Asentar falla, GuardarSplits devuelve error aunque
// la escritura en el repositorio ya se haya hecho. No hay una transaccion que
// abarque los dos puertos -esa es una decision de infraestructura mas grande
// que este issue-, asi que un fallo aqui dice "la declaracion se guardo pero
// no quedo asiento", y quien opera el sistema tiene que poder distinguirlo de
// un fallo que no escribio nada.
func (d Declaraciones) GuardarSplits(ctx context.Context, obraID string, partes []repertorio.Parte, actorID string) (VersionDeclaracion, error) {
	decl, err := repertorio.NuevaDeclaracion(obraID, partes)
	if err != nil {
		return VersionDeclaracion{}, err
	}

	ahora := d.Reloj.Ahora()
	version, err := d.Gestion.Guardar(ctx, decl, ahora)
	if err != nil {
		return VersionDeclaracion{}, fmt.Errorf("guardar declaracion de la obra %q: %w", obraID, err)
	}

	vd := VersionDeclaracion{Version: version, VigenteDesde: ahora, Declaracion: decl}

	payload, err := json.Marshal(asientoDeclaracion{
		Version: version,
		Estado:  decl.Estado(),
		Partes:  decl.Partes,
	})
	if err != nil {
		return VersionDeclaracion{}, fmt.Errorf("serializar asiento de la obra %q: %w", obraID, err)
	}
	if err := d.Bitacora.Asentar(ctx, Asiento{
		Hecho:   "declaracion.guardada",
		RefTipo: "obra",
		RefID:   obraID,
		ActorID: actorID,
		Payload: payload,
		Cuando:  ahora,
	}); err != nil {
		return VersionDeclaracion{}, fmt.Errorf("asentar declaracion de la obra %q: %w", obraID, err)
	}

	return vd, nil
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

// asientoDeclaracion es la forma del payload JSONB del asiento. Vive aqui y
// no como forma de red porque el asiento tampoco lo es: es el registro
// interno de la bitacora (ADR 0006), no una respuesta HTTP.
type asientoDeclaracion struct {
	Version int                `json:"version"`
	Estado  string             `json:"estado"`
	Partes  []repertorio.Parte `json:"partes"`
}
