# Prompt para Figma — pantallas faltantes de Intela

Prompt para generar en **Figma Make** las pantallas que el issue #30 necesita y que hoy
no existen en el archivo.

- **Archivo Figma:** `https://www.figma.com/make/2rdkXhewKec4eAF1pgoBj3/Intela`
- **fileKey:** `2rdkXhewKec4eAF1pgoBj3`
- **Secciones faltantes:** catálogo, detalle de obra, editor de reparto, historial de versiones

> El archivo es **Figma Make** (`/make/`), no Design (`/design/`). Por eso "Copy link to
> selection" no produce un `node-id`: en Make no existen como tales. Las APIs de escritura
> de Figma (`use_figma`, `createPage`) son **solo de archivos Design**.

## Estado al escribir este prompt

En `docs/mockups/` solo existen `figma-dashboard-administrador.png`, `figma-login.png` y
`figma-panel-titular.png`. La sección de catálogo del archivo Make esta **vacia**.

## Nombres de los screenshots

Guardar en `docs/mockups/` con estos nombres:

| Archivo | Pantalla |
| ------- | -------- |
| `figma-catalogo.png` | Catálogo con filtros y tabla |
| `figma-obra-detalle.png` | Detalle con la declaración vigente |
| `figma-obra-declaracion-editor.png` | Editor de reparto |
| `figma-obra-historial.png` | Historial de versiones |

Para detalle y editor, capturar los **tres estados del total** (completa / incompleta /
inválida): son criterio de aceptación explícito del issue.

---

## Prompt

```text
Diseña las pantallas faltantes del portal administrador de Intela, el sistema de
reconocimiento de obras y distribución de ingresos por propiedad intelectual de
REDES SGC (sociedad de gestión colectiva de escritores audiovisuales de Colombia).

El archivo ya tiene la estética definida: dashboard administrador, login y panel titular.
Respeta esa estética existente: misma paleta, tipografías, radios de borde, densidad y
tratamiento de tarjetas. Estas pantallas son una extensión del mismo sistema, no un
rediseño. Reutiliza los componentes que ya existan (botones, inputs, tablas, badges).

IDIOMA (obligatorio)
Toda la interfaz va en español de Colombia. El lenguaje del dominio es el de los
reglamentos: obra, titular, reparto, declaración, porcentaje, vigencia. No traduzcas
estos términos.

1. CATÁLOGO MAESTRO (OE-2)

Listado de obras del catálogo. Es la pantalla de entrada del módulo.

- Barra de filtros con cuatro controles:
  - Título: campo de texto, búsqueda PARCIAL y sin distinguir mayúsculas. Es el único
    filtro para el que el usuario no necesita saber el dato exacto.
  - Género: desplegable, valor EXACTO (ej. "Drama").
  - IPI: campo de texto, valor EXACTO. Etiquétalo claramente como "IPI de coautor":
    el IPI identifica personas, no obras.
  - Año: campo numérico, valor EXACTO.
- Tabla de resultados con columnas: Título, Género, Año, Tipo, IDA, y un indicador del
  estado de la declaración (ver punto 2).
- Paginación al pie (el servidor pagina, no es scroll infinito).
- Estado vacío diseñado: "Ninguna obra coincide con los filtros". No dejes la tabla
  vacía sin mensaje.
- Cada fila navega al detalle de la obra.

2. DETALLE DE OBRA

Cabecera con los metadatos de la obra (título, género, año, tipo, IDA, EIDR, IMDb) y la
lista de coautores con nombre, IPI y rol (guionista, libretista).

Debajo, la Declaración de Obra vigente: una tabla de partes donde cada fila es un titular
con su nombre, su IPI y su porcentaje.

Diseña tres estados visuales distintos y claramente distinguibles para el total:

- COMPLETA (suma = 100%): confirmación. Es el único estado que habilita el reparto.
- INCOMPLETA (suma < 100%): advertencia. NO es un error: es un estado válido. El sistema
  retiene el importe completo de esa obra hasta que se complete. El texto debe decir que
  se retiene, no que "falta algo por error".
- INVÁLIDA (suma > 100%): error. El guardado está deshabilitado. El backend la rechaza igual.

CRÍTICO: el estado "Incompleta" NO debe verse como un fallo del usuario. Por reglamento
(R-04, RD 13.1.3) una obra incompleta es una situación normal y el dinero se retiene en
reserva. El copy debe ser explicativo, no alarmante.

3. EDITOR DE REPARTO

Editor de la Declaración de Obra. Es la pantalla más importante del issue.

- Filas de titulares editables: agregar fila, eliminar fila.
- Cada fila: selector de titular, campo de IPI, campo de porcentaje.
- El porcentaje es decimal, hasta 4 decimales. No uses pasos de 1% ni un slider: la
  precisión de 4 decimales es real y el backend rechaza más.
- Total corriente visible y fijo mientras se edita (no obligues a hacer scroll).
- Barra de progreso o indicador del 100% en los tres estados del punto 2.
- Botón Guardar habilitado solo cuando el total es menor o igual a 100. Deshabilitado si
  es mayor.
- Un porcentaje de 0% no es válido: un titular al 0% no es titular. Si lo dibujas, que sea
  como estado de error.
- Zona de error del backend diseñada: si el guardado falla (por ejemplo, un titular que no
  está en el padrón), el mensaje del servidor se muestra aquí, legible y sin perder lo que
  el usuario escribió.
- Si la edición abre una versión nueva, avísalo: el usuario debe entender que no está
  corrigiendo la versión anterior, sino abriendo una nueva.

4. HISTORIAL DE VERSIONES (SOLO LECTURA)

Lista de versiones de la Declaración, de la más antigua a la más reciente. Cada versión
muestra:

- Número de versión.
- Ventana de vigencia: desde y hasta. La versión vigente tiene "hasta" vacío y debe verse
  marcada como "Vigente".
- Estado (completa / incompleta).
- Sus partes con titular, IPI y porcentaje.

No hay acciones de edición aquí. Una versión pasada no se edita: se abre una nueva. El
diseño debe dejar claro que es consulta.

Diseña también el estado vacío: una obra sin ninguna declaración todavía.

NOTAS DE TONO

El sistema mueve dinero de terceros y está sujeto a auditoría. Los textos deben ser
precisos y no prometer más certeza de la que el sistema tiene. Evita superlativos y evita
mensajes que afirmen éxito sin confirmación del servidor.
```

## Trazabilidad al contrato

Cada elemento del prompt sale de una fuente verificable, no de una suposición:

| Elemento del prompt | Fuente |
| ------------------- | ------ |
| Filtros del catálogo y su naturaleza (parcial vs exacto) | `api/openapi.yaml`, `GET /obras` |
| Paginación del servidor | `api/openapi.yaml`, params `limite` / `desplazamiento` |
| Campos de metadatos de la obra | schema `Obra` / `MetadatosObra` |
| Coautores con nombre, IPI y rol | schema `Coautor` |
| Partes con titular, IPI y porcentaje | schema `Parte` |
| Estados completa / incompleta | schema `VersionDeclaracion`, campo `estado` |
| Ventana de vigencia (desde / hasta) | schema `VersionDeclaracion` |
| 4 decimales y 0% inválido | descripción de `Parte` en el contrato |
| Incompleta es estado válido y se retiene el total | `R-04`, `RD 13.1.3` |
| Los porcentajes solo salen de la Declaración | `R-03`, `RD 7.3.1`, `RD 7.3.4` |
| Historial solo lectura, no se edita una versión pasada | `GET .../declaracion/historial` |

Ver [`docs/dominio/reglas-negocio.md`](../dominio/reglas-negocio.md) para el registro
completo de reglas y [`api/openapi.yaml`](../../api/openapi.yaml) para el contrato.
