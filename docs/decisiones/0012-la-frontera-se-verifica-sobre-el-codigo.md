# 0012 La frontera se verifica sobre el codigo, no sobre el diagrama

Fecha: 2026-08-24
Estado: Vigente
Sustituye a: [0011 La verificacion del diagrama avisa, no bloquea](0011-verificacion-del-diagrama-como-aviso.md)
Modifica a: [0002 Arquitectura hexagonal con frontera Clean](0002-arquitectura-hexagonal.md),
[0003 Monolito modular, no microservicios](0003-monolito-modular.md)

## Contexto

`0002` y `0003` asumen el mismo riesgo —que la frontera se erosione sola y nadie lo note hasta la
auditoria— y nombran la misma mitigacion: un test de arquitectura en CI. `0003` es explicito en lo
que pasa si no existe: *"si esa prueba no existe, esta decision se degrada sola a monolito sin
modulos"*. `0002` anade una segunda mitad, y es la que trajo el problema: *"que el criterio de
aceptacion del diagrama sea el mismo que el del test"*.

De ahi salio `src/scripts/check_arquitectura.py`, que recorria las tres paginas del `.drawio` y
comprobaba cinco cosas: la regla de dependencia, que el borde de salida estuviera dibujado como
realizacion, que todo puerto de entrada fuera alcanzable, que los modulos de la pagina 2 fueran
aciclicos y que ningun sistema externo quedara sin consumir.

`0011` ya concedio el diagnostico de fondo —valida un dibujo, no el sistema— y lo degrado a aviso
con `continue-on-error`. Dejo escrita ademas una condicion de retirada: *"cuando exista `go.mod` y
el job `codigo` corra en verde en `main`, el job `diagrama` se elimina entero"*.

Esta decision **ejecuta esa retirada antes de que se cumpla la condicion**, y conviene decir por
que, porque no es un detalle.

**Primera razon, la de fondo: validaba un dibujo, no el sistema.** El diagrama puede estar
impecable mientras el codigo viola todas las reglas, y al reves. Es prueba de intencion, no de
conformidad. Lo que `0002` queria mitigar es un `import` del cliente HTTP dentro de un caso de uso;
ninguna comprobacion sobre el `.drawio` puede ver ese import.

**Segunda: era fragil por razones que no son de arquitectura.** El parser solo leia los `<mxCell>`
que cuelgan directamente de `<root>`. En cuanto alguien anadia un tooltip o una propiedad
personalizada desde la interfaz de draw.io, la herramienta envolvia esa celda en `<object>` y la
celda desaparecia del scan en silencio. Ademas, la correspondencia entre id de celda y anillo
(`p1-dom`, `p1-a1`, …) estaba escrita a mano en el propio script: anadir una caja al diagrama sin
actualizar esos conjuntos daba verde sin comprobar nada.

**Tercera, y es la que decide el momento: el aviso no sobrevive a la reescritura de CI.** Este
mismo cambio reconstruye el pipeline entero —etapas en ingles, `lint-go`, `test-go`,
`build-frontend`, imagenes separadas—. Conservar la capa 1 obliga a portar un script de 178 lineas
y su job a la estructura nueva, y a mantenerlos, para producir un aviso que el propio `0011`
describe como material de retirada. El mismo `0011` lo argumenta: *"un aviso que nadie lee es
ruido"*. Pagar la migracion de algo que ya tiene fecha de caducidad es el orden inverso.

## Decision

**Se elimina la capa del diagrama. La frontera se verifica solo sobre el codigo.**

`src/scripts/check_arquitectura.py` se borra y el job que lo ejecutaba desaparece de
`.github/workflows/`. La etapa `Architecture boundary` (`architecture.yml`) queda con un solo job:
`depguard` con `--enable-only=depguard`, aislado del lint de estilo para que un check en
rojo diga cual de las dos cosas se rompio. Las reglas, con la cita al ADR de cada `deny`, viven en
`.golangci.yml`.

El `.drawio` **se conserva** como documentacion: sigue siendo la mejor explicacion de que cruza la
frontera y por que puerto. Lo que se retira es su poder de veto sobre un merge.

Queda derogada la segunda mitad de `0002` —*"que el criterio de aceptacion del diagrama sea el
mismo que el del test"*—. El criterio de aceptacion es el del codigo.

## Alternativas consideradas

**Esperar a que se cumpla la condicion de `0011`.** Es lo ortodoxo: `go.mod` entra en `main`,
`depguard` corre en verde, y solo entonces se borra la capa 1. Descartada por la tercera razon: el
PR que reescribe el pipeline llega antes que el PR del codigo Go, asi que esperar significa migrar
el script y su job a la estructura nueva para borrarlos poco despues. Se asume a cambio la ventana
que se describe en Consecuencias.

**Arreglar el parser para que lea `<object>`.** Resuelve la segunda razon y ninguna de las otras
dos. Se seguiria validando un dibujo.

**Conservarlo como herramienta local, sin job**, que es la otra mitad de lo que propone `0011`.
Es defendible y casi se hace. Descartada porque la tabla de ids escrita a mano es el problema que
no se va: una herramienta que hay que actualizar a mano para que siga diciendo la verdad, y que
nada obliga a correr, dice la verdad hasta la primera caja nueva. El `.drawio` se mantiene a mano
igual que el resto de la documentacion.

## Consecuencias

Positivas: la comprobacion de frontera pasa a mirar lo que de verdad importa, los `import`. `main`
deja de poder ponerse en rojo —o en amarillo— por una edicion cosmetica del diagrama. Y desaparece
un script de 178 lineas con una tabla de ids mantenida a mano.

A cambio, y hay que decirlo sin adornos: **entre este PR y el que traiga `go.mod`, el repositorio
no tiene ninguna comprobacion de frontera.** `depguard` solo corre cuando existe el modulo. Es
exactamente el escenario que `0003` describe como degradacion automatica, y en esa ventana la
mitigacion que `0002` y `0003` nombran queda en manos de la revision humana y de la lista del
`PULL_REQUEST_TEMPLATE`. Se acepta porque la ventana es corta y esta acotada por un PR concreto ya
abierto; si se alarga, esto hay que revisitarlo antes que asumir que sigue siendo barato.

La etapa no necesita ningun cambio de workflow para activarse: `architecture.yml` esta guardada
por la existencia de `go.mod`, de modo que el commit que introduzca el modulo enciende `depguard`
en ese mismo commit.

Segunda consecuencia: **el diagrama ya no esta verificado por nada.** Puede quedar desactualizado
respecto al codigo sin que CI lo note. La mitigacion es que deja de ser normativo: la fuente de
verdad de la frontera es `.golangci.yml`, que si se ejecuta. Si el diagrama y las reglas discrepan,
mandan las reglas.

Riesgo asumido: que `depguard` de una falsa sensacion de cobertura. Solo ve `import`; no ve que un
caso de uso reciba un `*pgx.Conn` por parametro a traves de una interfaz mal declarada. Para eso
esta la revision humana y la lista de comprobacion del `PULL_REQUEST_TEMPLATE`.
