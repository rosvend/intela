package anomalias

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// ---------------------------------------------------------------------------
// Acciones admitidas por tipo (#164)

func TestAccionesDeCadaTipo(t *testing.T) {
	t.Parallel()

	casos := []struct {
		tipo   string
		quiero []string
	}{
		{TipoDuplicadoRegistro, []string{AccionExcluirUso, AccionAceptarTalCual}},
		{TipoDuplicadoArchivo, []string{AccionExcluirEntrega, AccionAceptarTalCual}},
		// Aceptar tal cual una fila sin tipo abriria la compuerta hacia un
		// motor que aborta la corrida igual (ErrRepartoInvalido).
		{TipoTipoObraSinMapear, []string{AccionAsignarTipoObra}},
		{TipoONI, nil},
		{TipoTitularSinPorcentaje, nil},
		{TipoReservaDeclaracionIncompleta, nil},
	}
	for _, c := range casos {
		t.Run(c.tipo, func(t *testing.T) {
			t.Parallel()
			if got := AccionesDe(c.tipo); !slices.Equal(got, c.quiero) {
				t.Fatalf("AccionesDe(%q) = %v, se esperaba %v", c.tipo, got, c.quiero)
			}
		})
	}
}

// Toda critica tiene al menos una accion y ninguna no critica tiene alguna: si
// manana un tipo pasa a critico sin accion, cerrarlo seria imposible, y si una
// no critica ganara accion, el cierre tocaria un dato que no le toca.
func TestLasCriticasSonExactamenteLasQueTienenAccion(t *testing.T) {
	t.Parallel()
	for _, tipo := range Tipos() {
		if EsCritica(tipo) != (len(AccionesDe(tipo)) > 0) {
			t.Errorf("tipo %q: critica=%v, acciones=%v", tipo, EsCritica(tipo), AccionesDe(tipo))
		}
	}
}

func TestAccionesListaLasCuatro(t *testing.T) {
	t.Parallel()
	quiero := []string{"excluir_uso", "excluir_entrega", "asignar_tipo_obra", "aceptar_tal_cual"}
	if got := Acciones(); !slices.Equal(got, quiero) {
		t.Fatalf("Acciones() = %v, se esperaba %v (el CHECK alerta_accion_valida de 00025 los fija)", got, quiero)
	}
}

// ---------------------------------------------------------------------------
// ValidarForma

func TestValidarFormaRecortaYNormaliza(t *testing.T) {
	t.Parallel()

	got, err := ValidarForma(PedidoDeCorreccion{Accion: " asignar_tipo_obra ", TipoObra: " Serie "})
	if err != nil {
		t.Fatalf("ValidarForma: %v", err)
	}
	if got.Accion != AccionAsignarTipoObra || got.TipoObra != "serie" {
		t.Fatalf("pedido = %+v, se esperaba la accion recortada y el tipo en minusculas", got)
	}
}

func TestValidarFormaRechazaLoQueNoCuadra(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		pedido PedidoDeCorreccion
	}{
		{"accion desconocida", PedidoDeCorreccion{Accion: "borrar"}},
		{"objetivo sin accion", PedidoDeCorreccion{UsoID: "u1"}},
		{"aceptar con objetivo", PedidoDeCorreccion{Accion: AccionAceptarTalCual, ReporteID: "r1"}},
		{"excluir uso con reporte", PedidoDeCorreccion{Accion: AccionExcluirUso, ReporteID: "r1"}},
		{"excluir uso con tipo", PedidoDeCorreccion{Accion: AccionExcluirUso, TipoObra: "serie"}},
		{"excluir entrega con uso", PedidoDeCorreccion{Accion: AccionExcluirEntrega, UsoID: "u1"}},
		{"asignar tipo con uso", PedidoDeCorreccion{Accion: AccionAsignarTipoObra, UsoID: "u1", TipoObra: "serie"}},
		{"asignar tipo sin tipo", PedidoDeCorreccion{Accion: AccionAsignarTipoObra}},
		// RD 9.1.1 pondera por categorias cerradas: un tipo inventado
		// abortaria el motor igual que uno vacio.
		{"asignar tipo inventado", PedidoDeCorreccion{Accion: AccionAsignarTipoObra, TipoObra: "documental"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if _, err := ValidarForma(c.pedido); !errors.Is(err, ErrAccionInvalida) {
				t.Fatalf("ValidarForma(%+v) = %v, se esperaba ErrAccionInvalida", c.pedido, err)
			}
		})
	}
}

func TestValidarFormaAdmiteLoQueCadaAccionLee(t *testing.T) {
	t.Parallel()

	for _, p := range []PedidoDeCorreccion{
		{},
		{Accion: AccionAceptarTalCual},
		{Accion: AccionExcluirUso},
		{Accion: AccionExcluirUso, UsoID: "u1"},
		{Accion: AccionExcluirEntrega},
		{Accion: AccionExcluirEntrega, ReporteID: "r1"},
		{Accion: AccionAsignarTipoObra, TipoObra: "telenovela"},
	} {
		if _, err := ValidarForma(p); err != nil {
			t.Errorf("ValidarForma(%+v) = %v, se esperaba nil", p, err)
		}
	}
}

// ---------------------------------------------------------------------------
// CorreccionPara

func TestUnaCriticaSinAccionNoSeCierra(t *testing.T) {
	t.Parallel()

	for _, tipo := range []string{TipoDuplicadoRegistro, TipoDuplicadoArchivo, TipoTipoObraSinMapear} {
		_, err := CorreccionPara(tipo, "ref-1", PedidoDeCorreccion{})
		if !errors.Is(err, ErrAccionInvalida) {
			t.Fatalf("%s sin accion dio %v, se esperaba ErrAccionInvalida", tipo, err)
		}
		// El mensaje dice que accion falta: quien lo lee en el tablero tiene
		// que saber que mandar sin abrir el codigo.
		for _, accion := range AccionesDe(tipo) {
			if !strings.Contains(err.Error(), accion) {
				t.Errorf("%s: el mensaje %q no nombra la accion %q", tipo, err, accion)
			}
		}
	}
}

func TestUnaNoCriticaSeCierraSoloConLaNota(t *testing.T) {
	t.Parallel()

	c, err := CorreccionPara(TipoONI, "u1", PedidoDeCorreccion{})
	if err != nil || c != (Correccion{}) {
		t.Fatalf("una ONI sin accion dio %+v, %v; se esperaba un cierre sin correccion", c, err)
	}
	if _, err := CorreccionPara(TipoONI, "u1", PedidoDeCorreccion{Accion: AccionExcluirUso}); !errors.Is(err, ErrAccionInvalida) {
		t.Fatalf("una ONI con accion dio %v, se esperaba ErrAccionInvalida", err)
	}
}

func TestUnaAccionQueElTipoNoAdmiteSeRechaza(t *testing.T) {
	t.Parallel()

	casos := []struct {
		tipo   string
		accion string
	}{
		{TipoDuplicadoRegistro, AccionExcluirEntrega},
		{TipoDuplicadoArchivo, AccionExcluirUso},
		{TipoTipoObraSinMapear, AccionAceptarTalCual},
		{TipoTipoObraSinMapear, AccionExcluirUso},
	}
	for _, c := range casos {
		if _, err := CorreccionPara(c.tipo, "ref-1", PedidoDeCorreccion{Accion: c.accion}); !errors.Is(err, ErrAccionInvalida) {
			t.Errorf("%s con %s dio %v, se esperaba ErrAccionInvalida", c.tipo, c.accion, err)
		}
	}
}

func TestElObjetivoPorDefectoEsElRegistroDeLaAlerta(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		tipo   string
		pedido PedidoDeCorreccion
		quiero Correccion
	}{
		{"excluir el uso de la alerta", TipoDuplicadoRegistro,
			PedidoDeCorreccion{Accion: AccionExcluirUso}, Correccion{AccionExcluirUso, "ref-1"}},
		{"excluir la otra copia", TipoDuplicadoRegistro,
			PedidoDeCorreccion{Accion: AccionExcluirUso, UsoID: "u-otra"}, Correccion{AccionExcluirUso, "u-otra"}},
		{"excluir la entrega de la alerta", TipoDuplicadoArchivo,
			PedidoDeCorreccion{Accion: AccionExcluirEntrega}, Correccion{AccionExcluirEntrega, "ref-1"}},
		{"excluir la otra entrega", TipoDuplicadoArchivo,
			PedidoDeCorreccion{Accion: AccionExcluirEntrega, ReporteID: "r-otra"}, Correccion{AccionExcluirEntrega, "r-otra"}},
		{"asignar el tipo", TipoTipoObraSinMapear,
			PedidoDeCorreccion{Accion: AccionAsignarTipoObra, TipoObra: "serie"}, Correccion{AccionAsignarTipoObra, "serie"}},
		// Aceptar no actua sobre ningun registro: el CHECK
		// alerta_accion_tiene_objetivo lo exige vacio.
		{"aceptar tal cual", TipoDuplicadoArchivo,
			PedidoDeCorreccion{Accion: AccionAceptarTalCual}, Correccion{AccionAceptarTalCual, ""}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			got, err := CorreccionPara(c.tipo, "ref-1", c.pedido)
			if err != nil {
				t.Fatalf("CorreccionPara: %v", err)
			}
			if got != c.quiero {
				t.Fatalf("correccion = %+v, se esperaba %+v", got, c.quiero)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidarNota

func TestLaNotaDeUnaCorreccionTieneElTopeDeLaResolucionManual(t *testing.T) {
	t.Parallel()

	larga := strings.Repeat("ñ", MaxNotaCorreccion+1)
	justa := strings.Repeat("ñ", MaxNotaCorreccion)

	// Las que escriben en `usos.nota_resolucion` topan (CHECK de 00022).
	for _, accion := range []string{AccionExcluirUso, AccionExcluirEntrega, AccionAsignarTipoObra} {
		if err := ValidarNota(Correccion{Accion: accion}, larga); !errors.Is(err, ErrNotaDemasiadoLarga) {
			t.Errorf("%s con nota de %d runas dio %v", accion, MaxNotaCorreccion+1, err)
		}
		// En runas, no en bytes: una nota en espanol no se corta antes de tiempo.
		if err := ValidarNota(Correccion{Accion: accion}, justa); err != nil {
			t.Errorf("%s con nota de %d runas dio %v", accion, MaxNotaCorreccion, err)
		}
	}
	// Las que no tocan `usos` no topan aqui.
	for _, c := range []Correccion{{}, {Accion: AccionAceptarTalCual}} {
		if err := ValidarNota(c, larga); err != nil {
			t.Errorf("%+v con nota larga dio %v", c, err)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidarExclusionDeUso

const claveDup = "id_ficha=7|fecha=2025-01-02|hora=20:00:00"

func usosDuplicados() []Uso {
	return []Uso{
		{ID: "u1", ReporteID: "r1", Fuente: "caracol", Escalon: identificacion.EscalonAlias, ObraID: "o1", ClaveRegistro: claveDup},
		{ID: "u2", ReporteID: "r2", Fuente: "caracol", Escalon: identificacion.EscalonAlias, ObraID: "o1", ClaveRegistro: claveDup},
		// Misma clave en otra fuente: no es el mismo registro.
		{ID: "u3", ReporteID: "r3", Fuente: "netflix", Escalon: identificacion.EscalonAlias, ObraID: "o1", ClaveRegistro: claveDup},
		// La misma emision a otra hora: otro registro.
		{ID: "u4", ReporteID: "r1", Fuente: "caracol", Escalon: identificacion.EscalonAlias, ObraID: "o1",
			ClaveRegistro: "id_ficha=7|fecha=2025-01-02|hora=21:00:00"},
	}
}

func TestExcluirCualquieraDeLasDosCopias(t *testing.T) {
	t.Parallel()

	// La alerta cae sobre u2 (la que llego segunda), pero la que no manda
	// puede ser u1: la entrega corregida puede ser la segunda.
	for _, objetivo := range []string{"u2", "u1"} {
		got, err := ValidarExclusionDeUso(usosDuplicados(), "u2", objetivo)
		if err != nil {
			t.Fatalf("excluir %s: %v", objetivo, err)
		}
		if got.ID != objetivo {
			t.Fatalf("excluir %s devolvio %s", objetivo, got.ID)
		}
	}
}

func TestExcluirUnUsoQueNoRepiteElRegistroSeRechaza(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		objetivo string
	}{
		{"otra fuente con la misma clave", "u3"},
		{"otra hora del mismo programa", "u4"},
		{"una fila que no es del periodo", "u-fantasma"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if _, err := ValidarExclusionDeUso(usosDuplicados(), "u2", c.objetivo); !errors.Is(err, ErrAccionInvalida) {
				t.Fatalf("excluir %s dio %v, se esperaba ErrAccionInvalida", c.objetivo, err)
			}
		})
	}
}

func TestExcluirUnUsoQueYaNoAplica(t *testing.T) {
	t.Parallel()

	t.Run("la fila de la alerta ya no esta", func(t *testing.T) {
		t.Parallel()
		if _, err := ValidarExclusionDeUso(usosDuplicados(), "u-borrado", "u1"); !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
	})
	t.Run("la fila ya se excluyo", func(t *testing.T) {
		t.Parallel()
		usos := usosDuplicados()
		usos[1].Escalon, usos[1].ObraID = identificacion.EscalonDuplicado, ""
		usos = append(usos, Uso{ID: "u5", ReporteID: "r5", Fuente: "caracol", Escalon: identificacion.EscalonAlias, ClaveRegistro: claveDup})
		if _, err := ValidarExclusionDeUso(usos, "u2", "u2"); !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
	})
	// El caso que mas caro sale: excluir la UNICA copia que queda borra la
	// emision del reparto en vez de desduplicarla.
	t.Run("es la unica copia que queda en juego", func(t *testing.T) {
		t.Parallel()
		usos := usosDuplicados()
		usos[0].Escalon, usos[0].ObraID = identificacion.EscalonDuplicado, ""
		_, err := ValidarExclusionDeUso(usos, "u2", "u2")
		if !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
		if !strings.Contains(err.Error(), "sin contar") {
			t.Errorf("el mensaje no dice por que: %v", err)
		}
	})
	// Una copia descartada (#175) tampoco cuenta como la que queda: no pondera.
	t.Run("la otra copia esta descartada", func(t *testing.T) {
		t.Parallel()
		usos := usosDuplicados()
		usos[0].Escalon, usos[0].ObraID = identificacion.EscalonDescartado, ""
		if _, err := ValidarExclusionDeUso(usos, "u2", "u2"); !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
	})
}

// Una copia 'excluido' (R-27) si sigue en juego: la cascada la reevalua en
// cada corrida y puede volver a ponderar, asi que es la que queda.
func TestUnaCopiaExcluidaPorR27CuentaComoLaQueQueda(t *testing.T) {
	t.Parallel()

	usos := usosDuplicados()
	usos[0].Escalon, usos[0].ObraID = identificacion.EscalonExcluido, ""
	if _, err := ValidarExclusionDeUso(usos, "u2", "u2"); err != nil {
		t.Fatalf("dio %v, se esperaba nil", err)
	}
}

func TestSigueEnJuego(t *testing.T) {
	t.Parallel()

	fuera := []string{identificacion.EscalonDescartado, identificacion.EscalonDuplicado}
	for _, e := range []string{
		identificacion.EscalonPendiente, identificacion.EscalonAlias, identificacion.EscalonIDGlobal,
		identificacion.EscalonDifuso, identificacion.EscalonManual, identificacion.EscalonONI,
		identificacion.EscalonExcluido, identificacion.EscalonDescartado, identificacion.EscalonDuplicado,
	} {
		if got, quiero := SigueEnJuego(e), !slices.Contains(fuera, e); got != quiero {
			t.Errorf("SigueEnJuego(%q) = %v, se esperaba %v", e, got, quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidarExclusionDeEntrega

func entregasDuplicadas() []Entrega {
	return []Entrega{
		{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
		{ID: "r2", Fuente: "netflix", Periodo: "2025-01", SHA256: "aa"},
		{ID: "r3", Fuente: "cine", Periodo: "2024-12", SHA256: "aa"},
		{ID: "r4", Fuente: "caracol", Periodo: "2025-01", SHA256: "bb"},
	}
}

func TestExcluirCualquieraDeLasDosEntregasDelPeriodo(t *testing.T) {
	t.Parallel()

	for _, objetivo := range []string{"r1", "r2"} {
		got, err := ValidarExclusionDeEntrega(entregasDuplicadas(), "2025-01", "r1", objetivo)
		if err != nil {
			t.Fatalf("excluir %s: %v", objetivo, err)
		}
		if got.ID != objetivo {
			t.Fatalf("excluir %s devolvio %s", objetivo, got.ID)
		}
	}
}

func TestExcluirUnaEntregaQueNoCuadraSeRechaza(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		objetivo string
	}{
		{"otros bytes", "r4"},
		// El cerrojo que serializa la correccion es el del periodo de la
		// alerta: tocar otro mes se colaria entre su compuerta y su calculo.
		{"la otra pata vive en otro periodo", "r3"},
		{"no existe", "r-fantasma"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if _, err := ValidarExclusionDeEntrega(entregasDuplicadas(), "2025-01", "r1", c.objetivo); !errors.Is(err, ErrAccionInvalida) {
				t.Fatalf("excluir %s dio %v, se esperaba ErrAccionInvalida", c.objetivo, err)
			}
		})
	}
}

func TestExcluirUnaEntregaQueYaNoAplica(t *testing.T) {
	t.Parallel()

	t.Run("ya esta excluida", func(t *testing.T) {
		t.Parallel()
		entregas := entregasDuplicadas()
		entregas[0].Excluida = true
		if _, err := ValidarExclusionDeEntrega(entregas, "2025-01", "r1", "r1"); !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
	})
	t.Run("es la unica con esos bytes que sigue en el reparto", func(t *testing.T) {
		t.Parallel()
		entregas := entregasDuplicadas()
		entregas[1].Excluida, entregas[2].Excluida = true, true
		if _, err := ValidarExclusionDeEntrega(entregas, "2025-01", "r1", "r1"); !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
	})
	t.Run("la entrega de la alerta ya no existe", func(t *testing.T) {
		t.Parallel()
		if _, err := ValidarExclusionDeEntrega(entregasDuplicadas(), "2025-01", "r-borrada", "r1"); !errors.Is(err, ErrAccionNoAplica) {
			t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
		}
	})
}

// ---------------------------------------------------------------------------
// La correccion apaga el detector

// Excluir una copia -- la de la alerta o la otra -- deja de levantar el
// duplicado: es lo que hace que la siguiente pasada no vuelva a pedir nada.
func TestExcluirUnaCopiaApagaElDuplicadoDeRegistro(t *testing.T) {
	t.Parallel()

	for _, i := range []int{0, 1} {
		usos := usosDuplicados()
		usos[i].Escalon, usos[i].ObraID = identificacion.EscalonDuplicado, ""
		if got := refsDe(duplicadosPorRegistro(usos)); len(got) != 0 {
			t.Fatalf("con %s excluida siguen saliendo duplicados: %v", usos[i].ID, got)
		}
	}
	// Sin excluir, la alerta cae sobre la segunda.
	if got := refsDe(duplicadosPorRegistro(usosDuplicados())); !slices.Equal(got, []string{"duplicado_registro|uso:u2"}) {
		t.Fatalf("sin correccion = %v", got)
	}
}

// Excluir una entrega apaga la alerta de las DOS patas del par: la de la
// excluida no se levanta y la otra ya no colisiona con nada. Con tres patas,
// las dos que siguen en el reparto se siguen alertando entre si.
func TestExcluirUnaEntregaApagaElDuplicadoDeHuella(t *testing.T) {
	t.Parallel()

	par := []Entrega{
		{ID: "r1", Fuente: "caracol", Periodo: "2025-01", SHA256: "aa"},
		{ID: "r2", Fuente: "netflix", Periodo: "2025-01", SHA256: "aa", Excluida: true},
	}
	if got := refsDe(duplicadosPorHuella("2025-01", par)); len(got) != 0 {
		t.Fatalf("con una pata excluida siguen saliendo duplicados: %v", got)
	}

	trio := append(par, Entrega{ID: "r3", Fuente: "cine", Periodo: "2025-01", SHA256: "aa"})
	got := refsDe(duplicadosPorHuella("2025-01", trio))
	if !slices.Equal(got, []string{"duplicado_archivo|reporte:r1", "duplicado_archivo|reporte:r3"}) {
		t.Fatalf("con tres patas y una excluida = %v", got)
	}
}
