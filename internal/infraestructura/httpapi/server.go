package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/xuri/excelize/v2"
)

type API struct {
	Svc *aplicacion.Servicio
	Repo aplicacion.Repositorios
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) })
	r.Post("/api/sesiones", a.login)
	r.Group(func(r chi.Router) {
		r.Use(a.auth)
		r.Get("/api/me", a.me)
		r.Get("/api/obras", a.obras)
		r.Get("/api/oni", a.oni)
		r.Post("/api/oni/{id}/resolver", a.resolverONI)
		r.Post("/api/reportes", a.cargar)
		r.Get("/api/bolsas", a.bolsas)
		r.Get("/api/procesos", a.procesos)
		r.Post("/api/procesos", a.abrir)
		r.Post("/api/procesos/{id}/calcular", a.calcular)
		r.Post("/api/procesos/{id}/firmar", a.firmar)
		r.Post("/api/procesos/{id}/avanzar", a.avanzar)
		r.Post("/api/procesos/{id}/rechazar", a.rechazar)
		r.Get("/api/procesos/{id}/resultado", a.resultado)
		r.Get("/api/liquidaciones", a.liquidaciones)
		r.Get("/api/liquidaciones/{id}/xlsx", a.xlsx)
		r.Get("/api/liquidaciones/{id}/pdf", a.pdf)
		r.Get("/api/asientos", a.asientos)
		r.Get("/api/asientos/{id}/explicar", a.explicar)
		r.Get("/api/parametros", a.parametros)
		r.Get("/api/alertas", a.alertas)
		r.Get("/api/anticipos", a.anticipos)
		r.Get("/api/reclamaciones", a.reclamaciones)
	})
	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error, code int) {
	http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), code)
}

type ctxKey int

const userKey ctxKey = 1

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if h == "" {
			http.Error(w, `{"error":"no autorizado"}`, 401)
			return
		}
		u, err := a.Repo.UsuarioPorToken(r.Context(), h)
		if err != nil {
			http.Error(w, `{"error":"sesion invalida"}`, 401)
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithUser(r, u)))
	})
}

func contextWithUser(r *http.Request, u aplicacion.Usuario) context.Context {
	return context.WithValue(r.Context(), userKey, u)
}

func user(r *http.Request) aplicacion.Usuario {
	u, _ := r.Context().Value(userKey).(aplicacion.Usuario)
	return u
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Clave string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	tok, u, err := a.Svc.Login(r.Context(), in.Email, in.Clave)
	if err != nil {
		writeErr(w, err, 401)
		return
	}
	writeJSON(w, map[string]any{"token": tok, "usuario": u})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, user(r)) }

func (a *API) obras(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.ListarObras(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) oni(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.ListarONI(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) resolverONI(w http.ResponseWriter, r *http.Request) {
	var in struct{ ObraID string `json:"obra_id"` }
	_ = json.NewDecoder(r.Body).Decode(&in)
	if err := a.Svc.ResolverONI(r.Context(), user(r), chi.URLParam(r, "id"), in.ObraID); err != nil {
		writeErr(w, err, 400); return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}
func (a *API) cargar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil { writeErr(w, err, 400); return }
	fuente := r.FormValue("fuente")
	periodo := r.FormValue("periodo")
	f, h, err := r.FormFile("archivo")
	if err != nil { writeErr(w, err, 400); return }
	defer f.Close()
	id, n, err := a.Svc.CargarReporte(r.Context(), user(r), fuente, periodo, h.Filename, f)
	if err != nil { writeErr(w, err, 400); return }
	_, _ = a.Svc.IdentificarPendientes(r.Context())
	writeJSON(w, map[string]any{"reporte_id": id, "registros": n})
}
func (a *API) bolsas(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.ListarBolsas(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) procesos(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.ListarProcesos(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) abrir(w http.ResponseWriter, r *http.Request) {
	var in struct{ BolsaID, Circuito string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	p, err := a.Svc.AbrirProceso(r.Context(), user(r), in.BolsaID, in.Circuito)
	if err != nil { writeErr(w, err, 400); return }
	writeJSON(w, p)
}
func (a *API) calcular(w http.ResponseWriter, r *http.Request) {
	res, err := a.Svc.Calcular(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 400); return }
	writeJSON(w, res)
}
func (a *API) firmar(w http.ResponseWriter, r *http.Request) {
	p, err := a.Svc.Firmar(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 400); return }
	writeJSON(w, p)
}
func (a *API) avanzar(w http.ResponseWriter, r *http.Request) {
	p, err := a.Svc.Avanzar(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 400); return }
	writeJSON(w, p)
}
func (a *API) rechazar(w http.ResponseWriter, r *http.Request) {
	var in struct{ Motivo string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	p, err := a.Svc.Rechazar(r.Context(), user(r), chi.URLParam(r, "id"), in.Motivo)
	if err != nil { writeErr(w, err, 400); return }
	writeJSON(w, p)
}
func (a *API) resultado(w http.ResponseWriter, r *http.Request) {
	res, err := a.Repo.ResultadoDe(r.Context(), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 404); return }
	writeJSON(w, res)
}
func (a *API) liquidaciones(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if u.Rol == "titular" && u.TitularID != "" {
		res, proc, err := a.Repo.LiquidacionesDeTitular(r.Context(), u.TitularID)
		if err != nil { writeErr(w, err, 500); return }
		writeJSON(w, map[string]any{"proceso_id": proc, "resultado": res})
		return
	}
	a.procesos(w, r)
}
func (a *API) xlsx(w http.ResponseWriter, r *http.Request) {
	res, err := a.Repo.ResultadoDe(r.Context(), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 404); return }
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "liquidacion")
	_ = f.SetCellValue("liquidacion", "A1", "obra")
	_ = f.SetCellValue("liquidacion", "B1", "titular")
	_ = f.SetCellValue("liquidacion", "C1", "ipi")
	_ = f.SetCellValue("liquidacion", "D1", "porcentaje")
	_ = f.SetCellValue("liquidacion", "E1", "importe")
	for i, t := range res.Titulares {
		row := i + 2
		_ = f.SetCellValue("liquidacion", fmt.Sprintf("A%d", row), t.ObraID)
		_ = f.SetCellValue("liquidacion", fmt.Sprintf("B%d", row), t.TitularID)
		_ = f.SetCellValue("liquidacion", fmt.Sprintf("C%d", row), t.IPI)
		_ = f.SetCellValue("liquidacion", fmt.Sprintf("D%d", row), t.Porcentaje.String())
		_ = f.SetCellValue("liquidacion", fmt.Sprintf("E%d", row), t.Importe.String())
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=liquidacion.xlsx")
	_ = f.Write(w)
}
func (a *API) pdf(w http.ResponseWriter, r *http.Request) {
	res, err := a.Repo.ResultadoDe(r.Context(), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 404); return }
	var b strings.Builder
	b.WriteString("%PDF-1.1\n1 0 obj<< /Type /Catalog /Pages 2 0 R >>endobj\n")
	txt := "Intela liquidacion\n"
	for _, t := range res.Titulares {
		txt += fmt.Sprintf("%s %s %s\n", t.ObraID, t.TitularID, t.Importe)
	}
	stream := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET", strings.ReplaceAll(txt, "(", " "))
	b.WriteString("2 0 obj<< /Type /Pages /Kids [3 0 R] /Count 1 >>endobj\n")
	b.WriteString("3 0 obj<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>endobj\n")
	b.WriteString(fmt.Sprintf("4 0 obj<< /Length %d >>stream\n%s\nendstream\nendobj\n", len(stream), stream))
	b.WriteString("5 0 obj<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>endobj\n")
	b.WriteString("xref\n0 6\ntrailer<< /Root 1 0 R /Size 6 >>\nstartxref\n0\n%%EOF")
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = io.WriteString(w, b.String())
}
func (a *API) asientos(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.Asientos(r.Context(), r.URL.Query().Get("tipo"), r.URL.Query().Get("id"))
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) explicar(w http.ResponseWriter, r *http.Request) {
	v, err := a.Svc.ExplicarCifra(r.Context(), chi.URLParam(r, "id"))
	if err != nil { writeErr(w, err, 404); return }
	writeJSON(w, v)
}
func (a *API) parametros(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.FilasParametros(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) alertas(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.Alertas(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) anticipos(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.Anticipos(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}
func (a *API) reclamaciones(w http.ResponseWriter, r *http.Request) {
	v, err := a.Repo.Reclamaciones(r.Context())
	if err != nil { writeErr(w, err, 500); return }
	writeJSON(w, v)
}

