package postgres

import (
	"fmt"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
)

// sembrarMuchasCriticas escribe n alertas CRITICAS y abiertas en el periodo,
// saltandose el caso de uso: lo que se prueba aqui es la lectura, y sembrar por
// `Evaluar` exigiria fabricar n anomalias de verdad.
//
// `duplicado_registro` es critica ([anomalias.EsCritica]) y `ref_titular` se
// queda vacia, que es lo que el CHECK alerta_ref_titular_solo_de_titulares
// exige de todo lo que no sea `titular_sin_porcentaje`.
func sembrarMuchasCriticas(t *testing.T, s *Store, periodo string, n int) {
	t.Helper()

	alertas := make([]aplicacion.Alerta, 0, n)
	for i := range n {
		alertas = append(alertas, aplicacion.Alerta{
			Periodo:   periodo,
			Tipo:      anomalias.TipoDuplicadoRegistro,
			RefTipo:   anomalias.RefUso,
			RefID:     fmt.Sprintf("uso-%04d", i),
			Detalle:   "sembrada para medir la paginacion",
			Detectada: instanteAlertas,
		})
	}
	nuevas, err := s.GuardarAlertas(t.Context(), alertas)
	if err != nil {
		t.Fatalf("sembrar %d alertas: %v", n, err)
	}
	if nuevas != n {
		t.Fatalf("entraron %d alertas de %d", nuevas, n)
	}
}

// TestLaCompuertaCuentaMasAlertasQueUnaPagina es la guarda de que la
// paginacion de `GET /alertas` NO se puede llevar por delante la cifra que
// decide si un periodo se reparte.
//
// El modo de fallo es concreto y silencioso: si alguien reimplementa
// `CriticasAbiertas` sobre `ListarAlertas` -- que es la tentacion obvia, porque
// ya devuelve las alertas del periodo -- la cuenta se corta en
// [aplicacion.LimiteObrasPorDefecto] y un periodo con 250 criticas abiertas
// informa de 100. Peor: pedida la segunda pagina de un periodo limpio informa
// de 0. Una compuerta que cuenta DE MENOS abre el paso al reparto, que es la
// unica direccion en la que puede fallar sin que nadie se entere. Es el mismo
// modo de fallo que el cardinality(NULL) que ya obligo a un COALESCE aqui al
// lado.
//
// Por eso `ContarAlertasSinResolver` es un COUNT(*) en la base y no un len()
// de la lista, y por eso esto siembra MAS alertas que el tamano de pagina.
func TestLaCompuertaCuentaMasAlertasQueUnaPagina(t *testing.T) {
	s, _ := sembrar(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	// Mas que el tope por defecto, para que una cuenta paginada no pueda dar
	// la cifra correcta por casualidad.
	const cuantas = aplicacion.LimiteObrasPorDefecto + 150
	sembrarMuchasCriticas(t, s, periodoAlertas, cuantas)

	abiertas, err := svc.CriticasAbiertas(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("CriticasAbiertas: %v", err)
	}
	if abiertas != cuantas {
		t.Fatalf("CriticasAbiertas = %d, se esperaban %d.\n"+
			"Si la cifra se parece al tamano de pagina (%d), la cuenta se esta haciendo sobre el listado paginado: "+
			"una compuerta que cuenta de menos deja repartir un periodo bloqueado.",
			abiertas, cuantas, aplicacion.LimiteObrasPorDefecto)
	}

	// Y el listado, que SI pagina, no devuelve las 250 de golpe.
	pagina, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("ListarAlertas: %v", err)
	}
	if len(pagina) != aplicacion.LimiteObrasPorDefecto {
		t.Fatalf("la pagina por defecto trajo %d filas, se esperaban %d",
			len(pagina), aplicacion.LimiteObrasPorDefecto)
	}
}

// TestListarAlertasSinPaginacionExplicitaNoDevuelveCero fija el contrato de
// `Paginacion` cero en el sitio donde mas caro sale equivocarse.
//
// `Paginacion{}` significa "el por defecto" (ver su godoc), NO "cero filas".
// Sin el ConDefecto() del adaptador, el Limite en cero se traducia a `LIMIT 0`
// y toda llamada directa -- que es como llaman las pruebas de este paquete y
// como llamaria cualquier consumidor interno que no piense en paginar --
// recibia la lista VACIA. Sobre esta tabla eso se lee como "el periodo esta
// limpio", que es la respuesta mas cara que puede dar.
func TestListarAlertasSinPaginacionExplicitaNoDevuelveCero(t *testing.T) {
	s, _ := sembrar(t)
	sembrarMuchasCriticas(t, s, periodoAlertas, 3)

	alertas, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("ListarAlertas: %v", err)
	}
	if len(alertas) != 3 {
		t.Fatalf("ListarAlertas sin paginacion explicita devolvio %d de 3 alertas: "+
			"el cero de Paginacion tiene que significar el defecto, no LIMIT 0", len(alertas))
	}
}

// TestListarAlertasPaginaYLimiteSinTopeTraeTodo cubre las dos direcciones del
// recorte: que la pagina recorta de verdad y que hay forma EXPLICITA de pedir
// el listado entero.
//
// Las dos paginas no se pueden solapar ni saltarse una fila, y eso lo sostiene
// el desempate por id del ORDER BY: con `detectada` solo -- que es el mismo
// instante para toda una pasada, a proposito -- el orden entre filas empatadas
// es arbitrario y OFFSET devuelve cualquier cosa.
func TestListarAlertasPaginaYLimiteSinTopeTraeTodo(t *testing.T) {
	s, _ := sembrar(t)
	const cuantas = 10
	sembrarMuchasCriticas(t, s, periodoAlertas, cuantas)

	pagina := func(limite, desplazamiento int) []aplicacion.Alerta {
		t.Helper()
		out, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
			Periodo:    periodoAlertas,
			Paginacion: aplicacion.Paginacion{Limite: limite, Desplazamiento: desplazamiento},
		})
		if err != nil {
			t.Fatalf("ListarAlertas(limite=%d, desplazamiento=%d): %v", limite, desplazamiento, err)
		}
		return out
	}

	primera, segunda := pagina(4, 0), pagina(4, 4)
	if len(primera) != 4 || len(segunda) != 4 {
		t.Fatalf("paginas de %d y %d filas, se esperaban 4 y 4", len(primera), len(segunda))
	}
	vistos := make(map[string]bool, 8)
	for _, a := range append(append([]aplicacion.Alerta{}, primera...), segunda...) {
		if vistos[a.ID] {
			t.Fatalf("la alerta %q salio en las dos paginas: el orden no es total", a.ID)
		}
		vistos[a.ID] = true
	}

	// Pedir TODO se dice, no se obtiene dejando el campo a cero.
	todas := pagina(aplicacion.LimiteSinTope, 0)
	if len(todas) != cuantas {
		t.Fatalf("LimiteSinTope devolvio %d de %d alertas", len(todas), cuantas)
	}
}
