package asistente

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// sembrarProcesoYONI deja una corrida nacional en verificacion con una firma y dos ONI de 2026-01 (una por fuente).
func sembrarProcesoYONI(t *testing.T) aplicacion.CatalogoHerramientas {
	t.Helper()
	pool := testhelp.Pool(t)
	store := postgres.Nuevo(pool)
	ctx := t.Context()
	for _, sql := range []string{
		`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('canal-1', 'Canal 1', 'tv_abierta')`,
		`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto) VALUES ('bolsa-1', 'canal-1', '2026-01', 'nacional', 1000.00)`,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash) VALUES ('usr-dist', 'dist@redes.test', 'Dist', 'distribucion', 'hash-de-prueba-suficientemente-larga')`,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes) VALUES ('rep-1', 'caracol', '2026-01', repeat('a', 64), 'reportes/rep-1.csv', 10)`,
		`INSERT INTO usos (id, reporte_id, fuente, titulo, titulo_original, ids_fuente, modalidad, escalon, oni, puntaje, emisiones) VALUES
		   ('u-1', 'rep-1', 'caracol', 'La reina', '', '', 'tv', 'oni', TRUE, 0, 1),
		   ('u-2', 'rep-1', 'rcn', 'El rey', '', '', 'tv', 'oni', TRUE, 0, 1)`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	p := aplicacion.ProcesoVista{ID: "proc-1", Circuito: reparto.Nacional, Etapa: reparto.EtapaVerificacion, Periodo: "2026-01",
		BolsaID: "bolsa-1", SnapshotID: "snap-1", Reglamento: "IX", Revision: 1}
	if err := store.GuardarProceso(ctx, p, aplicacion.RevisionAlta); err != nil {
		t.Fatalf("guardar proceso: %v", err)
	}
	if err := store.GuardarFirma(ctx, "proc-1", reparto.Firma{Rol: "distribucion", ActorID: "usr-dist", SobreRev: 1}); err != nil {
		t.Fatalf("guardar firma: %v", err)
	}
	c, err := Herramientas(store)
	if err != nil {
		t.Fatalf("Herramientas: %v", err)
	}
	return c
}

func ejecutarJSON(t *testing.T, c aplicacion.CatalogoHerramientas, actor aplicacion.Usuario, nombre, args string) (string, error) {
	t.Helper()
	res, err := c.Ejecutar(t.Context(), actor, nombre, json.RawMessage(args))
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(res)
	return string(b), nil
}

func TestIntegracionEstadoCorridaYListarONIContraPostgres(t *testing.T) {
	c := sembrarProcesoYONI(t)
	admin := aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador}

	estado, err := ejecutarJSON(t, c, aplicacion.Usuario{ID: "usr-aud", Rol: aplicacion.RolAuditor}, "estado_corrida", `{"proceso_id":"proc-1"}`)
	if err != nil {
		t.Fatalf("estado_corrida: %v", err)
	}
	for _, quiere := range []string{`"circuito":"nacional"`, `"etapa":"verificacion"`, `"firmas_faltantes":["contabilidad"]`, `"siguiente_etapa":"liquidacion_final"`} {
		if !strings.Contains(estado, quiere) {
			t.Errorf("estado_corrida sin %s: %s", quiere, estado)
		}
	}
	if lista, err := ejecutarJSON(t, c, admin, "estado_corrida", `{"periodo":"2026"}`); err != nil || !strings.Contains(lista, `"total":1`) {
		t.Errorf("estado_corrida por periodo: %v %s", err, lista)
	}

	oni, err := ejecutarJSON(t, c, admin, "listar_oni", `{"periodo":"2026-01","fuente":"caracol"}`)
	if err != nil {
		t.Fatalf("listar_oni: %v", err)
	}
	if !strings.Contains(oni, `"pendientes_total":1`) || !strings.Contains(oni, `"titulo":"La reina"`) || strings.Contains(oni, "El rey") {
		t.Errorf("listar_oni filtrado por fuente: %s", oni)
	}

	titular := aplicacion.Usuario{ID: "usr-tit", Rol: aplicacion.RolTitular, TitularID: "tit-1"}
	for _, nombre := range []string{"estado_corrida", "listar_oni"} {
		if _, err := ejecutarJSON(t, c, titular, nombre, `{}`); !errors.Is(err, aplicacion.ErrNoAutorizado) {
			t.Errorf("%s con titular: err = %v, se esperaba ErrNoAutorizado", nombre, err)
		}
	}
}
