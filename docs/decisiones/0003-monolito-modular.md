# 0003 Monolito modular, no microservicios

Fecha: 2026-08-10
Estado: Vigente
Modificada por: [0012 La frontera se verifica sobre el codigo, no sobre el diagrama](0012-la-frontera-se-verifica-sobre-el-codigo.md) — la mitigacion en CI
sigue existiendo, pero se verifica sobre los `import` con `depguard`, no sobre el diagrama.
(Via [0011](0011-verificacion-del-diagrama-como-aviso.md), ya sustituida.)

## Contexto

El nucleo de Intela tiene contextos delimitados con fronteras nitidas: Afiliacion, Repertorio,
Identificacion de Obras, Recaudo, Reparto, Liquidacion y Pago, Anticipos, Reclamaciones, ONI y
Prescripcion. Es exactamente la lista que en una presentacion de arquitectura se convierte en nueve
microservicios.

Antes de partir nada conviene mirar la carga real:

- El reglamento exige **al menos una distribucion por ano calendario** (`RD 12`), y en la practica
  se viene haciendo en la primera semana de diciembre (`RD 10.1`). No es un sistema de alto trafico:
  es un sistema con un pico anual.
- El universo son los afiliados de una sociedad de gestion colectiva colombiana de escritores
  audiovisuales. Miles de obras, no millones.
- Las muestras que hay del cliente son de 59 y 49 filas (`docs/dominio/fuentes-datos.md`).
- El equipo son estudiantes de ingenieria en ciencia de datos, no un equipo de plataforma.

Y hay una restriccion del dominio que empuja en direccion contraria a la de repartir el sistema en
servicios: **una corrida de reparto tiene que ser reproducible bit a bit anos despues**
(`0005-reparto-determinista-y-reproducible.md`). Con el calculo repartido entre procesos separados,
cada llamada de red es una fuente de no determinismo y de fallo parcial que hay que compensar.

## Decision

**Un solo desplegable, con los modulos aislados por dentro.**

Los contextos delimitados existen como modulos con frontera real: cada uno tiene su propio paquete,
su modelo y su interfaz publica, y las dependencias permitidas entre ellos son las que dibuja la
pagina 2 de `docs/diagrams/PATIC2 - Arquitectura.drawio`. Lo que no existe es un limite de proceso entre
ellos.

Las reglas de dependencia son parte de la decision, no un detalle:

- `Reparto` lee de `Repertorio` y de `Recaudo`. Ninguno de los dos conoce a `Reparto`.
- `Recaudo` es el unico que conoce Usuario, Convenio y Tarifa. Aguas abajo solo circula una bolsa.
- `Identificacion` escribe alias y emite ONI, y no toca dinero.
- Ningun modulo escribe en la trazabilidad de otro.

El mismo binario se despliega con tres puntos de entrada distintos: `api` (adaptadores HTTP),
`scheduler` (calendario y trabajos con reloj propio) y `worker` (matching y reparto por lotes).
Escalar el reparto es levantar mas replicas de `worker`, no partir el dominio.

## Alternativas consideradas

**Un microservicio por contexto delimitado.** Descartada. Los nueve servicios tendrian que coordinar
una transaccion que hoy es local: una corrida de reparto lee el catalogo, la bolsa y las
declaraciones, y escribe liquidaciones y asientos. Distribuido, eso exige sagas y compensaciones
para un proceso que corre una vez al ano y que **por reglamento no puede quedar a medias**
(`RD 13.5` define etapas con dueno y verificacion). Se pagaria complejidad operativa permanente para
resolver un problema de escala que no existe.

**Separar solo el matching como servicio**, con el argumento de que es la parte cara en computo.
Descartada por ahora, aunque es la primera candidata si algun dia hace falta. Hoy no hay evidencia
de que sea cara: no se puede medir con 59 y 49 filas. Sacarla a un servicio antes de tener esa
medicion seria decidir sin datos, y el aislamiento que daria ya lo da el modulo.

**Un monolito sin modulos, organizado por capas tecnicas** (todos los modelos juntos, todos los
servicios juntos). Descartada: es lo que convierte un monolito en una bola de barro. Sin frontera de
modulo, en seis meses `Reparto` importa el repositorio de tarifas y el corte que hace imposible
sumar dinero por fila desaparece.

**Serverless por caso de uso.** Descartada: el perfil de carga es un pico anual largo, no eventos
esporadicos, y el arranque en frio y los limites de tiempo de ejecucion estorban a un proceso por
lotes. Ademas el equipo tendria que aprender el modelo de despliegue antes de escribir dominio.

## Consecuencias

Positivas: una transaccion de base de datos cubre una etapa completa de reparto, sin sagas. Se
depura con un depurador y no con trazas distribuidas. `docker compose up` levanta el sistema
entero, que es lo que pide `docs/context.md`. Y las fronteras de modulo estan escritas, asi que si
alguna vez hace falta extraer un servicio, la costura ya esta marcada.

A cambio: un despliegue reconstruye todo, y un fallo en un modulo puede tumbar el proceso completo.
Con una distribucion anual y una consola administrativa de uso interno, la ventana de indisponibilidad
tolerable es amplia, asi que el intercambio es favorable.

Riesgo asumido: sin limite de proceso, nada impide fisicamente que un modulo importe el modelo
interno de otro, y la primera vez que pase nadie lo va a notar. La mitigacion es la misma que la de
`0002`: un test de arquitectura en CI que falle ante cualquier dependencia entre modulos que no este
en la pagina 2 del diagrama. Si esa prueba no existe, esta decision se degrada sola a monolito
sin modulos.
