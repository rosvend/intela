package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Dobles de los puertos de ingesta. Stdlib y nada mas, como el resto del
// repositorio, y en este fichero porque son la forma de este caso de uso.

// almacenMemoria imita a objetos.Disco en lo unico que importa aqui: NO
// sobrescribe. Poner sobre una clave ya escrita devuelve ErrObjetoYaExiste y
// deja los bytes anteriores intactos, que es lo que da el O_EXCL del adaptador
// real y lo que el ADR 0006 exige de la boveda.
type almacenMemoria struct {
	objetos  map[string][]byte
	errPoner error
	puestas  int
}

func nuevoAlmacen() *almacenMemoria {
	return &almacenMemoria{objetos: map[string][]byte{}}
}

func (a *almacenMemoria) Poner(_ context.Context, clave string, datos []byte) error {
	a.puestas++
	if a.errPoner != nil {
		return a.errPoner
	}
	if _, hay := a.objetos[clave]; hay {
		return ErrObjetoYaExiste
	}
	a.objetos[clave] = append([]byte(nil), datos...)
	return nil
}

func (a *almacenMemoria) Obtener(_ context.Context, clave string) ([]byte, error) {
	datos, hay := a.objetos[clave]
	if !hay {
		return nil, ErrNoEncontrado
	}
	return datos, nil
}

// repoIngestaMemoria imita el UNIQUE (sha256, fuente) de la tabla reportes,
// que es la unica fuente de verdad de "reporte duplicado".
type repoIngestaMemoria struct {
	reportes map[string]Reporte
	huellas  map[string]bool
	usos     []UsoPersistido

	errReporte error
	errUsos    error
}

func nuevoRepoIngesta() *repoIngestaMemoria {
	return &repoIngestaMemoria{
		reportes: map[string]Reporte{},
		huellas:  map[string]bool{},
	}
}

func (r *repoIngestaMemoria) GuardarReporte(_ context.Context, id, fuente, periodo, sha, claveObjeto string, nbytes int) error {
	if r.errReporte != nil {
		return r.errReporte
	}
	if r.huellas[fuente+"|"+sha] {
		return ErrReporteDuplicado
	}
	r.huellas[fuente+"|"+sha] = true
	r.reportes[id] = Reporte{
		ID: id, Fuente: fuente, Periodo: periodo,
		SHA256: sha, ClaveObjeto: claveObjeto, NBytes: nbytes,
	}
	return nil
}

func (r *repoIngestaMemoria) GuardarUsos(_ context.Context, usos []UsoPersistido) error {
	if r.errUsos != nil {
		return r.errUsos
	}
	r.usos = append(r.usos, usos...)
	return nil
}

func (r *repoIngestaMemoria) UsosSinResolver(context.Context) ([]UsoPersistido, error) {
	return r.canonicos(), nil
}

func (r *repoIngestaMemoria) UsosDePeriodo(context.Context, string) ([]UsoPersistido, error) {
	return r.canonicos(), nil
}

func (r *repoIngestaMemoria) UsoPorID(_ context.Context, id string) (UsoPersistido, error) {
	for _, u := range r.canonicos() {
		if u.ID == id {
			return u, nil
		}
	}
	return UsoPersistido{}, ErrNoEncontrado
}

func (r *repoIngestaMemoria) ListarRechazos(context.Context) ([]UsoPersistido, error) {
	var us []UsoPersistido
	for _, u := range r.usos {
		if u.RechazoMotivo != "" {
			us = append(us, u)
		}
	}
	if us == nil {
		us = []UsoPersistido{}
	}
	return us, nil
}

// canonicos deja fuera las filas rechazadas, igual que el adaptador real: las
// guarda, pero no las devuelve por las lecturas canonicas.
func (r *repoIngestaMemoria) canonicos() []UsoPersistido {
	var us []UsoPersistido
	for _, u := range r.usos {
		if u.RechazoMotivo == "" {
			us = append(us, u)
		}
	}
	return us
}

func nuevaIngesta() (Ingesta, *repoIngestaMemoria, *almacenMemoria) {
	repo, almacen := nuevoRepoIngesta(), nuevoAlmacen()
	return Ingesta{Reportes: repo, Almacen: almacen}, repo, almacen
}

// usoBueno es una fila canonica minima que el esquema acepta: modalidad de las
// cuatro, titulo, y sin obra porque todavia no se ha identificado nada.
func usoBueno(titulo string) UsoPersistido {
	return UsoPersistido{
		Fuente:    "caracol",
		Titulo:    titulo,
		Modalidad: reparto.TV,
		Escalon:   "pendiente",
		ONI:       true,
		Emisiones: 1,
	}
}

// repDePrueba es el acuse del que cuelgan las filas. GuardarUsos recibe el
// Reporte entero y no solo su id porque la fila hereda de el las dos cosas que
// la atan a la entrega: el reporte y la fuente.
func repDePrueba() Reporte {
	return Reporte{ID: "rep-1", Fuente: "caracol"}
}

// ---------------------------------------------------------------------------
// Huella y derivacion de clave
// ---------------------------------------------------------------------------

// Vectores publicados de SHA-256. Se comprueban contra constantes y no contra
// otra llamada a sha256: comparar la funcion consigo misma pasa aunque cambie
// el algoritmo, y la huella es lo que ata una cifra a un byte concreto.
func TestHuellaEsElSHA256DeLosBytes(t *testing.T) {
	casos := map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for datos, quiero := range casos {
		if hay := huella([]byte(datos)); hay != quiero {
			t.Fatalf("huella(%q) = %q, se esperaba %q", datos, hay, quiero)
		}
	}
}

// La clave la compone el nucleo, pero la escribe un adaptador que rechaza todo
// lo que no sea [A-Za-z0-9._-] separado por barras. Si la derivacion se saliera
// de ese alfabeto, la subida fallaria en produccion y no aqui.
func TestLaClaveDelObjetoSaleDeLaHuellaYEsSegura(t *testing.T) {
	segura := regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

	uno, dos := claveObjeto(huella([]byte("abc"))), claveObjeto(huella([]byte("xyz")))
	if uno == dos {
		t.Fatal("dos contenidos distintos no pueden compartir clave")
	}
	if uno != claveObjeto(huella([]byte("abc"))) {
		t.Fatal("la clave tiene que ser funcion del contenido y de nada mas")
	}
	if !segura.MatchString(uno) {
		t.Fatalf("clave %q fuera del alfabeto que acepta el almacen", uno)
	}
}

// El id sale del par que el UNIQUE (sha256, fuente) hace unico, no del sha
// solo: el esquema permite a proposito que dos fuentes declaren los mismos
// bytes, y con un id derivado solo del contenido la segunda chocaria contra la
// clave primaria.
func TestElIDDelReporteDistingueLaFuente(t *testing.T) {
	sha := huella([]byte("titulo,emisiones\n"))
	caracol, netflix := idReporte("caracol", sha), idReporte("netflix", sha)

	if caracol == netflix {
		t.Fatal("dos fuentes con los mismos bytes no pueden compartir id")
	}
	if caracol != idReporte("caracol", sha) {
		t.Fatal("el id tiene que ser funcion de (fuente, huella) y de nada mas")
	}
}

// ---------------------------------------------------------------------------
// GuardarReporte
// ---------------------------------------------------------------------------

func TestGuardarReporteDejaLaEvidenciaYLaFila(t *testing.T) {
	ingesta, repo, almacen := nuevaIngesta()
	datos := []byte("titulo,emisiones\nLa Casa,3\n")

	rep, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos)
	if err != nil {
		t.Fatalf("GuardarReporte: %v", err)
	}
	if rep.SHA256 != huella(datos) {
		t.Fatalf("SHA256 = %q, se esperaba %q", rep.SHA256, huella(datos))
	}
	if rep.NBytes != len(datos) {
		t.Fatalf("NBytes = %d, se esperaba %d", rep.NBytes, len(datos))
	}
	if rep.Fuente != "caracol" || rep.Periodo != "2026-01" {
		t.Fatalf("procedencia mal registrada: %+v", rep)
	}

	guardado, err := almacen.Obtener(t.Context(), rep.ClaveObjeto)
	if err != nil {
		t.Fatalf("la evidencia no quedo en la boveda: %v", err)
	}
	if string(guardado) != string(datos) {
		t.Fatalf("la boveda guardo %q, se subio %q", guardado, datos)
	}
	if _, hay := repo.reportes[rep.ID]; !hay {
		t.Fatalf("no hay fila de reportes para %q", rep.ID)
	}
}

// El criterio de aceptacion textual del issue: resubir los mismos bytes da
// error de duplicado y el objeto queda SIN CAMBIOS.
func TestGuardarReporteRechazaLaResubidaSinTocarElObjeto(t *testing.T) {
	ingesta, repo, almacen := nuevaIngesta()
	datos := []byte("titulo,emisiones\nLa Casa,3\n")

	rep, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos)
	if err != nil {
		t.Fatalf("primera subida: %v", err)
	}

	_, err = ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos)
	if !errors.Is(err, ErrReporteDuplicado) {
		t.Fatalf("se esperaba ErrReporteDuplicado, se obtuvo %v", err)
	}
	if len(repo.reportes) != 1 {
		t.Fatalf("se esperaba 1 fila en reportes, hay %d", len(repo.reportes))
	}

	guardado, err := almacen.Obtener(t.Context(), rep.ClaveObjeto)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if string(guardado) != string(datos) {
		t.Fatalf("el objeto cambio en la resubida: %q", guardado)
	}
}

// Los mismos bytes desde dos fuentes distintas son dos entregas distintas: el
// UNIQUE (sha256, fuente) las permite a proposito. Comparten un unico objeto
// porque la clave es el contenido, y eso es lo correcto, no una colision.
func TestGuardarReporteAdmiteLosMismosBytesDeOtraFuente(t *testing.T) {
	ingesta, repo, almacen := nuevaIngesta()
	datos := []byte("titulo,vistas\nLa Casa,7\n")

	uno, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos)
	if err != nil {
		t.Fatalf("primera fuente: %v", err)
	}
	dos, err := ingesta.GuardarReporte(t.Context(), "netflix", "2026-01", datos)
	if err != nil {
		t.Fatalf("segunda fuente: %v", err)
	}

	if uno.ID == dos.ID {
		t.Fatal("dos entregas distintas no pueden compartir id")
	}
	if uno.ClaveObjeto != dos.ClaveObjeto {
		t.Fatal("los mismos bytes tienen que compartir objeto: la clave es el contenido")
	}
	if len(repo.reportes) != 2 {
		t.Fatalf("se esperaban 2 filas en reportes, hay %d", len(repo.reportes))
	}
	if len(almacen.objetos) != 1 {
		t.Fatalf("se esperaba 1 objeto en la boveda, hay %d", len(almacen.objetos))
	}
}

// El estado que no puede existir: un acuse en reportes que apunte a una
// evidencia que no se llego a escribir. Por eso la boveda va primero.
func TestGuardarReporteNoDejaAcuseSinEvidencia(t *testing.T) {
	ingesta, repo, almacen := nuevaIngesta()
	almacen.errPoner = errors.New("disco lleno")

	_, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", []byte("x"))
	if err == nil {
		t.Fatal("se esperaba error: la evidencia no se pudo escribir")
	}
	if len(repo.reportes) != 0 {
		t.Fatalf("no puede haber fila de reportes sin evidencia: %+v", repo.reportes)
	}
}

// El fallo a medias de la vez anterior: el objeto se escribio y el acuse no.
// El reintento tiene que completarse, no quedarse bloqueado por su propio
// resto. Es la contrapartida de escribir la boveda primero.
func TestGuardarReporteCompletaUnaSubidaAMedias(t *testing.T) {
	ingesta, repo, almacen := nuevaIngesta()
	datos := []byte("titulo,emisiones\nLa Casa,3\n")

	// Objeto huerfano: esta en la boveda y no hay fila que lo referencie.
	if err := almacen.Poner(t.Context(), claveObjeto(huella(datos)), datos); err != nil {
		t.Fatalf("preparar el huerfano: %v", err)
	}

	if _, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos); err != nil {
		t.Fatalf("el reintento tenia que completarse: %v", err)
	}
	if len(repo.reportes) != 1 {
		t.Fatalf("se esperaba 1 fila en reportes, hay %d", len(repo.reportes))
	}
}

// H4b: "ya estaba" no es lo mismo que "son estos bytes".
//
// La clave es la huella, asi que lo que hay bajo ella DEBERIA ser el contenido
// que se esta subiendo. Pero eso lo garantiza quien escribio, no quien lee: un
// objeto desgarrado por un fallo anterior, una copia restaurada a medias o un
// almacen que no escriba atomicamente dejan ahi otra cosa. Sin comprobarlo, el
// acuse certifica un SHA-256 que el objeto real no tiene, y no se recupera
// solo: la resubida choca con ErrReporteDuplicado.
//
// Es independiente del adaptador a proposito. Aunque Disco ya escriba
// atomicamente, esto sigue siendo cierto el dia que entre MinIO o S3.
func TestGuardarReporteNoCertificaUnObjetoQueNoEsElSuyo(t *testing.T) {
	ingesta, repo, almacen := nuevaIngesta()
	datos := []byte("titulo,emisiones\nLa Casa,3\n")

	// Objeto DESGARRADO bajo la clave: los primeros bytes de la entrega y nada
	// mas, que es lo que deja una escritura que murio a medias.
	almacen.objetos[claveObjeto(huella(datos))] = datos[:10]

	_, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos)
	if !errors.Is(err, ErrEvidenciaCorrupta) {
		t.Fatalf("se esperaba ErrEvidenciaCorrupta, se obtuvo %v", err)
	}
	if len(repo.reportes) != 0 {
		t.Fatalf(
			"no puede quedar un acuse que certifique una huella que el objeto no tiene: %+v",
			repo.reportes)
	}

	// El mensaje tiene que nombrar las DOS huellas. La esperada sola no sirve
	// de nada: la clave del objeto se DERIVA de ella, asi que un mensaje que
	// solo la lleve la dice dos veces y se lee como "X no corresponde a X". Lo
	// que hace falta para depurar es que hay ahi de verdad.
	esperada, real := huella(datos), huella(datos[:10])
	if esperada == real {
		t.Fatal("el fixture no sirve: el objeto desgarrado tiene que hashear distinto")
	}
	if !strings.Contains(err.Error(), real) {
		t.Errorf("el mensaje no dice la huella REAL del objeto (%s): %v", real, err)
	}
	if !strings.Contains(err.Error(), esperada) {
		t.Errorf("el mensaje no dice la huella esperada (%s): %v", esperada, err)
	}

	// Y los DOS tamanos, que son la mitad del mensaje que nadie estaba
	// comprobando. Las huellas dicen QUE no cuadra; los tamanos separan los dos
	// casos que hay que tratar distinto:
	//
	//   - menos bytes de los que se suben es un objeto CORTADO -escritura que
	//     murio a medias, copia restaurada mal-, y la clave se puede liberar;
	//   - el mismo tamano con otra huella es contenido AJENO bajo esa clave, que
	//     es un problema de otro orden y no se arregla reescribiendo.
	//
	// Sin esta asercion, borrar los dos %d del formato deja el mensaje sin esa
	// distincion y ninguna prueba se entera.
	//
	// Cada tamano se busca PEGADO a su huella y no suelto: un "10" a secas
	// aparece por azar en cualquiera de los dos hexadecimales de 64 caracteres,
	// asi que buscarlo solo daria una prueba que pasa aunque se quiten los
	// tamanos. Emparejados, la asercion caza ademas el fallo de cruzarlos.
	quiero := []string{
		fmt.Sprintf("%d bytes de huella %s", len(datos[:10]), real),
		fmt.Sprintf("%d bytes de huella %s", len(datos), esperada),
	}
	for _, q := range quiero {
		if !strings.Contains(err.Error(), q) {
			t.Errorf("el mensaje no dice %q: %v", q, err)
		}
	}
}

// La contrapartida de la prueba de arriba, y la razon de que la comprobacion
// sea por HUELLA y no por "habia algo": el huerfano legitimo -bytes correctos,
// acuse que falto- tiene que seguir completandose. Es lo que ya comprueba
// TestGuardarReporteCompletaUnaSubidaAMedias; aqui se fija que la verificacion
// nueva no le quita esa propiedad ni siquiera cuando el objeto lo puso otra
// fuente.
func TestGuardarReporteAceptaElObjetoAjenoSiEsElMismoContenido(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()
	datos := []byte("titulo,vistas\nLa Casa,7\n")

	if _, err := ingesta.GuardarReporte(t.Context(), "caracol", "2026-01", datos); err != nil {
		t.Fatalf("primera fuente: %v", err)
	}
	// La segunda fuente encuentra el objeto ya puesto por la primera. Los bytes
	// son los mismos, asi que la verificacion pasa y la entrega se acepta.
	if _, err := ingesta.GuardarReporte(t.Context(), "netflix", "2026-01", datos); err != nil {
		t.Fatalf("los mismos bytes de otra fuente son una entrega valida: %v", err)
	}
	if len(repo.reportes) != 2 {
		t.Fatalf("se esperaban 2 acuses, hay %d", len(repo.reportes))
	}
}

// La estructura minima se comprueba ANTES de tocar la boveda: un periodo mal
// formateado lo rechazaria el CHECK de la tabla despues de haber escrito un
// objeto que ya no se puede borrar.
func TestGuardarReporteRechazaLoQueElEsquemaNoAdmite(t *testing.T) {
	casos := map[string]struct {
		fuente, periodo string
		datos           []byte
	}{
		"sin fuente":       {"", "2026-01", []byte("x")},
		"periodo vacio":    {"caracol", "", []byte("x")},
		"periodo con dia":  {"caracol", "2026-01-15", []byte("x")},
		"periodo en letra": {"caracol", "enero", []byte("x")},
		"sin bytes":        {"caracol", "2026-01", nil},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, almacen := nuevaIngesta()

			_, err := ingesta.GuardarReporte(t.Context(), c.fuente, c.periodo, c.datos)
			if !errors.Is(err, ErrReporteInvalido) {
				t.Fatalf("se esperaba ErrReporteInvalido, se obtuvo %v", err)
			}
			if almacen.puestas != 0 {
				t.Fatal("no se puede escribir en la boveda una entrega que no se va a aceptar")
			}
			if len(repo.reportes) != 0 {
				t.Fatal("no puede quedar fila de una entrega rechazada")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GuardarUsos y el log de rechazos
// ---------------------------------------------------------------------------

// El caso que nombra el issue: dos filas buenas y una malformada dan dos usos
// canonicos y un rechazo con motivo. Ninguna de las tres se pierde.
func TestGuardarUsosSeparaLoMalformadoSinDescartarlo(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	mala := usoBueno("Radio Novela")
	mala.Modalidad = "radio"

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(), []UsoPersistido{
		usoBueno("La Casa de las Dos Palmas"),
		mala,
		usoBueno("Cronica de una Muerte"),
	})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	if len(rechazados) != 1 {
		t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
	}
	if rechazados[0].Titulo != "Radio Novela" {
		t.Fatalf("se rechazo la fila equivocada: %+v", rechazados[0])
	}
	if rechazados[0].RechazoMotivo == "" {
		t.Fatal("un rechazo sin motivo no es un rechazo, es una perdida")
	}
	// El motivo tiene que decir QUE campo esta mal, no fallar genericamente.
	if !regexp.MustCompile(`modalidad`).MatchString(rechazados[0].RechazoMotivo) {
		t.Fatalf("el motivo no nombra el campo: %q", rechazados[0].RechazoMotivo)
	}

	// Las tres llegan al repositorio: el log de rechazos es persistencia, no
	// un descarte con mensaje.
	if len(repo.usos) != 3 {
		t.Fatalf("se esperaban 3 filas persistidas, llegaron %d", len(repo.usos))
	}
	if len(repo.canonicos()) != 2 {
		t.Fatalf("se esperaban 2 usos canonicos, hay %d", len(repo.canonicos()))
	}
}

// Un lote entero de basura tampoco se descarta, y no llega ni una fila a la
// tabla canonica.
func TestGuardarUsosConTodoMalNoEsUnError(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	sinTitulo := usoBueno("")
	sinModalidad := usoBueno("X")
	sinModalidad.Modalidad = ""

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
		[]UsoPersistido{sinTitulo, sinModalidad})
	if err != nil {
		t.Fatalf("un lote invalido no es un fallo del caso de uso: %v", err)
	}
	if len(rechazados) != 2 {
		t.Fatalf("se esperaban 2 rechazos, llegaron %d", len(rechazados))
	}
	if len(repo.canonicos()) != 0 {
		t.Fatalf("no puede quedar nada en usos: %+v", repo.canonicos())
	}
	if len(repo.usos) != 2 {
		t.Fatalf("las 2 filas tienen que quedar en el log: llegaron %d", len(repo.usos))
	}
}

// El id y el reporte los estampa el caso de uso: la fila tiene que poder
// senalar el reporte y la posicion exacta de la que salio (ADR 0006).
func TestGuardarUsosEstampaReporteEIdentificadorTrazable(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	propio := usoBueno("Con id propio")
	propio.ID = "uso-elegido-por-quien-llama"

	if _, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
		[]UsoPersistido{usoBueno("Sin id"), propio}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	if repo.usos[0].ID == "" {
		t.Fatal("una fila sin id no se puede referenciar")
	}
	if repo.usos[0].ID == repo.usos[1].ID {
		t.Fatal("dos filas del mismo lote no pueden compartir id")
	}
	if repo.usos[1].ID != "uso-elegido-por-quien-llama" {
		t.Fatalf("un id ya asignado no se pisa: %q", repo.usos[1].ID)
	}
	for _, u := range repo.usos {
		if u.ReporteID != "rep-1" {
			t.Fatalf("ReporteID = %q, se esperaba \"rep-1\"", u.ReporteID)
		}
	}
}

func TestGuardarUsosSinFilasNoTocaElRepositorio(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(), nil)
	if err != nil {
		t.Fatalf("un lote vacio no es un error: %v", err)
	}
	if len(rechazados) != 0 || len(repo.usos) != 0 {
		t.Fatal("un lote vacio no escribe nada")
	}
}

// Cada regla de aqui refleja un CHECK de la tabla usos. No es duplicar el
// esquema por gusto: una fila que viola un CHECK aborta el INSERT del lote
// ENTERO y se lleva por delante las filas buenas que la acompanan.
//
// El motivo se compara ENTERO y no por substring. Comprobar que "nombra el
// campo" es mas debil de lo que parece: varias reglas distintas nombran el
// mismo campo, asi que una asercion por substring no distingue "rechazada por
// la regla que se esta probando" de "rechazada por otra que dice algo falso".
// Ya paso: la regla de obra_id de H5 se podia quitar entera y estas pruebas
// seguian verdes porque el motivo de la coherencia oni/obra tambien decia
// `obra_id`.
func TestValidarUsoNombraElCampoQueFalla(t *testing.T) {
	conObra := usoBueno("Identificada")
	conObra.ObraID = "obra-1"
	conObra.ONI = true

	negativo := usoBueno("Negativa")
	negativo.Vistas = decimal.NewFromInt(-1)

	manual := usoBueno("Manual")
	manual.Escalon = "manual"

	casos := map[string]struct {
		uso    UsoPersistido
		motivo string
	}{
		"buena":                 {usoBueno("La Casa"), ""},
		"sin titulo":            {usoBueno("   "), "titulo vacio: sin titulo no hay nada que identificar"},
		"modalidad desconocida": {func() UsoPersistido { u := usoBueno("X"); u.Modalidad = "radio"; return u }(), `modalidad "radio" fuera de tv|cine|ott|hotel`},
		"escalon desconocido":   {func() UsoPersistido { u := usoBueno("X"); u.Escalon = "adivinado"; return u }(), `escalon "adivinado" en la ingesta: solo sale "pendiente" de aqui`},
		// Ya no la caza la coherencia oni/obra sino la regla de H5, que es
		// anterior y mas estricta: con obra_id puesto no se mira nada mas. Que el
		// motivo se compare entero es lo que lo demuestra.
		"con obra por ingesta": {conObra, "obra_id en la ingesta: identificar es trabajo de la cascada (ADR 0007)"},
		// La otra mitad de lo que N4 argumenta: `evidencia` tampoco puede
		// mentir. Esta fila no se delata por obra_id ni por el escalon.
		"con evidencia por ingesta": {func() UsoPersistido {
			u := usoBueno("X")
			u.Evidencia = "alias caracol/ID_Ficha=1234"
			return u
		}(), "evidencia en la ingesta: como se reconocio lo escribe la cascada (ADR 0007)"},
		// A proposito VALIDA, y no es un descuido: la mitad "identificada y sin
		// obra" del CHECK uso_resuelto_tiene_obra ya no se comprueba aqui porque
		// GuardarUsos estampa ONI = true antes de llamar, asi que esta forma no
		// existe por esa ruta. Quien lo garantiza es
		// TestGuardarUsosRellenaLosDefaultsDeUnaFilaRecienParseada, no esta
		// funcion. Dejarlo escrito para que la frontera se vea, en vez de que la
		// linea desaparezca sin dejar rastro.
		"resuelta sin obra, que aqui ya no se mira": {func() UsoPersistido { u := usoBueno("X"); u.ONI = false; return u }(), ""},
		"medida negativa":     {negativo, "vistas negativa: -1"},
		"emisiones negativas": {func() UsoPersistido { u := usoBueno("X"); u.Emisiones = -1; return u }(), "emisiones negativas: -1"},
		"manual por ingesta":  {manual, "escalon manual: una resolucion manual necesita autor e instante, y no entra por ingesta"},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			if motivo := validarUso(c.uso); motivo != c.motivo {
				t.Fatalf("motivo = %q, se esperaba %q", motivo, c.motivo)
			}
		})
	}
}

// H5: la cascada (ADR 0007) es el UNICO camino a obra_id.
//
// Una fila que llega ya identificada -con obra_id puesto o con un escalon
// adelantado- es una entrada corrupta o manipulada, y tiene que acabar en el log
// de rechazos con su motivo, nunca en `usos`. Lo que cierra son tres cosas:
// `evidencia` y `puntaje` describirian una decision de la cascada que nunca
// ocurrio (pregunta 3 del ADR 0006); un obra_id inexistente es una violacion de
// clave foranea que revienta el INSERT del lote ENTERO; y `UsosSinResolver`
// filtra por escalon = 'pendiente', asi que una fila que entrara ya resuelta se
// saltaria la cascada en silencio.
//
// # El motivo se compara ENTERO, y esa es la mitad del valor de esta prueba
//
// Con la asercion anterior -substring "obra_id"- el caso "con obra_id" pasaba
// aunque se borrara la regla que dice probar: la fila caia entonces en la
// comprobacion de coherencia oni/obra, cuyo motivo ("marcada como identificada
// y sin obra_id") tambien contiene la palabra. La fila SI se rechazaba, pero
// por una razon FALSA -decia "sin obra_id" de una fila que traia obra_id- y la
// prueba no sabia notar la diferencia. Un rechazo con el motivo equivocado no
// es el comportamiento que H5 promete: el log de rechazos existe para poder
// pedirle al cliente exactamente lo que falla.
func TestGuardarUsosRechazaLaFilaQueLlegaYaIdentificada(t *testing.T) {
	casos := map[string]struct {
		ajustar func(*UsoPersistido)
		motivo  string
	}{
		"con obra_id": {
			func(u *UsoPersistido) { u.ObraID = "obra-1"; u.ONI = false },
			"obra_id en la ingesta: identificar es trabajo de la cascada (ADR 0007)",
		},
		"escalon alias": {
			func(u *UsoPersistido) { u.Escalon = "alias" },
			`escalon "alias" en la ingesta: solo sale "pendiente" de aqui`,
		},
		"escalon id_global": {
			func(u *UsoPersistido) { u.Escalon = "id_global" },
			`escalon "id_global" en la ingesta: solo sale "pendiente" de aqui`,
		},
		"escalon difuso": {
			func(u *UsoPersistido) { u.Escalon = "difuso" },
			`escalon "difuso" en la ingesta: solo sale "pendiente" de aqui`,
		},
		"escalon oni": {
			func(u *UsoPersistido) { u.Escalon = "oni" },
			`escalon "oni" en la ingesta: solo sale "pendiente" de aqui`,
		},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			manipulada := usoBueno("Llega Ya Resuelta")
			c.ajustar(&manipulada)

			rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
				[]UsoPersistido{manipulada, usoBueno("La Casa")})
			if err != nil {
				t.Fatalf("una fila manipulada no es un fallo del caso de uso: %v", err)
			}

			if len(rechazados) != 1 {
				t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
			}
			if rechazados[0].Titulo != "Llega Ya Resuelta" {
				t.Fatalf("se rechazo la fila equivocada: %+v", rechazados[0])
			}
			if rechazados[0].RechazoMotivo != c.motivo {
				t.Fatalf("motivo = %q, se esperaba %q", rechazados[0].RechazoMotivo, c.motivo)
			}

			// No en `usos`: la fila buena que la acompana si, la manipulada no.
			canonicos := repo.canonicos()
			if len(canonicos) != 1 {
				t.Fatalf("se esperaba 1 uso canonico, hay %d: %+v", len(canonicos), canonicos)
			}
			if canonicos[0].Titulo != "La Casa" {
				t.Fatalf("la fila canonica no es la buena: %+v", canonicos[0])
			}
			// Y no se descarta: el log de rechazos la conserva con su motivo.
			if len(repo.usos) != 2 {
				t.Fatalf("las 2 filas tienen que persistirse, llegaron %d", len(repo.usos))
			}
		})
	}
}

// blancos son las formas de "vacio en espiritu" que traen los archivos reales.
//
// El NBSP (U+00A0) esta a proposito y no es rebuscado: es con lo que Excel
// rellena las celdas que se ven vacias, y es ademas el que separa un recorte
// hecho a mano -" \t\n" y poco mas- de strings.TrimSpace, cuya definicion de
// blanco es unicode.IsSpace y si lo incluye.
var blancos = map[string]string{
	"espacio":          " ",
	"tabulador":        "\t",
	"salto de linea":   "\n",
	"retorno":          "\r",
	"nbsp":             " ",
	"varios mezclados": " \t\r\n  ",
}

// Un obra_id de solo blancos es "sin obra", no "con obra", y tiene que seguir
// el camino normal de ONI.
//
// # Por que existe este caso borde
//
// H5 rechaza la fila que llega ya identificada. Pero "identificada" se decidia
// comparando con la cadena vacia EN CRUDO, y un espacio no es la cadena vacia:
// una fila sin obra ninguna, con un blanco donde el export dejo la celda,
// acababa en el log de rechazos con el motivo de H5 -"obra_id en la ingesta"-
// acusandola de traer una identificacion que no traia. Es el mismo motivo FALSO
// que H5 vino a arreglar, un borde mas alla.
//
// # La asercion que importa es la del VALOR GUARDADO, no la del rechazo
//
// Que no se rechace lo cumple cualquier TrimSpace puesto en la comprobacion de
// turno. Lo que hace falta es que el valor que sale hacia el repositorio sea la
// cadena vacia EXACTA, porque el INSERT lo pasa por un NULLIF contra la cadena
// vacia literal y esa comparacion no se puede aflojar desde Go.
//
// Un TrimSpace escrito en las comprobaciones en vez de sobre el campo deja pasar
// la fila y manda el blanco intacto al SQL, donde el NULLIF no lo anula y el
// CHECK uso_resuelto_tiene_obra aborta el lote ENTERO. Por eso se comprueba
// `guardado.ObraID == ""`: es lo unico que distingue la normalizacion de verdad
// del parche que la aparenta.
func TestGuardarUsosTrataElObraIDEnBlancoComoSinObra(t *testing.T) {
	for nombre, blanco := range blancos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			// Una fila que de verdad no tiene obra: lo unico raro es el blanco.
			sinObra := usoBueno("Sin Obra De Verdad")
			sinObra.ObraID = blanco
			// ONI a false a proposito, y no el true que trae usoBueno: es el
			// valor cero de Go, o sea lo que deja un adaptador de formato (#25)
			// que mapee lo que hay en el archivo, porque `oni` no es columna de
			// ninguna parrilla. Con el true del fixture, la asercion de mas abajo
			// pasaria sin que la guarda llegara a ejecutarse nunca.
			sinObra.ONI = false

			rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
				[]UsoPersistido{sinObra})
			if err != nil {
				t.Fatalf("GuardarUsos: %v", err)
			}
			if len(rechazados) != 0 {
				t.Fatalf(
					"una fila sin obra rechazada por traer obra: motivo %q",
					rechazados[0].RechazoMotivo)
			}
			if len(repo.canonicos()) != 1 {
				t.Fatalf("se esperaba 1 uso canonico, hay %d", len(repo.canonicos()))
			}

			guardado := repo.usos[0]
			if guardado.ObraID != "" {
				t.Errorf(
					"ObraID = %q, se esperaba la cadena vacia EXACTA: "+
						"el INSERT compara con NULLIF($6, ''), que no recorta nada",
					guardado.ObraID)
			}
			if !guardado.ONI {
				t.Error("sin obra es ONI: la guarda que estampa ONI tiene que ver " +
					"el blanco como vacio, o la fila llega al INSERT con oni = false " +
					"y obra_id NULL, que es justo lo que el CHECK prohibe")
			}
			if guardado.Escalon != "pendiente" {
				t.Errorf("Escalon = %q, se esperaba \"pendiente\"", guardado.Escalon)
			}
		})
	}
}

// La contrapartida, y la que impide que el arreglo se pase de frenada: recortar
// los blancos NO afloja H5.
//
// Una fila que trae una obra de verdad sigue rechazada con su motivo verdadero,
// venga pegada al margen o rodeada de espacios. El recorte decide si el campo
// esta vacio, no si la regla aplica.
func TestGuardarUsosNoAflojaH5AlRecortarLosBlancos(t *testing.T) {
	const motivoH5 = "obra_id en la ingesta: identificar es trabajo de la cascada (ADR 0007)"

	casos := map[string]string{
		"pegada":            "obra-1",
		"con espacios":      "  obra-1  ",
		"con tabuladores":   "\tobra-1\t",
		"con nbsp":          " obra-1 ",
		"blanco por dentro": "obra 1",
		"solo un guion":     "-",
	}

	for nombre, obraID := range casos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			conObra := usoBueno("Llega Ya Resuelta")
			conObra.ObraID = obraID
			conObra.ONI = false

			rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
				[]UsoPersistido{conObra, usoBueno("La Casa")})
			if err != nil {
				t.Fatalf("GuardarUsos: %v", err)
			}
			if len(rechazados) != 1 {
				t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
			}
			// El motivo ENTERO: un rechazo por la razon equivocada no es el
			// comportamiento que H5 promete.
			if rechazados[0].RechazoMotivo != motivoH5 {
				t.Fatalf("motivo = %q, se esperaba %q", rechazados[0].RechazoMotivo, motivoH5)
			}
			if len(repo.canonicos()) != 1 || repo.canonicos()[0].Titulo != "La Casa" {
				t.Fatalf("la fila buena no sobrevivio: %+v", repo.canonicos())
			}
		})
	}
}

// Un blanco en UNA fila no puede costar el lote entero.
//
// Es la mitad cara del defecto y la que no se ve en la capa de aplicacion sin
// buscarla: con el criterio de "vacio" repartido entre Go y el NULLIF del SQL,
// la fila del blanco esquiva las dos comprobaciones de Go, viola el CHECK
// uso_resuelto_tiene_obra en el INSERT y se lleva por delante a las buenas que
// la acompanan -la escritura del lote es UNA transaccion a proposito-. Aqui se
// fija la forma que sale del caso de uso; contra PostgreSQL lo comprueba
// TestIngestaNoPierdeElLotePorUnObraIDEnBlanco, que es donde el CHECK existe.
func TestGuardarUsosNoPierdeElLotePorUnObraIDEnBlanco(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	conBlanco := usoBueno("Blanco En Obra")
	conBlanco.ObraID = "  \t"

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(), []UsoPersistido{
		usoBueno("Buena Uno"),
		conBlanco,
		usoBueno("Buena Dos"),
	})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 0 {
		t.Fatalf("ninguna de las tres es rechazable: %+v", rechazados)
	}
	if len(repo.canonicos()) != 3 {
		t.Fatalf("se esperaban 3 usos canonicos, hay %d: %+v", len(repo.canonicos()), repo.canonicos())
	}
	// Y las tres salen con obra_id vacio EXACTO, que es lo que el INSERT sabe
	// convertir en NULL.
	for _, u := range repo.usos {
		if u.ObraID != "" {
			t.Errorf("%q sale con ObraID = %q: el NULLIF del INSERT no lo va a anular",
				u.Titulo, u.ObraID)
		}
	}
}

// La otra mitad de H5, y la que impide que el arreglo se pase de frenada: una
// fila recien parseada NO trae escalon, y el relleno a "pendiente" corre ANTES
// de la validacion. Si la regla nueva mirara el valor sin rellenar, rechazaria
// el 100% de un lote recien mapeado por un adaptador de formato (#25), que es
// justo el fallo que ya costo H1.
func TestGuardarUsosNoConfundeElEscalonVacioConUnoAdelantado(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	recien := UsoPersistido{Titulo: "Recien Parseada", Modalidad: reparto.TV}

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(), []UsoPersistido{recien})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 0 {
		t.Fatalf("una fila recien parseada no puede salir rechazada: %q", rechazados[0].RechazoMotivo)
	}
	if len(repo.canonicos()) != 1 {
		t.Fatalf("se esperaba 1 uso canonico, hay %d", len(repo.canonicos()))
	}
}

// Un escalon vacio es lo que trae una fila recien parseada, y significa
// "pendiente". Rechazarla obligaria a todo adaptador de formato a conocer el
// vocabulario del esquema.
//
// Y "vacio" incluye los blancos, que es el borde donde el motivo salia FALSO.
// Un escalon de un solo espacio no era "" para la guarda que rellena el
// defecto, asi que llegaba SIN rellenar a validarUso y se rechazaba con
//
//	escalon " " en la ingesta: solo sale "pendiente" de aqui
//
// acusando a la fila de traer un escalon adelantado -de la cascada, o manual-
// cuando lo que traia era una celda vacia. Es el mismo motivo FALSO que ya
// costo el borde de obra_id: el log de rechazos existe para pedirle al cliente
// exactamente lo que falla, y ahi le pedia que arreglara un escalon que nunca
// declaro.
//
// El caso "cadena vacia" se queda en la tabla: es el que trae de verdad un
// adaptador de formato (#25), y sin el el arreglo del blanco podria pasarse de
// frenada sin que se notara.
func TestGuardarUsosTrataElEscalonVacioComoPendiente(t *testing.T) {
	casos := map[string]string{"cadena vacia": ""}
	for nombre, blanco := range blancos {
		casos[nombre] = blanco
	}

	for nombre, escalon := range casos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			recien := usoBueno("Recien parseada")
			recien.Escalon = escalon

			rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(), []UsoPersistido{recien})
			if err != nil {
				t.Fatalf("GuardarUsos: %v", err)
			}
			if len(rechazados) != 0 {
				t.Fatalf("no se esperaba rechazo: %q", rechazados[0].RechazoMotivo)
			}
			if repo.usos[0].Escalon != "pendiente" {
				t.Fatalf("Escalon = %q, se esperaba \"pendiente\"", repo.usos[0].Escalon)
			}
		})
	}
}

// La forma exacta que produce un adaptador de formato (#25): SOLO los campos
// que existen en el archivo del cliente. Fuente, Escalon, ONI y Emisiones no
// son columna de ninguna parrilla ni de ningun reporte OTT, asi que llegan en
// el valor cero de Go y los tiene que rellenar el caso de uso.
//
// Lo de ONI es ademas lo UNICO que hoy impide la fila que violaria el CHECK
// uso_resuelto_tiene_obra: validarUso ya no repite ese CHECK, precisamente
// porque este relleno hace inalcanzable la unica de sus dos ramas que quedaba.
// Si alguien quita el relleno, la fila deja de rechazarse y pasa a reventar el
// INSERT del lote ENTERO contra la base; el fallo se ve aqui o no se ve hasta
// produccion.
//
// Los tres sintomas que cubre, y ninguno se ve como un error:
//
//   - ONI en false con obra vacia sale de validarUso como VALIDA -esa rama del
//     CHECK ya no se comprueba en Go- y llega a `usos` con oni = false y
//     obra_id NULL, que es justo lo que el CHECK prohibe: aborta el lote entero,
//     buenas incluidas, y encima el reporte ya esta escrito, asi que reintentar
//     el mismo archivo choca con ErrReporteDuplicado.
//   - Emisiones en 0 pasa el CHECK y deja la fila canonica aportando CERO
//     puntos a su obra, porque las emisiones multiplican (RD 9.1.1).
//   - Fuente vacia satisface el TEXT NOT NULL y hace que Alias(), que indexa
//     por fuente, no case NUNCA: parece un catalogo incompleto.
func TestGuardarUsosRellenaLosDefaultsDeUnaFilaRecienParseada(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	rep, err := ingesta.GuardarReporte(
		t.Context(), "caracol", "2026-01", []byte("Titulo,Duracion\nLa Casa,48\n"))
	if err != nil {
		t.Fatalf("GuardarReporte: %v", err)
	}

	recien := UsoPersistido{
		Titulo:      "La Casa de las Dos Palmas",
		Modalidad:   reparto.TV,
		IDsFuente:   "ID_Ficha=1234",
		TipoObra:    "serie",
		DuracionMin: decimal.NewFromInt(48),
	}

	rechazados, err := ingesta.GuardarUsos(t.Context(), rep, []UsoPersistido{recien})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 0 {
		t.Fatalf("una fila recien parseada no puede salir rechazada: %q", rechazados[0].RechazoMotivo)
	}
	if len(repo.canonicos()) != 1 {
		t.Fatalf("se esperaba 1 uso canonico, hay %d", len(repo.canonicos()))
	}

	guardado := repo.usos[0]
	if guardado.Escalon != "pendiente" {
		t.Errorf("Escalon = %q, se esperaba \"pendiente\"", guardado.Escalon)
	}
	if !guardado.ONI {
		t.Error("a la salida de ingesta ninguna fila esta identificada: ONI tiene que ser true")
	}
	if guardado.Emisiones != 1 {
		t.Errorf("Emisiones = %d, se esperaba 1: el DEFAULT de la columna no llega a aplicarse", guardado.Emisiones)
	}
	if guardado.Fuente != rep.Fuente {
		t.Errorf("Fuente = %q, se esperaba %q: Alias() indexa por fuente", guardado.Fuente, rep.Fuente)
	}
	if guardado.ReporteID != rep.ID {
		t.Errorf("ReporteID = %q, se esperaba %q", guardado.ReporteID, rep.ID)
	}
}

// El motivo que ya trae una fila lo puso el adaptador de formato, que vio lo
// que aqui ya no se ve. Pisarlo con el motivo generico dejaria el log de
// rechazos sin la unica explicacion que sirve para volver a pedirle el dato al
// cliente.
func TestGuardarUsosNoPisaElMotivoQueTraeLaFila(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	delAdaptador := usoBueno("Capitulo con placeholder")
	delAdaptador.RechazoMotivo = "episode_nbr: placeholder \"--\", no se pudo coercionar a entero"

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(), []UsoPersistido{delAdaptador})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 1 {
		t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
	}
	if rechazados[0].RechazoMotivo != delAdaptador.RechazoMotivo {
		t.Fatalf("el motivo se piso: %q", rechazados[0].RechazoMotivo)
	}
	if len(repo.canonicos()) != 0 {
		t.Fatalf("una fila con motivo no es canonica: %+v", repo.canonicos())
	}
}

// El acuse tiene que traer las dos cosas que la fila hereda de el. Que la
// fuente venga vacia no da error en ninguna capa de abajo -`usos.fuente` es
// TEXT NOT NULL sin DEFAULT, y la cadena vacia lo satisface-, asi que si no se
// corta aqui el lote entero queda en la base sin procedencia y Alias() deja de
// casar en silencio. Se corta antes de escribir nada, como en GuardarReporte.
func TestGuardarUsosExigeUnAcuseCompleto(t *testing.T) {
	casos := map[string]Reporte{
		"sin id":     {ID: "", Fuente: "caracol"},
		"sin fuente": {ID: "rep-1", Fuente: ""},
		"en blanco":  {ID: "rep-1", Fuente: "   "},
	}
	for nombre, rep := range casos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			_, err := ingesta.GuardarUsos(t.Context(), rep, []UsoPersistido{usoBueno("La Casa")})
			if !errors.Is(err, ErrReporteInvalido) {
				t.Fatalf("se esperaba ErrReporteInvalido, se obtuvo %v", err)
			}
			if len(repo.usos) != 0 {
				t.Fatalf("no puede quedar fila de un lote rechazado: %+v", repo.usos)
			}
		})
	}
}

// Un motivo de solo blancos no es un motivo, y la diferencia cuesta el lote
// ENTERO.
//
// GuardarUsos lee RechazoMotivo para DOS decisiones distintas y las dos
// comparaban contra la cadena vacia en crudo: si la fila se valida (`== ""`) y
// si la fila va al log de rechazos (`!= ""`). Un blanco no satisface ninguna de
// las dos como toca: la validacion se SALTA y la fila se rutea al log de todas
// formas. Basta con que un adaptador de formato (#25) escriba " ", "\n" o el
// NBSP que los exports de Excel dejan en las celdas vacias.
//
// Lo que pasa a partir de ahi lo decide `usos_rechazados`, y las dos ramas son
// malas. Las dos se comprueban contra la base en
// TestIngestaNoPierdeElLotePorUnBlancoEnUnaFila:
//
//   - Con ESPACIOS, el CHECK sobre btrim(motivo) la rechaza con un 23514 dentro
//     de la transaccion del lote. No se pierde esa fila: se pierden todas. Y el
//     reporte ya quedo escrito, asi que reintentar el mismo archivo choca con
//     ErrReporteDuplicado y la entrega no se recupera sin cirugia en la base.
//   - Con un TABULADOR, un salto de linea o el NBSP no salta nada: `btrim` sin
//     segundo argumento quita SOLO espacios, asi que el motivo pasa el CHECK.
//     Una fila BUENA queda archivada en el log de rechazos con un motivo en
//     blanco -fuera de `usos`, o sea sin ponderar la bolsa- y nadie recibe un
//     error: el acuse dice que el archivo entro completo.
//
// # La asercion que importa es la del VALOR GUARDADO
//
// Que no se rechace lo cumple cualquier TrimSpace puesto en la comparacion de
// turno. Lo que hace falta es que el motivo salga hacia el repositorio como la
// cadena vacia EXACTA, porque el encaminamiento del adaptador -a `usos` o a
// `usos_rechazados`- vuelve a compararlo con `!= ""` y esa comparacion no se
// puede aflojar desde aqui.
func TestGuardarUsosNoRuteaAlLogDeRechazosUnMotivoEnBlanco(t *testing.T) {
	for nombre, blanco := range blancos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			conBlanco := usoBueno("Buena Con Motivo En Blanco")
			conBlanco.RechazoMotivo = blanco

			rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
				[]UsoPersistido{conBlanco, usoBueno("La Casa")})
			if err != nil {
				t.Fatalf("GuardarUsos: %v", err)
			}
			if len(rechazados) != 0 {
				t.Fatalf("una fila buena rechazada por un blanco: motivo %q",
					rechazados[0].RechazoMotivo)
			}
			if len(repo.canonicos()) != 2 {
				t.Fatalf("se esperaban 2 usos canonicos, hay %d: %+v",
					len(repo.canonicos()), repo.canonicos())
			}
			if repo.usos[0].RechazoMotivo != "" {
				t.Errorf(
					"RechazoMotivo = %q, se esperaba la cadena vacia EXACTA: "+
						"el adaptador rutea con `!= \"\"` y el CHECK de la tabla es btrim(motivo) <> ''",
					repo.usos[0].RechazoMotivo)
			}
		})
	}
}

// La contrapartida: recortar el motivo no lo pierde ni afloja el rechazo.
//
// Un motivo de verdad con blancos alrededor -un `\n` de un CSV mal cerrado- es
// un motivo, y la fila sigue yendo al log de rechazos. Se guarda recortado, que
// es la unica forma de que la comparacion de Go y el CHECK de la tabla
// signifiquen lo mismo.
func TestGuardarUsosRecortaSinPerderElMotivoQueTraeLaFila(t *testing.T) {
	const motivo = `episode_nbr: placeholder "--", no se pudo coercionar a entero`

	ingesta, repo, _ := nuevaIngesta()

	delAdaptador := usoBueno("Capitulo con placeholder")
	delAdaptador.RechazoMotivo = "  " + motivo + "\n"

	rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
		[]UsoPersistido{delAdaptador})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 1 {
		t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
	}
	if rechazados[0].RechazoMotivo != motivo {
		t.Fatalf("motivo = %q, se esperaba %q", rechazados[0].RechazoMotivo, motivo)
	}
	if len(repo.canonicos()) != 0 {
		t.Fatalf("una fila con motivo no es canonica: %+v", repo.canonicos())
	}
}

// Un id de solo blancos no es un id, y dos de ellos en el mismo lote chocan
// contra la clave primaria.
//
// La escapatoria `if u.ID == ""` existe para que quien ya tenga un id trazable
// lo conserve. Un " " la esquiva, se salta la derivacion, y llega al INSERT
// como id literal: la segunda fila del lote que traiga el mismo blanco -o la
// segunda entrega, porque los contadores de fila del estilo Id_Ntx se
// renumeran en cada entrega y repiten- choca con un 23505 dentro de la
// transaccion y se lleva el lote entero.
//
// Los ids se comprueban ENTEROS y contra la derivacion esperada: lo que tiene
// que quedar es la entrega y la LINEA exactas de las que salio la fila (ADR
// 0006), no dos ids cualesquiera que resulten distintos.
func TestGuardarUsosDerivaElIDQueLlegaEnBlanco(t *testing.T) {
	for nombre, blanco := range blancos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			rep := repDePrueba()
			primera, segunda := usoBueno("Buena Uno"), usoBueno("Buena Dos")
			primera.ID, segunda.ID = blanco, blanco

			if _, err := ingesta.GuardarUsos(t.Context(), rep,
				[]UsoPersistido{primera, segunda}); err != nil {
				t.Fatalf("GuardarUsos: %v", err)
			}
			if len(repo.usos) != 2 {
				t.Fatalf("se esperaban 2 filas, hay %d", len(repo.usos))
			}
			for n, u := range repo.usos {
				quiero := rep.ID + "-" + strconv.Itoa(n)
				if u.ID != quiero {
					t.Errorf("ID = %q, se esperaba %q: un blanco no es un id",
						u.ID, quiero)
				}
			}
		})
	}
}

// La otra mitad del argumento de N4: `usos.evidencia` tampoco puede mentir.
//
// La decision es que la cascada (ADR 0007) sea el UNICO camino a obra_id, y su
// primera razon es que `evidencia` y `puntaje` dejen de poder describir una
// decision que nunca ocurrio -la pregunta 3 del ADR 0006, "COMO se
// reconocio"-. Con la regla puesta solo sobre obra_id y escalon, una fila con
// `Evidencia: "alias caracol/ID_Ficha=1234"` y el escalon vacio pasa entera:
// el relleno la deja en "pendiente" y la evidencia se escribe VERBATIM en la
// columna. Queda una fila que dice como se reconocio una obra que nadie
// reconocio, y despues no hay forma de distinguirla de una que si.
//
// `puntaje` no necesita regla porque UsoPersistido no tiene ese campo: la
// columna se queda siempre en su DEFAULT 0. El dia que se anada, entra aqui.
//
// El blanco va en la misma tabla y por el mismo motivo que en los demas
// campos: una evidencia de un solo espacio es una celda vacia, no una
// afirmacion, y rechazarla daria un motivo falso.
func TestGuardarUsosRechazaLaEvidenciaQueLaCascadaNoEscribio(t *testing.T) {
	const motivo = "evidencia en la ingesta: como se reconocio lo escribe la cascada (ADR 0007)"

	casos := map[string]struct {
		evidencia string
		motivo    string
	}{
		"de un alias":    {"alias caracol/ID_Ficha=1234", motivo},
		"de un difuso":   {"difuso titulo=0.93", motivo},
		"cadena vacia":   {"", ""},
		"espacio":        {" ", ""},
		"nbsp":           {" ", ""},
		"salto de linea": {"\n", ""},
		"varios blancos": {" \t\r\n ", ""},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			ingesta, repo, _ := nuevaIngesta()

			fila := usoBueno("Dice Como Se Reconocio")
			fila.Evidencia = c.evidencia
			// El escalon vacio es lo que hace el caso REAL: la fila no se
			// delata por el escalon -el relleno la deja en "pendiente"-, solo
			// por la evidencia.
			fila.Escalon = ""

			rechazados, err := ingesta.GuardarUsos(t.Context(), repDePrueba(),
				[]UsoPersistido{fila})
			if err != nil {
				t.Fatalf("GuardarUsos: %v", err)
			}

			if c.motivo == "" {
				if len(rechazados) != 0 {
					t.Fatalf("una evidencia en blanco no es una afirmacion: motivo %q",
						rechazados[0].RechazoMotivo)
				}
				if repo.usos[0].Evidencia != "" {
					t.Errorf("Evidencia = %q, se esperaba la cadena vacia EXACTA",
						repo.usos[0].Evidencia)
				}
				return
			}
			if len(rechazados) != 1 {
				t.Fatalf("se esperaba 1 rechazo, llegaron %d", len(rechazados))
			}
			if rechazados[0].RechazoMotivo != c.motivo {
				t.Fatalf("motivo = %q, se esperaba %q", rechazados[0].RechazoMotivo, c.motivo)
			}
			if len(repo.canonicos()) != 0 {
				t.Fatalf("no puede quedar canonica: %+v", repo.canonicos())
			}
		})
	}
}

// El acuse se valida recortado y se estampa CRUDO, que es la misma clase de
// defecto que el de obra_id un piso mas arriba.
//
// GuardarUsos comprueba `strings.TrimSpace(rep.ID) == ""` y
// `strings.TrimSpace(rep.Fuente) == ""`, y despues copia los dos valores tal
// como vinieron a cada fila del lote. Con un acuse construido a mano -este
// metodo es publico- eso son dos danos distintos:
//
//   - `usos.reporte_id` es REFERENCES reportes(id): " rep-1 " no existe, la
//     clave foranea salta con un 23503 y se lleva la transaccion del lote
//     ENTERA. Ni una fila queda, y ninguna trae motivo que explicarle al
//     cliente.
//   - `usos.fuente` es TEXT NOT NULL y aguanta cualquier cosa, asi que el dano
//     es silencioso: RepositorioIdentificacion.Alias indexa por fuente, y
//     " caracol " no casa con ningun alias NUNCA. El sintoma no es un error,
//     es un catalogo que parece incompleto.
func TestGuardarUsosNormalizaElAcuseQueEstampaEnCadaFila(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	rep := Reporte{ID: "  rep-1\n", Fuente: "\tcaracol "}

	if _, err := ingesta.GuardarUsos(t.Context(), rep,
		[]UsoPersistido{usoBueno("La Casa")}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	guardado := repo.usos[0]
	if guardado.ReporteID != "rep-1" {
		t.Errorf("ReporteID = %q, se esperaba %q: la columna es REFERENCES reportes(id)",
			guardado.ReporteID, "rep-1")
	}
	if guardado.Fuente != "caracol" {
		t.Errorf("Fuente = %q, se esperaba %q: Alias() indexa por fuente",
			guardado.Fuente, "caracol")
	}
	// Y el id derivado cuelga del reporte recortado, no del crudo: si no, la
	// fila no se puede rastrear hasta la entrega.
	if guardado.ID != "rep-1-0" {
		t.Errorf("ID = %q, se esperaba %q", guardado.ID, "rep-1-0")
	}
}

// validarUso reflejaba los CHECK de `usos` pero no la PRECISION de sus
// columnas, y ese es el otro camino por el que una fila revienta el lote.
//
// Las seis medidas son NUMERIC(p,s), y un NUMERIC(p,s) no acepta mas de p-s
// digitos enteros: `rating` es NUMERIC(12,6), asi que su tope es
// 999999.999999. Un rating de 2500000 pasaba la unica comprobacion que habia
// -no es negativo- y moria en el INSERT con
//
//	SQLSTATE 22003 numeric field overflow
//
// dentro de la transaccion del lote: se pierden todas las filas buenas que lo
// acompanan, el operador recibe un SQLSTATE crudo en vez de un motivo por fila,
// y el reporte ya esta escrito, asi que reintentar choca con
// ErrReporteDuplicado.
//
// No es un caso teorico. Los reportes de television colombianos entregan la
// audiencia de las dos formas -como porcentaje y como personas absolutas-, asi
// que un adaptador de formato (#25) que mapee la columna equivocada mete
// millones donde caben seis digitos. Es lo primero que va a pasar cuando entre
// el issue #25.
//
// # Los limites se comprueban justo por debajo Y justo por encima
//
// Una prueba que solo mire un valor absurdo la pasa cualquier tope inventado.
// El par (maximo que cabe, primer valor que no cabe) es lo que fija el tope
// EXACTO de cada columna, que es el que tiene la base.
func TestValidarUsoRechazaLaMedidaQueNoCabeEnLaColumna(t *testing.T) {
	dec := decimal.RequireFromString

	casos := map[string]struct {
		ajustar func(*UsoPersistido)
		motivo  string
	}{
		// duracion_min NUMERIC(12,4): 8 digitos enteros.
		"duracion al tope": {
			func(u *UsoPersistido) { u.DuracionMin = dec("99999999.9999") }, "",
		},
		"duracion pasada": {
			func(u *UsoPersistido) { u.DuracionMin = dec("100000000") },
			"duracion_min 100000000: la columna es NUMERIC(12,4) y no admite mas de 8 digitos enteros",
		},
		// rating NUMERIC(12,6): 6 digitos enteros. El caso del reporte de TV que
		// trae personas absolutas donde se espera un porcentaje.
		"rating al tope": {
			func(u *UsoPersistido) { u.Rating = dec("999999.999999") }, "",
		},
		"rating con personas absolutas": {
			func(u *UsoPersistido) { u.Rating = dec("2500000") },
			"rating 2500000: la columna es NUMERIC(12,6) y no admite mas de 6 digitos enteros",
		},
		// El borde que no se ve sin buscarlo: cabe en 6 digitos enteros TAL
		// COMO viene, y no cabe una vez la base lo redondea a la escala de la
		// columna. Postgres redondea PRIMERO a los 6 decimales -que da
		// 1000000.000000- y comprueba la precision DESPUES.
		"rating que solo desborda al redondear": {
			func(u *UsoPersistido) { u.Rating = dec("999999.9999996") },
			"rating 999999.9999996: la columna es NUMERIC(12,6) y no admite mas de 6 digitos enteros",
		},
		// taquilla y vistas son NUMERIC(18,2): 16 digitos enteros.
		"taquilla al tope": {
			func(u *UsoPersistido) { u.Taquilla = dec("9999999999999999.99") }, "",
		},
		"taquilla pasada": {
			func(u *UsoPersistido) { u.Taquilla = dec("10000000000000000") },
			"taquilla 10000000000000000: la columna es NUMERIC(18,2) y no admite mas de 16 digitos enteros",
		},
		"vistas pasada": {
			func(u *UsoPersistido) { u.Vistas = dec("10000000000000000") },
			"vistas 10000000000000000: la columna es NUMERIC(18,2) y no admite mas de 16 digitos enteros",
		},
		// minutos_vistos y pb son NUMERIC(18,4): 14 digitos enteros.
		"minutos vistos al tope": {
			func(u *UsoPersistido) { u.MinutosVistos = dec("99999999999999.9999") }, "",
		},
		"minutos vistos pasados": {
			func(u *UsoPersistido) { u.MinutosVistos = dec("100000000000000") },
			"minutos_vistos 100000000000000: la columna es NUMERIC(18,4) y no admite mas de 14 digitos enteros",
		},
		"pb pasado": {
			func(u *UsoPersistido) { u.PB = dec("100000000000000") },
			"pb 100000000000000: la columna es NUMERIC(18,4) y no admite mas de 14 digitos enteros",
		},
		// Mas decimales de los que tiene la columna NO es un rechazo: la base
		// redondea a la escala y guarda. Rechazarlo apartaria filas buenas.
		"mas decimales de los que caben": {
			func(u *UsoPersistido) { u.DuracionMin = dec("52.123456789") }, "",
		},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			u := usoBueno("La Casa")
			c.ajustar(&u)

			if motivo := validarUso(u); motivo != c.motivo {
				t.Fatalf("motivo = %q, se esperaba %q", motivo, c.motivo)
			}
		})
	}
}
