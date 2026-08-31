// Package semilla construye y carga el dataset sintetico de desarrollo.
//
// Es el cmd/seed: un juego pequeno y legible, no un generador de volumen.
// Los valores que el cliente no ha entregado van etiquetados como sinteticos
// (ADR 0004). No se hacen pasar por datos de REDES SGC.
package semilla

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Identificadores estables. El seed es reproducible (ADR 0005): los mismos
// ids en cada corrida, y Construir no mira el reloj ni tira dados.
const (
	TitularAna   = "tit-ana"
	TitularBeto  = "tit-beto"
	TitularCarla = "tit-carla"

	UsuarioAdmin        = "usr-admin"
	UsuarioDistribucion = "usr-distribucion"
	UsuarioContabilidad = "usr-contabilidad"
	UsuarioAuditor      = "usr-auditor"
	UsuarioTitular      = "usr-titular"

	EmailAdmin        = "admin@redes.co"
	EmailDistribucion = "distribucion@redes.co"
	EmailContabilidad = "contabilidad@redes.co"
	EmailAuditor      = "auditor@redes.co"
	EmailTitular      = "ana@redes.co"

	ObraCine     = "obra-cine"
	ObraUnitario = "obra-unitario"
	ObraSerie    = "obra-serie"
	ObraSketch   = "obra-sketch"

	Periodo = "2025-01"

	FuenteTV   = "caracol"
	FuenteCine = "procinal"
	FuenteOTT  = "netflix"

	// Procedencia de los coeficientes OTT que el reglamento no publica.
	// ARRANQUE.md y el issue #22 piden marcarlos; el esquema no tiene
	// columna `origen`, asi que vive en `reglamento` y `organo`.
	ReglamentoSintetico = "RD-IX-seed-sintetico"
	OrganoSintetico     = "sintetico"
)

// Dataset es el juego completo, listo para persistir. No lleva hashes de
// contrasena: bcrypt no es determinista, y el hash se calcula al cargar.
type Dataset struct {
	Periodo       string
	Titulares     []Titular
	Usuarios      []Usuario
	Obras         []aplicacion.Obra
	Declaraciones []repertorio.Declaracion
	Reportes      []Reporte
	Bolsas        []aplicacion.BolsaPersistida
	Parametros    []Parametro
}

type Titular struct {
	ID             string
	Nombre         string
	IPI            string
	PersonaNatural bool
	Clase          string
	Email          string
}

type Usuario struct {
	ID        string
	Email     string
	Nombre    string
	Rol       aplicacion.Rol
	TitularID string
}

type Reporte struct {
	Fuente  string
	Periodo string
	Bytes   []byte
	Usos    []aplicacion.UsoPersistido
}

type Parametro struct {
	Clave        string
	Valor        decimal.Decimal
	VigenteDesde string
	Organo       string
	Reglamento   string
}

// Construir arma el dataset. Es una funcion pura: la misma entrada (ninguna)
// produce el mismo resultado. Las pruebas de unidad lo comprueban.
func Construir() Dataset {
	d := Dataset{Periodo: Periodo}
	d.titulares()
	d.usuarios()
	d.obrasYDeclaraciones()
	d.reportes()
	d.bolsas()
	d.parametros()
	return d
}

func (d *Dataset) titulares() {
	d.Titulares = []Titular{
		{ID: TitularAna, Nombre: "Ana Escritora", IPI: "IPI-00000001", PersonaNatural: true, Clase: "socio", Email: EmailTitular},
		{ID: TitularBeto, Nombre: "Beto Libretista", IPI: "IPI-00000002", PersonaNatural: true, Clase: "administrado"},
		{ID: TitularCarla, Nombre: "Carla Guionista", IPI: "IPI-00000003", PersonaNatural: true, Clase: "socio", Email: "carla@redes.co"},
	}
}

func (d *Dataset) usuarios() {
	d.Usuarios = []Usuario{
		{ID: UsuarioAdmin, Email: EmailAdmin, Nombre: "Admin", Rol: aplicacion.RolAdministrador},
		{ID: UsuarioDistribucion, Email: EmailDistribucion, Nombre: "Distribucion", Rol: aplicacion.RolDistribucion},
		{ID: UsuarioContabilidad, Email: EmailContabilidad, Nombre: "Contabilidad", Rol: aplicacion.RolContabilidad},
		{ID: UsuarioAuditor, Email: EmailAuditor, Nombre: "Auditor", Rol: aplicacion.RolAuditor},
		{ID: UsuarioTitular, Email: EmailTitular, Nombre: "Ana Escritora", Rol: aplicacion.RolTitular, TitularID: TitularAna},
	}
}

func (d *Dataset) obrasYDeclaraciones() {
	d.Obras = []aplicacion.Obra{
		{ID: ObraCine, Titulo: "Pelicula X", IDA: "IDA-PX", EIDR: "EIDR-PX", IMDB: "tt0001", Tipo: "cinematografica"},
		{ID: ObraUnitario, Titulo: "El Tercer Acto", IDA: "IDA-ETA", Tipo: "unitario"},
		{ID: ObraSerie, Titulo: "Serie Y", IDA: "IDA-SY", Tipo: "serie"},
		{ID: ObraSketch, Titulo: "Minuto Comico", Tipo: "sketches"},
	}

	pct := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }

	d.Declaraciones = []repertorio.Declaracion{
		// Completa: 60 + 40 = 100, IPI en las dos partes.
		{
			ObraID: ObraCine,
			Partes: []repertorio.Parte{
				{TitularID: TitularAna, IPI: "IPI-00000001", Porcentaje: pct("60")},
				{TitularID: TitularBeto, IPI: "IPI-00000002", Porcentaje: pct("40")},
			},
		},
		// Multi-coautor: tres partes que suman 100.
		{
			ObraID: ObraUnitario,
			Partes: []repertorio.Parte{
				{TitularID: TitularAna, IPI: "IPI-00000001", Porcentaje: pct("40")},
				{TitularID: TitularBeto, IPI: "IPI-00000002", Porcentaje: pct("35")},
				{TitularID: TitularCarla, IPI: "IPI-00000003", Porcentaje: pct("25")},
			},
		},
		// Incompleta: 60. Se retiene el total en reserva (R-04, RD 13.1.3).
		{
			ObraID: ObraSerie,
			Partes: []repertorio.Parte{
				{TitularID: TitularAna, IPI: "IPI-00000001", Porcentaje: pct("60")},
			},
		},
		// Completa de un solo autor, para cubrir sketches en la ponderacion.
		{
			ObraID: ObraSketch,
			Partes: []repertorio.Parte{
				{TitularID: TitularAna, IPI: "IPI-00000001", Porcentaje: pct("100")},
			},
		},
	}
}

func (d *Dataset) reportes() {
	// Las filas de TV reproducen el ejemplo numerico de formulas.md / RD 9.1.1
	// (Pelicula X, Serie Y) y anaden unitario y sketch para ejercitar la
	// tabla de ponderacion completa (5.0 / 2.8 / 1.3 / 0.8).
	tv := []aplicacion.UsoPersistido{
		usoTV(ObraCine, "Pelicula X", "PX-1", "cinematografica", "70", 1, "4.5"),
		usoTV(ObraSerie, "Serie Y", "SY-1", "serie", "48", 10, "9.0"),
		usoTV(ObraUnitario, "El Tercer Acto", "ETA-1", "unitario", "48", 1, "3.0"),
		usoTV(ObraSketch, "Minuto Comico", "MC-1", "sketches", "10", 2, "2.0"),
	}
	for i := range tv {
		tv[i].Fuente = FuenteTV
	}

	cine := []aplicacion.UsoPersistido{
		usoCine(ObraCine, "Pelicula X", "PX-1", "10000"),
	}
	cine[0].Fuente = FuenteCine

	ott := []aplicacion.UsoPersistido{
		usoOTT(ObraSerie, "Serie Y", "n-1", "1000", "40000", "1.3"),
	}
	ott[0].Fuente = FuenteOTT

	d.Reportes = []Reporte{
		{Fuente: FuenteTV, Periodo: Periodo, Usos: tv, Bytes: csvDe(tv)},
		{Fuente: FuenteCine, Periodo: Periodo, Usos: cine, Bytes: csvDe(cine)},
		{Fuente: FuenteOTT, Periodo: Periodo, Usos: ott, Bytes: csvDe(ott)},
	}
}

func (d *Dataset) bolsas() {
	bruto := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	d.Bolsas = []aplicacion.BolsaPersistida{
		{ID: "bolsa-caracol-" + Periodo + "-nacional", UsuarioID: "caracol", Periodo: Periodo, Circuito: reparto.Nacional, Bruto: bruto("1000000.00")},
		{ID: "bolsa-procinal-" + Periodo + "-nacional", UsuarioID: "procinal", Periodo: Periodo, Circuito: reparto.Nacional, Bruto: bruto("1000000.00")},
		{ID: "bolsa-netflix-" + Periodo + "-nacional", UsuarioID: "netflix", Periodo: Periodo, Circuito: reparto.Nacional, Bruto: bruto("500000.00")},
		{ID: "bolsa-dago-" + Periodo + "-internacional", UsuarioID: "dago-films", Periodo: Periodo, Circuito: reparto.Internacional, Bruto: bruto("200000.00")},
	}
}

func (d *Dataset) parametros() {
	n := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	desde := "2024-01-01"

	publicado := func(clave, valor, organo, reglamento string) Parametro {
		return Parametro{Clave: clave, Valor: n(valor), VigenteDesde: desde, Organo: organo, Reglamento: reglamento}
	}
	sintetico := func(clave, valor string) Parametro {
		return Parametro{Clave: clave, Valor: n(valor), VigenteDesde: desde, Organo: OrganoSintetico, Reglamento: ReglamentoSintetico}
	}

	d.Parametros = []Parametro{
		publicado("deduccion.administrativa", "0.20", "Asamblea General", "R-06 Ley 44/1993 Art. 21"),
		publicado("deduccion.social", "0.10", "Asamblea General", "R-06 Ley 44/1993 Art. 21"),
		publicado("reserva.errores_tecnicos", "0.05", "Asamblea General", "R-07 RD 14.5.1"),
		publicado("ponderacion.cinematografica", "5.0", "Consejo Directivo", "RD 9.1.1"),
		publicado("ponderacion.unitario", "2.8", "Consejo Directivo", "RD 9.1.1"),
		publicado("ponderacion.serie", "1.3", "Consejo Directivo", "RD 9.1.1"),
		publicado("ponderacion.sketches", "0.8", "Consejo Directivo", "RD 9.1.1"),
		publicado("matching.umbral", "0.60", "Consejo Directivo", "ADR 0007"),
		// Wa/Wb/Wc no estan publicados (RD 9.7, ADR 0004). Cifras redondas a
		// proposito: nadie las confunde con un valor aprobado. Suman 1.
		sintetico("ott.wa", "0.50"),
		sintetico("ott.wb", "0.30"),
		sintetico("ott.wc", "0.20"),
	}
}

func usoTV(obraID, titulo, idFuente, tipo, duracion string, emisiones int64, rating string) aplicacion.UsoPersistido {
	return usoIdentificado(obraID, titulo, idFuente, reparto.TV, tipo, duracion, emisiones, rating, "0", "0", "0", "0")
}

func usoCine(obraID, titulo, idFuente, taquilla string) aplicacion.UsoPersistido {
	return usoIdentificado(obraID, titulo, idFuente, reparto.Cine, "cinematografica", "0", 1, "0", taquilla, "0", "0", "0")
}

func usoOTT(obraID, titulo, idFuente, vistas, minutos, pb string) aplicacion.UsoPersistido {
	return usoIdentificado(obraID, titulo, idFuente, reparto.OTT, "serie", "0", 1, "0", "0", vistas, minutos, pb)
}

func usoIdentificado(obraID, titulo, idFuente string, modalidad reparto.Modalidad, tipo, duracion string, emisiones int64, rating, taquilla, vistas, minutos, pb string) aplicacion.UsoPersistido {
	n := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	return aplicacion.UsoPersistido{
		Titulo:        titulo,
		IDsFuente:     idFuente,
		ObraID:        obraID,
		Escalon:       "alias",
		Evidencia:     "semilla: identificacion sintetica por titulo",
		ONI:           false,
		Modalidad:     modalidad,
		TipoObra:      tipo,
		DuracionMin:   n(duracion),
		Emisiones:     emisiones,
		Rating:        n(rating),
		Taquilla:      n(taquilla),
		Vistas:        n(vistas),
		MinutosVistos: n(minutos),
		PB:            n(pb),
	}
}

func csvDe(usos []aplicacion.UsoPersistido) []byte {
	var b strings.Builder
	b.WriteString("titulo,ids_fuente,modalidad,tipo_obra,duracion_min,emisiones,rating,taquilla,vistas,minutos_vistos,pb\n")
	for _, u := range usos {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s,%d,%s,%s,%s,%s,%s\n",
			u.Titulo, u.IDsFuente, u.Modalidad, u.TipoObra,
			u.DuracionMin, u.Emisiones, u.Rating, u.Taquilla, u.Vistas, u.MinutosVistos, u.PB)
	}
	return []byte(b.String())
}
