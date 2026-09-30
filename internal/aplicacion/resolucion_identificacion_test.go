package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// ---------------------------------------------------------------------------
// Dobles

// usoEnCola es una fila de `usos` en memoria: lo que la resolucion lee y lo que
// sobreescribe.
//
// Puntaje, ResueltoPor, ResueltoEn y Nota viven aqui y no en UsoPersistido
// porque ese tipo es la proyeccion de columnas del adaptador (columnasUso, que
// no las trae) y el caso de uso no las lee: las escribe.
type usoEnCola struct {
	Uso         UsoPersistido
	Periodo     string
	Candidatos  []identificacion.Candidato
	Nota        string
	Puntaje     decimal.Decimal
	ResueltoPor string
	ResueltoEn  *time.Time
}

// resolucionFalsa es el repositorio en memoria de la resolucion manual.
//
// APUNTA el orden de las llamadas: el cerrojo de periodo tiene que ir antes de
// bloquear la fila (D3), y el reloj tiene que leerse con el cerrojo ya tomado.
// Sin ese registro, invertir las dos primeras llamadas no rompe ninguna otra
// asercion y la prueba pasaria igual.
type resolucionFalsa struct {
	usos  map[string]*usoEnCola
	alias map[string]string // claveAlias -> obraID
	obras map[string]string // obraID -> titulo

	llamadas []string

	// noEscribeAlias emula la carrera del ON CONFLICT DO NOTHING: GuardarAlias
	// no escribe, asi que la relectura devuelve lo que ya hubiera.
	noEscribeAlias bool

	errPeriodo   error
	errBloqueo   error
	errCaso      error
	errTitulo    error
	errAlias     error
	errGuardar   error
	errRelectura error
	errEjemplo   error

	ejemplos map[string]EjemploResolucion
}

func claveAlias(fuente, tipo, valor string) string {
	return fuente + "\x00" + tipo + "\x00" + valor
}

func (f *resolucionFalsa) apuntar(que string) { f.llamadas = append(f.llamadas, que) }

func (f *resolucionFalsa) PeriodoDeUso(_ context.Context, usoID string) (string, error) {
	f.apuntar("periodo")
	if f.errPeriodo != nil {
		return "", f.errPeriodo
	}
	u, hay := f.usos[usoID]
	if !hay {
		return "", ErrNoEncontrado
	}
	return u.Periodo, nil
}

func (f *resolucionFalsa) BloquearPeriodoDeUsos(_ context.Context, periodo string) error {
	f.apuntar("bloquear:" + periodo)
	return f.errBloqueo
}

func (f *resolucionFalsa) CasoParaResolver(_ context.Context, usoID string) (CasoParaResolver, error) {
	f.apuntar("caso")
	if f.errCaso != nil {
		return CasoParaResolver{}, f.errCaso
	}
	u, hay := f.usos[usoID]
	if !hay {
		return CasoParaResolver{}, ErrNoEncontrado
	}
	return CasoParaResolver{Uso: u.Uso, Periodo: u.Periodo, Candidatos: u.Candidatos}, nil
}

func (f *resolucionFalsa) TituloDeObra(_ context.Context, obraID string) (string, error) {
	f.apuntar("titulo")
	if f.errTitulo != nil {
		return "", f.errTitulo
	}
	t, hay := f.obras[obraID]
	if !hay {
		return "", ErrNoEncontrado
	}
	return t, nil
}

func (f *resolucionFalsa) Alias(_ context.Context, fuente, tipo, valor string) (string, error) {
	f.apuntar("alias")
	if f.errAlias != nil {
		return "", f.errAlias
	}
	obraID, hay := f.alias[claveAlias(fuente, tipo, valor)]
	if !hay {
		return "", ErrNoEncontrado
	}
	return obraID, nil
}

func (f *resolucionFalsa) GuardarAlias(_ context.Context, fuente, tipo, valor, obraID, quien string) error {
	f.apuntar("guardar_alias")
	if f.errGuardar != nil {
		return f.errGuardar
	}
	if f.noEscribeAlias {
		return nil
	}
	if _, hay := f.alias[claveAlias(fuente, tipo, valor)]; hay {
		return nil // ON CONFLICT DO NOTHING
	}
	f.alias[claveAlias(fuente, tipo, valor)] = obraID
	f.apuntar("quien:" + quien)
	return nil
}

func (f *resolucionFalsa) GuardarResolucionManual(_ context.Context, r ResolucionManual) error {
	f.apuntar("guardar_resolucion")
	if f.errGuardar != nil {
		return f.errGuardar
	}
	f.aplicar(r)
	return nil
}

func (f *resolucionFalsa) aplicar(r ResolucionManual) {
	u, hay := f.usos[r.UsoID]
	if !hay || u.Uso.Escalon != identificacion.EscalonONI {
		return
	}
	u.Uso.ObraID = r.Resultado.ObraID
	u.Uso.Escalon = r.Resultado.Escalon
	u.Uso.Evidencia = r.Resultado.Evidencia
	u.Puntaje = r.Resultado.Puntaje
	u.Uso.ONI = r.Resultado.ONI
	u.Nota = r.Nota
	u.ResueltoPor, u.ResueltoEn = r.ActorID, &r.Cuando
}

func (f *resolucionFalsa) CasoIdentificacionPorID(_ context.Context, usoID string) (CasoIdentificacion, error) {
	f.apuntar("leer_caso")
	if f.errRelectura != nil {
		return CasoIdentificacion{}, f.errRelectura
	}
	u, hay := f.usos[usoID]
	if !hay {
		return CasoIdentificacion{}, ErrNoEncontrado
	}
	caso := CasoIdentificacion{
		UsoID: u.Uso.ID, Titulo: u.Uso.Titulo, TituloOriginal: u.Uso.TituloOrig,
		Fuente: u.Uso.Fuente, ReporteID: u.Uso.ReporteID, Periodo: u.Periodo,
		IDsFuente: u.Uso.IDsFuente, Evidencia: u.Uso.Evidencia, Escalon: u.Uso.Escalon,
		Nota: u.Nota, ReporteCreado: instanteDePrueba, Candidatos: []CandidatoCaso{},
	}
	if u.Uso.ObraID != "" {
		caso.ObraAsignada = &ObraAsignada{ID: u.Uso.ObraID, Titulo: f.obras[u.Uso.ObraID]}
	}
	if u.ResueltoPor != "" {
		caso.ResueltoPor = &Resolutor{ID: u.ResueltoPor}
		caso.ResueltoEn = u.ResueltoEn
	}
	return caso, nil
}

func (f *resolucionFalsa) HistoriaPorClaves(_ context.Context, claves []string) ([]identificacion.EjemploEtiquetado, error) {
	if f.errEjemplo != nil {
		return nil, f.errEjemplo
	}
	quiere := map[string]bool{}
	for _, c := range claves {
		quiere[c] = true
	}
	var out []identificacion.EjemploEtiquetado
	for _, e := range f.ejemplos {
		if len(quiere) > 0 && !quiere[e.Clave] {
			continue
		}
		if len(quiere) == 0 {
			continue
		}
		out = append(out, identificacion.EjemploEtiquetado{
			Clave: e.Clave, Decision: e.Decision, ObraID: e.ObraElegida,
		})
	}
	return out, nil
}

func (f *resolucionFalsa) GuardarEjemplo(_ context.Context, e EjemploResolucion) error {
	if f.errEjemplo != nil {
		return f.errEjemplo
	}
	if f.ejemplos == nil {
		f.ejemplos = map[string]EjemploResolucion{}
	}
	f.ejemplos[e.UsoID] = e
	return nil
}

func (f *resolucionFalsa) EjemplosDe(_ context.Context, usoIDs []string) (map[string]EjemploGuardado, error) {
	out := map[string]EjemploGuardado{}
	for _, id := range usoIDs {
		e, hay := f.ejemplos[id]
		if !hay {
			continue
		}
		out[id] = EjemploGuardado{
			SugerenciaDecision: e.SugerenciaDecision,
			SugerenciaObraID:   e.SugerenciaObraID,
			Confianza:          e.Confianza,
			Motivo:             e.Motivo,
			Orden:              e.Orden,
			Aceptada:           e.Aceptada,
		}
	}
	return out, nil
}

// instantanea copia el estado para que la unidad pueda revertirlo.
func (f *resolucionFalsa) instantanea() map[string]usoEnCola {
	copia := make(map[string]usoEnCola, len(f.usos))
	for id, u := range f.usos {
		copia[id] = *u
	}
	return copia
}

func (f *resolucionFalsa) restaurar(s map[string]usoEnCola) {
	for id := range f.usos {
		if v, hay := s[id]; hay {
			copia := v
			f.usos[id] = &copia
		} else {
			delete(f.usos, id)
		}
	}
}

// unidadConRollback emula la transaccion: si fn falla, el doble vuelve a como
// estaba. Es lo unico que puede comprobar "el asiento falla, asi que NADA esta
// hecho" sin Postgres.
type unidadConRollback struct {
	repo     *resolucionFalsa
	alias    map[string]string
	entradas int
	confirmo bool
}

func (u *unidadConRollback) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	u.entradas++
	antes, aliasAntes, ejemplosAntes := u.repo.instantanea(), copiarAlias(u.repo.alias), copiarEjemplos(u.repo.ejemplos)
	if err := fn(ctx); err != nil {
		u.repo.restaurar(antes)
		u.repo.alias = aliasAntes
		u.repo.ejemplos = ejemplosAntes
		return err
	}
	u.confirmo = true
	return nil
}

func copiarAlias(m map[string]string) map[string]string {
	copia := make(map[string]string, len(m))
	for k, v := range m {
		copia[k] = v
	}
	return copia
}

func copiarEjemplos(m map[string]EjemploResolucion) map[string]EjemploResolucion {
	copia := make(map[string]EjemploResolucion, len(m))
	for k, v := range m {
		copia[k] = v
	}
	return copia
}

// relojEspia cuenta las lecturas: el instante se lee UNA vez y con el cerrojo
// tomado.
type relojEspia struct {
	instante time.Time
	lecturas int
}

func (r *relojEspia) Ahora() time.Time {
	r.lecturas++
	return r.instante
}

// ---------------------------------------------------------------------------
// Fixture

const (
	actorResolucion = "usr-admin"
	nombreActor     = "Ana Perez"
	usoDePrueba     = "uso-1"
	periodoUno      = "2024-11"
)

// servicioDeResolucion cablea los cuatro puertos como lo hace cmd/api.
func servicioDeResolucion(repo *resolucionFalsa, libro *bitacoraFalsa, reloj Reloj) (ResolucionIdentificacion, *unidadConRollback) {
	unidad := &unidadConRollback{repo: repo, alias: repo.alias}
	return ResolucionIdentificacion{
		Repo: repo, Bitacora: libro, Unidad: unidad, Reloj: reloj,
		Ejemplos: repo, Rankeador: rankeadorDeDominio{},
	}, unidad
}

// repoDePrueba siembra un caso ONI con dos candidatos y un alias que no existe.
func repoDePrueba() *resolucionFalsa {
	return &resolucionFalsa{
		usos: map[string]*usoEnCola{
			usoDePrueba: {
				Periodo: periodoUno,
				Uso: UsoPersistido{
					ID: usoDePrueba, ReporteID: "rep-1", Fuente: "caracol",
					Titulo: "La Nina T3 E12", IDsFuente: "id_ficha=871732",
					Escalon: identificacion.EscalonONI, ONI: true,
					Evidencia: "banda ambigua: 2 candidatos, mejor obra-12 (0.52941)",
				},
				Candidatos: []identificacion.Candidato{
					{ObraID: "obra-12", Puntaje: decimal.RequireFromString("0.52941")},
					{ObraID: "obra-40", Puntaje: decimal.RequireFromString("0.41000")},
				},
			},
		},
		alias:    map[string]string{},
		obras:    map[string]string{"obra-12": "La Nina T3", "obra-40": "Otra obra"},
		ejemplos: map[string]EjemploResolucion{},
	}
}

func solicitudAsignar(obraID string) SolicitudResolucion {
	return SolicitudResolucion{UsoID: usoDePrueba, Decision: "asignar", ObraID: obraID, Nota: "  coincide la ficha  "}
}

func solicitudDescartar() SolicitudResolucion {
	return SolicitudResolucion{UsoID: usoDePrueba, Decision: "descartar", Nota: "no es del repertorio"}
}

// payloadDe vuelve a leer el payload del unico asiento asentado.
func payloadDe(t *testing.T, libro *bitacoraFalsa) map[string]any {
	t.Helper()
	if len(libro.asientos) != 1 {
		t.Fatalf("asientos = %d, se esperaba exactamente uno", len(libro.asientos))
	}
	var out map[string]any
	if err := json.Unmarshal(libro.asientos[0].Payload, &out); err != nil {
		t.Fatalf("payload del asiento no es JSON: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Asignar

func TestResolverAsignaAUnaCandidata(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	reloj := &relojEspia{instante: instanteDePrueba}
	svc, _ := servicioDeResolucion(repo, libro, reloj)

	caso, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	fila := repo.usos[usoDePrueba]
	if fila.Uso.Escalon != identificacion.EscalonManual || fila.Uso.ObraID != "obra-12" || fila.Uso.ONI {
		t.Fatalf("la fila quedo %+v", fila.Uso)
	}
	if fila.ResueltoPor != actorResolucion || fila.ResueltoEn == nil || !fila.ResueltoEn.Equal(instanteDePrueba) {
		t.Fatalf("la fila no quedo firmada: %+v", fila.Uso)
	}
	if fila.Nota != "coincide la ficha" {
		t.Fatalf("la nota guardada es %q, se esperaba recortada", fila.Nota)
	}
	if fila.Puntaje.String() != "0.52941" {
		t.Fatalf("el puntaje del candidato no se conservo: %s", fila.Puntaje)
	}

	// El alias se aprende con quien = el actor: es lo que hace que el escalon 1
	// resuelva el siguiente reporte sin pasar por la cola (ADR 0007).
	if got := repo.alias[claveAlias("caracol", "id_ficha", "871732")]; got != "obra-12" {
		t.Fatalf("alias = %q, se esperaba obra-12", got)
	}
	if !contiene(repo.llamadas, "quien:"+actorResolucion) {
		t.Fatalf("el alias no se aprendio a nombre del actor: %v", repo.llamadas)
	}

	if caso.Estado != EstadoCasoAsignado || caso.ObraAsignada == nil || caso.ObraAsignada.ID != "obra-12" {
		t.Fatalf("caso devuelto = %+v", caso)
	}
	if caso.Nota != "coincide la ficha" {
		t.Fatalf("la respuesta no trae la nota: %+v", caso)
	}

	a := libro.asientos[0]
	if a.Hecho != HechoIdentificacionAsignada || a.RefTipo != RefObra || a.RefID != "obra-12" {
		t.Fatalf("asiento = %q sobre %s/%s", a.Hecho, a.RefTipo, a.RefID)
	}
	if a.ActorID != actorResolucion || !a.Cuando.Equal(instanteDePrueba) {
		t.Fatalf("el asiento no va firmado: %+v", a)
	}

	p := payloadDe(t, libro)
	if p["uso_id"] != usoDePrueba || p["decision"] != "asignar" || p["obra_id"] != "obra-12" {
		t.Fatalf("payload = %v", p)
	}
	if p["candidata"] != true || p["puntaje"] != "0.52941" || p["candidatos_propuestos"] != float64(2) {
		t.Fatalf("el payload no dice que salio de una candidata: %v", p)
	}
	// El estado ANTERIOR, que la fila sobreescribe: sin esto no queda en ningun sitio.
	if p["escalon_anterior"] != identificacion.EscalonONI {
		t.Fatalf("el payload no guarda el escalon anterior: %v", p)
	}
	if p["evidencia_anterior"] != "banda ambigua: 2 candidatos, mejor obra-12 (0.52941)" {
		t.Fatalf("el payload no guarda la evidencia anterior: %v", p)
	}
	if p["titulo"] != "La Nina T3 E12" || p["fuente"] != "caracol" ||
		p["periodo"] != periodoUno || p["reporte_id"] != "rep-1" || p["ids_fuente"] != "id_ficha=871732" {
		t.Fatalf("al payload le falta el estado del uso al decidir: %v", p)
	}
	if p["nota"] != "coincide la ficha" || p["actor_nombre"] != nombreActor {
		t.Fatalf("el payload no lleva nota o nombre del actor: %v", p)
	}
	alias, ok := p["alias"].(map[string]any)
	if !ok || alias["fuente"] != "caracol" || alias["tipo_id"] != "id_ficha" ||
		alias["valor"] != "871732" || alias["aprendido"] != true {
		t.Fatalf("payload alias = %v", p["alias"])
	}
}

// Una obra que el motor no propuso: el asiento lo dice y no hereda puntaje.
func TestResolverAsignaAUnaObraBuscada(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	caso, err := svc.Resolver(t.Context(), solicitudAsignar("obra-40x"), actorResolucion, nombreActor)
	if err == nil {
		t.Fatalf("una obra fuera del catalogo tiene que fallar: %+v", caso)
	}
	if !errors.Is(err, ErrObraInexistente) {
		t.Fatalf("error = %v, se esperaba ErrObraInexistente", err)
	}

	// Ahora una que SI esta en el catalogo pero no era candidata.
	repo.obras["obra-40x"] = "Buscada a mano"
	caso, err = svc.Resolver(t.Context(), solicitudAsignar("obra-40x"), actorResolucion, nombreActor)
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if caso.Estado != EstadoCasoAsignado {
		t.Fatalf("estado = %q", caso.Estado)
	}
	p := payloadDe(t, libro)
	if p["candidata"] != false {
		t.Fatalf("el payload tiene que decir que no era candidata: %v", p)
	}
	if _, hay := p["puntaje"]; hay {
		t.Fatalf("una obra buscada no tiene puntaje que heredar: %v", p)
	}
}

// ---------------------------------------------------------------------------
// Descartar

func TestResolverDescarta(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	caso, err := svc.Resolver(t.Context(), solicitudDescartar(), actorResolucion, nombreActor)
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	fila := repo.usos[usoDePrueba]
	if fila.Uso.Escalon != identificacion.EscalonDescartado || fila.Uso.ObraID != "" || fila.Uso.ONI {
		t.Fatalf("la fila quedo %+v: un descartado no tiene obra y no es ONI", fila.Uso)
	}
	if fila.ResueltoPor != actorResolucion || fila.ResueltoEn == nil || fila.Nota == "" {
		t.Fatalf("un descarte va firmado y con nota: %+v", fila)
	}

	// Descartar NO aprende alias: no hay obra a la que apuntar.
	if len(repo.alias) != 0 {
		t.Fatalf("descartar aprendio un alias: %v", repo.alias)
	}
	if contiene(repo.llamadas, "alias") || contiene(repo.llamadas, "guardar_alias") {
		t.Fatalf("descartar no debe tocar el alias: %v", repo.llamadas)
	}

	if caso.Estado != EstadoCasoDescartado || caso.ObraAsignada != nil {
		t.Fatalf("caso devuelto = %+v", caso)
	}

	a := libro.asientos[0]
	if a.Hecho != HechoIdentificacionDescartada || a.RefTipo != RefUso || a.RefID != usoDePrueba {
		t.Fatalf("asiento = %q sobre %s/%s", a.Hecho, a.RefTipo, a.RefID)
	}

	p := payloadDe(t, libro)
	if p["decision"] != "descartar" {
		t.Fatalf("payload = %v", p)
	}
	for _, ausente := range []string{"obra_id", "candidata", "puntaje", "candidatos_propuestos"} {
		if _, hay := p[ausente]; hay {
			t.Errorf("un descarte no lleva %q en el payload: %v", ausente, p)
		}
	}
	if v, hay := p["alias"]; !hay || v != nil {
		t.Fatalf("el payload de un descarte lleva \"alias\": null, tiene %v", p["alias"])
	}
}

// ---------------------------------------------------------------------------
// Alias

func TestResolverAlias(t *testing.T) {
	t.Run("ya apunta a la misma obra: no se escribe y el payload lo dice", func(t *testing.T) {
		repo, libro := repoDePrueba(), &bitacoraFalsa{}
		repo.alias[claveAlias("caracol", "id_ficha", "871732")] = "obra-12"
		svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

		if _, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor); err != nil {
			t.Fatalf("Resolver: %v", err)
		}
		if contiene(repo.llamadas, "guardar_alias") {
			t.Fatalf("no habia nada que aprender: %v", repo.llamadas)
		}
		alias, _ := payloadDe(t, libro)["alias"].(map[string]any)
		if alias["aprendido"] != false {
			t.Fatalf("el payload tiene que decir aprendido=false: %v", alias)
		}
	})

	t.Run("ya apunta a otra obra: conflicto y nada escrito", func(t *testing.T) {
		repo, libro := repoDePrueba(), &bitacoraFalsa{}
		repo.alias[claveAlias("caracol", "id_ficha", "871732")] = "obra-40"
		svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

		_, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
		if !errors.Is(err, ErrAliasEnConflicto) {
			t.Fatalf("error = %v, se esperaba ErrAliasEnConflicto", err)
		}
		if !strings.Contains(err.Error(), "obra-40") {
			t.Errorf("el mensaje tiene que nombrar la obra del alias: %v", err)
		}
		if repo.usos[usoDePrueba].Uso.Escalon != identificacion.EscalonONI {
			t.Fatalf("el conflicto se rechaza ANTES de escribir: %+v", repo.usos[usoDePrueba].Uso)
		}
		if len(libro.asientos) != 0 {
			t.Fatalf("un conflicto no deja asiento: %v", libro.asientos)
		}
	})

	t.Run("carrera: GuardarAlias no escribe y la relectura da otra obra", func(t *testing.T) {
		repo, libro := repoDePrueba(), &bitacoraFalsa{}
		// Otra peticion gano: el alias ya existe, pero la primera lectura no lo
		// vio porque corrio antes.
		repo.alias[claveAlias("caracol", "id_ficha", "871732")] = "obra-34"
		repo.noEscribeAlias = true
		svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

		_, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
		if !errors.Is(err, ErrAliasEnConflicto) {
			t.Fatalf("error = %v, se esperaba ErrAliasEnConflicto", err)
		}
		if repo.usos[usoDePrueba].Uso.Escalon != identificacion.EscalonONI || len(libro.asientos) != 0 {
			t.Fatalf("la carrera no puede dejar nada escrito: %+v", repo.usos[usoDePrueba].Uso)
		}
	})

	t.Run("un fallo de base no es 'no hay alias'", func(t *testing.T) {
		repo, libro := repoDePrueba(), &bitacoraFalsa{}
		repo.errAlias = errors.New("se cayo la base")
		svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

		_, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
		if !strings.Contains(err.Error(), "se cayo la base") {
			t.Fatalf("el error de base tiene que subir: %v", err)
		}
	})

	t.Run("uso sin par local: no hay alias y el payload lo dice", func(t *testing.T) {
		repo, libro := repoDePrueba(), &bitacoraFalsa{}
		repo.usos[usoDePrueba].Uso.IDsFuente = ""
		svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

		if _, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor); err != nil {
			t.Fatalf("Resolver: %v", err)
		}
		if contiene(repo.llamadas, "alias") {
			t.Fatalf("sin par local no se sondea el alias: %v", repo.llamadas)
		}
		if v, hay := payloadDe(t, libro)["alias"]; !hay || v != nil {
			t.Fatalf("alias = %v, se esperaba null", v)
		}
	})
}

// ---------------------------------------------------------------------------
// Validacion temprana

func TestResolverValidaAntesDeTocarLaBase(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*SolicitudResolucion)
		actor  string
		quiero error
	}{
		{"nota vacia", func(s *SolicitudResolucion) { s.Nota = "" }, actorResolucion, identificacion.ErrNotaVacia},
		{"nota de solo espacios", func(s *SolicitudResolucion) { s.Nota = "   " }, actorResolucion, identificacion.ErrNotaVacia},
		{
			"nota de 301 runas", func(s *SolicitudResolucion) { s.Nota = strings.Repeat("ñ", 301) },
			actorResolucion, identificacion.ErrNotaDemasiadoLarga,
		},
		{"actor vacio", func(*SolicitudResolucion) {}, "", ErrActorAusente},
		{"actor de solo espacios", func(*SolicitudResolucion) {}, "   ", ErrActorAusente},
		{"decision invalida", func(s *SolicitudResolucion) { s.Decision = "reasignar" }, actorResolucion, identificacion.ErrDecisionInvalida},
		{"decision vacia", func(s *SolicitudResolucion) { s.Decision = "" }, actorResolucion, identificacion.ErrDecisionInvalida},
		{"asignar sin obra", func(s *SolicitudResolucion) { s.ObraID = "" }, actorResolucion, identificacion.ErrDecisionInvalida},
		{"asignar con obra de solo espacios", func(s *SolicitudResolucion) { s.ObraID = "  " }, actorResolucion, identificacion.ErrDecisionInvalida},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo, libro := repoDePrueba(), &bitacoraFalsa{}
			svc, unidad := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})
			s := solicitudAsignar("obra-12")
			c.mutar(&s)

			_, err := svc.Resolver(t.Context(), s, c.actor, nombreActor)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiero)
			}
			if len(repo.llamadas) != 0 {
				t.Errorf("se toco el repositorio con un pedido invalido: %v", repo.llamadas)
			}
			if unidad.entradas != 0 {
				t.Errorf("se abrio una unidad con un pedido invalido")
			}
		})
	}
}

func TestResolverDescartarConObraEsInvalido(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	s := solicitudDescartar()
	s.ObraID = "obra-12"
	if _, err := svc.Resolver(t.Context(), s, actorResolucion, nombreActor); !errors.Is(err, identificacion.ErrDecisionInvalida) {
		t.Fatalf("error = %v, se esperaba ErrDecisionInvalida", err)
	}
}

// ---------------------------------------------------------------------------
// Estado del caso

func TestResolverEstadoDelCaso(t *testing.T) {
	casos := []struct {
		nombre  string
		escalon string
		quiero  error
	}{
		{"ya resuelto a mano", identificacion.EscalonManual, identificacion.ErrCasoYaResuelto},
		{"ya descartado", identificacion.EscalonDescartado, identificacion.ErrCasoYaResuelto},
		{"un pendiente que la cascada no corrio", identificacion.EscalonPendiente, identificacion.ErrCasoNoPendiente},
		{"ya resuelto por alias", identificacion.EscalonAlias, identificacion.ErrCasoNoPendiente},
		{"excluido", identificacion.EscalonExcluido, identificacion.ErrCasoNoPendiente},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo, libro := repoDePrueba(), &bitacoraFalsa{}
			repo.usos[usoDePrueba].Uso.Escalon = c.escalon
			svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

			_, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiero)
			}
			if len(libro.asientos) != 0 {
				t.Fatalf("un caso no resoluble no deja asiento")
			}
		})
	}
}

func TestResolverCasoDesconocidoEsNoEncontrado(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	s := solicitudAsignar("obra-12")
	s.UsoID = "uso-que-no-existe"
	_, err := svc.Resolver(t.Context(), s, actorResolucion, nombreActor)
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}

// ---------------------------------------------------------------------------
// El asiento es parte de "hecho" (ADR 0006)

func TestResolverSiElAsientoFallaNoQuedaNadaHecho(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{err: errors.New("la bitacora no esta")}
	svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	_, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
	if err == nil {
		t.Fatal("un asiento que falla tiene que hacer fallar el caso de uso")
	}

	fila := repo.usos[usoDePrueba]
	if fila.Uso.Escalon != identificacion.EscalonONI || fila.Uso.ObraID != "" || fila.Nota != "" ||
		fila.ResueltoPor != "" || fila.ResueltoEn != nil {
		t.Fatalf("la fila no se revirtio: %+v", fila.Uso)
	}
	if _, hay := repo.alias[claveAlias("caracol", "id_ficha", "871732")]; hay {
		t.Fatalf("el alias no se revirtio: %v", repo.alias)
	}
	if len(repo.ejemplos) != 0 {
		t.Fatalf("el ejemplo etiquetado no se revirtio: %+v", repo.ejemplos)
	}
}

// ---------------------------------------------------------------------------
// Orden y cableado

func TestResolverTomaElCerrojoAntesDeBloquearLaFila(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	reloj := &relojEspia{instante: instanteDePrueba}
	svc, _ := servicioDeResolucion(repo, libro, reloj)

	if _, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor); err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	quiero := []string{
		"periodo", "bloquear:" + periodoUno, "caso", "titulo",
		"alias", "guardar_alias", "quien:" + actorResolucion, "alias",
		"guardar_resolucion", "leer_caso",
	}
	for i, paso := range quiero {
		if i >= len(repo.llamadas) || repo.llamadas[i] != paso {
			t.Fatalf("orden de llamadas = %v, se esperaba empezar por %v", repo.llamadas, quiero)
		}
	}
	if reloj.lecturas != 1 {
		t.Fatalf("el reloj se leyo %d veces, se esperaba una", reloj.lecturas)
	}
}

func TestResolverMalCableadoNoAbreUnidad(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	completo, unidad := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	casos := map[string]ResolucionIdentificacion{
		"sin RepositorioResolucionIdentificacion": {Bitacora: libro, Unidad: unidad, Reloj: relojFijo{}},
		"sin BitacoraAuditoria":                   {Repo: repo, Unidad: unidad, Reloj: relojFijo{}},
		"sin UnidadDeTrabajo":                     {Repo: repo, Bitacora: libro, Reloj: relojFijo{}},
		"sin Reloj":                               {Repo: repo, Bitacora: libro, Unidad: unidad},
		"sin RepositorioEjemplosResolucion": {
			Repo: repo, Bitacora: libro, Unidad: unidad, Reloj: relojFijo{}, Rankeador: rankeadorDeDominio{},
		},
		"sin PuertoRankeadorDeResoluciones": {
			Repo: repo, Bitacora: libro, Unidad: unidad, Reloj: relojFijo{}, Ejemplos: repo,
		},
	}
	for nombre, svc := range casos {
		t.Run(nombre, func(t *testing.T) {
			antes := unidad.entradas
			if _, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor); err == nil {
				t.Fatal("un servicio a medias tiene que fallar")
			}
			if unidad.entradas != antes {
				t.Fatalf("se abrio una unidad con el servicio mal cableado")
			}
		})
	}

	if _, err := completo.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor); err != nil {
		t.Fatalf("el servicio completo tiene que funcionar: %v", err)
	}
}

func contiene(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
