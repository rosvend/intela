package ingesta

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/normalizacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// KR-1: un lote de 10.000 registros se procesa en menos de 5 minutos.
//
// El lote recorre el pipeline CPU en memoria, de punta a punta y sin base de
// datos: parseo CSV del adaptador (Lector + MapaCaracol), normalizacion al
// esquema canonico, cascada de identificacion contra un padron en memoria, y
// motor de reparto. Lo que NO mide, a proposito: los roundtrips a Postgres
// del caso de uso (eso lo cubren las pruebas de integracion con
// testcontainers) ni la red. Si este test se pone lento, el culpable es
// algoritmo, no infraestructura.
//
// Los datos son sinteticos y deterministas -sin `rand`, que el ADR 0005
// prohibe en el nucleo-: 200 obras con 50 emisiones cada una. Los titulos no
// son de ningun canal real.
//
// Corre en CI dos veces con propositos distintos: `test-go` lo ejecuta CON
// `-race` (correccion bajo detector de carreras) y la etapa dedicada
// `perf-10k` SIN el (el KR habla de tiempo de pared sin instrumentacion).
const (
	filasLote10k  = 10_000
	obrasLote10k  = 200
	presupuestoKR = 5 * time.Minute
)

var tiposObraLote = []string{"serie", "cinematografica", "unitario", "telenovela", "sketches"}

// dentroDePresupuesto dice si una duracion ya medida cabe en el tope. Separada
// del `time.Since` para probarla sin esperar: el tiempo se mide una vez, la
// comparacion es pura.
func dentroDePresupuesto(transcurrido, tope time.Duration) bool {
	return transcurrido < tope
}

func TestDentroDePresupuesto(t *testing.T) {
	if !dentroDePresupuesto(time.Second, presupuestoKR) {
		t.Fatal("un segundo cabe en el presupuesto de la KR-1")
	}
	if dentroDePresupuesto(presupuestoKR, presupuestoKR) {
		t.Fatal("el tope exacto ya no cabe: el presupuesto es estricto")
	}
	if dentroDePresupuesto(6*time.Minute, presupuestoKR) {
		t.Fatal("seis minutos exceden el presupuesto de la KR-1")
	}
}

// csvLote genera la parrilla sintetica con las columnas que pide MapaCaracol.
// La clave de registro (ID_Ficha+Fecha+Hora) es unica por construccion: con
// 50 emisiones por obra y 28 dias x 24 horas, el par (dia, hora) no se repite
// dentro de una obra (mcm(28,24)=168 > 50) y el adaptador no rechaza
// duplicados que el lote no tiene.
func csvLote() []byte {
	var b strings.Builder
	b.WriteString("Canal,Titulo,Titulo_original,TIPO,ID_Ficha,Fecha,Hora,Duracion_total\n")
	for i := range filasLote10k {
		obra := i % obrasLote10k
		k := i / obrasLote10k
		fmt.Fprintf(&b, "CARACOL,Lote Diez Mil %03d,Lote Diez Mil %03d,SE,F%04d,202501%02d,%02d:00,%d\n",
			obra+1, obra+1, obra+1, 1+(k%28), k%24, 30+(i%60))
	}
	return []byte(b.String())
}

// filaDeLote lleva un UsoPersistido del adaptador a la fila cruda que pide
// normalizacion. Espeja a `aFila` de aplicacion (no exportada): el dia que el
// caso de uso exponga la conversion, esta copia sobra.
//
// TipoObra, Emisiones y Rating son constantes sinteticas, no columnas del
// mapa: la parrilla de Caracol no las trae. Sin magnitud distinta de cero el
// motor no ejercitaria la aritmetica real (puntos cero), asi que se fijan
// aqui y se documentan como lo que son: insumo de la prueba, no del cliente.
func filaDeLote(u aplicacion.UsoPersistido, tipoObra string) normalizacion.Fila {
	duracion := u.DuracionTexto
	if duracion == "" {
		duracion = u.DuracionMin.String()
	}
	return normalizacion.Fila{
		ID:             u.ID,
		Fuente:         u.Fuente,
		Modalidad:      string(u.Modalidad),
		Titulo:         u.Titulo,
		TituloOrig:     u.TituloOrig,
		IDsFuente:      u.IDsFuente,
		TipoObra:       tipoObra,
		Fecha:          u.Fecha,
		Hora:           u.Hora,
		Duracion:       duracion,
		Emisiones:      "2",
		Rating:         "5.0",
		UnidadDuracion: u.UnidadDuracion,
		CanalID:        "z",
	}
}

// idFicha saca el valor de `id_ficha=` del campo multiclave (ADR 0018: una
// linea `clave=valor` por identificador).
func idFicha(ids string) string {
	for _, linea := range strings.Split(ids, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(linea), "id_ficha="); ok {
			return v
		}
	}
	return ""
}

func TestLote10kEnMenosDe5Min(t *testing.T) {
	inicio := time.Now()

	lector, err := NuevoLector(MapaCaracol(), aplicacion.FormatoCSV)
	if err != nil {
		t.Fatal(err)
	}
	filas, err := lector.Leer(csvLote())
	if err != nil {
		t.Fatal(err)
	}
	if len(filas) != filasLote10k {
		t.Fatalf("filas = %d, se esperaban %d", len(filas), filasLote10k)
	}
	for i, f := range filas {
		if f.RechazoMotivo != "" {
			t.Fatalf("fila %d rechazada: %s", i, f.RechazoMotivo)
		}
	}

	parametros := normalizacion.Parametros{
		DuracionArtisticaPct: decimal.RequireFromString("80"),
		MinutosHoraTV:        decimal.RequireFromString("48"),
	}
	alias := make(map[string]string, obrasLote10k)
	for o := range obrasLote10k {
		alias[fmt.Sprintf("F%04d", o+1)] = fmt.Sprintf("obra-%04d", o+1)
	}
	tipos := make(map[string]string, obrasLote10k)
	for o := range obrasLote10k {
		tipos[fmt.Sprintf("obra-%04d", o+1)] = tiposObraLote[o%len(tiposObraLote)]
	}

	usos := make([]reparto.Uso, 0, filasLote10k)
	for i, f := range filas {
		uso, rev := normalizacion.Normalizar(filaDeLote(f, tipos[alias[idFicha(f.IDsFuente)]]), parametros)
		if rev != nil {
			t.Fatalf("fila %d a revision: %s %s", i, rev.Codigo, rev.Detalle)
		}
		res := identificacion.Resolver(identificacion.Entrada{
			Fuente:     uso.Fuente,
			Titulo:     uso.Titulo,
			TituloOrig: uso.TituloOrig,
			TipoID:     "id_ficha",
			ValorID:    idFicha(uso.IDsFuente),
		}, identificacion.Consulta{AliasObraID: alias[idFicha(uso.IDsFuente)]}, nil,
			// Todas las filas entran por el escalon 1 (alias): los umbrales del
			// escalon 3 no se usan, pero Resolver los pide (#32).
			identificacion.Umbrales{Match: d10k("0.60"), Banda: d10k("0.45")})
		if res.ObraID == "" || res.ONI {
			t.Fatalf("fila %d sin identificar: %+v", i, res)
		}
		usos = append(usos, reparto.Uso{
			ObraID:      res.ObraID,
			Modalidad:   reparto.TV,
			TipoObra:    uso.TipoObra,
			CanalID:     uso.CanalID,
			DuracionMin: uso.DuracionMin,
			Emisiones:   uso.Emisiones,
			Rating:      uso.Rating,
		})
	}

	decls := make([]repertorio.Declaracion, 0, obrasLote10k)
	for o := range obrasLote10k {
		obra := fmt.Sprintf("obra-%04d", o+1)
		titular := fmt.Sprintf("tit-%04d", o+1)
		dd, err := repertorio.NuevaDeclaracion(obra, []repertorio.Parte{
			{TitularID: titular, IPI: fmt.Sprintf("IPI-%08d", o+1), Porcentaje: decimal.RequireFromString("100")},
		})
		if err != nil {
			t.Fatal(err)
		}
		decls = append(decls, dd)
	}
	bolsa, err := recaudo.NuevaBolsa("u1", "2025", recaudo.Nacional, decimal.RequireFromString("1000000000"))
	if err != nil {
		t.Fatal(err)
	}
	snap := reparto.Snapshot{
		AdminPct: d10k("20"), SocialPct: d10k("10"), ReservaPct: d10k("5"),
		PondCine: d10k("5.0"), PondUnitario: d10k("2.8"), PondSerie: d10k("1.3"),
		PondSketch: d10k("0.8"), Wa: d10k("0.5"), Wb: d10k("0.3"), Wc: d10k("0.2"),
		BaseCineTeatro: reparto.BaseEspectadores, Reglamento: "RD-IX",
		DuracionArtisticaPct: decimal.RequireFromString("80"),
		MinutosHoraTV:        decimal.RequireFromString("48"),
	}
	r, err := reparto.Reparto(bolsa, usos, snap, decls, reparto.Opciones{SinDeducciones: true})
	if err != nil {
		t.Fatal(err)
	}
	// Cierre: lo repartido mas lo retenido, el residuo y lo no distribuido
	// tiene que igualar el neto. Un lote que no cierra no es rapido: esta mal.
	sumaTitulares := decimal.Zero
	for _, tt := range r.Titulares {
		sumaTitulares = sumaTitulares.Add(tt.Importe)
	}
	if got := sumaTitulares.Add(r.Retenido).Add(r.Residuo).Add(r.NoDistribuido); !got.Equal(r.Neto) {
		t.Fatalf("el lote no cierra: %s != neto %s", got, r.Neto)
	}
	if len(r.Titulares) != obrasLote10k {
		t.Fatalf("titulares = %d, se esperaba uno por obra (%d)", len(r.Titulares), obrasLote10k)
	}

	transcurrido := time.Since(inicio)
	t.Logf("lote de %d registros en %s (%.0f filas/s)", filasLote10k, transcurrido,
		float64(filasLote10k)/transcurrido.Seconds())
	if !dentroDePresupuesto(transcurrido, presupuestoKR) {
		t.Fatalf("el lote tardo %s, mas que el presupuesto KR-1 de %s", transcurrido, presupuestoKR)
	}
}

func d10k(s string) decimal.Decimal { return decimal.RequireFromString(s) }
