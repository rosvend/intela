// Command metricas-matching mide el escalon 3 contra un conjunto etiquetado.
//
// Corre el motor de similitud y los umbrales REALES sobre los titulos de
// data/etiquetado/matching.json y compara con la obra que cada uno deberia
// resolver. No escribe nada: solo lee y cuenta.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

	// La fecha de hoy: esto mide el criterio VIGENTE, no el de un periodo pasado.
	hoy := time.Now().UTC()
	umbrales, err := umbralesVigentes(ctx, store, hoy)
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

	imprimir(c, evs, umbrales, hoy)
	return nil
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

func imprimir(c conjunto, evs []identificacion.Evaluacion, u identificacion.Umbrales, fecha time.Time) {
	r := identificacion.Metricas(evs)

	fmt.Printf("umbral=%s  banda=%s  (vigentes el %s)\n",
		u.Match.StringFixed(2), u.Banda.StringFixed(2), fecha.Format(time.DateOnly))
	fmt.Printf("etiquetados=%d  automaticas=%d (%s%%)  aciertos=%d  fallos=%d  precision=%s%%\n",
		r.Total, r.Automaticas, r.TasaAutoAsociacionPct().StringFixed(1),
		r.Aciertos, r.Fallos, r.PrecisionPct().StringFixed(1))
	fmt.Printf("banda=%d  oni=%d  excluidas=%d\n\n", r.ABanda, r.AONI, r.Excluidas)

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
