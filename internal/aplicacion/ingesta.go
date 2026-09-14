package aplicacion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Ingesta recibe entregas de reportes de uso y las deja en forma canonica.
//
// Es la columna vertebral de KR-1: toda cifra que el sistema produzca tiene
// que poder rastrearse hasta el byte exacto del que salio. Eso son dos cosas
// distintas y las dos viven aqui:
//
//   - La evidencia CRUDA se congela con su huella SHA-256 (ADR 0006). Una
//     corrida no referencia "el archivo de Caracol", referencia unos bytes.
//   - La forma CANONICA se persiste sin importes. Un reporte de uso PONDERA la
//     bolsa, no la aporta; el esquema lo refuerza no teniendo columna de dinero.
//
// # Que NO decide
//
// Como se lee un .xlsx o un CSV: eso es un adaptador de formato (#25). Aqui
// llegan bytes y filas ya mapeadas al esquema canonico.
//
// Y a que obra corresponde cada fila: la cascada de identificacion es otro
// modulo (ADR 0007). Todo lo que entra por aqui sale con escalon "pendiente".
type Ingesta struct {
	Reportes RepositorioIngesta
	Almacen  AlmacenObjetos
}

// huella devuelve el SHA-256 hexadecimal de unos bytes.
//
// En minusculas y sin separadores porque asi lo exige el CHECK del esquema
// (`sha256 ~ '^[0-9a-f]{64}$'`), que es la forma en la que se cita una
// evidencia en toda la trazabilidad.
func huella(datos []byte) string {
	suma := sha256.Sum256(datos)
	return hex.EncodeToString(suma[:])
}

// claveObjeto deriva la clave del almacen a partir de la huella.
//
// La clave ES el contenido, y eso tiene tres consecuencias buscadas:
//
//  1. Los mismos bytes ocupan un solo objeto, aunque los declaren dos fuentes
//     distintas -que el UNIQUE (sha256, fuente) permite a proposito-.
//  2. Reescribir una clave existente es reescribir contenido identico, asi que
//     la inmutabilidad del ADR 0006 no se puede violar por esta via ni
//     queriendo.
//  3. No entra en la clave nada que venga del formulario de subida. El nombre
//     del fichero llega en multipart.FileHeader.Filename, que la propia
//     documentacion de Go advierte que no es de fiar, y el almacen rechaza
//     todo lo que no sea [A-Za-z0-9._-]: componer la clave con la huella la
//     deja dentro del alfabeto por construccion.
func claveObjeto(sha string) string {
	return "reportes/" + sha
}

// idReporte deriva el identificador de una entrega.
//
// Sale del PAR (fuente, huella) y no de la huella sola: el esquema admite que
// dos fuentes entreguen los mismos bytes, y un id derivado solo del contenido
// haria que la segunda chocara contra la clave primaria en vez de aceptarse.
//
// Que sea derivado y no aleatorio es deliberado, y no contradice el ADR 0006:
// alli el aviso es sobre los ASIENTOS, donde un mismo hecho ocurrido dos veces
// tiene que dejar dos filas. Aqui es al reves -una misma entrega dos veces
// tiene que dejar UNA-, asi que un id derivado del mismo par que el
// UNIQUE (sha256, fuente) hace unico impide que la clave primaria y la
// restriccion de unicidad puedan discrepar.
func idReporte(fuente, sha string) string {
	suma := sha256.Sum256([]byte(fuente + "\x00" + sha))
	return "rep-" + hex.EncodeToString(suma[:])
}

// GuardarReporte congela una entrega: los bytes en la boveda y el acuse en la
// base.
//
// # El orden importa, y este es el motivo
//
// Primero la boveda, despues la fila. El estado que no puede existir es un
// acuse en `reportes` que apunte a una evidencia que no se llego a escribir:
// eso es una cifra que dice de donde salio y no se puede comprobar, que es
// justo lo que el ADR 0006 existe para impedir. Al reves, un objeto sin acuse
// es evidencia inerte que no referencia nadie, y ademas se recupera sola: el
// reintento vuelve a poner los mismos bytes bajo la misma clave -la clave es
// el contenido- y completa la fila que falto.
//
// # La boveda queda FUERA de cualquier transaccion, y no puede ser de otra
// forma
//
// De un fichero escrito no se hace rollback. Meter el Poner dentro de una
// transaccion SQL daria la ilusion de atomicidad y no la propiedad: si el
// COMMIT falla despues, el objeto sigue ahi igualmente. Se asume por escrito
// el unico resto posible -objetos huerfanos- en vez de fingir que no existe.
//
// # El duplicado lo decide la base
//
// La deteccion por huella es el UNIQUE (sha256, fuente), y esa comprobacion
// llega DESPUES de tocar la boveda. No es un problema: el almacen no
// sobrescribe, asi que una resubida de los mismos bytes deja el objeto
// literalmente sin cambios y despues se rechaza con ErrReporteDuplicado.
func (i Ingesta) GuardarReporte(ctx context.Context, fuente, periodo string, datos []byte) (Reporte, error) {
	// UNA sola normalizacion de la fuente, y ANTES de la validacion, por lo
	// mismo que la de obra_id en GuardarUsos: la fuente se validaba recortada
	// y se usaba CRUDA en los tres sitios que vienen despues -la fila de
	// `reportes`, la derivacion del id de la entrega, y la fuente que
	// GuardarUsos estampa en cada uso-.
	//
	// El dano no es cosmetico, y el mas caro es el del id. idReporte deriva de
	// (fuente, huella), asi que "caracol" y " caracol " dan DOS ids distintos
	// para los mismos bytes; y el UNIQUE (sha256, fuente) tampoco los junta,
	// porque la fuente difiere. O sea: dos filas de `reportes` apuntando al
	// MISMO objeto de la boveda -la clave del objeto es solo la huella-,
	// ErrReporteDuplicado que no salta, y los dos juegos de filas ponderando la
	// bolsa. Cada obra de ese archivo puntua DOS VECES. Es el invariante 1 del
	// sistema roto por un espacio de un formulario.
	//
	// Y el silencioso: RepositorioIdentificacion.Alias indexa por fuente, asi
	// que la variante con espacios no casa con ningun alias NUNCA y todas sus
	// filas caen a ONI. El sintoma no es un error, es un catalogo que parece
	// incompleto.
	//
	// TrimSpace y no un recorte propio, por lo mismo que alli: su definicion de
	// blanco es unicode.IsSpace, que incluye el NBSP (U+00A0) de los exports de
	// Excel.
	fuente = strings.TrimSpace(fuente)

	switch {
	case fuente == "":
		return Reporte{}, fmt.Errorf("%w: falta la fuente", ErrReporteInvalido)
	// periodoValido vive en trabajos.go, una sola vez para el paquete. Se
	// comprueba aqui y no solo en la base porque GuardarReporte escribe la
	// boveda ANTES que la fila, y de la boveda no se puede borrar nada.
	case !periodoValido.MatchString(periodo):
		return Reporte{}, fmt.Errorf(
			"%w: periodo %q, se esperaba AAAA o AAAA-MM", ErrReporteInvalido, periodo)
	case len(datos) == 0:
		return Reporte{}, fmt.Errorf("%w: la entrega no trae bytes", ErrReporteInvalido)
	}

	rep := Reporte{
		Fuente:  fuente,
		Periodo: periodo,
		SHA256:  huella(datos),
		NBytes:  len(datos),
	}
	rep.ID = idReporte(fuente, rep.SHA256)
	rep.ClaveObjeto = claveObjeto(rep.SHA256)

	// ErrObjetoYaExiste no es un fallo por si mismo: la clave es la huella, asi
	// que lo que ya hay bajo ella DEBERIA ser estos mismos bytes. Puede venir
	// de una resubida -que la fila de abajo rechazara-, de otra fuente que
	// entrego lo mismo, o de un intento anterior que murio entre el Poner y el
	// INSERT.
	//
	// Pero ese "deberia" lo garantiza quien ESCRIBIO, no quien lee. Un objeto
	// desgarrado por un fallo anterior, una copia restaurada a medias o un
	// almacen que no escriba atomicamente dejan bajo la clave contenido que no
	// hashea a rep.SHA256, y sin comprobarlo el acuse de abajo lo certificaria:
	// una fila de `reportes` que dice de que bytes salio una cifra, apuntando a
	// unos bytes que no son. Asi que "ya estaba" solo autoriza a seguir si lo
	// que hay son ESTOS bytes.
	//
	// Cuesta una lectura, y solo en el camino de colision, que es el raro. Se
	// hace aqui y no en el adaptador a proposito: el dia que entre MinIO o S3
	// la comprobacion sigue siendo cierta sin que nadie tenga que acordarse.
	if err := i.Almacen.Poner(ctx, rep.ClaveObjeto, datos); err != nil {
		if !errors.Is(err, ErrObjetoYaExiste) {
			return Reporte{}, fmt.Errorf("guardar los bytes crudos de %q: %w", fuente, err)
		}
		ya, errLeer := i.Almacen.Obtener(ctx, rep.ClaveObjeto)
		if errLeer != nil {
			return Reporte{}, fmt.Errorf(
				"comprobar la evidencia ya presente en %q: %w", rep.ClaveObjeto, errLeer)
		}
		// El mensaje lleva las DOS huellas, y los dos tamanos. La version
		// anterior imprimia la esperada DOS veces sin darse cuenta -claveObjeto
		// la deriva de ella, asi que la clave ya la contiene-, y se leia como
		// "X no corresponde a X": confirmaba que algo iba mal y no daba un solo
		// dato para averiguar el que. Lo que hace falta saber es que hay AHI.
		//
		// Con la huella real y el tamano, los dos casos que hay que separar se
		// distinguen de un vistazo: un objeto CORTADO -menos bytes, otra huella-
		// viene de una escritura que murio a medias o de una copia restaurada
		// mal, y la clave se puede liberar; una huella distinta con el tamano
		// intacto es contenido AJENO bajo esa clave, que es un problema de otro
		// orden. La huella real es ademas con lo que se busca el objeto en el
		// almacen para ver de donde salio.
		if huellaReal := huella(ya); huellaReal != rep.SHA256 {
			return Reporte{}, fmt.Errorf(
				"%w: bajo %q hay %d bytes de huella %s; la entrega son %d bytes de huella %s",
				ErrEvidenciaCorrupta, rep.ClaveObjeto,
				len(ya), huellaReal, rep.NBytes, rep.SHA256)
		}
	}

	if err := i.Reportes.GuardarReporte(
		ctx, rep.ID, rep.Fuente, rep.Periodo, rep.SHA256, rep.ClaveObjeto, rep.NBytes,
	); err != nil {
		// Envuelto como los demas caminos de este fichero. errors.Is sigue
		// casando con ErrReporteDuplicado -es lo que comprueban las pruebas-,
		// pero el mensaje ya dice DE QUE subida se trata: con varias entregas
		// en vuelo, un centinela pelado no distingue cual choco.
		return Reporte{}, fmt.Errorf(
			"registrar la entrega de %q para %q: %w", rep.Fuente, rep.Periodo, err)
	}
	return rep, nil
}

// GuardarUsos persiste las filas de un reporte y devuelve las RECHAZADAS, cada
// una con su motivo.
//
// Un lote con filas malas no es un error: es el caso normal. Los archivos
// reales del cliente traen columnas vacias al 100%, placeholders y tipos
// mixtos, y detener la entrega entera por una fila haria inservible la ingesta.
// Las buenas pasan a la forma canonica, las malas al log de rechazos, y
// NINGUNA se descarta.
//
// El lote viaja al repositorio en UNA sola llamada, valido y rechazado
// mezclados, precisamente para que las dos escrituras sean el mismo hecho: un
// lote guardado a medias deja una entrega cuyo recuento no cuadra con el
// archivo, y nadie sabria cual de las dos mitades falta.
//
// # Por que el Reporte entero y no su id
//
// La fila hereda de la entrega las DOS cosas que la atan a ella: el reporte y
// la fuente. `usos.fuente` es TEXT NOT NULL sin DEFAULT, asi que la cadena
// vacia lo satisface sin ruido, y RepositorioIdentificacion.Alias indexa por
// fuente: una fila con fuente vacia no casaria con ningun alias NUNCA, y el
// sintoma no seria un error sino un catalogo que parece incompleto. Quien llama
// acaba de recibir el Reporte de GuardarReporte, asi que pedirlo entero no le
// cuesta nada.
//
// # Los valores por defecto los pone este metodo
//
// Escalon, ONI y Emisiones no son columna de ningun fichero del cliente, asi
// que un adaptador de formato (#25) que mapee lo que hay en el archivo los deja
// en el valor cero de Go. Sus DEFAULT del esquema no llegan a aplicarse -el
// adaptador de persistencia manda el valor siempre-, asi que los que valen son
// estos.
func (i Ingesta) GuardarUsos(ctx context.Context, rep Reporte, usos []UsoPersistido) ([]UsoPersistido, error) {
	if len(usos) == 0 {
		return nil, nil
	}
	// El acuse se validaba recortado y se estampaba CRUDO en cada fila del
	// lote, que es la misma trampa que la de obra_id un piso mas abajo: dos
	// criterios para el mismo campo. Con un Reporte construido a mano -este
	// metodo es publico- son dos danos distintos:
	//
	//   - `usos.reporte_id` es REFERENCES reportes(id), asi que " rep-1 " no
	//     existe: 23503 dentro de la transaccion del lote, que se lleva TODAS
	//     las filas, y ninguna con un motivo que explicarle al cliente.
	//   - `usos.fuente` es TEXT NOT NULL y aguanta cualquier cosa, asi que el
	//     dano es silencioso: Alias() indexa por fuente y " caracol " no casa
	//     con ningun alias nunca.
	//
	// Recortando aqui, lo que se valida y lo que se estampa son el MISMO valor.
	rep.ID = strings.TrimSpace(rep.ID)
	rep.Fuente = strings.TrimSpace(rep.Fuente)

	switch {
	case rep.ID == "":
		return nil, fmt.Errorf("%w: falta el reporte del que salen las filas", ErrReporteInvalido)
	case rep.Fuente == "":
		// Estampar la fuente no basta si la que se estampa viene vacia.
		// `usos.fuente` es TEXT NOT NULL SIN DEFAULT, asi que la cadena vacia
		// entra sin ruido, y a partir de ahi Alias() -que indexa por fuente- no
		// casa NUNCA: el sintoma no seria un error sino un catalogo que parece
		// incompleto. Un Reporte que salga de GuardarReporte siempre la trae
		// -alli se exige-, pero este metodo es publico y el precio de
		// comprobarlo es una cadena.
		return nil, fmt.Errorf(
			"%w: el reporte %q no dice de que fuente viene", ErrReporteInvalido, rep.ID)
	}

	lote := make([]UsoPersistido, len(usos))
	var rechazados []UsoPersistido

	for n, u := range usos {
		// UNA sola normalizacion de obra_id, y va aqui arriba porque el problema
		// no es el espacio: es que el campo se lee TRES veces en DOS capas
		// -la guarda de ONI de abajo, la regla de H5 en validarUso, y el
		// NULLIF($6, '') del INSERT- y cada lectura decide "vacio" por su cuenta.
		// Mientras el criterio se escriba tres veces, puede discrepar tres veces.
		//
		// Discrepaba ya. Un obra_id de solo blancos -un espacio, un tabulador, un
		// NBSP de un Excel- no es "" para ninguna de las dos comprobaciones de Go,
		// asi que la fila se rechazaba con el motivo de H5, "obra_id en la
		// ingesta", diciendo que traia una obra que NO traia. Es el mismo motivo
		// FALSO que H5 vino a arreglar, en el borde que se quedo sin cubrir: el
		// log de rechazos existe para pedirle al cliente exactamente lo que falla,
		// y ahi le pedia que quitara una identificacion inexistente.
		//
		// Y arreglarlo solo en Go lo empeora, que es la razon de que la
		// normalizacion sea UNA y este ANTES de todo. Con TrimSpace en las dos
		// comparaciones de arriba pero no en el SQL, la fila pasa como vacia,
		// llega al INSERT con el espacio intacto, NULLIF no la anula -no es
		// literalmente ''- y el CHECK uso_resuelto_tiene_obra la rechaza: 23514
		// dentro de la transaccion del lote, que se lleva por delante TODAS las
		// filas buenas que la acompanan. Un rechazo con el motivo equivocado se
		// convertiria asi en una entrega entera perdida.
		//
		// Normalizando aqui el valor viaja ya limpio a las tres lecturas, incluido
		// el que se manda al INSERT, y "vacio" pasa a significar lo mismo en Go y
		// en SQL por construccion, no por acuerdo.
		//
		// TrimSpace y no un recorte propio: su definicion de blanco es
		// unicode.IsSpace, que incluye el NBSP (U+00A0) con el que los exports de
		// Excel rellenan las celdas "vacias".
		//
		// No maquilla ningun rechazo: `usos_rechazados` no guarda obra_id, y una
		// fila que SI trae obra -" obra-1 "- sigue cayendo en la regla de H5 con
		// su motivo verdadero.
		u.ObraID = strings.TrimSpace(u.ObraID)

		// Los otros cuatro campos donde el blanco esquiva una comprobacion, por
		// el razonamiento de arriba, que vale igual para todos: mientras el
		// criterio de "vacio" se escriba en mas de un sitio puede discrepar en
		// mas de un sitio, y cada discrepancia acaba en el mismo lugar, que es
		// una restriccion de la base abortando el INSERT del lote ENTERO. UNA
		// normalizacion por campo, aqui arriba, ANTES de que nada los lea.
		//
		// Lo que se pierde por cada uno si el blanco no se recorta AQUI:
		//
		//   - `id`: la escapatoria `if u.ID == ""` de mas abajo se salta, el
		//     blanco viaja como id LITERAL y la segunda fila que traiga el mismo
		//     blanco choca con la clave primaria (23505). No es rebuscado que se
		//     repita: la propia fuente reutiliza contadores de fila del estilo
		//     Id_Ntx, que se renumeran en cada entrega.
		//   - `escalon`: el relleno a "pendiente" se salta, y validarUso rechaza
		//     la fila con el motivo FALSO `escalon " " en la ingesta`, cuando la
		//     verdad es que llego vacio. Un motivo equivocado en un log que
		//     existe para pedirle al cliente exactamente lo que falla es peor
		//     que no tener motivo.
		//   - `evidencia`: la regla nueva de validarUso la rechazaria por decir
		//     COMO se reconocio una obra, cuando lo que trae es una celda vacia.
		//     Mismo motivo falso, otro campo.
		//   - `rechazo_motivo`: el peor de los cuatro, porque decide DOS cosas a
		//     la vez -si la fila se valida (`== ""`, mas abajo) y si la fila va
		//     al log de rechazos (`!= ""`, al final del bucle)-. Un blanco no
		//     satisface ninguna como toca: la validacion se SALTA y la fila se
		//     rutea al log de todas formas. Lo que pasa despues depende del
		//     blanco, y las dos ramas son malas:
		//
		//     Con espacios, el CHECK (btrim(motivo) <> '') de `usos_rechazados`
		//     la rechaza con un 23514 y se lleva la transaccion del lote: una
		//     celda de una fila de una parrilla de 500 pierde las otras 499. Y
		//     el reporte ya esta escrito, asi que reintentar el archivo choca
		//     con ErrReporteDuplicado y la entrega no se recupera sin cirugia.
		//
		//     Con un tabulador, un salto de linea o el NBSP NO salta nada:
		//     `btrim` sin segundo argumento quita SOLO espacios, asi que el
		//     motivo pasa el CHECK. Eso es peor de encontrar, porque no hay
		//     error: una fila BUENA queda archivada en `usos_rechazados` con un
		//     motivo en blanco, o sea fuera de `usos`, o sea sin ponderar la
		//     bolsa, y el acuse de la entrega dice que el archivo entro
		//     completo.
		//
		// Recortar el motivo no lo pierde: uno de verdad con blancos alrededor
		// sigue siendo un motivo y sigue yendo al log, ya recortado, que es la
		// unica forma de que la comparacion de Go y el CHECK de la tabla
		// signifiquen lo mismo.
		u.ID = strings.TrimSpace(u.ID)
		u.Escalon = strings.TrimSpace(u.Escalon)
		u.Evidencia = strings.TrimSpace(u.Evidencia)
		u.RechazoMotivo = strings.TrimSpace(u.RechazoMotivo)

		u.ReporteID = rep.ID
		u.Fuente = rep.Fuente
		if u.ID == "" {
			// Derivado del reporte y de la posicion en el lote: la fila puede
			// senalar la entrega y la linea exactas de las que salio (ADR
			// 0006). No se reutiliza ningun contador de la fuente -Id_Ntx y
			// companeros se renumeran en cada entrega-, pero el reporte si es
			// estable porque su id sale de la huella.
			u.ID = rep.ID + "-" + strconv.Itoa(n)
		}
		if u.Escalon == "" {
			// Lo que trae una fila recien parseada. Exigirle el vocabulario
			// del esquema a cada adaptador de formato seria filtrar la base
			// hacia afuera.
			u.Escalon = "pendiente"
		}
		// Compara contra "" a secas, y tiene que seguir siendo asi: el TrimSpace
		// esta arriba, una vez. Repetirlo aqui volveria a dar dos criterios que
		// pueden separarse, y el de mas abajo -el NULLIF del INSERT- no se puede
		// repetir en Go de ninguna manera.
		if u.ObraID == "" {
			// A la salida de ingesta ninguna fila esta identificada: es lo que
			// dice el doc de Ingesta y lo que asume la cascada (ADR 0007). El
			// DEFAULT TRUE de la columna no llega a aplicarse porque
			// insertarUso manda el valor siempre, asi que el que vale es este.
			//
			// Es ademas lo que deja el CHECK uso_resuelto_tiene_obra fuera del
			// alcance de esta ruta, y por eso validarUso ya no lo repite.
			//
			// La guarda no sobra, aunque la fila con obra_id acabe rechazada
			// igualmente: esa fila -la de H5- llega hasta aqui. Estampar
			// ONI = true sin mirar le inventaria un estado que nunca tuvo
			// -en ONI y con obra a la vez- justo en el registro que se devuelve
			// como acuse de lo que llego. Un rechazo describe lo RECIBIDO; si de
			// paso lo normaliza, deja de ser prueba de nada.
			u.ONI = true
		}
		if u.Emisiones == 0 {
			// Igual que Escalon y ONI: el DEFAULT 1 de la columna no se aplica
			// porque insertarUso manda el valor siempre. Una parrilla real
			// nunca declara cero emisiones -la granularidad es la emision, no
			// la obra, y RD 9.1.1 las multiplica-, asi que el cero es el valor
			// vacio de Go, no un dato.
			u.Emisiones = 1
		}
		if u.RechazoMotivo == "" {
			// Un motivo que ya viene puesto lo escribio el adaptador de
			// formato, que vio cosas que aqui ya no se ven -coercion de tipo,
			// un placeholder como el `--` de episode_nbr-. Pisarlo con el
			// motivo generico perderia la unica explicacion util del log de
			// rechazos.
			u.RechazoMotivo = validarUso(u)
		}

		lote[n] = u
		if u.RechazoMotivo != "" {
			rechazados = append(rechazados, u)
		}
	}

	if err := i.Reportes.GuardarUsos(ctx, lote); err != nil {
		return nil, fmt.Errorf("guardar las filas del reporte %q: %w", rep.ID, err)
	}
	return rechazados, nil
}

// validarUso devuelve el motivo por el que una fila no es canonica, o "" si lo
// es.
//
// Cada regla refleja una restriccion de la tabla `usos`, y la duplicacion es
// el objetivo, no un descuido: una sola fila que viole un CHECK aborta el
// INSERT del lote ENTERO y se lleva por delante las filas buenas que la
// acompanan. Comprobarlo antes es lo que convierte "la entrega fallo" en "esta
// linea fallo, y por esto".
//
// El motivo NOMBRA EL CAMPO. La skill de ingesta lo pide explicitamente: el
// mensaje tiene que decir que falta o que esta mal formateado, porque es lo
// que permite volver a pedirle al cliente exactamente eso.
//
// Lo que NO se valida aqui: que obra_id exista. Eso es una clave foranea y
// mirarla costaria una consulta por fila. No hace falta: una fila que traiga
// obra_id se rechaza ANTES por venir ya identificada, asi que el unico obra_id
// que puede llegar a `usos` desde este camino es la cadena vacia. Identificar
// es trabajo de la cascada (ADR 0007).
func validarUso(u UsoPersistido) string {
	if strings.TrimSpace(u.Titulo) == "" {
		return "titulo vacio: sin titulo no hay nada que identificar"
	}
	switch u.Modalidad {
	case reparto.TV, reparto.Cine, reparto.OTT, reparto.Hotel:
	default:
		return fmt.Sprintf("modalidad %q fuera de tv|cine|ott|hotel", u.Modalidad)
	}
	if u.Escalon == "manual" {
		return "escalon manual: una resolucion manual necesita autor e instante, y no entra por ingesta"
	}

	// La cascada del ADR 0007 es el UNICO camino a obra_id (decision N4 de la
	// revision de #72). Nada entra por ingesta ya identificado: es lo que el
	// doc de Ingesta promete y lo que la cascada asume, y hasta aqui dependia de
	// que ningun adaptador de formato lo intentara. Ahora la ingesta es
	// ESTRUCTURALMENTE incapaz de producir una fila resuelta.
	//
	// Tres razones, en orden de peso:
	//
	//  1. `usos.evidencia` y `usos.puntaje` dejan de poder mentir. Son la
	//     pregunta 3 del ADR 0006, "COMO se reconocio": un escalon "alias"
	//     puesto por un adaptador de formato describiria una decision de la
	//     cascada que nunca ocurrio, y despues no hay forma de distinguirlo.
	//  2. Desaparece el camino que revienta el lote entero. Un obra_id que sale
	//     del archivo y no existe en `obras` es una violacion de clave foranea
	//     que esta funcion NO ve -es lo que demuestra TestGuardarUsosEsAtomico-
	//     y se lleva por delante las filas buenas que lo acompanan.
	//  3. Es lo que ya asume todo lo demas: UsosSinResolver filtra por
	//     escalon = 'pendiente' porque "una pendiente ni siquiera se ha
	//     intentado". Si la ingesta entregara filas ya resueltas, la cascada se
	//     saltaria trabajo en silencio.
	//
	// Si algun dia una fuente trae un identificador global de fiar, entra por el
	// escalon `id_global` DE LA CASCADA, con su evidencia y su puntaje, no por
	// un atajo aqui.
	// Contra "" a secas: GuardarUsos ya recorto los blancos antes de llamar, y
	// esa es la UNICA normalizacion del campo en todo el camino. Un TrimSpace
	// tambien aqui no seria redundante sino peligroso: sugeriria que esta funcion
	// se puede llamar con un valor sin normalizar, y el INSERT -que compara con
	// NULLIF($6, '')- no puede hacer esa misma concesion.
	if u.ObraID != "" {
		return "obra_id en la ingesta: identificar es trabajo de la cascada (ADR 0007)"
	}
	if u.Escalon != "pendiente" {
		return fmt.Sprintf("escalon %q en la ingesta: solo sale \"pendiente\" de aqui", u.Escalon)
	}
	// La otra mitad de la razon 1, que sin esta linea se quedaba a medias. Una
	// fila puede traer evidencia y NO delatarse por ninguna de las dos reglas
	// de arriba: obra_id vacio y escalon vacio -que el relleno de GuardarUsos
	// deja en "pendiente"-. La evidencia se escribia entonces VERBATIM en la
	// columna, y quedaba una fila pendiente diciendo como se reconocio una obra
	// que nadie reconocio. Despues no hay forma de distinguirla de una que si.
	//
	// `puntaje` no necesita regla porque UsoPersistido no tiene ese campo: la
	// columna se queda en su DEFAULT 0 y la ingesta no la puede tocar. El dia
	// que se anada el campo, la regla entra aqui.
	//
	// Contra "" a secas, igual que obra_id y por lo mismo: el TrimSpace esta en
	// GuardarUsos, una vez.
	if u.Evidencia != "" {
		return "evidencia en la ingesta: como se reconocio lo escribe la cascada (ADR 0007)"
	}

	// El CHECK uso_resuelto_tiene_obra NO se repite aqui, y sus DOS ramas se
	// quedan fuera por el mismo motivo: con la regla de obra_id puesta ninguna
	// de las dos puede fallar por este camino.
	//
	//   - "en ONI y con obra a la vez" no llega: la regla de obra_id de arriba
	//     devuelve antes.
	//   - "identificada y sin obra" tampoco: GuardarUsos estampa ONI = true en
	//     toda fila con obra_id vacio ANTES de llamar aqui. Eso -y no una
	//     comprobacion en esta funcion- es lo que impide la fila que violaria el
	//     CHECK, y lo fija TestGuardarUsosRellenaLosDefaultsDeUnaFilaRecienParseada.
	//
	// La segunda rama SI estuvo escrita, y salio cara: su texto tambien nombraba
	// `obra_id`, asi que las pruebas de H5 -que casaban por substring- se daban
	// por satisfechas con ella y seguian en verde aunque se quitara la regla de
	// obra_id. Rechazaban la fila por un motivo FALSO y no lo distinguian. Una
	// comprobacion que no puede fallar no prueba nada, y encima puede tapar a la
	// que si.

	// Las columnas de medida, con las DOS cosas que la base les exige: el CHECK
	// de no negatividad y la precision del tipo.
	//
	// La negativa no es un uso pequeno: es un dato roto, y ponderaria a la baja.
	//
	// La precision es la que se quedaba sin comprobar, y cuesta lo mismo que un
	// CHECK: un NUMERIC(p,s) no admite mas de p-s digitos ENTEROS, asi que un
	// `rating` -NUMERIC(12,6), tope 999999.999999- con la audiencia en personas
	// absolutas en vez de en porcentaje pasaba la validacion y moria en el
	// INSERT con SQLSTATE 22003. Dentro de la transaccion del lote, o sea
	// llevandose las filas buenas, y dejandole al operador un SQLSTATE crudo en
	// vez de un motivo por fila. Los reportes de television colombianos
	// entregan la audiencia de las dos formas, asi que es alcanzable el dia que
	// entren los adaptadores de formato del #25.
	//
	// La precision y la escala van en la tabla, con los mismos nombres de
	// columna que ya estaban, y no en seis condiciones escritas a mano: una
	// tabla se puede leer contra 00001_init.sql de un vistazo, y anadir una
	// medida es anadir una fila.
	medidas := []struct {
		campo string
		valor decimal.Decimal
		// Los de la definicion de la columna en migrations/00001_init.sql:
		// NUMERIC(precision, escala).
		precision int32
		escala    int32
	}{
		{"duracion_min", u.DuracionMin, 12, 4},
		{"rating", u.Rating, 12, 6},
		{"taquilla", u.Taquilla, 18, 2},
		{"vistas", u.Vistas, 18, 2},
		{"minutos_vistos", u.MinutosVistos, 18, 4},
		{"pb", u.PB, 18, 4},
	}
	for _, m := range medidas {
		if m.valor.IsNegative() {
			return fmt.Sprintf("%s negativa: %s", m.campo, m.valor)
		}
		// Se compara el valor REDONDEADO a la escala de la columna, que es el
		// orden en el que Postgres lo hace: redondea primero y comprueba la
		// precision despues. Sin el redondeo, 999999.9999996 cabe en seis
		// digitos enteros aqui, se convierte en 1000000.000000 alla y desborda
		// igual -el mismo 22003, en el borde que se habria quedado sin cubrir-.
		//
		// El tope es 10^(precision-escala) y es EXCLUSIVO: NUMERIC(12,6) llega
		// hasta 999999.999999, asi que 1000000 ya no cabe. decimal.New(1, e)
		// es 1 * 10^e.
		//
		// Mas decimales de los que tiene la columna no son un rechazo: la base
		// redondea a la escala y guarda. Apartar esas filas seria perder datos
		// buenos.
		if m.valor.Round(m.escala).GreaterThanOrEqual(decimal.New(1, m.precision-m.escala)) {
			return fmt.Sprintf(
				"%s %s: la columna es NUMERIC(%d,%d) y no admite mas de %d digitos enteros",
				m.campo, m.valor, m.precision, m.escala, m.precision-m.escala)
		}
	}
	if u.Emisiones < 0 {
		return fmt.Sprintf("emisiones negativas: %d", u.Emisiones)
	}
	return ""
}
