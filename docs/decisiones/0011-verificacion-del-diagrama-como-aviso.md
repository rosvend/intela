# 0011 La verificacion del diagrama avisa, no bloquea

Fecha: 2026-08-23
Estado: Vigente
Modifica a: [0002 Arquitectura hexagonal con frontera Clean](0002-arquitectura-hexagonal.md),
[0003 Monolito modular, no microservicios](0003-monolito-modular.md)

## Contexto

`0002` y `0003` asumen el mismo riesgo —que la frontera se erosione sola y nadie lo note hasta la
auditoria— y nombran la misma mitigacion: un test de arquitectura en CI. `0003` es explicito en lo
que pasa si no existe: *"si esa prueba no existe, esta decision se degrada sola a monolito sin
modulos"*. `0002` anade una segunda mitad, y es la que trae el problema: *"que el criterio de
aceptacion del diagrama sea el mismo que el del test"*.

De ahi salio `src/scripts/check_arquitectura.py`, que recorre las tres paginas del `.drawio` y
comprueba cinco cosas: la regla de dependencia, que el borde de salida este dibujado como
realizacion, que todo puerto de entrada sea alcanzable, que los modulos de la pagina 2 sean
aciclicos y que ningun sistema externo quede sin consumir. Corre en la etapa `arquitectura` de
`ci.yml`, sin filtro por ruta, y `arquitectura` es un `needs:` de la compuerta `ci`, que es el
unico check obligatorio en la proteccion de `main`.

Es decir: **hoy un dibujo puede bloquear un merge.** Eso resulta dificil de defender por cuatro
razones.

**Primera, y es la de fondo: valida un dibujo, no el sistema.** El diagrama puede estar impecable
mientras el codigo viola todas las reglas, y al reves. Es prueba de intencion, no de conformidad.
Lo que `0002` queria mitigar es un `import` del cliente HTTP dentro de un caso de uso; ninguna
comprobacion sobre el `.drawio` puede ver ese import.

**Segunda: es fragil por razones que no son de arquitectura.** El parser solo lee los `<mxCell>`
que cuelgan directamente de `<root>`. En cuanto alguien anade un tooltip o una propiedad
personalizada desde la interfaz de draw.io, la herramienta envuelve esa celda en `<object>` y la
celda desaparece del scan en silencio. Un check obligatorio que se pone en rojo por una edicion
cosmetica ensena al equipo a forzar el merge, que es peor que no tener el check.

**Tercera: corre siempre, sin filtro.** Un PR que solo toca documentacion lo paga igual. Se
decidio asi a proposito —un filtro mal escrito lo apagaria en silencio— pero el coste se acumula.

**Cuarta: se vuelve redundante.** `0010` puso Go, y con Go la frontera la sostiene el compilador:
los ciclos de importacion no compilan y `internal/` es un limite que el lenguaje entiende. Cuando
exista `go.mod`, `depguard` traduce estos mismos ADR a reglas de importacion sobre el codigo real.
A partir de ese momento la capa del diagrama es ceremonia.

Lo que no estaba escrito en ninguna parte es **cuando deja de ser util**. Sin esa fecha, una
comprobacion pensada como andamio se queda para siempre.

## Decision

**La capa 1 (el diagrama) avisa; la capa 2 (el codigo) bloquea.**

En `.github/workflows/arquitectura.yml`, el paso que ejecuta `check_arquitectura.py` lleva
`continue-on-error: true` y un paso siguiente que escribe un `::warning::` y un resumen cuando
falla. El job `codigo`, que corre `depguard`, no cambia: sigue bloqueando.

Un detalle de implementacion que no es opcional: **`continue-on-error` va en el paso, no en el
job**. Un job que llama a un workflow reutilizable con `uses:` solo admite `name`, `uses`, `with`,
`secrets`, `needs`, `if` y `permissions`; `actionlint` —que este mismo repo corre en la etapa
`quality-workflows`— rechaza la forma a nivel de job. Ponerlo mal rompe el CI que se pretende
arreglar.

**Condicion de retirada, explicita:** cuando exista `go.mod` y el job `codigo` corra en verde en
`main`, el job `diagrama` se elimina entero de `arquitectura.yml` y `check_arquitectura.py` pasa a
ser una herramienta local que se corre a mano al editar el diagrama. No se queda como aviso
permanente: un aviso que nadie lee es ruido.

## Alternativas consideradas

**Dejarlo bloqueando hasta que llegue `depguard`.** Es la lectura literal de `0002` y `0003`, y
tiene a favor que hoy es la unica comprobacion de frontera que existe. Descartada por la segunda
razon de arriba: el modo de fallo dominante no es "alguien rompio la arquitectura", es "alguien
edito el diagrama en draw.io". Un check obligatorio con mas falsos positivos que verdaderos
entrena al equipo a ignorarlo, y para cuando llegue `depguard` la costumbre ya estara hecha.

**Filtrarlo por ruta, que solo corra cuando cambie el diagrama.** Reduce el coste pero no toca la
fragilidad: cuando corre, sigue pudiendo bloquear por un `<object>`. Y `ci.yml` documenta por que
esta etapa no se filtra —un filtro mal escrito la apaga en silencio—, asi que anadir el filtro
cambia una decision ya razonada para resolver el sintoma equivocado.

**Borrarlo ya.** Deja el repo sin ninguna comprobacion de frontera hasta que exista `go.mod`, que
es justo el escenario que `0003` describe como degradacion automatica. El aviso cuesta nueve
segundos y mantiene la senal visible.

## Consecuencias

Positivas: `main` deja de poder bloquearse por una edicion cosmetica del diagrama. La senal se
conserva —el aviso sale en el resumen del job— sin el poder de veto. Y queda escrita la fecha de
caducidad, que era lo que faltaba.

A cambio, y hay que decirlo claro: **`arquitectura` en verde ya no significa que el diagrama
cumpla.** Hay que leer el aviso. Mientras no exista `go.mod` no hay ninguna comprobacion de
frontera que bloquee, asi que en esta ventana la mitigacion que `0002` y `0003` nombran queda en
suspenso y el repo depende de la revision humana. La ventana deberia ser corta; si se alarga,
conviene revisitar esta decision antes que asumir que sigue siendo barata.

Riesgo asumido: que la condicion de retirada no se cumpla nunca y el aviso se quede de adorno. La
mitigacion es que la retirada esta escrita aqui y en el encabezado de `arquitectura.yml`, y que el
primer PR que introduzca `go.mod` tiene que tocar ese workflow de todas formas para activar
`verificar-codigo`.
