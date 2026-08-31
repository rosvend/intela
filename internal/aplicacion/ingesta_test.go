package aplicacion

import (
	"context"
	"errors"
	"regexp"
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

	rechazados, err := ingesta.GuardarUsos(t.Context(), "rep-1", []UsoPersistido{
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

	rechazados, err := ingesta.GuardarUsos(t.Context(), "rep-1",
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

	if _, err := ingesta.GuardarUsos(t.Context(), "rep-1",
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

	rechazados, err := ingesta.GuardarUsos(t.Context(), "rep-1", nil)
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
func TestValidarUsoNombraElCampoQueFalla(t *testing.T) {
	conObra := usoBueno("Identificada")
	conObra.ObraID = "obra-1"
	conObra.ONI = true

	negativo := usoBueno("Negativa")
	negativo.Vistas = decimal.NewFromInt(-1)

	manual := usoBueno("Manual")
	manual.Escalon = "manual"

	casos := map[string]struct {
		uso      UsoPersistido
		nombra   string
		esValido bool
	}{
		"buena":                 {usoBueno("La Casa"), "", true},
		"sin titulo":            {usoBueno("   "), "titulo", false},
		"modalidad desconocida": {func() UsoPersistido { u := usoBueno("X"); u.Modalidad = "radio"; return u }(), "modalidad", false},
		"escalon desconocido":   {func() UsoPersistido { u := usoBueno("X"); u.Escalon = "adivinado"; return u }(), "escalon", false},
		"oni con obra":          {conObra, "oni", false},
		"resuelta sin obra":     {func() UsoPersistido { u := usoBueno("X"); u.ONI = false; return u }(), "obra", false},
		"medida negativa":       {negativo, "vistas", false},
		"emisiones negativas":   {func() UsoPersistido { u := usoBueno("X"); u.Emisiones = -1; return u }(), "emisiones", false},
		"manual por ingesta":    {manual, "manual", false},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			motivo := validarUso(c.uso)
			if c.esValido {
				if motivo != "" {
					t.Fatalf("se esperaba valida, se rechazo con %q", motivo)
				}
				return
			}
			if motivo == "" {
				t.Fatal("se esperaba un motivo de rechazo")
			}
			if !regexp.MustCompile(c.nombra).MatchString(motivo) {
				t.Fatalf("el motivo %q no nombra %q", motivo, c.nombra)
			}
		})
	}
}

// Un escalon vacio es lo que trae una fila recien parseada, y significa
// "pendiente". Rechazarla obligaria a todo adaptador de formato a conocer el
// vocabulario del esquema.
func TestGuardarUsosTrataElEscalonVacioComoPendiente(t *testing.T) {
	ingesta, repo, _ := nuevaIngesta()

	recien := usoBueno("Recien parseada")
	recien.Escalon = ""

	rechazados, err := ingesta.GuardarUsos(t.Context(), "rep-1", []UsoPersistido{recien})
	if err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}
	if len(rechazados) != 0 {
		t.Fatalf("no se esperaba rechazo: %q", rechazados[0].RechazoMotivo)
	}
	if repo.usos[0].Escalon != "pendiente" {
		t.Fatalf("Escalon = %q, se esperaba \"pendiente\"", repo.usos[0].Escalon)
	}
}
