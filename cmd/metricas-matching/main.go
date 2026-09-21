// Command metricas-matching mide el escalon 3 contra un conjunto etiquetado.
//
// Corre el motor de similitud y los umbrales REALES sobre los titulos de
// data/etiquetado/matching.json y compara con la obra que cada uno deberia
// resolver. No escribe nada: solo lee y cuenta.
//
// Los umbrales se resuelven contra una fecha, y el informe la dice: por defecto
// HOY (UTC), o la de la variable FECHA (AAAA-MM-DD) para medir el criterio que
// regia en un periodo pasado, que es como los resuelve la cascada (D3). Junto a
// cada parametro imprime desde cuando rige: un cambio de vigencia a mitad de
// camino cambia lo que se mide, y el informe tiene que poder decirlo.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

const etiquetadoPorDefecto = "./data/etiquetado/matching.json"

type conjunto struct {
	Nota  string `json:"nota"`
	Casos []struct {
		Titulo       string `json:"titulo"`
		ObraEsperada string `json:"obra_esperada"`
	} `json:"casos"`
}

func main() {
	log := config.Logger("metricas-matching")
	if err := ejecutar(context.Background()); err != nil {
		log.Error("metricas fallidas", slog.Any("error", err))
		os.Exit(1)
	}
}

func ejecutar(ctx context.Context) error {
	dsn := config.Cadena("DATABASE_URL", "")
	if dsn == "" {
		return errors.New("falta DATABASE_URL")
	}
	c, err := leerConjunto(config.Cadena("ETIQUETADO", etiquetadoPorDefecto))
	if err != nil {
		return err
	}

	ctx, cancelar := context.WithTimeout(ctx, config.Duracion("METRICAS_TIMEOUT", time.Minute))
	defer cancelar()

	store, err := postgres.Abrir(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.CerrarPool()

	// Por defecto hoy: mide el criterio VIGENTE. FECHA mide el de otro dia, que es
	// el que aplico la cascada a un periodo pasado (D3).
	fecha, origen, err := fechaDeResolucion(config.Cadena("FECHA", ""), time.Now().UTC())
	if err != nil {
		return err
	}
	umbrales, err := umbralesVigentes(ctx, store, fecha)
	if err != nil {
		return err
	}
	vigencias, err := vigenciasDelEscalon(ctx, store, fecha)
	if err != nil {
		return err
	}

	evs := make([]identificacion.Evaluacion, 0, len(c.Casos))
	for _, caso := range c.Casos {
		candidatos, err := store.Candidatos(ctx, caso.Titulo, umbrales.Banda)
		if err != nil {
			return fmt.Errorf("candidatos de %q: %w", caso.Titulo, err)
		}
		res := identificacion.Resolver(
			identificacion.Entrada{Titulo: caso.Titulo},
			identificacion.Consulta{Candidatos: candidatos},
			nil, umbrales,
		)
		evs = append(evs, identificacion.Evaluacion{ObraEsperada: caso.ObraEsperada, Resultado: res})
	}

	imprimir(c, evs, umbrales, fecha, origen, vigencias)
	return nil
}

// fechaDeResolucion dice contra que dia se resuelven los umbrales y de donde sale
// ese dia: "FECHA" si la trajo la variable, "hoy" si no. Pura, para poder
// probarla sin base de datos.
func fechaDeResolucion(valor string, hoy time.Time) (time.Time, string, error) {
	if valor == "" {
		return hoy, "hoy", nil
	}
	fecha, err := time.Parse(time.DateOnly, valor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("FECHA=%q no tiene forma AAAA-MM-DD: %w", valor, err)
	}
	return fecha, "FECHA", nil
}

// vigenciasDelEscalon devuelve desde cuando rige cada uno de los dos parametros
// del escalon 3 en la fecha. Sale de Store.Vigentes -la lectura de
// administracion del ADR 0004- y no de un metodo nuevo del Store.
func vigenciasDelEscalon(ctx context.Context, s *postgres.Store, fecha time.Time) ([]aplicacion.FilaParametro, error) {
	filas, err := s.Vigentes(ctx, fecha)
	if err != nil {
		return nil, fmt.Errorf("vigencia de los parametros del escalon 3: %w", err)
	}
	return soloDelEscalon(filas), nil
}

// soloDelEscalon se queda con los dos parametros de la cascada difusa, en el
// orden en que se imprimen: primero el umbral y despues el piso de la banda.
func soloDelEscalon(filas []aplicacion.FilaParametro) []aplicacion.FilaParametro {
	var out []aplicacion.FilaParametro
	for _, clave := range []string{aplicacion.ClaveUmbralMatch, aplicacion.ClaveUmbralBanda} {
		if i := slices.IndexFunc(filas, func(f aplicacion.FilaParametro) bool { return f.Clave == clave }); i >= 0 {
			out = append(out, filas[i])
		}
	}
	return out
}

func umbralesVigentes(ctx context.Context, s *postgres.Store, fecha time.Time) (identificacion.Umbrales, error) {
	match, err := s.ParametroVigente(ctx, aplicacion.ClaveUmbralMatch, fecha)
	if err != nil {
		return identificacion.Umbrales{}, err
	}
	banda, err := s.ParametroVigente(ctx, aplicacion.ClaveUmbralBanda, fecha)
	if err != nil {
		return identificacion.Umbrales{}, err
	}
	// Los mismos cortes que aplica la cascada, con la misma comprobacion: medir
	// contra unos umbrales que ResolverUsos rechazaria seria medir otra cosa.
	u := identificacion.Umbrales{Match: match, Banda: banda}
	if err := u.Validar(); err != nil {
		return identificacion.Umbrales{}, fmt.Errorf("parametros incoherentes (%s, %s): %w",
			aplicacion.ClaveUmbralBanda, aplicacion.ClaveUmbralMatch, err)
	}
	return u, nil
}

func leerConjunto(ruta string) (conjunto, error) {
	bruto, err := os.ReadFile(ruta)
	if err != nil {
		return conjunto{}, fmt.Errorf("leer el conjunto etiquetado: %w", err)
	}
	var c conjunto
	if err := json.Unmarshal(bruto, &c); err != nil {
		return conjunto{}, fmt.Errorf("%s: %w", ruta, err)
	}
	if len(c.Casos) == 0 {
		return conjunto{}, fmt.Errorf("%s no trae ningun caso", ruta)
	}
	return c, nil
}

func imprimir(c conjunto, evs []identificacion.Evaluacion, u identificacion.Umbrales, fecha time.Time, origen string, vigencias []aplicacion.FilaParametro) {
	r := identificacion.Metricas(evs)

	fmt.Printf("fecha de resolucion: %s (%s)\n", fecha.Format(time.DateOnly), origen)
	for _, v := range vigencias {
		valor := u.Match
		if v.Clave == aplicacion.ClaveUmbralBanda {
			valor = u.Banda
		}
		fmt.Printf("  %-22s %s  vigente desde %s\n", v.Clave, valor.StringFixed(2), v.VigenteDesde.Format(time.DateOnly))
	}
	fmt.Printf("etiquetados=%d  automaticas=%d (%s%% de lo que se intento identificar)  aciertos=%d  fallos=%d  precision=%s%%\n",
		r.Total, r.Automaticas, r.TasaAutoAsociacionPct().StringFixed(1),
		r.Aciertos, r.Fallos, r.PrecisionPct().StringFixed(1))
	fmt.Printf("banda=%d  oni=%d  excluidas=%d  manuales=%d\n\n", r.ABanda, r.AONI, r.Excluidas, r.Manuales)

	for i, ev := range evs {
		fmt.Printf("  %-24s %s\n", c.Casos[i].Titulo, desenlace(ev))
	}

	fmt.Printf("\n%s\n", c.Nota)
}

func desenlace(ev identificacion.Evaluacion) string {
	switch {
	case ev.Resultado.ObraID != "" && ev.Resultado.ObraID == ev.ObraEsperada:
		return "OK    " + ev.Resultado.ObraID + " (" + ev.Resultado.Puntaje.StringFixed(4) + ")"
	case ev.Resultado.ObraID != "":
		return "FALLO " + ev.Resultado.ObraID + " en vez de " + vacioOEsperada(ev.ObraEsperada)
	case len(ev.Resultado.Candidatos) > 0:
		return fmt.Sprintf("BANDA %d candidatos, mejor %s", len(ev.Resultado.Candidatos), ev.Resultado.Candidatos[0].ObraID)
	default:
		return "ONI"
	}
}

func vacioOEsperada(esperada string) string {
	if esperada == "" {
		return "nada"
	}
	return esperada
}
