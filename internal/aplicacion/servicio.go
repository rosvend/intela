package aplicacion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

type Servicio struct {
	Repo  Repositorios
	Reloj Reloj
	Obj   AlmacenObjetos
	Notif Notificador
	Sim   Similitud
}

func (s *Servicio) Login(ctx context.Context, email, clave string) (token string, u Usuario, err error) {
	u, hash, err := s.Repo.UsuarioPorEmail(ctx, email)
	if err != nil {
		return "", Usuario{}, fmt.Errorf("credenciales")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(clave)) != nil {
		return "", Usuario{}, fmt.Errorf("credenciales")
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token = hex.EncodeToString(b)
	if err := s.Repo.GuardarSesion(ctx, token, u.ID); err != nil {
		return "", Usuario{}, err
	}
	return token, u, nil
}

func (s *Servicio) CargarReporte(ctx context.Context, actor Usuario, fuente, periodo, nombre string, r io.Reader) (reporteID string, n int, err error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	reporteID = "rep-" + sha[:12]
	clave := "reportes/" + periodo + "/" + sha + "/" + nombre
	if err := s.Obj.Poner(ctx, clave, raw); err != nil {
		return "", 0, err
	}
	if err := s.Repo.GuardarReporte(ctx, reporteID, fuente, periodo, sha, clave, len(raw)); err != nil {
		return "", 0, err
	}
	usos, err := parsearCSV(reporteID, fuente, raw)
	if err != nil {
		return "", 0, err
	}
	if err := s.Repo.GuardarUsos(ctx, usos); err != nil {
		return "", 0, err
	}
	_ = s.Repo.Asentar(ctx, Asiento{
		ID: "as-" + reporteID, Hecho: "ingesta.reporte", RefTipo: "reporte", RefID: reporteID,
		Payload: fmt.Sprintf(`{"fuente":%q,"sha256":%q,"registros":%d}`, fuente, sha, len(usos)),
		Cuando:  s.Reloj.Ahora(),
	})
	_ = s.Repo.Encolar(ctx, "identificar", reporteID)
	return reporteID, len(usos), nil
}

func parsearCSV(reporteID, fuente string, raw []byte) ([]UsoPersistido, error) {
	rd := csv.NewReader(strings.NewReader(string(raw)))
	rd.TrimLeadingSpace = true
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("csv vacio")
	}
	head := map[string]int{}
	for i, h := range rows[0] {
		head[strings.ToLower(strings.TrimSpace(h))] = i
	}
	col := func(row []string, names ...string) string {
		for _, n := range names {
			if i, ok := head[n]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	var out []UsoPersistido
	for i, row := range rows[1:] {
		titulo := col(row, "titulo", "title", "show_name")
		if titulo == "" {
			continue
		}
		mod := strings.ToLower(col(row, "modalidad", "fuente_tipo"))
		if mod == "" {
			mod = "tv"
			if strings.Contains(strings.ToLower(fuente), "ott") || strings.Contains(strings.ToLower(fuente), "netflix") {
				mod = "ott"
			}
			if strings.Contains(strings.ToLower(fuente), "cine") {
				mod = "cine"
			}
		}
		dec := func(names ...string) decimal.Decimal {
			v := col(row, names...)
			if v == "" {
				return decimal.Zero
			}
			d, err := decimal.NewFromString(strings.ReplaceAll(v, ",", "."))
			if err != nil {
				return decimal.Zero
			}
			return d
		}
		em := dec("emisiones", "emision").IntPart()
		if em == 0 {
			em = 1
		}
		out = append(out, UsoPersistido{
			ID:        fmt.Sprintf("%s-%d", reporteID, i+1),
			ReporteID: reporteID, Fuente: fuente, Titulo: titulo,
			IDsFuente:     col(row, "id_ficha", "show_id", "netflix_id", "id"),
			Modalidad:     reparto.Modalidad(mod),
			TipoObra:      col(row, "tipo_obra", "tipo", "content_type"),
			DuracionMin:   dec("duracion", "duracion_min", "duration"),
			Emisiones:     em,
			Rating:        dec("rating"),
			Taquilla:      dec("taquilla"),
			Vistas:        dec("vistas", "stream_starts", "v"),
			MinutosVistos: dec("minutos_vistos", "du"),
			PB:            dec("pb"),
			ONI:           true, Escalon: "pendiente",
		})
	}
	return out, nil
}

func (s *Servicio) IdentificarPendientes(ctx context.Context) (int, error) {
	usos, err := s.Repo.UsosSinResolver(ctx)
	if err != nil {
		return 0, err
	}
	snap, err := s.Repo.SnapshotVigente(ctx, s.Reloj.Ahora())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, u := range usos {
		alias, _ := s.Repo.Alias(ctx, u.Fuente, "id_fuente", u.IDsFuente)
		glob, _ := s.Repo.ObraPorIDGlobal(ctx, "", "", "")
		cands, _ := s.Sim.Candidatos(ctx, u.Titulo)
		res := identificacion.Cascada(identificacion.Entrada{
			Fuente: u.Fuente, TipoID: "id_fuente", ValorID: u.IDsFuente, Titulo: u.Titulo,
		}, alias, glob, cands, snap.UmbralMatch)
		if err := s.Repo.ActualizarUsoMatch(ctx, u.ID, res.ObraID, res.Escalon, res.ONI); err != nil {
			return n, err
		}
		_ = s.Repo.Asentar(ctx, Asiento{
			ID: "as-id-" + u.ID, Hecho: "identificacion.match", RefTipo: "uso", RefID: u.ID,
			Payload: fmt.Sprintf(`{"escalon":%q,"obra_id":%q,"oni":%v,"puntaje":%s}`, res.Escalon, res.ObraID, res.ONI, res.Puntaje.String()),
			Cuando:  s.Reloj.Ahora(),
		})
		n++
	}
	return n, nil
}

func (s *Servicio) ResolverONI(ctx context.Context, actor Usuario, usoID, obraID string) error {
	if actor.Rol != "administrador" && actor.Rol != "distribucion" {
		return fmt.Errorf("no autorizado")
	}
	u, err := usoPorID(ctx, s, usoID)
	if err != nil {
		return err
	}
	if err := s.Repo.ActualizarUsoMatch(ctx, usoID, obraID, "manual", false); err != nil {
		return err
	}
	if u.IDsFuente != "" {
		_ = s.Repo.GuardarAlias(ctx, u.Fuente, "id_fuente", u.IDsFuente, obraID, actor.ID)
	}
	return s.Repo.Asentar(ctx, Asiento{
		ID: "as-oni-" + usoID, Hecho: "identificacion.manual", RefTipo: "uso", RefID: usoID,
		Payload: fmt.Sprintf(`{"obra_id":%q,"actor":%q}`, obraID, actor.ID), Cuando: s.Reloj.Ahora(),
	})
}

func usoPorID(ctx context.Context, s *Servicio, id string) (UsoPersistido, error) {
	all, err := s.Repo.ListarONI(ctx)
	if err != nil {
		return UsoPersistido{}, err
	}
	pend, _ := s.Repo.UsosSinResolver(ctx)
	all = append(all, pend...)
	for _, u := range all {
		if u.ID == id {
			return u, nil
		}
	}
	usos, err := s.Repo.UsosDePeriodo(ctx, "")
	if err == nil {
		for _, u := range usos {
			if u.ID == id {
				return u, nil
			}
		}
	}
	return UsoPersistido{ID: id}, nil
}

func (s *Servicio) AbrirProceso(ctx context.Context, actor Usuario, bolsaID, circuito string) (ProcesoVista, error) {
	b, err := s.Repo.BolsaPorID(ctx, bolsaID)
	if err != nil {
		return ProcesoVista{}, err
	}
	id := fmt.Sprintf("proc-%s-%d", b.Periodo, s.Reloj.Ahora().Unix()%100000)
	p := ProcesoVista{
		Proceso: reparto.Proceso{ID: id, Circuito: reparto.Circuito(circuito), Etapa: reparto.EtapaRecaudo, Periodo: b.Periodo, Revision: 1},
		Periodo: b.Periodo, BolsaID: bolsaID,
	}
	if err := s.Repo.GuardarProceso(ctx, p); err != nil {
		return ProcesoVista{}, err
	}
	_ = s.Repo.Asentar(ctx, Asiento{ID: "as-open-" + id, Hecho: "proceso.abierto", RefTipo: "proceso", RefID: id, Payload: `{"bolsa":"` + bolsaID + `"}`, Cuando: s.Reloj.Ahora()})
	return p, nil
}

func (s *Servicio) Calcular(ctx context.Context, actor Usuario, procesoID string) (reparto.Resultado, error) {
	p, err := s.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return reparto.Resultado{}, err
	}
	b, err := s.Repo.BolsaPorID(ctx, p.BolsaID)
	if err != nil {
		return reparto.Resultado{}, err
	}
	snap, err := s.Repo.SnapshotVigente(ctx, s.Reloj.Ahora())
	if err != nil {
		return reparto.Resultado{}, err
	}
	decs, err := s.Repo.Declaraciones(ctx)
	if err != nil {
		return reparto.Resultado{}, err
	}
	raw, err := s.Repo.UsosDePeriodo(ctx, p.Periodo)
	if err != nil {
		return reparto.Resultado{}, err
	}
	var usos []reparto.Uso
	for _, u := range raw {
		if u.ONI || u.ObraID == "" {
			continue
		}
		usos = append(usos, reparto.Uso{
			ObraID: u.ObraID, Modalidad: u.Modalidad, TipoObra: u.TipoObra,
			DuracionMin: u.DuracionMin, Emisiones: u.Emisiones, Rating: u.Rating,
			Taquilla: u.Taquilla, Vistas: u.Vistas, MinutosVistos: u.MinutosVistos, PB: u.PB,
		})
	}
	bolsa := reparto.Bolsa{UsuarioID: b.UsuarioID, Periodo: b.Periodo, Circuito: b.Circuito, Bruto: b.Bruto}
	res := reparto.Repartir(bolsa, usos, decs, snap)
	if err := s.Repo.GuardarResultado(ctx, procesoID, res); err != nil {
		return res, err
	}
	payload, _ := json.Marshal(map[string]any{"neto": res.Neto.String(), "valor_punto": res.ValorPunto.String(), "obras": len(res.Obras)})
	_ = s.Repo.Asentar(ctx, Asiento{ID: "as-calc-" + procesoID, Hecho: "reparto.calculo", RefTipo: "proceso", RefID: procesoID, Payload: string(payload), Cuando: s.Reloj.Ahora()})
	return res, nil
}

func (s *Servicio) Firmar(ctx context.Context, actor Usuario, procesoID string) (reparto.Proceso, error) {
	p, err := s.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return reparto.Proceso{}, err
	}
	np, err := reparto.Firmar(p.Proceso, actor.Rol, actor.ID)
	if err != nil {
		return np, err
	}
	p.Proceso = np
	if err := s.Repo.GuardarProceso(ctx, p); err != nil {
		return np, err
	}
	_ = s.Repo.GuardarFirma(ctx, procesoID, actor.Rol, actor.ID, np.Revision)
	_ = s.Repo.Asentar(ctx, Asiento{ID: fmt.Sprintf("as-firma-%s-%s-%d", procesoID, actor.Rol, np.Revision), Hecho: "proceso.firma", RefTipo: "proceso", RefID: procesoID, Payload: `{"rol":"` + actor.Rol + `"}`, Cuando: s.Reloj.Ahora()})
	return np, nil
}

func (s *Servicio) Avanzar(ctx context.Context, actor Usuario, procesoID string) (reparto.Proceso, error) {
	p, err := s.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return reparto.Proceso{}, err
	}
	np, err := reparto.Avanzar(p.Proceso, actor.Rol)
	if err != nil {
		return np, err
	}
	p.Proceso = np
	if err := s.Repo.GuardarProceso(ctx, p); err != nil {
		return np, err
	}
	if np.Etapa == reparto.EtapaLiquidacionParcial || np.Etapa == reparto.EtapaLiquidacionFinal {
		if acuse, nerr := s.Notif.Notificar(ctx, "titulares@intela.local", "liquidacion "+procesoID, "liquidacion disponible"); nerr == nil {
			_ = s.Repo.Asentar(ctx, Asiento{ID: "as-notif-" + procesoID + "-" + string(np.Etapa), Hecho: "notificacion.acuse", RefTipo: "proceso", RefID: procesoID, Payload: `{"acuse":"` + acuse + `"}`, Cuando: s.Reloj.Ahora()})
		}
	}
	_ = s.Repo.Asentar(ctx, Asiento{ID: "as-av-" + procesoID + "-" + string(np.Etapa), Hecho: "proceso.avance", RefTipo: "proceso", RefID: procesoID, Payload: `{"etapa":"` + string(np.Etapa) + `"}`, Cuando: s.Reloj.Ahora()})
	return np, nil
}

func (s *Servicio) Rechazar(ctx context.Context, actor Usuario, procesoID, motivo string) (reparto.Proceso, error) {
	p, err := s.Repo.ProcesoPorID(ctx, procesoID)
	if err != nil {
		return reparto.Proceso{}, err
	}
	p.Proceso = reparto.Rechazar(p.Proceso, motivo)
	if err := s.Repo.GuardarProceso(ctx, p); err != nil {
		return p.Proceso, err
	}
	return p.Proceso, nil
}

func (s *Servicio) ExplicarCifra(ctx context.Context, asientoID string) ([]Asiento, error) {
	a, err := s.Repo.AsientoPorID(ctx, asientoID)
	if err != nil {
		return nil, err
	}
	rel, err := s.Repo.Asientos(ctx, a.RefTipo, a.RefID)
	if err != nil {
		return nil, err
	}
	return rel, nil
}

func (s *Servicio) AbrirDesdeCalendario(ctx context.Context) error {
	hoy := s.Reloj.Ahora().Format("2006-01-02")
	periodos, err := s.Repo.CalendarioPendiente(ctx, hoy)
	if err != nil {
		return err
	}
	for _, per := range periodos {
		_ = s.Repo.Encolar(ctx, "abrir_proceso", per)
		_ = s.Repo.MarcarCalendarioDisparado(ctx, per)
	}
	return nil
}

func (s *Servicio) ProcesarCola(ctx context.Context) error {
	id, tipo, payload, ok, err := s.Repo.TomarTrabajo(ctx)
	if err != nil || !ok {
		return err
	}
	var run error
	switch tipo {
	case "identificar":
		_, run = s.IdentificarPendientes(ctx)
	case "abrir_proceso":
		bolsas, e := s.Repo.ListarBolsas(ctx)
		run = e
		for _, b := range bolsas {
			if b.Periodo == payload {
				_, _ = s.AbrirProceso(ctx, Usuario{Rol: "sistema"}, b.ID, b.Circuito)
			}
		}
	default:
		run = fmt.Errorf("tipo desconocido %s", tipo)
	}
	msg := ""
	if run != nil {
		msg = run.Error()
	}
	return s.Repo.CerrarTrabajo(ctx, id, msg)
}
