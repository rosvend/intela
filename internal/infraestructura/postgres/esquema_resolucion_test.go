package postgres

import (
	"strings"
	"testing"
)

// TestElCheckDeUsosSostieneLaResolucionManual comprueba la ULTIMA defensa del
// escalon 'descartado' y de la nota (#175, migracion 00022).
//
// Escribe saltandose el caso de uso a proposito, por lo mismo que
// [TestLosCheckDeUsosSostienenElVocabularioDeRD9]: el nucleo ya rechaza estas
// filas, y lo que queda en pie si manana se escribe en `usos` por otro camino
// -una migracion de datos, un script, el sembrador- son los CHECK.
//
// Un solo Restore para todas las subpruebas: cada una usa ids distintos y el
// contenedor es compartido por el binario entero.
func TestElCheckDeUsosSostieneLaResolucionManual(t *testing.T) {
	_, pool := sembrarReportes(t)
	ctx := t.Context()

	// Un revisor para que resuelto_por tenga a quien apuntar: la columna es FK.
	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 VALUES ('revisor-1', 'revisor@redes.test', 'Revisora Uno', 'administrador', $1)`,
		hashBcrypt); err != nil {
		t.Fatalf("sembrar el usuario revisor: %v", err)
	}

	// insertar escribe el crudo: escalon, oni, obra, firmas y nota, sin pasar
	// por ninguna validacion del nucleo.
	insertar := func(id, escalon string, oni bool, obraID, resueltoPor, nota string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad,
			                   escalon, oni, obra_id, resuelto_por, resuelto_en, nota_resolucion)
			 VALUES ($1, $2, 'caracol', 'Una obra', 'tv',
			         $3, $4, NULLIF($5, ''), NULLIF($6, ''), now(), $7)`,
			id, reporteEnero, escalon, oni, obraID, resueltoPor, nota)
		return err
	}

	rechaza := func(t *testing.T, nombre, restriccion string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("la base acepto %s", nombre)
		}
		if !strings.Contains(err.Error(), restriccion) {
			t.Errorf("el rechazo no viene de %s: %v", restriccion, err)
		}
	}

	t.Run("acepta un descartado firmado y con nota", func(t *testing.T) {
		if err := insertar("uso-desc-ok", "descartado", false, "", "revisor-1",
			"no es un uso del repertorio"); err != nil {
			t.Fatalf("la base rechaza un descarte valido: %v", err)
		}
	})

	t.Run("acepta un manual con nota", func(t *testing.T) {
		if err := insertar("uso-man-ok", "manual", false, obraImdb, "revisor-1",
			"coincide la ficha tecnica"); err != nil {
			t.Fatalf("la base rechaza un manual con nota: %v", err)
		}
	})

	// Un descartado es "sin obra y sin ONI", como un excluido: las dos ramas
	// de uso_resuelto_tiene_obra son excluyentes.
	t.Run("rechaza un descartado con obra", func(t *testing.T) {
		rechaza(t, "un descartado con obra", "uso_resuelto_tiene_obra",
			insertar("uso-desc-obra", "descartado", false, obraImdb, "revisor-1", "nota"))
	})

	t.Run("rechaza un descartado con oni", func(t *testing.T) {
		rechaza(t, "un descartado marcado ONI", "uso_resuelto_tiene_obra",
			insertar("uso-desc-oni", "descartado", true, "", "revisor-1", "nota"))
	})

	// La firma es del descarte, no solo de la asignacion: sin actor no hay
	// decision humana que auditar (ADR 0006).
	t.Run("rechaza un descartado sin resuelto_por", func(t *testing.T) {
		rechaza(t, "un descartado sin actor", "manual_tiene_autor",
			insertar("uso-desc-sin-actor", "descartado", false, "", "", "nota"))
	})

	t.Run("rechaza un manual sin resuelto_por", func(t *testing.T) {
		rechaza(t, "un manual sin actor", "manual_tiene_autor",
			insertar("uso-man-sin-actor", "manual", false, obraImdb, "", "nota"))
	})

	// La nota es obligatoria en los dos: la firma dice quien, la nota dice por que.
	t.Run("rechaza un descartado sin nota", func(t *testing.T) {
		rechaza(t, "un descartado sin nota", "resolucion_manual_tiene_nota",
			insertar("uso-desc-sin-nota", "descartado", false, "", "revisor-1", ""))
	})

	t.Run("rechaza un descartado con nota de solo espacios", func(t *testing.T) {
		rechaza(t, "un descartado con nota en blanco", "resolucion_manual_tiene_nota",
			insertar("uso-desc-nota-blanca", "descartado", false, "", "revisor-1", "   "))
	})

	t.Run("rechaza un manual sin nota", func(t *testing.T) {
		rechaza(t, "un manual sin nota", "resolucion_manual_tiene_nota",
			insertar("uso-man-sin-nota", "manual", false, obraImdb, "revisor-1", ""))
	})

	t.Run("rechaza una nota de 301 caracteres", func(t *testing.T) {
		rechaza(t, "una nota de 301 caracteres", "nota_resolucion_tope",
			insertar("uso-desc-nota-larga", "descartado", false, "", "revisor-1",
				strings.Repeat("a", 301)))
	})

	t.Run("acepta una nota de 300 caracteres", func(t *testing.T) {
		if err := insertar("uso-desc-nota-justa", "descartado", false, "", "revisor-1",
			strings.Repeat("a", 300)); err != nil {
			t.Fatalf("la base rechaza una nota de 300 caracteres: %v", err)
		}
	})

	// char_length cuenta caracteres, no bytes: 300 enes son 600 bytes y tienen
	// que pasar. Con octet_length el tope real seria la mitad.
	t.Run("el tope se mide en caracteres y no en bytes", func(t *testing.T) {
		if err := insertar("uso-desc-nota-multibyte", "descartado", false, "", "revisor-1",
			strings.Repeat("ñ", 300)); err != nil {
			t.Fatalf("la base mide bytes en vez de caracteres: %v", err)
		}
	})

	// Un pendiente no lleva nota y no se le exige: la columna es NOT NULL con
	// DEFAULT '' para no obligar a los otros seis escalones a inventar una.
	t.Run("un pendiente sigue sin nota", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad, escalon, oni)
			 VALUES ('uso-pend-sin-nota', $1, 'caracol', 'Otra', 'tv', 'pendiente', true)`,
			reporteEnero); err != nil {
			t.Fatalf("la base exige nota a un pendiente: %v", err)
		}
	})
}

// TestUnDescartadoQuedaFueraDeLasVistas comprueba lo que la migracion 00022
// promete en su cabecera: con oni=false y escalon distinto de 'pendiente', la
// fila descartada no sale en el listado publico ONI ni en el indice parcial de
// pendientes.
func TestUnDescartadoQuedaFueraDeLasVistas(t *testing.T) {
	_, pool := sembrarReportes(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 VALUES ('revisor-1', 'revisor@redes.test', 'Revisora Uno', 'administrador', $1)`,
		hashBcrypt); err != nil {
		t.Fatalf("sembrar el usuario revisor: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad,
		                   escalon, oni, resuelto_por, resuelto_en, nota_resolucion)
		 VALUES ('uso-descartado', $1, 'caracol', 'No es del repertorio', 'tv',
		         'descartado', FALSE, 'revisor-1', now(), 'no es un uso del repertorio')`,
		reporteEnero); err != nil {
		t.Fatalf("sembrar el descartado: %v", err)
	}

	var enPublico int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM oni_publico WHERE id = 'uso-descartado'`).Scan(&enPublico); err != nil {
		t.Fatalf("leer oni_publico: %v", err)
	}
	if enPublico != 0 {
		t.Error("un descartado no puede salir en el listado publico ONI (R-18)")
	}

	var pendientes int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos WHERE escalon = 'pendiente' AND id = 'uso-descartado'`).Scan(&pendientes); err != nil {
		t.Fatalf("leer pendientes: %v", err)
	}
	if pendientes != 0 {
		t.Error("un descartado no es una fila pendiente de identificar")
	}
}

// TestResumenExclusionesCuentaLosDescartados cierra el hueco de reparto.go: sin
// la cuarta columna, un descarte era invisible para quien mira por que una fila
// no pondero.
func TestResumenExclusionesCuentaLosDescartados(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 VALUES ('revisor-1', 'revisor@redes.test', 'Revisora Uno', 'administrador', $1)`,
		hashBcrypt); err != nil {
		t.Fatalf("sembrar el usuario revisor: %v", err)
	}
	sembrar := func(id, escalon string, oni bool) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad, canal_id,
			                   escalon, oni, resuelto_por, resuelto_en, nota_resolucion)
			 VALUES ($1, $2, 'caracol', 'Una obra', 'tv', 'caracol',
			         $3, $4, 'revisor-1', now(), 'nota')`,
			id, reporteEnero, escalon, oni); err != nil {
			t.Fatalf("sembrar %s: %v", id, err)
		}
	}
	sembrar("uso-pend", "pendiente", true)
	sembrar("uso-oni", "oni", true)
	sembrar("uso-excl", "excluido", false)
	sembrar("uso-desc", "descartado", false)

	r, err := s.resumenExclusiones(ctx, "2026-01", "caracol")
	if err != nil {
		t.Fatalf("resumenExclusiones: %v", err)
	}
	if r.Pendientes != 1 || r.ONI != 1 || r.Excluidos != 1 || r.Descartados != 1 {
		t.Fatalf("resumen = %+v, se esperaba uno de cada", r)
	}
}
