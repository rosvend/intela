package postgres

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

const (
	shaRCN       = "3333333333333333333333333333333333333333333333333333333333333333"
	reporteRCN   = "rep-rcn-enero"
	periodoDosTV = "2026-01" // el mismo de reporteEnero
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// sembrarDosCanales deja dos canales de TV abierta emitiendo en el MISMO
// periodo, cada uno en su propia entrega, mas una fila de otro periodo.
//
// Es la forma minima que hace observable la independencia de `RD 9.1`: con un
// solo canal, "el valor punto es por canal" no se distingue de "el valor punto
// es por periodo".
func sembrarDosCanales(t *testing.T) *Store {
	t.Helper()

	s, pool := sembrarReportes(t) // reporteEnero: caracol, 2026-01; reporteFebrero: 2026-02
	ctx := t.Context()

	if err := s.GuardarReporte(ctx, reporteRCN, "rcn", periodoDosTV,
		shaRCN, "reportes/"+shaRCN, 64); err != nil {
		t.Fatalf("sembrar la entrega de rcn: %v", err)
	}

	// Identificadas de una vez -- obra_id, escalon alias, oni=false -- porque
	// UsosDeCanal solo devuelve filas con obra identificada: una fila
	// pendiente no puede probar nada del valor punto.
	uso := func(id, reporteID, canal, tipo string, emisiones int64, dur, rating string) aplicacion.UsoPersistido {
		obraID := "obra-" + id
		if _, err := pool.Exec(ctx,
			`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ($1, $2, 'Drama', 2020, $3)`,
			obraID, "Obra "+id, tipo); err != nil {
			t.Fatalf("sembrar obra de %q: %v", id, err)
		}
		u := usoPendiente(id, reporteID, "Obra "+id)
		u.CanalID = canal
		u.TipoObra = tipo
		u.Emisiones = emisiones
		u.DuracionMin = dec(dur)
		u.Rating = dec(rating)
		u.Escalon = "alias"
		u.ONI = false
		u.ObraID = obraID
		u.Evidencia = "alias " + canal + "/id=" + id
		return u
	}

	usos := []aplicacion.UsoPersistido{
		uso("uso-caracol-1", reporteEnero, "caracol", "cinematografica", 1, "70", "4.5"),
		uso("uso-caracol-2", reporteEnero, "caracol", "serie", 10, "48", "9"),
		uso("uso-rcn-1", reporteRCN, "rcn", "serie", 4, "48", "6"),
		// Mismo canal, OTRO periodo: no puede entrar en la corrida de enero.
		uso("uso-caracol-febrero", reporteFebrero, "caracol", "serie", 1, "48", "1"),
	}
	if err := s.GuardarUsos(ctx, usos); err != nil {
		t.Fatalf("sembrar usos de dos canales: %v", err)
	}
	return s
}

// TestLaCorridaSeAgrupaPorCanal es el criterio central de #119. `RD 9.1`
// reparte el dinero de cada canal de manera independiente y `RD 9.1.1` calcula
// el valor punto sobre los puntos DE ESE CANAL; sin atribucion de canal en el
// uso, los puntos de dos pagadores caerian en un solo cociente.
func TestLaCorridaSeAgrupaPorCanal(t *testing.T) {
	s := sembrarDosCanales(t)
	ctx := t.Context()

	// Por el caso de uso, que es lo que #33 va a consumir: es ahi donde el ano
	// de clasificacion se deriva del periodo.
	r := aplicacion.Reparto{Usos: s}
	usosDe := func(canal string) []reparto.Uso {
		t.Helper()
		usos, _, err := r.UsosDeCanal(ctx, periodoDosTV, canal)
		if err != nil {
			t.Fatalf("usos del canal %q: %v", canal, err)
		}
		return usos
	}

	caracol := usosDe("caracol")
	rcn := usosDe("rcn")

	t.Run("cada canal ve solo sus filas", func(t *testing.T) {
		if len(caracol) != 2 {
			t.Fatalf("usos de caracol = %d, se esperaban 2 (la de febrero no cuenta)", len(caracol))
		}
		if len(rcn) != 1 {
			t.Fatalf("usos de rcn = %d, se esperaba 1", len(rcn))
		}
		for _, u := range caracol {
			if u.CanalID != "caracol" {
				t.Errorf("la corrida de caracol vio un uso de %q", u.CanalID)
			}
		}
		for _, u := range rcn {
			if u.CanalID != "rcn" {
				t.Errorf("la corrida de rcn vio un uso de %q", u.CanalID)
			}
		}
	})

	t.Run("dos canales dan dos valor punto independientes", func(t *testing.T) {
		valorPunto := func(usos []reparto.Uso, usuario string) decimal.Decimal {
			t.Helper()
			// Misma bolsa en los dos: asi la diferencia solo puede venir de que
			// cada uno pondera con sus propios puntos.
			b, err := recaudo.NuevaBolsa(usuario, periodoDosTV, recaudo.Nacional, dec("1000000"))
			if err != nil {
				t.Fatalf("bolsa de %q: %v", usuario, err)
			}
			res, err := reparto.Reparto(b, usos, snapshotDePrueba(), nil,
				reparto.Opciones{SinDeducciones: true})
			if err != nil {
				t.Fatalf("reparto de %q: %v", usuario, err)
			}
			return res.ValorPunto
		}

		vpCaracol := valorPunto(caracol, "caracol")
		vpRCN := valorPunto(rcn, "rcn")

		if vpCaracol.IsZero() || vpRCN.IsZero() {
			t.Fatalf("valor punto en cero: caracol=%s rcn=%s", vpCaracol, vpRCN)
		}
		if vpCaracol.Equal(vpRCN) {
			t.Fatalf("los dos canales comparten valor punto (%s): RD 9.1 los reparte "+
				"de manera independiente", vpCaracol)
		}
	})

	t.Run("un canal sin emisiones devuelve vacio y no un error", func(t *testing.T) {
		filas, _, err := s.UsosDeCanal(ctx, periodoDosTV, "telecaribe", 2025)
		if err != nil {
			t.Fatalf("un canal sin emisiones no es un fallo: %v", err)
		}
		if len(filas) != 0 {
			t.Fatalf("filas = %d, se esperaban 0", len(filas))
		}
	})

	t.Run("sin clasificacion el grupo llega vacio", func(t *testing.T) {
		filas, _, err := s.UsosDeCanal(ctx, periodoDosTV, "rcn", 2025)
		if err != nil {
			t.Fatalf("UsosDeCanal: %v", err)
		}
		if len(filas) == 0 {
			t.Fatal("sin filas no se puede comprobar nada")
		}
		// Vacio y no error: fuera de suscripcion el grupo no se usa, y dentro
		// el que falla ruidosamente es ParseGrupoCanal, en el nucleo.
		if filas[0].GrupoEfectivo != "" {
			t.Fatalf("grupo = %q, se esperaba vacio", filas[0].GrupoEfectivo)
		}
	})
}

// TestElGrupoSeResuelveContraElAnoAnterior fija `RD 9.5.4`: la clasificacion
// es la del ano inmediatamente anterior al periodo, no la vigente hoy. Sin
// esto, reejecutar un periodo pasado daria otro reparto (ADR 0005).
func TestElGrupoSeResuelveContraElAnoAnterior(t *testing.T) {
	s := sembrarDosCanales(t)
	ctx := t.Context()

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO canales (id, nombre, grupo_estructural)
		 VALUES ('rcn', 'RCN', 'privado_nacional')`); err != nil {
		t.Fatalf("registrar el canal: %v", err)
	}
	// El ano EQUIVOCADO se inserta PRIMERO a proposito. Sin el predicado
	// `anio_audiencia = $2`, un QueryRow sin ORDER BY devuelve la primera fila
	// fisica -- que aqui seria 'lideres_rating' de 2026 -- y el aserto de abajo
	// lo detectaria. Insertarlas en el orden "correcto" (2025 antes que 2026)
	// dejaba pasar esa mutacion por pura casualidad de insercion.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO canales_clasificacion (canal_id, anio_audiencia, grupo_efectivo)
		 VALUES ('rcn', 2026, 'lideres_rating'), ('rcn', 2025, 'privado_nacional')`,
	); err != nil {
		t.Fatalf("clasificar el canal: %v", err)
	}

	filas, _, err := s.UsosDeCanal(ctx, periodoDosTV, "rcn", 2025)
	if err != nil {
		t.Fatalf("UsosDeCanal: %v", err)
	}
	if len(filas) == 0 {
		t.Fatal("sin filas no se puede comprobar la clasificacion")
	}
	if filas[0].GrupoEfectivo != "privado_nacional" {
		t.Fatalf("grupo = %q, se esperaba el de 2025 y no el de 2026 (RD 9.5.4)",
			filas[0].GrupoEfectivo)
	}

	// Un ano SIN fila para este canal -- ni 2025 ni 2026 tienen 2023 -- tiene
	// que dar grupo vacio, no la clasificacion de otro ano por defecto.
	sinFila, _, err := s.UsosDeCanal(ctx, periodoDosTV, "rcn", 2023)
	if err != nil {
		t.Fatalf("UsosDeCanal (2023): %v", err)
	}
	if len(sinFila) == 0 {
		t.Fatal("sin filas no se puede comprobar la clasificacion")
	}
	if sinFila[0].GrupoEfectivo != "" {
		t.Fatalf("grupo para un ano sin clasificar = %q, se esperaba vacio", sinFila[0].GrupoEfectivo)
	}
}

// TestUsosDeCanalExcluyeFilasSinObraYLasCuenta comprueba el filtro que evita
// una obra fantasma. `usos.oni` tiene DEFAULT TRUE y `obra_id` es NULL hasta
// que la cascada resuelve, asi que toda fila recien ingerida cumple eso; sin
// filtrar por obra_id, COALESCE(obra_id, ”) las convertiria en una obra
// fantasma de id "" que suma puntos e importe de verdad. Pendiente y ONI son
// motivos distintos -- uno es "la cascada no ha corrido", el otro "corrio y no
// reconocio nada" -- y el reglamento los trata distinto (RD 13.8, R-18/R-19).
func TestUsosDeCanalExcluyeFilasSinObraYLasCuenta(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, genero, anio, tipo)
		 VALUES ('obra-1', 'La Casa', 'Telenovela', 1994, 'serie')`); err != nil {
		t.Fatalf("sembrar obra: %v", err)
	}

	identificada := usoPendiente("uso-identificada", reporteEnero, "La Casa")
	identificada.CanalID = "caracol"
	identificada.Escalon = "alias"
	identificada.ONI = false
	identificada.ObraID = "obra-1"
	identificada.Evidencia = "alias caracol/ID_Ficha=1234"

	oni := usoPendiente("uso-oni", reporteEnero, "Sin Reconocer")
	oni.CanalID = "caracol"
	oni.Escalon = "oni"
	oni.ONI = true

	pendiente := usoPendiente("uso-pendiente", reporteEnero, "Sin Intentar")
	pendiente.CanalID = "caracol"
	// Escalon y ONI se quedan en el default de usoPendiente: "pendiente" / true.

	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{identificada, oni, pendiente}); err != nil {
		t.Fatalf("sembrar usos: %v", err)
	}

	usos, resumen, err := s.UsosDeCanal(ctx, "2026-01", "caracol", 2025)
	if err != nil {
		t.Fatalf("UsosDeCanal: %v", err)
	}

	if len(usos) != 1 || usos[0].Uso.ID != "uso-identificada" {
		t.Fatalf("se esperaba solo la fila identificada, llego %+v", usos)
	}
	for _, u := range usos {
		if u.Uso.ObraID == "" {
			t.Fatal("una fila sin obra_id llego al resultado: seria una LineaObra{ObraID:\"\"} en el motor")
		}
	}

	if resumen != (aplicacion.ResumenUsosDeCanal{Pendientes: 1, ONI: 1}) {
		t.Fatalf("resumen = %+v, se esperaba 1 pendiente y 1 ONI (0 excluidos)", resumen)
	}
	if resumen.TotalSinIdentificar() != 2 {
		t.Fatalf("TotalSinIdentificar() = %d, se esperaban 2", resumen.TotalSinIdentificar())
	}
}

// TestUsosSinCanalCuentaLoQueNingunPagadorReclama es el B1 de la revision:
// mientras ningun adaptador de ingesta puebla canal_id (P-20), "cero usos de
// un canal" no se distingue de "el canal no emitio" sin este conteo aparte.
func TestUsosSinCanalCuentaLoQueNingunPagadorReclama(t *testing.T) {
	s, _ := sembrarReportes(t)
	ctx := t.Context()

	// Dos filas del mismo periodo sin canal_id -- el estado real de una
	// entrega de Caracol hoy -- y una con canal, que no debe contarse.
	usos := []aplicacion.UsoPersistido{
		usoPendiente("uso-sin-canal-1", reporteEnero, "Sin canal 1"),
		usoPendiente("uso-sin-canal-2", reporteEnero, "Sin canal 2"),
	}
	conCanal := usoPendiente("uso-con-canal", reporteEnero, "Con canal")
	conCanal.CanalID = "caracol"
	usos = append(usos, conCanal)

	if err := s.GuardarUsos(ctx, usos); err != nil {
		t.Fatalf("sembrar usos: %v", err)
	}

	n, err := s.UsosSinCanal(ctx, "2026-01")
	if err != nil {
		t.Fatalf("UsosSinCanal: %v", err)
	}
	if n != 2 {
		t.Fatalf("n = %d, se esperaban 2", n)
	}
}

func TestUsosDeCanalRechazaUnCanalVacioAntesDeConsultar(t *testing.T) {
	s, _ := sembrarReportes(t)
	r := aplicacion.Reparto{Usos: s}

	if _, _, err := r.UsosDeCanal(t.Context(), "2026-01", ""); !errors.Is(err, aplicacion.ErrCanalVacio) {
		t.Fatalf("se esperaba ErrCanalVacio, se obtuvo %v", err)
	}
}

func snapshotDePrueba() reparto.Snapshot {
	return reparto.Snapshot{
		PondCine:       dec("5.0"),
		PondUnitario:   dec("2.8"),
		PondSerie:      dec("1.3"),
		PondSketch:     dec("0.8"),
		BaseCineTeatro: reparto.BaseEspectadores,
		Reglamento:     "RD-IX",
	}
}
