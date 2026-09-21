package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Procesos son los casos de uso de RD 13.5: abrir una corrida, firmarla,
// avanzarla y rechazar una compuerta.
//
// El agregado ([reparto.ProcesoDeReparto]) hace la transicion; este servicio
// solo la orquesta -- carga la vista persistida, la traduce al agregado,
// llama al metodo de dominio, y guarda lo que vuelva (ADR 0008). El puerto
// [RepositorioProcesos] guarda [ProcesoVista], no el agregado: es una vista
// de persistencia, y la traduccion vive aqui a proposito.
type Procesos struct {
	Repo       RepositorioProcesos
	Parametros ParametrosNormativos

	// Los cinco de abajo solo hacen falta para valorizar (Nacional, RD 13.5):
	// abrir un proceso, firmarlo o rechazar una compuerta no los tocan. El
	// internacional no valoriza por puntos (RD 7.4) y nunca los usa.
	Bolsas        RepositorioRecaudo
	Declaraciones GestionDeclaraciones
	Usos          RepositorioUsosDeReparto
	Resultados    RepositorioResultados
	// Unidad ata la escritura de resultados_* (puerto Resultados) y la de
	// procesos.etapa (puerto Repo) a UN solo hecho: valorizar sin persistir la
	// nueva etapa dejaria una corrida cuyo resultado ya esta en la base pero
	// que un reintento volveria a valorizar -- doble escritura del mismo
	// dinero. Solo hace falta cuando AvanzarEtapa entra a valorizar.
	Unidad UnidadDeTrabajo
}

// aProcesoVista traduce el agregado a la forma que persiste el puerto.
func aProcesoVista(p reparto.ProcesoDeReparto) ProcesoVista {
	return ProcesoVista{
		ID:            p.ID,
		Circuito:      p.Circuito,
		Etapa:         p.Etapa,
		Periodo:       p.Periodo,
		BolsaID:       p.BolsaID,
		SnapshotID:    p.SnapshotID,
		Reglamento:    p.Reglamento,
		Revision:      p.Revision,
		Firmas:        p.Firmas,
		RechazoMotivo: p.RechazoMotivo,
	}
}

// unProceso traduce la vista persistida de vuelta al agregado.
func unProceso(v ProcesoVista) reparto.ProcesoDeReparto {
	return reparto.ProcesoDeReparto{
		ID:            v.ID,
		Periodo:       v.Periodo,
		Circuito:      v.Circuito,
		BolsaID:       v.BolsaID,
		SnapshotID:    v.SnapshotID,
		Reglamento:    v.Reglamento,
		Etapa:         v.Etapa,
		Revision:      v.Revision,
		Firmas:        v.Firmas,
		RechazoMotivo: v.RechazoMotivo,
	}
}

// IniciarProceso congela el snapshot vigente a la fecha del periodo y abre
// la corrida en EtapaRecaudo (ADR 0004/0005). Se llama UNA VEZ: un
// reproceso lee el snapshot ya congelado, no vuelve a pasar por aqui.
func (uc Procesos) IniciarProceso(ctx context.Context, id, periodo string, circuito reparto.Circuito, bolsaID string) (ProcesoVista, error) {
	// Idempotente por ID: un reintento de TrabajoEjecutarReparto vuelve a
	// tomar el MISMO trabajo (Intentos, no Corrida) y llamaria aqui otra vez.
	// Sin esta guarda, reabrir un proceso que un humano ya avanzo lo
	// reiniciaria a EtapaRecaudo revision 1 y borraria el progreso.
	if existente, err := uc.Repo.ProcesoPorID(ctx, id); err == nil {
		return existente, nil
	} else if !errors.Is(err, ErrNoEncontrado) {
		return ProcesoVista{}, fmt.Errorf("iniciar proceso %q: %w", id, err)
	}

	fecha, err := fechaDePeriodo(strings.TrimSpace(periodo))
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("iniciar proceso %q: %w", id, err)
	}
	snapshotID, snap, err := uc.Parametros.SnapshotEnFecha(ctx, fecha)
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("iniciar proceso %q: %w", id, err)
	}
	p, err := reparto.AbrirProceso(id, periodo, circuito, bolsaID, snapshotID, snap.Reglamento)
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("iniciar proceso %q: %w", id, err)
	}
	if err := uc.Repo.GuardarProceso(ctx, aProcesoVista(p)); err != nil {
		return ProcesoVista{}, fmt.Errorf("iniciar proceso %q: %w", id, err)
	}
	return aProcesoVista(p), nil
}

// AbrirCorridaDelPeriodo abre un ProcesoDeReparto por cada bolsa del
// periodo (ADR 0019: una corrida = una bolsa = un proceso; dos canales en
// el mismo periodo son dos bolsas y dos procesos).
//
// Es lo que dispara TrabajoEjecutarReparto: el trabajo ABRE, no ejecuta
// (ADR 0008 -- el calendario dispara AbrirProcesoDeReparto y de ahi en
// adelante el proceso avanza por firma humana o por AvanzarEtapa). El
// identificador es determinista por bolsa y corrida para que reintentar el
// mismo trabajo (Intentos, no Corrida) llame a [Procesos.IniciarProceso] con
// el mismo ID -- que ya es idempotente -- en vez de abrir un proceso
// duplicado por bolsa.
func (uc Procesos) AbrirCorridaDelPeriodo(ctx context.Context, periodo string, corrida int) error {
	bolsas, err := uc.Bolsas.BolsasDePeriodo(ctx, periodo)
	if err != nil {
		return fmt.Errorf("abrir corrida de %q: %w", periodo, err)
	}
	for _, b := range bolsas {
		id := fmt.Sprintf("proc-%s-%d", b.ID, corrida)
		if _, err := uc.IniciarProceso(ctx, id, periodo, b.Circuito, b.ID); err != nil {
			return fmt.Errorf("abrir corrida de %q: bolsa %q: %w", periodo, b.ID, err)
		}
	}
	return nil
}

// AvanzarEtapa mueve el proceso a la siguiente etapa y la persiste. Al
// entrar a EtapaImporteObra del circuito nacional invoca el motor puro de
// #33 -- el internacional nunca la alcanza (RD 7.4), asi que nunca valoriza.
func (uc Procesos) AvanzarEtapa(ctx context.Context, procesoID string) (ProcesoVista, error) {
	v, err := uc.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("avanzar etapa de %q: %w", procesoID, err)
	}
	p, err := unProceso(v).AvanzarEtapa()
	if err != nil {
		return ProcesoVista{}, err
	}

	if p.Circuito == reparto.Nacional && p.Etapa == reparto.EtapaImporteObra {
		// Valorizar y guardar la nueva etapa son un solo hecho: sin la unidad,
		// un fallo entre las dos escrituras deja un resultado ya guardado pero
		// el proceso todavia en deducciones, y un reintento lo volveria a
		// valorizar -- doble escritura del mismo dinero.
		if uc.Unidad == nil {
			return ProcesoVista{}, fmt.Errorf("avanzar etapa de %q: procesos mal cableado: falta UnidadDeTrabajo", procesoID)
		}
		err := uc.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
			if err := uc.valorizar(ctx, p); err != nil {
				return err
			}
			return uc.Repo.GuardarProceso(ctx, aProcesoVista(p))
		})
		if err != nil {
			return ProcesoVista{}, fmt.Errorf("avanzar etapa de %q: %w", procesoID, err)
		}
		return aProcesoVista(p), nil
	}

	if err := uc.Repo.GuardarProceso(ctx, aProcesoVista(p)); err != nil {
		return ProcesoVista{}, fmt.Errorf("avanzar etapa de %q: %w", procesoID, err)
	}
	return aProcesoVista(p), nil
}

// valorizar reune bolsa, usos y declaraciones y llama al motor puro de #33,
// luego persiste el resultado. El snapshot ya esta congelado desde
// [Procesos.IniciarProceso]: recalcular lee ESE snapshot, no vuelve a
// resolverlo (ADR 0004, ADR 0005).
func (uc Procesos) valorizar(ctx context.Context, p reparto.ProcesoDeReparto) error {
	bp, err := uc.Bolsas.BolsaPorID(ctx, p.BolsaID)
	if err != nil {
		return fmt.Errorf("bolsa %q: %w", p.BolsaID, err)
	}
	bolsa, err := recaudo.NuevaBolsa(bp.UsuarioID, bp.Periodo, bp.Circuito, bp.Bruto)
	if err != nil {
		return fmt.Errorf("bolsa %q: %w", p.BolsaID, err)
	}

	// ADR 0019: una corrida = una bolsa = un canal, y el usuario de recaudo de
	// television ES el canal (ver [recaudo.Usuario]).
	usos, _, err := (Reparto{Usos: uc.Usos}).UsosDeCanal(ctx, p.Periodo, bp.UsuarioID)
	if err != nil {
		return fmt.Errorf("usos del canal %q: %w", bp.UsuarioID, err)
	}

	obraIDs := make([]string, 0, len(usos))
	vistos := make(map[string]bool, len(usos))
	for _, u := range usos {
		if vistos[u.ObraID] {
			continue
		}
		vistos[u.ObraID] = true
		obraIDs = append(obraIDs, u.ObraID)
	}
	vigentes, err := uc.Declaraciones.VigentesDeObras(ctx, obraIDs)
	if err != nil {
		return fmt.Errorf("declaraciones vigentes: %w", err)
	}
	// Una obra ausente del mapa queda fuera de decls a proposito: el motor la
	// trata como declaracion_incompleta (R-04) y retiene su importe completo,
	// no como cero puntos silenciosos.
	decls := make([]repertorio.Declaracion, 0, len(vigentes))
	for _, obraID := range obraIDs {
		if vd, ok := vigentes[obraID]; ok {
			decls = append(decls, vd.Declaracion)
		}
	}

	snap, err := uc.Parametros.SnapshotPorID(ctx, p.SnapshotID)
	if err != nil {
		return fmt.Errorf("snapshot %q: %w", p.SnapshotID, err)
	}

	resultado, err := reparto.Reparto(bolsa, usos, snap, decls, reparto.Opciones{SnapshotID: p.SnapshotID})
	if err != nil {
		return fmt.Errorf("motor de reparto: %w", err)
	}
	if err := uc.Resultados.GuardarResultado(ctx, p.ID, resultado); err != nil {
		return fmt.Errorf("guardar resultado: %w", err)
	}
	return nil
}

// Firmar agrega una firma a la compuerta actual del proceso y la persiste.
func (uc Procesos) Firmar(ctx context.Context, procesoID string, rol reparto.RolAcompuerta, actorID string) (ProcesoVista, error) {
	v, err := uc.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("firmar %q: %w", procesoID, err)
	}
	p, err := unProceso(v).Firmar(rol, actorID)
	if err != nil {
		return ProcesoVista{}, err
	}
	// Firmar solo agrega una fila a `firmas`: no toca etapa, revision ni
	// rechazo de `procesos`, asi que no hay una segunda escritura que
	// coordinar con esta.
	nueva := p.Firmas[len(p.Firmas)-1]
	if err := uc.Repo.GuardarFirma(ctx, procesoID, nueva); err != nil {
		return ProcesoVista{}, fmt.Errorf("firmar %q: %w", procesoID, err)
	}
	return aProcesoVista(p), nil
}

// RechazarGate retrocede el proceso una etapa y persiste el rechazo.
func (uc Procesos) RechazarGate(ctx context.Context, procesoID, motivo string) (ProcesoVista, error) {
	v, err := uc.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("rechazar compuerta de %q: %w", procesoID, err)
	}
	p, err := unProceso(v).RechazarGate(motivo)
	if err != nil {
		return ProcesoVista{}, err
	}
	if err := uc.Repo.GuardarProceso(ctx, aProcesoVista(p)); err != nil {
		return ProcesoVista{}, fmt.Errorf("rechazar compuerta de %q: %w", procesoID, err)
	}
	return aProcesoVista(p), nil
}

// ConsultarEstadoProceso es la lectura de solo-consulta que #69 necesita
// para preguntar en que etapa esta una corrida sin escribir en ella.
func (uc Procesos) ConsultarEstadoProceso(ctx context.Context, procesoID string) (ProcesoVista, error) {
	v, err := uc.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return ProcesoVista{}, fmt.Errorf("consultar proceso %q: %w", procesoID, err)
	}
	return v, nil
}

// ListarProcesos es el mismo tipo de lectura que [Procesos.ConsultarEstadoProceso],
// para el panel de corridas.
func (uc Procesos) ListarProcesos(ctx context.Context) ([]ProcesoVista, error) {
	lista, err := uc.Repo.ListarProcesos(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar procesos: %w", err)
	}
	return lista, nil
}
