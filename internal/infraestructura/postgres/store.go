package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/shopspring/decimal"
)

type Store struct{ pool *pgxpool.Pool }

func Conectar(ctx context.Context, dsn string) (*Store, error) {
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{pool: p}, nil
}

func (s *Store) Cerrar() { s.pool.Close() }

func (s *Store) Migrar(ctx context.Context, sqlText string) error {
	_, err := s.pool.Exec(ctx, sqlText)
	return err
}

func (s *Store) UsuarioPorEmail(ctx context.Context, email string) (aplicacion.Usuario, string, error) {
	var u aplicacion.Usuario
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT id,email,nombre,rol,COALESCE(titular_id,''),password_hash FROM usuarios WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.Nombre, &u.Rol, &u.TitularID, &hash)
	return u, hash, err
}

func (s *Store) UsuarioPorID(ctx context.Context, id string) (aplicacion.Usuario, error) {
	var u aplicacion.Usuario
	err := s.pool.QueryRow(ctx, `SELECT id,email,nombre,rol,COALESCE(titular_id,'') FROM usuarios WHERE id=$1`, id).
		Scan(&u.ID, &u.Email, &u.Nombre, &u.Rol, &u.TitularID)
	return u, err
}

func (s *Store) GuardarSesion(ctx context.Context, token, usuarioID string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sesiones(token,usuario_id) VALUES($1,$2)`, token, usuarioID)
	return err
}

func (s *Store) UsuarioPorToken(ctx context.Context, token string) (aplicacion.Usuario, error) {
	var id string
	if err := s.pool.QueryRow(ctx, `SELECT usuario_id FROM sesiones WHERE token=$1`, token).Scan(&id); err != nil {
		return aplicacion.Usuario{}, err
	}
	return s.UsuarioPorID(ctx, id)
}

func (s *Store) ListarObras(ctx context.Context) ([]aplicacion.Obra, error) {
	rows, err := s.pool.Query(ctx, `SELECT o.id,o.titulo,o.ida,o.eidr,o.imdb,o.tipo,
		CASE WHEN COALESCE(SUM(d.porcentaje),0)=100 THEN 'completa' ELSE 'incompleta' END
		FROM obras o LEFT JOIN declaraciones d ON d.obra_id=o.id GROUP BY o.id ORDER BY o.titulo`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []aplicacion.Obra
	for rows.Next() {
		var o aplicacion.Obra
		if err := rows.Scan(&o.ID, &o.Titulo, &o.IDA, &o.EIDR, &o.IMDB, &o.Tipo, &o.EstadoDecl); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) Declaraciones(ctx context.Context) (map[string]repertorio.Declaracion, error) {
	rows, err := s.pool.Query(ctx, `SELECT obra_id,titular_id,ipi,porcentaje FROM declaraciones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]repertorio.Declaracion{}
	for rows.Next() {
		var obra, tit, ipi, pct string
		if err := rows.Scan(&obra, &tit, &ipi, &pct); err != nil {
			return nil, err
		}
		d := m[obra]
		d.ObraID = obra
		p, _ := decimal.NewFromString(pct)
		d.Partes = append(d.Partes, repertorio.Parte{TitularID: tit, IPI: ipi, Porcentaje: p})
		m[obra] = d
	}
	return m, rows.Err()
}

func (s *Store) Alias(ctx context.Context, fuente, tipo, valor string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT obra_id FROM alias_obra WHERE fuente=$1 AND tipo_id=$2 AND valor=$3`, fuente, tipo, valor).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Store) GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO alias_obra(fuente,tipo_id,valor,obra_id,quien) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (fuente,tipo_id,valor) DO UPDATE SET obra_id=EXCLUDED.obra_id`, fuente, tipo, valor, obraID, quien)
	return err
}

func (s *Store) ObraPorIDGlobal(ctx context.Context, ida, eidr, imdb string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM obras WHERE ($1<>'' AND ida=$1) OR ($2<>'' AND eidr=$2) OR ($3<>'' AND imdb=$3) LIMIT 1`, ida, eidr, imdb).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Store) GuardarReporte(ctx context.Context, id, fuente, periodo, sha, clave string, nbytes int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO reportes(id,fuente,periodo,sha256,clave_objeto,nbytes) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO NOTHING`, id, fuente, periodo, sha, clave, nbytes)
	return err
}

func (s *Store) GuardarUsos(ctx context.Context, usos []aplicacion.UsoPersistido) error {
	for _, u := range usos {
		_, err := s.pool.Exec(ctx, `INSERT INTO usos(id,reporte_id,fuente,titulo,ids_fuente,obra_id,escalon,oni,modalidad,tipo_obra,duracion_min,emisiones,rating,taquilla,vistas,minutos_vistos,pb)
			VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			u.ID, u.ReporteID, u.Fuente, u.Titulo, u.IDsFuente, u.ObraID, u.Escalon, u.ONI, string(u.Modalidad), u.TipoObra,
			u.DuracionMin.String(), u.Emisiones, u.Rating.String(), u.Taquilla.String(), u.Vistas.String(), u.MinutosVistos.String(), u.PB.String())
		if err != nil {
			return err
		}
	}
	return nil
}

func scanUso(rows pgx.Rows) ([]aplicacion.UsoPersistido, error) {
	defer rows.Close()
	var out []aplicacion.UsoPersistido
	for rows.Next() {
		var u aplicacion.UsoPersistido
		var obra *string
		var dur, rat, taq, vis, minv, pb string
		var mod string
		if err := rows.Scan(&u.ID, &u.ReporteID, &u.Fuente, &u.Titulo, &u.IDsFuente, &obra, &u.Escalon, &u.ONI, &mod, &u.TipoObra, &dur, &u.Emisiones, &rat, &taq, &vis, &minv, &pb); err != nil {
			return nil, err
		}
		if obra != nil {
			u.ObraID = *obra
		}
		u.Modalidad = reparto.Modalidad(mod)
		u.DuracionMin, _ = decimal.NewFromString(dur)
		u.Rating, _ = decimal.NewFromString(rat)
		u.Taquilla, _ = decimal.NewFromString(taq)
		u.Vistas, _ = decimal.NewFromString(vis)
		u.MinutosVistos, _ = decimal.NewFromString(minv)
		u.PB, _ = decimal.NewFromString(pb)
		out = append(out, u)
	}
	return out, rows.Err()
}

const usoCols = `id,reporte_id,fuente,titulo,ids_fuente,obra_id,escalon,oni,modalidad,tipo_obra,duracion_min,emisiones,rating,taquilla,vistas,minutos_vistos,pb`

func (s *Store) UsosSinResolver(ctx context.Context) ([]aplicacion.UsoPersistido, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+usoCols+` FROM usos WHERE escalon='pendiente'`)
	if err != nil {
		return nil, err
	}
	return scanUso(rows)
}

func (s *Store) UsosDePeriodo(ctx context.Context, periodo string) ([]aplicacion.UsoPersistido, error) {
	q := `SELECT u.id,u.reporte_id,u.fuente,u.titulo,u.ids_fuente,u.obra_id,u.escalon,u.oni,u.modalidad,u.tipo_obra,u.duracion_min,u.emisiones,u.rating,u.taquilla,u.vistas,u.minutos_vistos,u.pb
		FROM usos u JOIN reportes r ON r.id=u.reporte_id`
	if periodo != "" {
		q += ` WHERE r.periodo=$1`
		rows, err := s.pool.Query(ctx, q, periodo)
		if err != nil {
			return nil, err
		}
		return scanUso(rows)
	}
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	return scanUso(rows)
}

func (s *Store) ActualizarUsoMatch(ctx context.Context, usoID, obraID, escalon string, oni bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE usos SET obra_id=NULLIF($2,''), escalon=$3, oni=$4 WHERE id=$1`, usoID, obraID, escalon, oni)
	return err
}

func (s *Store) ListarONI(ctx context.Context) ([]aplicacion.UsoPersistido, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+usoCols+` FROM usos WHERE oni=TRUE AND escalon<>'pendiente'`)
	if err != nil {
		return nil, err
	}
	return scanUso(rows)
}

func (s *Store) ListarBolsas(ctx context.Context) ([]aplicacion.BolsaPersistida, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,usuario_id,periodo,circuito,bruto FROM bolsas`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []aplicacion.BolsaPersistida
	for rows.Next() {
		var b aplicacion.BolsaPersistida
		var bruto string
		if err := rows.Scan(&b.ID, &b.UsuarioID, &b.Periodo, &b.Circuito, &bruto); err != nil {
			return nil, err
		}
		b.Bruto, _ = decimal.NewFromString(bruto)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) BolsaPorID(ctx context.Context, id string) (aplicacion.BolsaPersistida, error) {
	var b aplicacion.BolsaPersistida
	var bruto string
	err := s.pool.QueryRow(ctx, `SELECT id,usuario_id,periodo,circuito,bruto FROM bolsas WHERE id=$1`, id).
		Scan(&b.ID, &b.UsuarioID, &b.Periodo, &b.Circuito, &bruto)
	b.Bruto, _ = decimal.NewFromString(bruto)
	return b, err
}

func (s *Store) SnapshotVigente(ctx context.Context, cuando time.Time) (reparto.Snapshot, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (clave) clave, valor, reglamento FROM parametros
		WHERE vigente_desde<=$1 AND (vigente_hasta IS NULL OR vigente_hasta>=$1) ORDER BY clave, vigente_desde DESC`, cuando)
	if err != nil {
		return reparto.Snapshot{}, err
	}
	defer rows.Close()
	m := map[string]string{}
	reg := "RD-IX-seed"
	for rows.Next() {
		var k, v, r string
		if err := rows.Scan(&k, &v, &r); err != nil {
			return reparto.Snapshot{}, err
		}
		m[k] = v
		reg = r
	}
	d := func(k string) decimal.Decimal { x, _ := decimal.NewFromString(m[k]); return x }
	return reparto.Snapshot{
		AdminPct: d("admin_pct"), SocialPct: d("social_pct"), ReservaPct: d("reserva_pct"),
		PondCine: d("pond_cine"), PondUnitario: d("pond_unitario"), PondSerie: d("pond_serie"), PondSketch: d("pond_sketch"),
		Wa: d("wa"), Wb: d("wb"), Wc: d("wc"), UmbralMatch: d("umbral_match"), Reglamento: reg,
	}, nil
}

func (s *Store) FilasParametros(ctx context.Context) ([]map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT clave,valor::text,vigente_desde::text,COALESCE(vigente_hasta::text,''),organo,reglamento FROM parametros ORDER BY clave`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var k, v, d, h, o, r string
		if err := rows.Scan(&k, &v, &d, &h, &o, &r); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"clave": k, "valor": v, "desde": d, "hasta": h, "organo": o, "reglamento": r})
	}
	return out, rows.Err()
}

func (s *Store) GuardarProceso(ctx context.Context, p aplicacion.ProcesoVista) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO procesos(id,circuito,etapa,periodo,bolsa_id,revision,rechazo)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET etapa=EXCLUDED.etapa, revision=EXCLUDED.revision, rechazo=EXCLUDED.rechazo`,
		p.ID, string(p.Circuito), string(p.Etapa), p.Periodo, p.BolsaID, p.Revision, p.RechazoMotivo)
	return err
}

func (s *Store) ProcesoPorID(ctx context.Context, id string) (aplicacion.ProcesoVista, error) {
	var p aplicacion.ProcesoVista
	var circ, et string
	err := s.pool.QueryRow(ctx, `SELECT id,circuito,etapa,periodo,bolsa_id,revision,rechazo FROM procesos WHERE id=$1`, id).
		Scan(&p.ID, &circ, &et, &p.Periodo, &p.BolsaID, &p.Revision, &p.RechazoMotivo)
	p.Circuito = reparto.Circuito(circ)
	p.Etapa = reparto.Etapa(et)
	if err != nil {
		return p, err
	}
	rows, err := s.pool.Query(ctx, `SELECT rol,actor_id,revision FROM firmas WHERE proceso_id=$1`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var f reparto.Firma
		if err := rows.Scan(&f.Rol, &f.ActorID, &f.SobreRev); err != nil {
			return p, err
		}
		p.Firmas = append(p.Firmas, f)
	}
	return p, nil
}

func (s *Store) ListarProcesos(ctx context.Context) ([]aplicacion.ProcesoVista, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM procesos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	var out []aplicacion.ProcesoVista
	for _, id := range ids {
		p, err := s.ProcesoPorID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GuardarFirma(ctx context.Context, procesoID, rol, actorID string, rev int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO firmas(proceso_id,rol,actor_id,revision) VALUES($1,$2,$3,$4)`, procesoID, rol, actorID, rev)
	return err
}

func (s *Store) GuardarResultado(ctx context.Context, procesoID string, r reparto.Resultado) error {
	_, _ = s.pool.Exec(ctx, `DELETE FROM resultados_obra WHERE proceso_id=$1`, procesoID)
	_, _ = s.pool.Exec(ctx, `DELETE FROM resultados_titular WHERE proceso_id=$1`, procesoID)
	for _, o := range r.Obras {
		_, err := s.pool.Exec(ctx, `INSERT INTO resultados_obra VALUES($1,$2,$3,$4,$5,$6)`, procesoID, o.ObraID, o.Puntos.String(), o.Importe.String(), o.Retenida, o.Motivo)
		if err != nil {
			return err
		}
	}
	for _, t := range r.Titulares {
		_, err := s.pool.Exec(ctx, `INSERT INTO resultados_titular VALUES($1,$2,$3,$4,$5,$6)`, procesoID, t.ObraID, t.TitularID, t.IPI, t.Porcentaje.String(), t.Importe.String())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ResultadoDe(ctx context.Context, procesoID string) (reparto.Resultado, error) {
	var r reparto.Resultado
	rows, err := s.pool.Query(ctx, `SELECT obra_id,puntos,importe,retenida,motivo FROM resultados_obra WHERE proceso_id=$1`, procesoID)
	if err != nil {
		return r, err
	}
	for rows.Next() {
		var l reparto.LineaObra
		var pts, imp string
		if err := rows.Scan(&l.ObraID, &pts, &imp, &l.Retenida, &l.Motivo); err != nil {
			rows.Close()
			return r, err
		}
		l.Puntos, _ = decimal.NewFromString(pts)
		l.Importe, _ = decimal.NewFromString(imp)
		r.Obras = append(r.Obras, l)
		r.Neto = r.Neto.Add(l.Importe)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT obra_id,titular_id,ipi,porcentaje,importe FROM resultados_titular WHERE proceso_id=$1`, procesoID)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var l reparto.LineaTitular
		var pct, imp string
		if err := rows.Scan(&l.ObraID, &l.TitularID, &l.IPI, &pct, &imp); err != nil {
			return r, err
		}
		l.Porcentaje, _ = decimal.NewFromString(pct)
		l.Importe, _ = decimal.NewFromString(imp)
		r.Titulares = append(r.Titulares, l)
	}
	return r, rows.Err()
}

func (s *Store) LiquidacionesDeTitular(ctx context.Context, titularID string) (reparto.Resultado, string, error) {
	var r reparto.Resultado
	rows, err := s.pool.Query(ctx, `SELECT obra_id,titular_id,ipi,porcentaje,importe,proceso_id FROM resultados_titular WHERE titular_id=$1`, titularID)
	if err != nil {
		return r, "", err
	}
	defer rows.Close()
	proc := ""
	for rows.Next() {
		var l reparto.LineaTitular
		var pct, imp, p string
		if err := rows.Scan(&l.ObraID, &l.TitularID, &l.IPI, &pct, &imp, &p); err != nil {
			return r, "", err
		}
		l.Porcentaje, _ = decimal.NewFromString(pct)
		l.Importe, _ = decimal.NewFromString(imp)
		r.Titulares = append(r.Titulares, l)
		r.Neto = r.Neto.Add(l.Importe)
		proc = p
	}
	return r, proc, rows.Err()
}

func (s *Store) Asentar(ctx context.Context, a aplicacion.Asiento) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO asientos(id,hecho,ref_tipo,ref_id,payload,cuando) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO NOTHING`, a.ID, a.Hecho, a.RefTipo, a.RefID, a.Payload, a.Cuando)
	return err
}

func (s *Store) Asientos(ctx context.Context, refTipo, refID string) ([]aplicacion.Asiento, error) {
	q := `SELECT id,hecho,ref_tipo,ref_id,payload,cuando FROM asientos`
	args := []any{}
	if refTipo != "" {
		q += ` WHERE ref_tipo=$1 AND ref_id=$2`
		args = append(args, refTipo, refID)
	}
	q += ` ORDER BY cuando`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []aplicacion.Asiento
	for rows.Next() {
		var a aplicacion.Asiento
		if err := rows.Scan(&a.ID, &a.Hecho, &a.RefTipo, &a.RefID, &a.Payload, &a.Cuando); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AsientoPorID(ctx context.Context, id string) (aplicacion.Asiento, error) {
	var a aplicacion.Asiento
	err := s.pool.QueryRow(ctx, `SELECT id,hecho,ref_tipo,ref_id,payload,cuando FROM asientos WHERE id=$1`, id).
		Scan(&a.ID, &a.Hecho, &a.RefTipo, &a.RefID, &a.Payload, &a.Cuando)
	return a, err
}

func (s *Store) Encolar(ctx context.Context, tipo, payload string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO cola_trabajos(tipo,payload) VALUES($1,$2)`, tipo, payload)
	return err
}

func (s *Store) TomarTrabajo(ctx context.Context) (int64, string, string, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, "", "", false, err
	}
	defer tx.Rollback(ctx)
	var id int64
	var tipo, payload string
	err = tx.QueryRow(ctx, `SELECT id,tipo,payload FROM cola_trabajos WHERE estado='pendiente' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &tipo, &payload)
	if err == pgx.ErrNoRows {
		return 0, "", "", false, nil
	}
	if err != nil {
		return 0, "", "", false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE cola_trabajos SET estado='tomado' WHERE id=$1`, id); err != nil {
		return 0, "", "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", "", false, err
	}
	return id, tipo, payload, true, nil
}

func (s *Store) CerrarTrabajo(ctx context.Context, id int64, errMsg string) error {
	est := "hecho"
	if errMsg != "" {
		est = "error"
	}
	_, err := s.pool.Exec(ctx, `UPDATE cola_trabajos SET estado=$2, error=$3 WHERE id=$1`, id, est, errMsg)
	return err
}

func (s *Store) CalendarioPendiente(ctx context.Context, hoy string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT periodo FROM calendario WHERE disparado=FALSE AND fecha_apertura<=$1::date`, hoy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) MarcarCalendarioDisparado(ctx context.Context, periodo string) error {
	_, err := s.pool.Exec(ctx, `UPDATE calendario SET disparado=TRUE WHERE periodo=$1`, periodo)
	return err
}

func (s *Store) Alertas(ctx context.Context) ([]aplicacion.Alerta, error) {
	var out []aplicacion.Alerta
	n := 0
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM usos WHERE oni=TRUE`).Scan(&n)
	if n > 0 {
		out = append(out, aplicacion.Alerta{ID: "oni", Tipo: "oni", Detalle: fmt.Sprintf("%d usos en ONI", n)})
	}
	n = 0
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM obras o WHERE (SELECT COALESCE(SUM(porcentaje),0) FROM declaraciones d WHERE d.obra_id=o.id)<>100`).Scan(&n)
	if n > 0 {
		out = append(out, aplicacion.Alerta{ID: "decl", Tipo: "declaracion_incompleta", Detalle: fmt.Sprintf("%d obras sin 100%%", n)})
	}
	return out, nil
}

func (s *Store) Anticipos(ctx context.Context) ([]aplicacion.Anticipo, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,titular_id,monto,estado FROM anticipos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []aplicacion.Anticipo
	for rows.Next() {
		var a aplicacion.Anticipo
		var m string
		if err := rows.Scan(&a.ID, &a.TitularID, &m, &a.Estado); err != nil {
			return nil, err
		}
		a.Monto, _ = decimal.NewFromString(m)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GuardarAnticipo(ctx context.Context, a aplicacion.Anticipo) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO anticipos VALUES($1,$2,$3,$4)`, a.ID, a.TitularID, a.Monto.String(), a.Estado)
	return err
}

func (s *Store) Reclamaciones(ctx context.Context) ([]map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,titular_id,detalle,estado FROM reclamaciones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var id, t, d, e string
		if err := rows.Scan(&id, &t, &d, &e); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"id": id, "titular_id": t, "detalle": d, "estado": e})
	}
	return out, rows.Err()
}

func (s *Store) SembrarSiVacio(ctx context.Context, hashAdmin, hashTitular, hashAuditor string) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM usuarios`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	sqls := []string{
		`INSERT INTO titulares VALUES
		 ('ana','Ana Restrepo','I-100','t','escritor'),
		 ('bruno','Bruno Mejia','I-200','t','escritor'),
		 ('carla','Carla Nieto','I-300','t','escritor')`,
		`INSERT INTO obras VALUES
		 ('pelicula-x','Pelicula X','IDA-X','','tt100','cinematografica'),
		 ('serie-y','Serie Y','IDA-Y','','tt200','serie'),
		 ('obra-z','Obra Z','IDA-Z','','','serie')`,
		`INSERT INTO declaraciones VALUES
		 ('pelicula-x','ana','I-100',100),
		 ('serie-y','ana','I-100',60),
		 ('serie-y','bruno','I-200',40),
		 ('obra-z','carla','I-300',40)`,
		`INSERT INTO alias_obra VALUES ('canal-z','id_fuente','PX-1','pelicula-x','seed')`,
		`INSERT INTO bolsas VALUES ('bolsa-z','canal-z','2025','nacional',1000000)`,
		`INSERT INTO parametros(clave,valor,vigente_desde,organo,reglamento) VALUES
		 ('admin_pct',20,'2024-01-01','asamblea','RD-IX-seed'),
		 ('social_pct',10,'2024-01-01','asamblea','RD-IX-seed'),
		 ('reserva_pct',5,'2024-01-01','asamblea','RD-IX-seed'),
		 ('pond_cine',5,'2024-01-01','consejo','RD-IX-seed'),
		 ('pond_unitario',2.8,'2024-01-01','consejo','RD-IX-seed'),
		 ('pond_serie',1.3,'2024-01-01','consejo','RD-IX-seed'),
		 ('pond_sketch',0.8,'2024-01-01','consejo','RD-IX-seed'),
		 ('wa',0.5,'2024-01-01','consejo','RD-IX-seed-sintetico'),
		 ('wb',0.3,'2024-01-01','consejo','RD-IX-seed-sintetico'),
		 ('wc',0.2,'2024-01-01','consejo','RD-IX-seed-sintetico'),
		 ('umbral_match',0.92,'2024-01-01','consejo','RD-IX-seed')`,
		`INSERT INTO calendario VALUES ('2025','2025-01-15',false)`,
		`INSERT INTO anticipos VALUES ('ant-1','ana',500000,'vigente')`,
		`INSERT INTO reclamaciones VALUES ('rec-1','bruno','diferencia de puntos TV','abierta')`,
	}
	for _, q := range sqls {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO usuarios(id,email,nombre,rol,titular_id,password_hash) VALUES
		('u-admin','admin@intela.local','Mesa de distribucion','administrador',NULL,$1),
		('u-dist','distribucion@intela.local','Rol distribucion','distribucion',NULL,$1),
		('u-cont','contabilidad@intela.local','Rol contabilidad','contabilidad',NULL,$1),
		('u-ana','titular@intela.local','Ana Restrepo','titular','ana',$2),
		('u-aud','auditor@intela.local','Revisor fiscal','auditoria',NULL,$3)`,
		hashAdmin, hashTitular, hashAuditor)
	return err
}

type RelojReal struct{}

func (RelojReal) Ahora() time.Time { return time.Now().UTC() }

type NotifLog struct{}

func (NotifLog) Notificar(ctx context.Context, dest, asunto, cuerpo string) (string, error) {
	return fmt.Sprintf("acuse-%d", time.Now().UnixNano()), nil
}

type Disco struct{ Dir string }

func (d Disco) Poner(ctx context.Context, clave string, datos []byte) error {
	path := d.Dir + "/" + clave
	if err := os.MkdirAll(path[:strings.LastIndex(path, "/")], 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, datos, 0o644)
}

type SimLocal struct{ S *Store }

func (m SimLocal) Candidatos(ctx context.Context, titulo string) ([]identificacion.Candidato, error) {
	obras, err := m.S.ListarObras(ctx)
	if err != nil {
		return nil, err
	}
	var out []identificacion.Candidato
	for _, o := range obras {
		p := identificacion.SimilitudTitulo(titulo, o.Titulo)
		if p.GreaterThan(decimal.NewFromFloat(0.3)) {
			out = append(out, identificacion.Candidato{ObraID: o.ID, Puntaje: p})
		}
	}
	return out, nil
}
