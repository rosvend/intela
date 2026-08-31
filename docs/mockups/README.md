# Mockups

Capturas del diseno de referencia del tablero. Sirven para que un PR se pueda revisar sin
abrir Figma, y como insumo de las pantallas de Sprint 3-5.

| Archivo | Pantalla |
| --- | --- |
| `figma-login.png` | Inicio de sesion |
| `figma-dashboard-administrador.png` | Panel de control, sesion de administrador |
| `figma-panel-titular.png` | Panel del titular con su liquidacion (pantalla de #42) |

## De donde salen

Archivo de **Figma Make**, `fileKey` `2rdkXhewKec4eAF1pgoBj3`, autor @emanuel684:

<https://www.figma.com/make/2rdkXhewKec4eAF1pgoBj3/Intela>

Las capturas de esta carpeta corresponden a la **version 6** ("Update logo and fonts"),
vigente al 31 de agosto de 2026.

El link que aparece en `docs/Intela OKRs, US and ubiquituous language.md` apunta al mismo
`fileKey` pero con el nombre de la plantilla de la que salio el archivo
("Spanish-Language-Practice-App"). Es el mismo archivo, renombrado.

## Por que se fija la version

Un Make file sigue evolucionando: quien lo abra dentro de seis meses no va a ver lo mismo
que se construyo. Las versiones 2 a 4, por ejemplo, tenian una paleta terracota con Oswald
y Open Sans que la version 5 **reemplazo a proposito** por neutros frios con el vinotinto
`#7D1A35` como unico acento. Una captura de aquellas versiones no es una referencia
valida: es una decision descartada.

Si se agregan capturas de una version posterior, actualizar el numero de version de arriba.

## Los tokens no se leen de estas imagenes

La paleta y la tipografia estan escritas literalmente en el historial del Make y viven en
`web/src/styles.css` como variables CSS. No sacar colores con cuentagotas de un PNG.

## Las cifras que se ven aqui son de relleno

El mockup muestra importes, conteos y porcentajes inventados, y **no cuadran entre si**:
una tarjeta dice "12 obras liquidadas" y la tabla de al lado lista 5. Es el mismo criterio
que el `CLAUDE.md` fija para el seed - no tratar esas cifras como datos reales, ni
copiarlas a un placeholder.

## Discrepancias conocidas

El mockup tiene once puntos donde choca con el dominio, el reglamento o el texto del issue.
Estan registrados como `M-1` a `M-11` en el hilo del issue
[#19](https://github.com/rosvend/intela/issues/19), cada uno con que se hizo y quien
decide. Dos de ellos -`M-1` navegacion sin filtrar por rol y `M-2` selector de rol en el
cliente- son divergencias deliberadas: copiarlos habria incumplido un criterio de
aceptacion y abierto una escalada de privilegios.

Dos se ven a simple vista en `figma-login.png`, y conviene senalarlos aqui para que nadie
los copie pensando que son intencionales:

- El wordmark del logo dice **"Intelia"**, con i, mientras que el pie de la misma pantalla
  dice "Intela" bien escrito. Es una errata del PNG del logo, no una decision de marca
  (`M-10`).
- El correo de demo del administrador aparece como **`ladmin@intela.co`**, con una ele de
  mas (`M-9`).
