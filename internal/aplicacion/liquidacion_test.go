package aplicacion

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

type repoLiquidacionMemoria struct {
	filas []FilaLiquidacion
	err   error
	visto struct {
		titularID string
		periodo   string
	}
}

func (r *repoLiquidacionMemoria) DeTitular(_ context.Context, titularID, periodo string) ([]FilaLiquidacion, error) {
	r.visto.titularID = titularID
	r.visto.periodo = periodo
	if r.err != nil {
		return nil, r.err
	}
	var out []FilaLiquidacion
	for _, f := range r.filas {
		if periodo != "" && f.Periodo != periodo {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

type exportadorEspia struct {
	pdf, excel Archivo
	errPDF     error
	errExcel   error
	vistoPDF   *Liquidacion
	vistoExcel *Liquidacion
}

func (e *exportadorEspia) PDF(liq Liquidacion) (Archivo, error) {
	copia := liq
	e.vistoPDF = &copia
	return e.pdf, e.errPDF
}

func (e *exportadorEspia) Excel(liq Liquidacion) (Archivo, error) {
	copia := liq
	e.vistoExcel = &copia
	return e.excel, e.errExcel
}

func ana() Usuario {
	return Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}
}

func filaCasa(periodo string) FilaLiquidacion {
	return FilaLiquidacion{
		Periodo:        periodo,
		ObraID:         "obra-completa",
		Titulo:         "La Casa de las Dos Palmas",
		Neto:           dec("3900"),
		ProcesoBruto:   dec("10000"),
		ProcesoAdmin:   dec("2000"),
		ProcesoSocial:  dec("1000"),
		ProcesoReserva: dec("500"),
		ProcesoNeto:    dec("6500"),
	}
}

func TestConsultarProrrateaBrutoYDeduccionesPorObra(t *testing.T) {
	repo := &repoLiquidacionMemoria{filas: []FilaLiquidacion{filaCasa("2026-01")}}
	s := ServicioLiquidacion{Repo: repo}

	liq, err := s.Consultar(context.Background(), ana(), "2026-01")
	if err != nil {
		t.Fatalf("Consultar: %v", err)
	}
	if repo.visto.titularID != "tit-ana" {
		t.Fatalf("consulto a %q, no al titular de la sesion", repo.visto.titularID)
	}
	if len(liq.Lineas) != 1 {
		t.Fatalf("lineas = %d", len(liq.Lineas))
	}
	l := liq.Lineas[0]
	if !l.Bruto.Equal(dec("6000")) || !l.Admin.Equal(dec("1200")) || !l.Social.Equal(dec("600")) || !l.Reserva.Equal(dec("300")) || !l.Neto.Equal(dec("3900")) {
		t.Fatalf("linea = %+v", l)
	}
	if !liq.Totales.Neto.Equal(dec("3900")) || !liq.Totales.Bruto.Equal(dec("6000")) {
		t.Fatalf("totales = %+v", liq.Totales)
	}
}

func TestConsultarRespetaElFiltroDePeriodo(t *testing.T) {
	repo := &repoLiquidacionMemoria{filas: []FilaLiquidacion{
		filaCasa("2026-01"),
		filaCasa("2026-02"),
	}}
	s := ServicioLiquidacion{Repo: repo}

	liq, err := s.Consultar(context.Background(), ana(), "2026-01")
	if err != nil {
		t.Fatalf("Consultar: %v", err)
	}
	if repo.visto.periodo != "2026-01" {
		t.Fatalf("periodo pasado al repo = %q", repo.visto.periodo)
	}
	if len(liq.Lineas) != 1 || liq.Lineas[0].Periodo != "2026-01" {
		t.Fatalf("no filtro: %+v", liq.Lineas)
	}
}

func TestConsultarRechazaPeriodoInvalido(t *testing.T) {
	s := ServicioLiquidacion{Repo: &repoLiquidacionMemoria{}}
	_, err := s.Consultar(context.Background(), ana(), "enero")
	if !errors.Is(err, ErrPeriodoInvalido) {
		t.Fatalf("err = %v, se esperaba ErrPeriodoInvalido", err)
	}
}

func TestConsultarTitularIDSaleDeLaSesionNoDeUnParametro(t *testing.T) {
	repo := &repoLiquidacionMemoria{}
	s := ServicioLiquidacion{Repo: repo}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	if _, err := s.Consultar(context.Background(), actor, ""); err != nil {
		t.Fatalf("Consultar: %v", err)
	}
	if repo.visto.titularID != "tit-ana" {
		t.Fatal("el titular de la sesion es quien se consulta, no uno que venga en la URL")
	}
}

func TestConsultarSinRolTitularEsNoAutorizado(t *testing.T) {
	s := ServicioLiquidacion{Repo: &repoLiquidacionMemoria{filas: []FilaLiquidacion{filaCasa("2026-01")}}}
	_, err := s.Consultar(context.Background(), Usuario{ID: "usr-1", Rol: RolAdministrador}, "")
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("err = %v", err)
	}
}

func TestConsultarTitularSinTitularIDEsNoAutorizado(t *testing.T) {
	s := ServicioLiquidacion{Repo: &repoLiquidacionMemoria{}}
	_, err := s.Consultar(context.Background(), Usuario{ID: "usr-ana", Rol: RolTitular}, "")
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("err = %v", err)
	}
}

func TestExportarPDFPasaLaMismaLiquidacionQueConsultar(t *testing.T) {
	repo := &repoLiquidacionMemoria{filas: []FilaLiquidacion{filaCasa("2026-01")}}
	exp := &exportadorEspia{pdf: Archivo{Nombre: "liquidacion-2026-01.pdf", TipoMIME: "application/pdf", Contenido: []byte("%PDF")}}
	s := ServicioLiquidacion{Repo: repo, Exportador: exp}

	archivo, err := s.Exportar(context.Background(), ana(), "2026-01", "pdf")
	if err != nil {
		t.Fatalf("Exportar: %v", err)
	}
	if archivo.Nombre != "liquidacion-2026-01.pdf" {
		t.Fatalf("nombre = %q", archivo.Nombre)
	}
	if exp.vistoPDF == nil || len(exp.vistoPDF.Lineas) != 1 {
		t.Fatal("el generador no recibio la liquidacion")
	}
	if !exp.vistoPDF.Totales.Neto.Equal(dec("3900")) {
		t.Fatalf("el PDF no lleva los mismos totales que el panel: %s", exp.vistoPDF.Totales.Neto)
	}
}

func TestExportarExcelPasaLaMismaLiquidacionQueConsultar(t *testing.T) {
	repo := &repoLiquidacionMemoria{filas: []FilaLiquidacion{filaCasa("2026-01")}}
	exp := &exportadorEspia{excel: Archivo{Nombre: "liquidacion-2026-01.xlsx"}}
	s := ServicioLiquidacion{Repo: repo, Exportador: exp}

	if _, err := s.Exportar(context.Background(), ana(), "2026-01", "XLSX"); err != nil {
		t.Fatalf("Exportar: %v", err)
	}
	if exp.vistoExcel == nil {
		t.Fatal("no se llamo al generador Excel")
	}
}

func TestExportarFormatoDesconocidoEsInvalido(t *testing.T) {
	s := ServicioLiquidacion{
		Repo:       &repoLiquidacionMemoria{},
		Exportador: &exportadorEspia{},
	}
	_, err := s.Exportar(context.Background(), ana(), "", "csv")
	if !errors.Is(err, ErrFormatoInvalido) {
		t.Fatalf("err = %v, se esperaba ErrFormatoInvalido", err)
	}
}

func TestConsultarSinFilasDevuelveListaVaciaNoNil(t *testing.T) {
	s := ServicioLiquidacion{Repo: &repoLiquidacionMemoria{}}
	liq, err := s.Consultar(context.Background(), ana(), "")
	if err != nil {
		t.Fatalf("Consultar: %v", err)
	}
	if liq.Lineas == nil {
		t.Fatal("lineas nil se serializa como null; tiene que ser slice vacio")
	}
}
