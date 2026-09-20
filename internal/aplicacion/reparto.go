package aplicacion

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Reparto reune las entradas del motor de valorizacion.
//
// El calculo no vive aqui: es la funcion pura de [reparto.Reparto], que no
// hace E/S ni lee el reloj (ADR 0005). Lo que falta para poder invocarla es el
// camino de lectura, y eso es lo que este caso de uso resuelve: de las filas
// canonicas de `usos` a los [reparto.Uso] que ponderan UNA bolsa.
//
// # Por que el filtro es (periodo, canal) y no solo el periodo
//
// `RD 9.1` reparte el dinero de television "de manera independiente conforme
// al pago que haya hecho cada canal", y el valor punto de `RD 9.1.1` es el
// cociente entre el reparto DE ESE CANAL y sus puntos. El ADR 0019 lo cierra:
// una corrida = una bolsa = un canal. Pedir los usos de un periodo entero
// mezclaria los puntos de dos pagadores en un solo valor punto.
//
// `fuente` no sirve para esto: dice quien ENTREGO el archivo (ADR 0018), no
// quien pago. Una sola entrega de Caracol puede cubrir varios canales.
type Reparto struct {
	Usos RepositorioUsosDeReparto
}

// UsosDeCanal reune los usos que ponderan la bolsa de un canal en un periodo.
//
// Un canal sin filas devuelve la lista vacia y no un error: que un canal no
// haya emitido repertorio en el periodo es un hecho del negocio, no un fallo.
// Un canal VACIO es otra cosa -- no identifica ninguna bolsa, y tratarlo como
// un filtro mas devolveria las filas sin atribuir de todos los pagadores
// mezcladas en una sola corrida -- y por eso es un error tipado.
//
// El [ResumenUsosDeCanal] que devuelve es tan parte del contrato como la
// lista: una fila pendiente, ONI o excluida nunca esta en `usos`, pero
// tampoco desaparece sin dejar rastro (RD 13.8, R-18/R-19).
func (r Reparto) UsosDeCanal(ctx context.Context, periodo, canalID string) ([]reparto.Uso, ResumenUsosDeCanal, error) {
	if strings.TrimSpace(canalID) == "" {
		return nil, ResumenUsosDeCanal{}, fmt.Errorf("usos de %q: %w", periodo, ErrCanalVacio)
	}

	anio, err := anioDeClasificacion(periodo)
	if err != nil {
		return nil, ResumenUsosDeCanal{}, err
	}

	filas, resumen, err := r.Usos.UsosDeCanal(ctx, periodo, canalID, anio)
	if err != nil {
		return nil, ResumenUsosDeCanal{}, fmt.Errorf("usos del canal %q en %q: %w", canalID, periodo, err)
	}

	usos := make([]reparto.Uso, 0, len(filas))
	for _, f := range filas {
		u, err := aUsoDeReparto(f)
		if err != nil {
			return nil, ResumenUsosDeCanal{}, fmt.Errorf("uso %q del canal %q: %w", f.Uso.ID, canalID, err)
		}
		usos = append(usos, u)
	}
	return usos, resumen, nil
}

// UsosSinCanal cuenta los usos de un periodo que ningun canal reclama.
//
// Existe porque hoy ningun adaptador de ingesta puebla `canal_id` con datos
// reales (P-20, docs/dominio/preguntas-cliente.md): mientras esa decision no
// llegue, "cero usos para un canal" no se distingue de "el canal no emitio"
// sin este conteo aparte. Un numero mayor que cero es una senal de que hay
// usos que ninguna corrida va a ponderar nunca, no un fallo en si mismo.
func (r Reparto) UsosSinCanal(ctx context.Context, periodo string) (int, error) {
	n, err := r.Usos.UsosSinCanal(ctx, periodo)
	if err != nil {
		return 0, fmt.Errorf("usos sin canal en %q: %w", periodo, err)
	}
	return n, nil
}

// aUsoDeReparto traduce una fila canonica al uso que consume el motor.
//
// Valida la modalidad en vez de convertir la cadena a secas: una modalidad que
// el motor no conoce cae por el `default` de su switch y vale CERO PUNTOS, que
// es una obra sin pagar en silencio. Aqui es un error tipado.
func aUsoDeReparto(f UsoDeReparto) (reparto.Uso, error) {
	if f.Uso.ObraID == "" {
		return reparto.Uso{}, fmt.Errorf("uso %q: %w", f.Uso.ID, ErrUsoSinObra)
	}

	mod, err := reparto.ParseModalidad(string(f.Uso.Modalidad))
	if err != nil {
		return reparto.Uso{}, err
	}

	u := reparto.Uso{
		ObraID:        f.Uso.ObraID,
		Modalidad:     mod,
		TipoObra:      f.Uso.TipoObra,
		CanalID:       f.Uso.CanalID,
		DuracionMin:   f.Uso.DuracionMin,
		Emisiones:     f.Uso.Emisiones,
		Rating:        f.Uso.Rating,
		Taquilla:      f.Uso.Taquilla,
		Espectadores:  f.Uso.Espectadores,
		Exhibiciones:  f.Uso.Exhibiciones,
		Vistas:        f.Uso.Vistas,
		MinutosVistos: f.Uso.MinutosVistos,
		PB:            f.Uso.PB,
	}

	// Solo suscripcion y hotel se reparten por grupos (RD 9.5, RD 9.6); el
	// motor rechaza un grupo no vacio en cualquier otra modalidad.
	if mod == reparto.Suscripcion || mod == reparto.Hotel {
		g, err := reparto.ParseGrupoCanal(f.GrupoEfectivo)
		if err != nil {
			return reparto.Uso{}, err
		}
		u.Grupo = g
	}
	return u, nil
}

// anioDeClasificacion devuelve el ano contra el que se resuelve el grupo de un
// canal: el inmediatamente anterior al periodo que se reparte (`RD 9.5.4`).
//
// Se calcula sobre el periodo y no sobre el reloj a proposito. Reejecutar un
// periodo de hace tres anos tiene que leer la clasificacion de entonces, no la
// vigente hoy (ADR 0005).
func anioDeClasificacion(periodo string) (int, error) {
	p, err := recaudo.ValidarPeriodo(periodo)
	if err != nil {
		return 0, fmt.Errorf("ano de clasificacion: %w", err)
	}
	anio, err := strconv.Atoi(p[:4])
	if err != nil {
		return 0, fmt.Errorf("ano de clasificacion de %q: %w", periodo, err)
	}
	return anio - 1, nil
}
