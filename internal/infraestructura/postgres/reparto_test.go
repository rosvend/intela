package postgres

import (
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

	s, _ := sembrarReportes(t) // reporteEnero: caracol, 2026-01; reporteFebrero: 2026-02
	ctx := t.Context()

	if err := s.GuardarReporte(ctx, reporteRCN, "rcn", periodoDosTV,
		shaRCN, "reportes/"+shaRCN, 64); err != nil {
		t.Fatalf("sembrar la entrega de rcn: %v", err)
	}

	uso := func(id, reporteID, canal, tipo string, emisiones int64, dur, rating string) aplicacion.UsoPersistido {
		u := usoPendiente(id, reporteID, "Obra "+id)
		u.CanalID = canal
		u.TipoObra = tipo
		u.Emisiones = emisiones
		u.DuracionMin = dec(dur)
		u.Rating = dec(rating)
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
		usos, err := r.UsosDeCanal(ctx, periodoDosTV, canal)
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
		filas, err := s.UsosDeCanal(ctx, periodoDosTV, "telecaribe", 2025)
		if err != nil {
			t.Fatalf("un canal sin emisiones no es un fallo: %v", err)
		}
		if len(filas) != 0 {
			t.Fatalf("filas = %d, se esperaban 0", len(filas))
		}
	})

	t.Run("sin clasificacion el grupo llega vacio", func(t *testing.T) {
		filas, err := s.UsosDeCanal(ctx, periodoDosTV, "rcn", 2025)
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
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO canales_clasificacion (canal_id, anio_audiencia, grupo_efectivo)
		 VALUES ('rcn', 2025, 'privado_nacional'), ('rcn', 2026, 'lideres_rating')`,
	); err != nil {
		t.Fatalf("clasificar el canal: %v", err)
	}

	filas, err := s.UsosDeCanal(ctx, periodoDosTV, "rcn", 2025)
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
