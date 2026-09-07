package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// quienCascada identifica el aprendizaje automatico en alias_obra.quien: es
// lo que distingue un alias que aprendio la cascada de uno que aprendio un
// usuario resolviendo a mano en la cola manual (#39, que usara el mismo
// puerto con el id de ese usuario).
const quienCascada = "cascada"

// parCanonicoPorFuente es la clave local preferida para el escalon 1 (D2 del
// diseno): la obra de REDES es el programa/show, no el capitulo, y esta
// tabla fija que columna identifica el show para cada fuente conocida.
var parCanonicoPorFuente = map[string]string{
	"caracol": "id_ficha",
	"netflix": "show_id",
}

// ResolverUsos corre los escalones 1-2 de la cascada del ADR 0007, mas el
// filtro de repertorio (R-27), sobre las filas pendientes de un periodo.
//
// No implementa la lectura de usos: consume RepositorioIngesta.UsosDePeriodo,
// declarado en puertos.go, cuyo adaptador de PostgreSQL trae #72 (abierta al
// escribir este caso de uso). Los tests de este fichero usan un doble; los de
// integracion (en el paquete postgres) combinan el Store real de
// RepositorioIdentificacion con un doble local para esta lectura.
type ResolverUsos struct {
	Usos           RepositorioIngesta
	Identificacion RepositorioIdentificacion
	// FueraDeRepertorio son las fuentes excluidas por R-27. Vacio en
	// produccion hasta que exista el dato de politica (D4 del diseno de #28):
	// nada se excluye por defecto.
	FueraDeRepertorio identificacion.FuentesExcluidas
}

// ResolverUsos resuelve las filas pendientes del periodo y devuelve cuantas
// resolvio. Las que no resuelvan quedan escalon='pendiente': es el insumo
// del escalon difuso (#32), no un fallo de esta corrida.
//
// Idempotente (D9): una fila que ya no este pendiente -ya resuelta, excluida
// en una corrida anterior, o clasificada a mano- no se vuelve a tocar.
func (r ResolverUsos) ResolverUsos(ctx context.Context, periodo string) (int, error) {
	usos, err := r.Usos.UsosDePeriodo(ctx, periodo)
	if err != nil {
		return 0, fmt.Errorf("leer los usos del periodo %q: %w", periodo, err)
	}

	resueltas := 0
	for _, u := range usos {
		if u.Escalon != "pendiente" {
			continue
		}

		e := entradaDesdeUso(u)
		res, err := r.resolverFila(ctx, u, e)
		if err != nil {
			return resueltas, err
		}
		if res.ObraID == "" {
			// Excluida (D4) o no resuelta (insumo de #32): no se escribe nada.
			continue
		}

		// Orden fijo (D5): primero el alias, despues el match. Si la corrida
		// muere entre los dos, el reintento re-resuelve la fila por escalon 1
		// -el alias ya quedo- y converge; al reves, la fila quedaria resuelta
		// sin que el conocimiento se aprendiera nunca.
		//
		// Sin par local (e.TipoID/e.ValorID vacios) no hay nada que aprender:
		// una fila que solo trae un identificador global no tiene id de
		// fuente al que asociar el alias. Sin este chequeo, GuardarAlias se
		// llamaria con valor="" -el CHECK btrim(valor) <> '' de alias_obra lo
		// rechaza- y ese error, al no ser ErrNoEncontrado, abortaria la
		// corrida entera (D8): ni esta fila ni las siguientes del lote se
		// procesarian, aunque esta si se hubiera identificado bien.
		if res.Escalon == identificacion.EscalonIDGlobal && e.TipoID != "" && e.ValorID != "" {
			if err := r.Identificacion.GuardarAlias(ctx, e.Fuente, e.TipoID, e.ValorID, res.ObraID, quienCascada); err != nil {
				return resueltas, fmt.Errorf("aprender alias de %q: %w", u.ID, err)
			}
		}
		if err := r.Identificacion.GuardarMatch(ctx, u.ID, res); err != nil {
			return resueltas, fmt.Errorf("guardar match de %q: %w", u.ID, err)
		}
		resueltas++
	}
	return resueltas, nil
}

// resolverFila sondea los escalones en orden y deja que
// [identificacion.Resolver] decida. ErrNoEncontrado en un sondeo significa
// "sigue con el siguiente escalon"; cualquier otro error aborta la corrida
// (D8): tragarse un fallo de red como "no hay match" reclasificaria una fila
// en silencio.
func (r ResolverUsos) resolverFila(ctx context.Context, u UsoPersistido, e identificacion.Entrada) (identificacion.Resultado, error) {
	// El filtro de repertorio corre antes que cualquier sondeo (D4: "escalon
	// 0, antes del alias"): una fila fuera de repertorio no consume un
	// sondeo de alias ni de id global, y [identificacion.Resolver] excluiria
	// igual con Consulta vacia -adelantarlo aqui solo evita la E/S que su
	// resultado va a descartar.
	if r.FueraDeRepertorio.Excluye(e.Fuente) {
		return identificacion.Resolver(e, identificacion.Consulta{}, r.FueraDeRepertorio), nil
	}

	c := identificacion.Consulta{}

	if e.TipoID != "" && e.ValorID != "" {
		obraID, err := r.Identificacion.Alias(ctx, e.Fuente, e.TipoID, e.ValorID)
		switch {
		case err == nil:
			c.AliasObraID = obraID
		case errors.Is(err, ErrNoEncontrado):
			// Sin alias: cae al escalon 2.
		default:
			return identificacion.Resultado{}, fmt.Errorf("escalon 1 de %q (%s=%s): %w",
				u.ID, e.TipoID, e.ValorID, err)
		}
	}

	if c.AliasObraID == "" {
		for _, g := range identificacion.OrdenIDGlobal {
			valor := valorGlobal(e, g)
			if valor == "" {
				continue // el escalon 2 no puede casar sin dato para este identificador
			}
			ida, eidr, imdb := idaEidrImdb(e, g)
			obraID, err := r.Identificacion.ObraPorIDGlobal(ctx, ida, eidr, imdb)
			switch {
			case err == nil:
				c.IDGlobalObraID, c.IDGlobalCual = obraID, g
			case errors.Is(err, ErrNoEncontrado):
				// Sigue con el siguiente identificador global.
			default:
				return identificacion.Resultado{}, fmt.Errorf("escalon 2 de %q (%s): %w", u.ID, g, err)
			}
			if c.IDGlobalObraID != "" {
				break
			}
		}
	}

	return identificacion.Resolver(e, c, r.FueraDeRepertorio), nil
}

// valorGlobal devuelve el identificador de e que corresponde a g.
func valorGlobal(e identificacion.Entrada, g identificacion.IDGlobal) string {
	switch g {
	case identificacion.IDA:
		return e.IDA
	case identificacion.EIDR:
		return e.EIDR
	case identificacion.IMDB:
		return e.IMDB
	default:
		return ""
	}
}

// idaEidrImdb arma los tres argumentos de ObraPorIDGlobal con solo g
// poblado: asi el sondeo prueba un identificador a la vez y la evidencia (D6)
// puede nombrar cual caso.
func idaEidrImdb(e identificacion.Entrada, g identificacion.IDGlobal) (ida, eidr, imdb string) {
	switch g {
	case identificacion.IDA:
		ida = e.IDA
	case identificacion.EIDR:
		eidr = e.EIDR
	case identificacion.IMDB:
		imdb = e.IMDB
	}
	return ida, eidr, imdb
}

// entradaDesdeUso construye la Entrada de dominio a partir de una fila
// persistida: parsea ids_fuente (D1) y elige el par local canonico (D2).
func entradaDesdeUso(u UsoPersistido) identificacion.Entrada {
	e := identificacion.Entrada{
		Fuente: u.Fuente,
		Titulo: u.Titulo,
	}

	locales := map[string]string{}
	for _, linea := range strings.Split(u.IDsFuente, "\n") {
		clave, valor, ok := strings.Cut(linea, "=")
		if !ok || clave == "" || valor == "" {
			continue // linea rota: se ignora (D1, el lector es tolerante)
		}
		switch clave {
		case "ida":
			e.IDA = valor
		case "eidr":
			e.EIDR = valor
		case "imdb":
			e.IMDB = valor
		default:
			locales[clave] = valor // clave repetida: gana la ultima (determinista)
		}
	}

	e.TipoID, e.ValorID = parLocalCanonico(u.Fuente, locales)
	return e
}

// parLocalCanonico elige el par que se sondea contra alias_obra en el
// escalon 1 (D2): la clave preferida de la fuente si esta mapeada; si no hay
// mapeo, la unica clave si hay una sola, o la primera en orden alfabetico si
// hay varias (determinismo, D9).
func parLocalCanonico(fuente string, locales map[string]string) (tipo, valor string) {
	if pref, ok := parCanonicoPorFuente[fuente]; ok {
		if v, ok := locales[pref]; ok {
			return pref, v
		}
		return "", ""
	}
	if len(locales) == 0 {
		return "", ""
	}
	claves := make([]string, 0, len(locales))
	for k := range locales {
		claves = append(claves, k)
	}
	slices.Sort(claves)
	return claves[0], locales[claves[0]]
}
