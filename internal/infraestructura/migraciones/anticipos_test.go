package migraciones_test

import (
	"database/sql"
	"strings"
	"testing"
)

// La 00031 (#56) le pone saldo a `anticipos`, que existia desde la 00001 como
// andamiaje. Lo que ya estuviera aprobado se queda debiendo todo lo otorgado
// -no hay rastro de descuentos anteriores, y suponer cero le giraria al socio
// lo que debe-, y lo que no esta aprobado arranca en cero. Sin el backfill, el
// CHECK `anticipo_saldo_solo_aprobado` (validado, no NOT VALID) rechazaria la
// migracion entera.
func TestAnticiposSaldoArrancaConTodoLoAprobadoAntesDeLaMigracion(t *testing.T) {
	p, db, _ := proveedorEnLaVersionAnteriorA(t, "_anticipos_saldo_y_embargos.sql")
	ctx := t.Context()
	ejecutar := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ejecutar(`INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
	          VALUES ('tit-1', 'Uno', 'IPI-1', TRUE, 'socio'), ('tit-2', 'Dos', 'IPI-2', TRUE, 'socio'),
	                 ('tit-3', 'Tres', 'IPI-3', TRUE, 'socio'), ('tit-4', 'Cuatro', 'IPI-4', TRUE, 'socio')`)
	ejecutar(`INSERT INTO anticipos (id, titular_id, monto, estado) VALUES
	            ('ant-aprobado',   'tit-1', 1000, 'aprobado'),
	            ('ant-solicitado', 'tit-2', 2000, 'solicitado'),
	            ('ant-descontado', 'tit-3', 3000, 'descontado'),
	            ('ant-rechazado',  'tit-4', 4000, 'rechazado')`)

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("subir la 00031 sobre anticipos ya existentes: %v", err)
	}

	quiero := map[string]string{
		"ant-aprobado":   "1000.00",
		"ant-solicitado": "0.00",
		"ant-descontado": "0.00",
		"ant-rechazado":  "0.00",
	}
	for id, saldo := range quiero {
		var got string
		if err := db.QueryRowContext(ctx, `SELECT saldo::text FROM anticipos WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("leer %s: %v", id, err)
		}
		if got != saldo {
			t.Errorf("%s: saldo = %s, se esperaba %s", id, got, saldo)
		}
	}
}

// Dos anticipos vigentes del mismo titular de antes de la migracion no se
// esconden detras de un indice creado a medias: la 00031 FALLA, y el despliegue
// se detiene donde alguien tiene que mirar esas filas. Es el fallo ruidoso que
// se eligio en vez de restricciones NOT VALID.
func TestAnticiposDosVigentesDelMismoTitularDetienenLaMigracion(t *testing.T) {
	p, db, version := proveedorEnLaVersionAnteriorA(t, "_anticipos_saldo_y_embargos.sql")
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
	                                  VALUES ('tit-1', 'Uno', 'IPI-1', TRUE, 'socio')`); err != nil {
		t.Fatalf("titular: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO anticipos (id, titular_id, monto, estado) VALUES
	                                    ('ant-1', 'tit-1', 100, 'solicitado'),
	                                    ('ant-2', 'tit-1', 200, 'aprobado')`); err != nil {
		t.Fatalf("anticipos: %v", err)
	}

	_, err := p.Up(ctx)
	if err == nil {
		t.Fatal("la 00031 subio con dos anticipos vigentes del mismo titular")
	}
	if !strings.Contains(err.Error(), "anticipos_uno_vigente_por_titular") {
		t.Fatalf("la 00031 fallo por otra cosa: %v", err)
	}
	actual, err := p.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if actual >= version {
		t.Fatalf("version = %d; la 00031 (%d) no puede quedar aplicada", actual, version)
	}
}

// El down de la 00031 no borra lo que no se puede reconstruir: con un anticipo
// o un embargo registrado se niega, y la base queda como estaba. Sin la
// negativa, bajar y volver a subir le devolvia a un aprobado de 1000 con 400
// de saldo `saldo = monto` -le cobraba otra vez los 600 ya descontados- y le
// borraba el embargo que impide desembolsarle (RA 3.1.e). Mismo precedente que
// TestDownDeParametrosTextualesNoDejaSnapshotsSinValor.
func TestDownDeAnticiposSeNiegaConAnticiposOEmbargos(t *testing.T) {
	casos := []struct {
		nombre, sembrar, cita string
	}{
		{
			nombre:  "con un anticipo aprobado y descontado en parte",
			sembrar: `INSERT INTO anticipos (id, titular_id, monto, saldo, estado) VALUES ('ant-1', 'tit-1', 1000, 400, 'aprobado')`,
			cita:    "R-33",
		},
		{
			nombre:  "con un embargo reportado",
			sembrar: `INSERT INTO embargos (titular_id, reportado, soporte) VALUES ('tit-1', DATE '2026-03-01', 'oficio juzgado 3')`,
			cita:    "RA 3.1.e",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p, db, version := proveedorEnLaVersionAnteriorA(t, "_anticipos_saldo_y_embargos.sql")
			ctx := t.Context()
			if _, err := p.UpTo(ctx, version); err != nil {
				t.Fatalf("subir la 00031: %v", err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
			                                  VALUES ('tit-1', 'Uno', 'IPI-1', TRUE, 'socio')`); err != nil {
				t.Fatalf("titular: %v", err)
			}
			if _, err := db.ExecContext(ctx, c.sembrar); err != nil {
				t.Fatalf("sembrar: %v", err)
			}
			antes := huellaDeAnticipos(t, db)

			_, err := p.DownTo(ctx, version-1)
			if err == nil || !strings.Contains(err.Error(), c.cita) {
				t.Fatalf("el down tenia que negarse citando %s, dio: %v", c.cita, err)
			}
			actual, err := p.GetDBVersion(ctx)
			if err != nil {
				t.Fatalf("version: %v", err)
			}
			if actual != version {
				t.Fatalf("version = %d tras la negativa, se esperaba %d: la 00031 sigue aplicada", actual, version)
			}
			if despues := huellaDeAnticipos(t, db); despues != antes {
				t.Fatalf("tras la negativa la base cambio:\nantes   %s\ndespues %s", antes, despues)
			}
		})
	}
}

// Con las dos tablas vacias no hay nada que perder: el down de la 00031 baja,
// y la vuelve a subir limpio. Es el camino que recorre una base recien
// desplegada, y el de TestUpYDownRecorrenTodasLasMigraciones.
func TestDownDeAnticiposSinDatosBaja(t *testing.T) {
	p, db, version := proveedorEnLaVersionAnteriorA(t, "_anticipos_saldo_y_embargos.sql")
	ctx := t.Context()
	if _, err := p.UpTo(ctx, version); err != nil {
		t.Fatalf("subir la 00031: %v", err)
	}
	if _, err := p.DownTo(ctx, version-1); err != nil {
		t.Fatalf("bajar con las tablas vacias tenia que funcionar: %v", err)
	}
	var embargos *string
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('embargos')::text`).Scan(&embargos); err != nil {
		t.Fatalf("buscar la tabla embargos: %v", err)
	}
	if embargos != nil {
		t.Fatal("tras el down la tabla embargos sigue existiendo")
	}
	if _, err := p.UpTo(ctx, version); err != nil {
		t.Fatalf("volver a subir: %v", err)
	}
}

// huellaDeAnticipos resume lo que el down de la 00031 borraria: cada anticipo
// con su estado, su saldo y su justificacion, y cada embargo con su oficio.
func huellaDeAnticipos(t *testing.T, db *sql.DB) string {
	t.Helper()
	var huella string
	if err := db.QueryRowContext(t.Context(), `
		SELECT COALESCE((SELECT string_agg(id || ':' || estado || ':' || saldo::text || ':' || justificacion, ',' ORDER BY id)
		                   FROM anticipos), '') || ' | ' ||
		       COALESCE((SELECT string_agg(titular_id || ':' || reportado::text || ':' || soporte, ',' ORDER BY id)
		                   FROM embargos), '')`).Scan(&huella); err != nil {
		t.Fatalf("leer anticipos y embargos: %v", err)
	}
	return huella
}
