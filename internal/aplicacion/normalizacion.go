package aplicacion

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/normalizacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Origenes de la cola de revision. El listado es uno; el tipo dice de cual
// detector salio la fila. "anomalia" lo rellena el #37. "adaptador" son los
// rechazos del mapa de columnas (#25): no se afirman como normalizacion.
const (
	TipoRevisionNormalizacion = "normalizacion"
	TipoRevisionAnomalia      = "anomalia"
	TipoRevisionAdaptador     = "adaptador"
)

// CodigoRechazoFormato es el codigo tipado de un rechazo del adaptador de
// formato. No se re-deriva del texto del motivo (B3).
const CodigoRechazoFormato = "rechazo_formato"

// Normalizacion orquesta el paso de esquema: filas crudas -> forma canonica
// o cola de revision.
//
// El dominio decide QUE fila no es canonica y por que. Este caso de uso
// decide que hacer con cada una: persistirla como uso o moverla al log de
// rechazos, que es la cola de revision de OE-1. No hay un tercer destino.
//
// "Usos sin resolver" aqui son las filas crudas que el adaptador de formato
// acaba de mapear, no las pendientes de identificacion. UsosSinResolver es
// el puerto de la cascada (ADR 0007); mezclarlo aqui reaplicaria el 80%
// sobre una duracion ya transformada.
type Normalizacion struct {
	Reportes RepositorioIngesta
}

// ResultadoLote es el recuento que pide la aceptacion: N canonicas, M en
// revision. Ninguna fila se pierde en el camino.
type ResultadoLote struct {
	Normalizados []UsoPersistido
	Revision     []UsoPersistido
}

// ParametrosDesde extrae del snapshot congelado lo que [normalizacion]
// necesita. Los coeficientes de duracion ausentes no se inventan (ADR 0004):
// el error es ErrParametroAusente. Una fila de cine no los usa, y el dominio
// ya manda a revision la de TV que los necesite y no los tenga; esta funcion
// es la guarda de quien arma el snapshot, no un segundo criterio.
//
// Las monedas reconocidas NO se escriben aqui: salen de s.Tasas (mas la
// base). Listar USD/EUR en Go era dato normativo en codigo (ADR 0004) y
// convertia el euro con la tasa del dolar (B4).
func ParametrosDesde(s reparto.Snapshot) (normalizacion.Parametros, error) {
	p := normalizacion.Parametros{
		DuracionArtisticaPct: s.DuracionArtisticaPct,
		MinutosHoraTV:        s.MinutosHoraTV,
		MonedaBase:           s.MonedaBase,
		Tasas:                s.Tasas,
	}
	faltanTV := !p.DuracionArtisticaPct.GreaterThan(decimal.Zero) ||
		!p.MinutosHoraTV.GreaterThan(decimal.Zero)
	if faltanTV {
		return p, fmt.Errorf("%w: duracion.artistica_pct o duracion.minutos_hora_tv", ErrParametroAusente)
	}
	return p, nil
}

// Procesar normaliza un lote en memoria. No toca el repositorio: es lo que
// hace idempotente re-ejecutarlo, y lo que permite afirmar N + M = len(filas)
// sin una transaccion.
func (n Normalizacion) Procesar(filas []normalizacion.Fila, p normalizacion.Parametros) ResultadoLote {
	out := ResultadoLote{
		Normalizados: make([]UsoPersistido, 0, len(filas)),
		Revision:     make([]UsoPersistido, 0),
	}
	for _, f := range filas {
		uso, rev := normalizacion.Normalizar(f, p)
		persistido := aPersistido(uso, rev)
		if rev != nil {
			out.Revision = append(out.Revision, persistido)
			continue
		}
		out.Normalizados = append(out.Normalizados, persistido)
	}
	return out
}

// ProcesarYGuardar normaliza y persiste. Reejecutarlo con las mismas filas
// (mismos ids) no duplica: las que ya estan en usos o en el log de rechazos
// se saltan. El recuento que devuelve es el del lote logico, no el de la
// escritura de esta pasada.
func (n Normalizacion) ProcesarYGuardar(
	ctx context.Context,
	rep Reporte,
	filas []normalizacion.Fila,
	p normalizacion.Parametros,
) (ResultadoLote, error) {
	resultado := n.Procesar(filas, p)
	if n.Reportes == nil {
		return resultado, nil
	}

	conocidos, err := n.idsYaPersistidos(ctx, append(append([]UsoPersistido{}, resultado.Normalizados...), resultado.Revision...))
	if err != nil {
		return ResultadoLote{}, err
	}

	lote := make([]UsoPersistido, 0, len(filas))
	for _, u := range append(append([]UsoPersistido{}, resultado.Normalizados...), resultado.Revision...) {
		if u.ID != "" && conocidos[u.ID] {
			continue
		}
		lote = append(lote, u)
	}
	if len(lote) == 0 {
		return resultado, nil
	}

	ingesta := Ingesta{Reportes: n.Reportes}
	if _, err := ingesta.GuardarUsos(ctx, rep, lote); err != nil {
		return ResultadoLote{}, fmt.Errorf("persistir el lote normalizado del reporte %q: %w", rep.ID, err)
	}
	return resultado, nil
}

// ListarRevision es la cola compartida con las anomalias del #37.
// tipo y codigo salen de columnas propias, no se re-derivan del texto del
// motivo (B3). Un listado vacio es una lista vacia, no nil.
func (n Normalizacion) ListarRevision(ctx context.Context) ([]ItemRevision, error) {
	if n.Reportes == nil {
		return []ItemRevision{}, nil
	}
	rechazos, err := n.Reportes.ListarRechazos(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar la cola de revision: %w", err)
	}
	items := make([]ItemRevision, 0, len(rechazos))
	for _, u := range rechazos {
		tipo := u.RechazoTipo
		if tipo == "" {
			// Filas anteriores a la migracion de codigo/tipo: no afirmar
			// normalizacion si no consta.
			tipo = TipoRevisionAdaptador
		}
		codigo := u.RechazoCodigo
		if codigo == "" {
			codigo = CodigoRechazoFormato
		}
		items = append(items, ItemRevision{
			ID:        u.ID,
			Tipo:      tipo,
			Codigo:    codigo,
			Motivo:    u.RechazoMotivo,
			Fuente:    u.Fuente,
			Titulo:    u.Titulo,
			ReporteID: u.ReporteID,
		})
	}
	return items, nil
}

func (n Normalizacion) idsYaPersistidos(ctx context.Context, lote []UsoPersistido) (map[string]bool, error) {
	conocidos := map[string]bool{}
	ids := make([]string, 0, len(lote))
	for _, u := range lote {
		if u.ID != "" {
			ids = append(ids, u.ID)
		}
	}
	if len(ids) == 0 {
		return conocidos, nil
	}
	if con, ok := n.Reportes.(interface {
		UsosPorIDs(context.Context, []string) (map[string]UsoPersistido, error)
	}); ok {
		usos, err := con.UsosPorIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("leer usos para idempotencia: %w", err)
		}
		for id := range usos {
			conocidos[id] = true
		}
	} else {
		for _, id := range ids {
			if _, err := n.Reportes.UsoPorID(ctx, id); err == nil {
				conocidos[id] = true
			}
		}
	}
	rechazos, err := n.Reportes.ListarRechazos(ctx)
	if err != nil {
		return nil, fmt.Errorf("leer rechazos para idempotencia: %w", err)
	}
	for _, u := range rechazos {
		if u.ID != "" {
			conocidos[u.ID] = true
		}
	}
	return conocidos, nil
}

// aFila lleva un [UsoPersistido] del adaptador a la [normalizacion.Fila]
// cruda que el dominio espera. Las medidas ya tipadas se re-serializan a
// texto: el dominio es quien decide como interpretarlas.
func aFila(u UsoPersistido) normalizacion.Fila {
	f := normalizacion.Fila{
		ID:             u.ID,
		Fuente:         u.Fuente,
		Modalidad:      string(u.Modalidad),
		Titulo:         u.Titulo,
		TituloOrig:     u.TituloOrig,
		IDsFuente:      u.IDsFuente,
		TipoObra:       u.TipoObra,
		CanalID:        u.CanalID,
		Fecha:          u.Fecha,
		Hora:           u.Hora,
		UnidadDuracion: u.UnidadDuracion,
		Moneda:         u.Moneda,
		Autopromo:      u.Autopromo,
	}
	if u.DuracionTexto != "" {
		f.Duracion = u.DuracionTexto
	} else if !u.DuracionMin.IsZero() {
		f.Duracion = u.DuracionMin.String()
	}
	if u.EmisionesTexto != "" {
		f.Emisiones = u.EmisionesTexto
	} else if u.Emisiones != 0 {
		f.Emisiones = strconv.FormatInt(u.Emisiones, 10)
	}
	if !u.Rating.IsZero() {
		f.Rating = u.Rating.String()
	}
	if !u.Taquilla.IsZero() {
		f.Taquilla = u.Taquilla.String()
	}
	if !u.Espectadores.IsZero() {
		f.Espectadores = u.Espectadores.String()
	}
	if u.Exhibiciones != 0 {
		f.Exhibiciones = strconv.FormatInt(u.Exhibiciones, 10)
	}
	if !u.Vistas.IsZero() {
		f.Vistas = u.Vistas.String()
	}
	if !u.MinutosVistos.IsZero() {
		f.MinutosVistos = u.MinutosVistos.String()
	}
	if !u.PB.IsZero() {
		f.PB = u.PB.String()
	}
	return f
}

func aPersistido(u normalizacion.Uso, rev *normalizacion.Revision) UsoPersistido {
	p := UsoPersistido{
		ID:            u.ID,
		Fuente:        u.Fuente,
		Titulo:        u.Titulo,
		TituloOrig:    u.TituloOrig,
		IDsFuente:     u.IDsFuente,
		Modalidad:     reparto.Modalidad(u.Modalidad),
		TipoObra:      u.TipoObra,
		CanalID:       u.CanalID,
		Fecha:         u.Fecha.String(),
		Hora:          u.Hora,
		DuracionMin:   u.DuracionMin,
		Emisiones:     u.Emisiones,
		Rating:        u.Rating,
		Taquilla:      u.Taquilla,
		Espectadores:  u.Espectadores,
		Exhibiciones:  u.Exhibiciones,
		Vistas:        u.Vistas,
		MinutosVistos: u.MinutosVistos,
		PB:            u.PB,
		Autopromo:     u.Autopromo,
	}
	if rev != nil {
		p.RechazoMotivo = rev.Motivo()
		p.RechazoTipo = TipoRevisionNormalizacion
		p.RechazoCodigo = rev.Codigo
	}
	if u.Emisiones == 0 {
		// Distingue cero explicito (S4) del cero de Go para prepararLote.
		p.EmisionesTexto = "0"
	}
	return p
}

// MarcarRechazoAdaptador estampa el origen tipado de un rechazo del mapa de
// columnas. Sin esto, ListarRevision afirmaba tipo=normalizacion sobre prosa
// del adaptador (B3).
func MarcarRechazoAdaptador(u *UsoPersistido) {
	if u.RechazoMotivo == "" {
		return
	}
	if u.RechazoTipo == "" {
		u.RechazoTipo = TipoRevisionAdaptador
	}
	if u.RechazoCodigo == "" {
		u.RechazoCodigo = CodigoRechazoFormato
	}
}

// cortarMotivo se conserva para pruebas que inspeccionan el texto de
// Revision.Motivo(); la cola de revision ya no lo usa para rellenar codigo.
func cortarMotivo(motivo string) (codigo, detalle string) {
	codigo, detalle, ok := strings.Cut(motivo, ": ")
	if !ok {
		return "", motivo
	}
	return codigo, detalle
}
