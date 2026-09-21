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
	"github.com/rosvend/intela/internal/dominio/recaudo"
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

	FuenteTV = "caracol"
	// "cine" y no "procinal": la fuente del reporte es lo que estampa el
	// adaptador (ingesta.FuenteCine) y lo que indexa alias_obra; "procinal"
	// es el pagador de recaudo (usuariosDeRecaudo), otro eje. Sembrar el
	// reporte como "procinal" dejaba sus alias sin casar con ninguna fila
	// ingerida de verdad.
	FuenteCine = "cine"
	FuenteOTT  = "netflix"
	// La clave de ids_fuente (ADR 0018) con que viaja el id de obra de cada
	// fuente, y la segunda mitad de la clave de `alias_obra`. Salen del
	// contrato y no se escriben a mano: un alias sembrado con otra grafia no lo
	// encontraria nunca la cascada. La de cine es sintetica como el resto de
	// su reporte, porque el cliente no ha entregado el formato de las salas.
	TipoIDCaracol = aplicacion.ClaveIDFicha
	TipoIDCine    = aplicacion.ClaveIDPelicula
	TipoIDNetflix = aplicacion.ClaveShowID

	// Procedencia de los coeficientes OTT que el reglamento no publica.
	// ARRANQUE.md y el issue #22 piden marcarlos; el esquema no tiene
	// columna `origen`, asi que vive en `reglamento` y `organo`.
	ReglamentoSintetico = "RD-IX-seed-sintetico"
	OrganoSintetico     = "sintetico"
)

// Dataset es el juego completo, listo para persistir. No lleva hashes de
// contrasena: bcrypt no es determinista, y el hash se calcula al cargar.
type Dataset struct {
	Periodo   string
	Titulares []Titular
	// Usuarios son las CUENTAS que inician sesion; UsuariosDeRecaudo son los
	// PAGADORES. El reglamento llama "usuario" al segundo (`RT 2`), y de ahi la
	// colision: son dos tablas distintas y ninguna referencia a la otra.
	Usuarios          []Usuario
	UsuariosDeRecaudo []UsuarioDeRecaudo
	Obras             []Obra
	Declaraciones     []repertorio.Declaracion
	Reportes          []Reporte
	Bolsas            []aplicacion.BolsaPersistida
	Parametros        []Parametro
}

// UsuarioDeRecaudo es el pagador que siembra el seed.
//
// No usa recaudo.Usuario porque ese tipo tiene el id privado y solo se
// construye por su constructor: el dataset es dato plano y determinista, y la
// construccion -- con su validacion -- ocurre al cargar, igual que con las obras.
type UsuarioDeRecaudo struct {
	ID        string
	Nombre    string
	NIT       string
	Categoria recaudo.CategoriaUsuario
}

// Obra es la entrada del catalogo que siembra el seed.
//
// Lleva genero y anio porque 00002 los exige, y no usa aplicacion.Obra, que
// todavia no los tiene. Los campos son los mismos que [repertorio.Metadatos]
// porque su unico destino es construir una: el seed no escribe `obras` con
// SQL propio, pasa por el constructor del dominio (ver registrarObras).
//
// Coautores no es opcional. Una obra del catalogo sin coautor con IPI no la
// reconstruye [repertorio.NuevaObra], asi que quedaria escrita y no se podria
// leer: es el 500 en GET /obras que este campo existe para hacer imposible.
// NO llevan porcentaje y no lo van a llevar: el catalogo es identidad, el
// reparto sale de la Declaracion de Obra y de ningun otro sitio (`R-02`,
// `R-03`).
type Obra struct {
	ID        string
	Titulo    string
	IDA       string
	EIDR      string
	IMDB      string
	Tipo      repertorio.TipoObra
	Genero    string
	Anio      int
	Coautores []repertorio.Coautor
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

// Reporte es una entrega del dataset: los bytes crudos y las filas que
// contienen.
//
// TipoID nombra la columna de la que sale IDsFuente en el archivo de esa
// fuente, y es la segunda parte de la clave de `alias_obra`
// (docs/dominio/identificadores.md). Sin el, el alias que siembra el seed no
// se podria escribir, y el escalon "alias" de cada uso seria una etiqueta que
// no apunta a ninguna fila.
type Reporte struct {
	Fuente  string
	TipoID  string
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
	d.usuariosDeRecaudo()
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
	d.Obras = []Obra{
		{
			ID: ObraCine, Titulo: "Pelicula X", IDA: "IDA-PX", EIDR: "EIDR-PX", IMDB: "tt0001",
			Tipo: repertorio.TipoCinematografica, Genero: "Drama", Anio: 2023,
			Coautores: []repertorio.Coautor{
				d.coautor(TitularAna, repertorio.RolGuionista),
				d.coautor(TitularBeto, repertorio.RolAdaptador),
			},
		},
		{
			ID: ObraUnitario, Titulo: "El Tercer Acto", IDA: "IDA-ETA",
			Tipo: repertorio.TipoUnitario, Genero: "Drama", Anio: 2024,
			Coautores: []repertorio.Coautor{
				d.coautor(TitularAna, repertorio.RolGuionista),
				d.coautor(TitularBeto, repertorio.RolLibretista),
				d.coautor(TitularCarla, repertorio.RolArgumentista),
			},
		},
		{
			ID: ObraSerie, Titulo: "Serie Y", IDA: "IDA-SY",
			Tipo: repertorio.TipoSerie, Genero: "Drama", Anio: 2024,
			// DOS coautores en el catalogo y una sola parte declarada, abajo.
			// Es lo que hace explicable el caso del 60%: la obra la
			// escribieron dos, y el 40% de Beto no esta declarado. Por eso no
			// se reparte nada de esta obra (`R-04`, `RD 13.1.3`), y por eso el
			// arreglo no es repartir el 60% sino pedirle a Beto que declare.
			Coautores: []repertorio.Coautor{
				d.coautor(TitularAna, repertorio.RolGuionista),
				d.coautor(TitularBeto, repertorio.RolLibretista),
			},
		},
		{
			ID: ObraSketch, Titulo: "Minuto Comico",
			Tipo: repertorio.TipoSketches, Genero: "Comedia", Anio: 2024,
			Coautores: []repertorio.Coautor{
				d.coautor(TitularAna, repertorio.RolGuionista),
			},
		},
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

// coautor arma un coautor del catalogo a partir del padron de titulares.
//
// El IPI se busca en d.Titulares en vez de repetirse en la lista de obras: es
// la MISMA persona que la declaracion nombra por su titular_id, y escribir el
// identificador dos veces es la forma de que un dia digan cosas distintas. El
// catalogo se busca por IPI (`RD 3`), asi que un IPI que no case con el del
// padron es una obra que no encuentra a su autor.
//
// Entra en panico si el titular no esta, igual que decimal.RequireFromString
// mas abajo: el dataset es una constante del binario, y un id que no existe es
// un error de programacion que tiene que salir en la primera prueba, no una
// obra sin coautor que se descubre leyendo el catalogo.
func (d *Dataset) coautor(titularID string, rol repertorio.RolAutoral) repertorio.Coautor {
	for _, t := range d.Titulares {
		if t.ID == titularID {
			return repertorio.Coautor{Nombre: t.Nombre, IPI: t.IPI, Rol: rol}
		}
	}
	panic("semilla: no hay titular " + titularID + " en el padron del dataset")
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
	cine := []aplicacion.UsoPersistido{
		usoCine(ObraCine, "Pelicula X", "PX-1", "10000"),
	}
	ott := []aplicacion.UsoPersistido{
		usoOTT(ObraSerie, "Serie Y", "n-1", "1000", "40000", "1.3"),
	}

	d.Reportes = []Reporte{
		{Fuente: FuenteTV, TipoID: TipoIDCaracol, Periodo: Periodo, Usos: tv},
		{Fuente: FuenteCine, TipoID: TipoIDCine, Periodo: Periodo, Usos: cine},
		{Fuente: FuenteOTT, TipoID: TipoIDNetflix, Periodo: Periodo, Usos: ott},
	}

	// La fuente, ids_fuente y la evidencia se estampan aqui y no en los
	// constructores de arriba porque son propiedades de la ENTREGA, no de la
	// fila: la misma "PX-1" viaja en el reporte de Caracol y en el de cine,
	// y lo que la distingue -la clave con que viaja y lo que la resuelve- es de
	// que fuente viene. Los constructores dejan en IDsFuente el valor solo, y
	// aqui se reescribe en el formato del contrato.
	for i := range d.Reportes {
		r := &d.Reportes[i]
		for j := range r.Usos {
			u := &r.Usos[j]
			valor := u.IDsFuente
			ids, err := aplicacion.EscribirIDsFuente(aplicacion.IDFuente{Clave: r.TipoID, Valor: valor})
			if err != nil {
				// Como en coautor: el dataset es una constante del binario, y un
				// id que no cumple el contrato es un error de programacion.
				panic("semilla: " + err.Error())
			}
			u.Fuente = r.Fuente
			u.IDsFuente = ids
			u.Evidencia = evidenciaAlias(r.Fuente, r.TipoID, valor)
		}
		r.Bytes = csvDe(r.Usos)
	}
}

// evidenciaAlias redacta el "como se reconocio" de un uso identificado por
// alias, nombrando la fila de `alias_obra` que lo resolvio.
//
// Es la pregunta 3 del ADR 0006 y tiene que poder seguirse: con la fuente, el
// tipo de id y el valor se compone la clave primaria de `alias_obra`, asi que
// un auditor que lea esta cadena llega a la fila exacta y de ahi a la obra.
// Antes decia "identificacion sintetica por titulo" mientras el escalon decia
// "alias": dos versiones distintas del mismo hecho, y ninguna comprobable.
func evidenciaAlias(fuente, tipoID, valor string) string {
	return "semilla: alias " + fuente + "/" + tipoID + "=" + valor
}

// usuariosDeRecaudo son los cuatro pagadores que las bolsas citan.
//
// Existen como tabla desde la migracion 00009 (#27): antes `bolsas.usuario_id`
// era texto libre sin nada al otro lado. La categoria no es decorativa -- decide
// que formula de reparto aplica aguas abajo (formulas.md 9.1, 9.2, 9.7)-- y por
// eso cada uno lleva la que le corresponde y no una generica.
//
// `dago-films` es el del circuito internacional y va como `sin_clasificar` a
// proposito: el recaudo internacional no lo paga un usuario colombiano de una
// categoria del `RT`, llega discriminado por una sociedad hermana (`RD 7.4`).
// Inventarle una categoria seria afirmar algo que el reglamento no dice.
//
// No llevan marca sintetica: no son parametros normativos, son datos de negocio
// de ejemplo, igual que las declaraciones. Los NIT si son inventados y por eso
// van vacios en vez de con un numero de aspecto real que alguien pudiera creer.
func (d *Dataset) usuariosDeRecaudo() {
	d.UsuariosDeRecaudo = []UsuarioDeRecaudo{
		{ID: "caracol", Nombre: "Caracol Television (sintetico)", Categoria: recaudo.TVAbierta},
		{ID: "procinal", Nombre: "Procinal Salas de Cine (sintetico)", Categoria: recaudo.Cine},
		{ID: "netflix", Nombre: "Netflix Colombia (sintetico)", Categoria: recaudo.MediosDigitales},
		{ID: "dago-films", Nombre: "Dago Films (sintetico)", Categoria: recaudo.SinClasificar},
	}
}

func (d *Dataset) bolsas() {
	bruto := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	// Convenio, tarifa y factura son la PROCEDENCIA de cada bolsa: la pregunta
	// 1 del ADR 0006, de donde salio este dinero. No son insumos de calculo --
	// bajo P-08 Intela recibe el importe ya cobrado y no liquida tarifas --,
	// pero sin ellas la cifra no se puede seguir hasta su origen, que es lo que
	// el reglamento exige de toda cifra del sistema.
	proc := func(usuario string) (string, string, string) {
		return "convenio-" + usuario + "-sintetico", "RT-VI-sintetica", "factura-" + usuario + "-sintetica"
	}
	bolsa := func(id, usuario string, c recaudo.Circuito, monto string) aplicacion.BolsaPersistida {
		conv, tar, fac := proc(usuario)
		return aplicacion.BolsaPersistida{
			ID: id, UsuarioID: usuario, Periodo: Periodo, Circuito: c, Bruto: bruto(monto),
			Convenio: conv, Tarifa: tar, Factura: fac,
		}
	}
	d.Bolsas = []aplicacion.BolsaPersistida{
		bolsa("bolsa-caracol-"+Periodo+"-nacional", "caracol", recaudo.Nacional, "1000000.00"),
		bolsa("bolsa-procinal-"+Periodo+"-nacional", "procinal", recaudo.Nacional, "1000000.00"),
		bolsa("bolsa-netflix-"+Periodo+"-nacional", "netflix", recaudo.Nacional, "500000.00"),
		bolsa("bolsa-dago-"+Periodo+"-internacional", "dago-films", recaudo.Internacional, "200000.00"),
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
		// Las cuatro ponderaciones son lo UNICO que aqui esta publicado de
		// verdad: la tabla de RD 9.1.1, verificada contra formulas.md.
		publicado("ponderacion.cinematografica", "5.0", "Consejo Directivo", "RD 9.1.1"),
		publicado("ponderacion.unitario", "2.8", "Consejo Directivo", "RD 9.1.1"),
		publicado("ponderacion.serie", "1.3", "Consejo Directivo", "RD 9.1.1"),
		publicado("ponderacion.sketches", "0.8", "Consejo Directivo", "RD 9.1.1"),
		// RD 9.1.1(c): duracion artistica y hora televisiva. Son cifras del
		// reglamento, no techos "hasta X": el texto dice "el 80%" y "48
		// minutos". Viven aqui y no en el codigo (ADR 0004).
		publicado("duracion.artistica_pct", "0.80", "Consejo Directivo", "RD 9.1.1"),
		publicado("duracion.minutos_hora_tv", "48", "Consejo Directivo", "RD 9.1.1"),

		// Tasas a COP. Viven aqui y no en una lista Go (ADR 0004 / B4).
		// USD es P-09 provisional (TRM de facturacion). EUR es sintetica
		// hasta que haya tasa propia: sin fila, normalizacion manda EUR a
		// revision en vez de multiplicar por la del dolar.
		sintetico("cambio.USD", "4000"),
		sintetico("cambio.EUR", "4300"),

		// El reglamento fija un TECHO, no la tasa. `R-06` dice "hasta 20% para
		// gastos administrativos" y "hasta 10% para programas de inversion
		// social" (Ley 44/1993 Art. 21); `R-07`, "se puede retener hasta 5% ...
		// El porcentaje lo aprueba la Asamblea General" (RD 14.5.1).
		//
		// Escribir el techo con "Asamblea General" en la columna del organo
		// convierte el limite legal en una resolucion que nadie tomo. Y no es
		// una etiqueta cosmetica: un reparto calculado con un 20% de deduccion
		// se defenderia en auditoria citando un acta que no existe. Van
		// sinteticos hasta que llegue el acta de la Asamblea con la tasa real,
		// que es la pregunta P-10 de docs/dominio/preguntas-cliente.md.
		sintetico("deduccion.administrativa", "0.20"),
		sintetico("deduccion.social", "0.10"),
		sintetico("reserva.errores_tecnicos", "0.05"),

		// El umbral de similitud es una decision de INGENIERIA (ADR 0007). Un
		// ADR no es un reglamento y el Consejo Directivo nunca aprobo 0.6: es
		// el corte con el que la muestra no produce ni un candidato
		// (identificadores.md), calibrable, y por tanto no es normativo.
		sintetico("matching.umbral", "0.60"),

		// Wa/Wb/Wc no estan publicados (RD 9.7, ADR 0004). Cifras redondas a
		// proposito: nadie las confunde con un valor aprobado. Suman 1.
		sintetico("ott.wa", "0.50"),
		sintetico("ott.wb", "0.30"),
		sintetico("ott.wc", "0.20"),

		// Porcentajes de grupo RD 9.5 y asignacion RD 9.7: unidad 0-100, la
		// misma que exige el motor al armar el Snapshot (no fracciones 0-1).
		sintetico("grupo.privados_pct", "50"),
		sintetico("grupo.regionales_pct", "20"),
		sintetico("grupo.premium_pct", "10"),
		sintetico("grupo.lideres_pct", "10"),
		sintetico("grupo.estandar_pct", "10"),
		sintetico("asignacion.terceros_pct", "5"),
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
		Titulo:    titulo,
		IDsFuente: idFuente,
		ObraID:    obraID,
		Escalon:   "alias",
		// Evidencia la estampa reportes(), que es donde se sabe de que fuente
		// viene la fila y por tanto cual es su alias.
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
