package repertorio

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrObraInvalida es el unico centinela que emite la construccion de una
// [Obra].
//
// Uno solo y no ocho: quien llama necesita distinguir "los datos que me
// mandaron no forman una obra" -que es un 400- de "la base no responde" -que
// es un 500-, y para eso basta un centinela. El detalle de QUE falta va en el
// texto envuelto, que es lo que lee una persona, no un switch.
var ErrObraInvalida = errors.New("obra invalida")

// TipoObra es la clasificacion del reglamento, la que usa la metodologia de
// distribucion (`RD 9.1.1`).
//
// Es un conjunto cerrado porque el reglamento lo cierra, y porque la columna
// `obras.tipo` lleva el mismo CHECK. Se valida aqui ademas de en el esquema a
// proposito: un INSERT que revienta por un CHECK dice "violacion de
// restriccion", no "el tipo 'documental' no existe en el reglamento".
type TipoObra string

const (
	TipoCinematografica TipoObra = "cinematografica"
	TipoUnitario        TipoObra = "unitario"
	TipoSerie           TipoObra = "serie"
	TipoTelenovela      TipoObra = "telenovela"
	TipoSketches        TipoObra = "sketches"
)

// TiposObra devuelve los tipos validos, en el orden en que los declara el
// CHECK de la migracion.
func TiposObra() []TipoObra {
	return []TipoObra{
		TipoCinematografica, TipoUnitario, TipoSerie, TipoTelenovela, TipoSketches,
	}
}

// RolAutoral es el aporte por el que una persona figura como coautora de la
// obra en el catalogo.
//
// El conjunto esta cerrado, y lo que compra al cerrarlo no es orden: es que
// `RD 7.3.3` deja FUERA por su nombre a productores ejecutivos, revisores,
// ejecutivos de cadena, actores y directores de casting. Con un texto libre,
// un "director" entra en el catalogo como coautor y nadie lo nota hasta que
// alguien lo lea como si generara derecho (`R-01`, `R-02`).
//
// Los cuatro valores son formas de ESCRIBIR la obra, que es el criterio que
// fija `RD 7.3.2`: "los que han de beneficiarse de esos derechos son los
// escritores, quienes efectivamente crean los guiones". `guionista` y
// `libretista` los nombra el reglamento literalmente (`RD 7.1`, `RD 7.3.4`).
//
// Figurar aqui NO da derecho a cobrar. El derecho sale de la Declaracion de
// Obra y de ningun otro sitio (`R-02`, `R-03`): ver [Declaracion].
type RolAutoral string

const (
	RolGuionista    RolAutoral = "guionista"
	RolLibretista   RolAutoral = "libretista"
	RolAdaptador    RolAutoral = "adaptador"
	RolArgumentista RolAutoral = "argumentista"
)

// RolesAutorales devuelve los roles validos, en el orden en que los declara el
// CHECK de la migracion.
func RolesAutorales() []RolAutoral {
	return []RolAutoral{RolGuionista, RolLibretista, RolAdaptador, RolArgumentista}
}

// Coautor es quien escribio la obra, tal como lo registra el catalogo.
//
// NO tiene porcentaje, y no lo va a tener. El catalogo es identidad y
// metadata; el porcentaje de cada autor sale solo de la Declaracion de Obra
// (`R-03`, `RD 13.1.4`), que vive en [Declaracion] y en la tabla
// `declaraciones`. Un campo de porcentaje aqui abriria un segundo camino
// hasta un pago, que es exactamente el que `R-02` cierra.
//
// El IPI es obligatorio porque es el identificador con el que se busca en el
// catalogo (`RD 3`) y lo unico que cruza entre sociedades: un coautor sin IPI
// es un nombre suelto que no resuelve a nadie.
type Coautor struct {
	Nombre string
	IPI    string
	Rol    RolAutoral
}

// Metadatos es todo lo de una obra que SI se puede corregir.
//
// Existe como tipo aparte para que la firma de [Obra.ConMetadatos] diga por si
// sola que el identificador no esta entre lo que se cambia. Sin el, la
// operacion recibiria una Obra entera y el "menos el id" viviria en un
// comentario.
type Metadatos struct {
	// Los cuatro obligatorios del catalogo maestro. Genero y Anio salen de la
	// propia Declaracion de Obra (`RD 13.1.2`); el IPI entra por Coautores,
	// porque el IPI identifica PERSONAS, no obras
	// (docs/dominio/identificadores.md).
	Titulo string
	Genero string
	Anio   int
	Tipo   TipoObra

	// Identificadores globales. Opcionales: hoy no hay ninguno poblado en
	// comun entre las fuentes de la muestra, y el escalon 2 de la cascada
	// solo consulta los que existan.
	IDA  string
	EIDR string
	IMDB string

	Coautores []Coautor
}

// Obra es una entrada del catalogo maestro: el cubo contra el que resuelve
// todo matching (docs/dominio/identificadores.md).
//
// # El identificador es inmutable, y lo impone el tipo
//
// `id` esta sin exportar y no hay setter. La unica forma de tener una Obra con
// otro identificador es construir otra, y [Obra.ConMetadatos] devuelve una
// copia que CONSERVA el id de origen. No es ceremonia: el id es lo que
// referencian `declaraciones`, `alias_obra` y `usos`; cambiarlo reasigna en
// silencio los autores y el dinero de una obra a otra.
//
// El id NO lo genera este tipo. Es el numero de obra de REDES-SYS, que se
// asigna fuera del sistema, y por eso [NuevaObra] lo recibe y el catalogo
// rechaza el duplicado en vez de acunar uno propio.
//
// # Que no vive aqui
//
// Ni dinero ni porcentajes. El uso vive en `usos` y los splits en
// `declaraciones`; el catalogo es identidad y metadata (`R-02`, `R-03`).
type Obra struct {
	id        string
	metadatos Metadatos
}

// NuevaObra construye una entrada del catalogo o falla.
//
// No hay forma de obtener una Obra invalida: el constructor es el unico
// camino, y el cero de [Obra] no pasa por ningun sitio que lo acepte -su id
// esta vacio, y toda escritura lo comprueba-.
//
// Las cadenas se recortan antes de validar y se guardan recortadas: un titulo
// de un solo espacio no es un titulo, y el CHECK con btrim del esquema opina
// lo mismo. Normalizar al construir evita que la base y el dominio discrepen
// sobre lo que esta vacio.
func NuevaObra(id string, m Metadatos) (Obra, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Obra{}, fmt.Errorf("%w: falta el identificador", ErrObraInvalida)
	}
	m, err := normalizar(m)
	if err != nil {
		return Obra{}, err
	}
	return Obra{id: id, metadatos: m}, nil
}

// ID es el identificador inmutable de la obra.
func (o Obra) ID() string { return o.id }

// Metadatos devuelve una copia. La rebanada de coautores se clona: sin eso,
// quien reciba los metadatos puede reescribir el rol de un coautor DENTRO de
// la obra, y la invariante que valido el constructor deja de valer sin que
// nadie haya llamado a nada.
func (o Obra) Metadatos() Metadatos {
	m := o.metadatos
	m.Coautores = slices.Clone(o.metadatos.Coautores)
	return m
}

// Coautores es el atajo de lectura mas usado. Tambien devuelve una copia.
func (o Obra) Coautores() []Coautor { return slices.Clone(o.metadatos.Coautores) }

// ConMetadatos devuelve la misma obra con otros metadatos, revalidada.
//
// Devuelve una Obra nueva en vez de mutar el receptor para que el id no pueda
// viajar por accidente: quien tenga la obra vieja sigue teniendo la vieja, y
// la nueva nace del mismo id o no nace.
func (o Obra) ConMetadatos(m Metadatos) (Obra, error) {
	m, err := normalizar(m)
	if err != nil {
		return Obra{}, err
	}
	return Obra{id: o.id, metadatos: m}, nil
}

// normalizar recorta, clona y valida. Es la unica puerta: [NuevaObra] y
// [Obra.ConMetadatos] pasan las dos por aqui, asi que una obra corregida
// cumple lo mismo que una recien creada.
func normalizar(m Metadatos) (Metadatos, error) {
	m.Titulo = strings.TrimSpace(m.Titulo)
	m.Genero = strings.TrimSpace(m.Genero)
	m.IDA = strings.TrimSpace(m.IDA)
	m.EIDR = strings.TrimSpace(m.EIDR)
	m.IMDB = strings.TrimSpace(m.IMDB)

	if m.Titulo == "" {
		return Metadatos{}, fmt.Errorf("%w: falta el titulo", ErrObraInvalida)
	}
	if m.Genero == "" {
		return Metadatos{}, fmt.Errorf("%w: falta el genero", ErrObraInvalida)
	}
	// Solo cota inferior. La superior seria "no despues de hoy", y el dominio
	// no sabe que dia es hoy: el instante entra por el puerto Reloj (ADR 0002),
	// y meterlo aqui para validar un anio costaria el determinismo de todo el
	// paquete. Que una obra declare 3025 lo ve una persona; que el nucleo lea
	// el reloj no lo ve nadie.
	if m.Anio <= 0 {
		return Metadatos{}, fmt.Errorf("%w: el anio de produccion tiene que ser positivo", ErrObraInvalida)
	}
	if !slices.Contains(TiposObra(), m.Tipo) {
		return Metadatos{}, fmt.Errorf("%w: tipo %q, se esperaba uno de %v",
			ErrObraInvalida, m.Tipo, TiposObra())
	}

	coautores, err := normalizarCoautores(m.Coautores)
	if err != nil {
		return Metadatos{}, err
	}
	m.Coautores = coautores
	return m, nil
}

// normalizarCoautores exige al menos uno, con IPI y rol validos, y sin
// repetir el par (IPI, rol).
//
// El duplicado se rechaza aqui y no solo en la clave primaria de
// `obra_coautores` porque un lote con la misma persona dos veces en el mismo
// rol es un error de quien lo manda, y decirselo como "violacion de clave
// primaria" no le sirve de nada.
func normalizarCoautores(cs []Coautor) ([]Coautor, error) {
	if len(cs) == 0 {
		return nil, fmt.Errorf("%w: una obra del catalogo necesita al menos un coautor con IPI", ErrObraInvalida)
	}

	limpios := make([]Coautor, 0, len(cs))
	vistos := make(map[Coautor]struct{}, len(cs))
	for i, c := range cs {
		c.Nombre = strings.TrimSpace(c.Nombre)
		c.IPI = strings.TrimSpace(c.IPI)
		if c.Nombre == "" {
			return nil, fmt.Errorf("%w: al coautor %d le falta el nombre", ErrObraInvalida, i+1)
		}
		if c.IPI == "" {
			return nil, fmt.Errorf("%w: al coautor %q le falta el IPI", ErrObraInvalida, c.Nombre)
		}
		if !slices.Contains(RolesAutorales(), c.Rol) {
			return nil, fmt.Errorf("%w: rol autoral %q del coautor %q, se esperaba uno de %v",
				ErrObraInvalida, c.Rol, c.Nombre, RolesAutorales())
		}
		clave := Coautor{IPI: c.IPI, Rol: c.Rol}
		if _, repetido := vistos[clave]; repetido {
			return nil, fmt.Errorf("%w: el IPI %q figura dos veces como %q",
				ErrObraInvalida, c.IPI, c.Rol)
		}
		vistos[clave] = struct{}{}
		limpios = append(limpios, c)
	}
	return limpios, nil
}
