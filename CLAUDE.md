# Intela

Sistema de reconocimiento de obras y distribucion de ingresos por propiedad intelectual para
**REDES SGC**, la sociedad de gestion colectiva de los **escritores audiovisuales** de
Colombia (guionistas y libretistas).

## Lo que hay que entender antes de escribir codigo

Cuatro cosas que se contradicen con la intuicion y que causan errores caros:

1. **El dinero no llega por fila.** Ningun reporte de uso trae importes. REDES SGC cobra una
   bolsa a cada usuario segun el Reglamento de Tarifas, y los reportes de uso solo sirven para
   **ponderar** el reparto de esa bolsa. Es un asignador de bolsa, no un atribuidor de
   ingresos transaccionales.

2. **Los porcentajes de reparto solo salen de la Declaracion de Obra.** Nunca de los reportes
   de los canales ni de los contratos de escritura. Las columnas `Autor*` y `Guionista*` de
   una parrilla son pistas para identificar la obra, jamas insumo de pago.

3. **Si los porcentajes declarados no suman 100%, no se reparte nada de esa obra.** Se retiene
   el total en reserva. Nunca se reparte parcialmente. `declaracion_incompleta` es un estado
   valido del modelo, no un error.

4. **Solo se paga a escritores personas naturales.** Productores, directores, actores,
   ejecutivos de cadena y revisores no generan derecho de autor aqui, por mucho que aparezcan
   en los metadatos.

Toda cifra que el sistema produzca debe ser explicable hasta su origen: fuente, reporte, regla
aplicada. El reglamento lo exige y la auditoria lo revisa.

## Donde esta el contexto

Cargar solo lo que haga falta para la tarea.

| Archivo | Para que |
| ------- | -------- |
| `docs/dominio/glosario.md` | Lenguaje ubicuo. Que es obra, titular, ONI, recaudo, reparto, IPI, IDA |
| `docs/dominio/reglas-negocio.md` | Registro de reglas con cita al reglamento. Empezar aqui |
| `docs/dominio/formulas.md` | Modelos de calculo por tipo de usuario (TV, cine, OTT, hoteles) |
| `docs/dominio/identificadores.md` | Por que los IDs de fuente no cruzan y como resolver obras |
| `docs/dominio/fuentes-datos.md` | Perfil real de los archivos del cliente y que falta pedir |
| `docs/reglamentos/` | Texto verbatim de los reglamentos, citable por numeral |
| `docs/decisiones/` | Por que el sistema quedo modelado asi |
| `docs/context.md` | Planteamiento academico original del proyecto |

Convencion de citas: `RD 13.1.3` es la seccion 13.1.3 del Reglamento de Distribucion IX.
`RT` Tarifas VI, `RS` Socios, `RA` Anticipos.

## Idioma

El dominio se nombra en **espanol**, igual que los reglamentos: `obra`, `titular`, `reparto`,
`recaudo`, `declaracion`. No traducir estos terminos en modelos, tablas ni variables. El
codigo de infraestructura puede ir en ingles.

## Estructura

```
data/files/      muestras del cliente
docs/dominio/    conocimiento destilado, con citas
docs/reglamentos/ texto verbatim + PDF originales en fuente/
src/scripts/     scripts sueltos, cada uno con su entorno PEP 723
```

## Scripts

Los scripts de `src/scripts/` declaran sus dependencias en linea (PEP 723) y se ejecutan con
`uv run --script <archivo>`. Cada uno resuelve su propio entorno efimero, para no interferir
con el stack de la aplicacion.

- `sample.py` diagnostico de los archivos de muestra
- `convert_reglamentos.py` regenera `docs/reglamentos/` desde los PDF

## Estado

Fase de analisis. No hay stack de aplicacion definido todavia. Antes de construir el motor de
matching o el de distribucion faltan datos del cliente: ver las preguntas abiertas al final de
`docs/dominio/reglas-negocio.md`.
