# **ENTREGABLE 1 \- PROYECTO APLICADO EN TICII**

**Grupo 2: Gestor de Reconocimiento y Distribución de Ingresos por Propiedad Intelectual**

Universidad Pontificia Bolivariana \- Ingeniería en Ciencia de Datos

Cliente de referencia: REDES SGC (Red Colombiana de Escritores Audiovisuales, de Teatro, Radio y Nuevas Tecnologías)

Equipo: Emanuel Acevedo, Roy Sandoval, Miguel Legarda, Santiago Mendoza

Fecha: 2 de agosto de 2026

# **1\. Resumen ejecutivo**

Este documento consolida el primer entregable del proyecto orientado a diseñar y desarrollar un sistema de gestión de datos que automatice el reconocimiento de obras de propiedad intelectual, el cálculo de ingresos y su distribución entre titulares, con trazabilidad y auditoría completa.

El contexto normativo y operativo se fundamenta en el Reglamento de Distribución de REDES SGC (Versión IX, aprobada el 27 de mayo de 2026), disponible en redescritores.com/reglamento-de-distribucion/, que establece principios, metodologías de reparto por tipo de usuario (televisión abierta, cine, teatros, OTT, suscripción, entre otros), deducciones legales, declaración de obras al 100% y requisitos de transparencia para los afiliados.

# **2\. Objetivo general del proyecto**

Diseñar y desarrollar un sistema de gestión de datos que reconozca obras de propiedad intelectual a partir de múltiples fuentes de reportes de uso e ingresos, las asocie a un catálogo maestro de titulares y derechos, y calcule y redistribuye los ingresos correspondientes de forma automatizada, transparente y auditable.

# **3\. Key Results (KR) \- 5 resultados clave**

Los siguientes Key Results definen los resultados medibles que el equipo debe alcanzar durante el desarrollo del proyecto, alineados con el Reglamento de Distribución de REDES SGC y los objetivos específicos del proyecto.

| KR | Descripción | Métrica de éxito | Horizonte |
| :---- | :---- | :---- | :---- |
| KR-1: Ingesta multi-fuente operativa | Conectar y normalizar reportes de al menos 3 fuentes distintas (televisión abierta, OTT/plataformas digitales y cine/exhibición) hacia un esquema común de datos. | ≥ 95% de registros cargados exitosamente por fuente; ≥ 3 fuentes integradas; tiempo de procesamiento \< 5 min por lote de 10.000 registros. | Sprint 2–3 |
| KR-2: Reconocimiento automático de obras | Implementar matching difuso/deduplicación para asociar registros de reportes con obras del catálogo maestro, incluso con variaciones de título y metadatos incompletos. | ≥ 90% de registros reconocidos automáticamente; ≤ 10% en cola de revisión manual; precisión ≥ 95% en casos validados. | Sprint 3–4 |
| KR-3: Motor de distribución conforme a reglas | Calcular y distribuir ingresos por obra, fuente y periodo según porcentajes de titularidad declarados (splits al 100%) y metodologías por tipo de usuario. | 100% de obras con splits completos distribuidas; 0 pagos duplicados; trazabilidad del origen de cada monto (fuente, reporte, regla aplicada). | Sprint 4–5 |
| KR-4: Detección y resolución de anomalías | Identificar obras no reconocidas (ONI), reportes duplicados, titulares sin porcentajes y reservas por falta de declaración al 100%. | 100% de anomalías detectadas y registradas; tiempo promedio de resolución manual \< 48 h; dashboard de alertas operativo. | Sprint 5 |
| KR-5: Panel y reportería para titulares y administradores | Entregar dashboard funcional con consulta de ingresos por obra/fuente/periodo y reportes exportables (PDF/Excel) auditables. | ≥ 2 perfiles de usuario (administrador y titular); reportes exportables disponibles; satisfacción del equipo ≥ 4/5 en prueba piloto. | Sprint 6 |

# **4\. Roles y responsabilidades del equipo**

La siguiente matriz define la distribución de responsabilidades entre los cuatro integrantes del Grupo 2\. R \= Responsable, A \= Aprobador, C \= Consultado, I \= Informado.

| Integrante | Rol | Responsabilidades principales | Fases |
| :---- | :---- | :---- | :---- |
| Emanuel Acevedo | Líder de proyecto / Product Owner | Coordinación general del equipo; definición de alcance y priorización del backlog; comunicación con stakeholders; validación de entregables; gestión de riesgos y cronograma. | Todas las fases |
| Roy Sandoval | Arquitecto de datos / Backend Lead | Diseño de arquitectura del sistema; modelado del catálogo maestro; pipeline de ingesta y normalización; motor de cálculo y distribución; Dockerización y despliegue. | Diseño, desarrollo e integración |
| Miguel Legarda | Ingeniero de datos / Entity Matching | Implementación de técnicas de matching difuso y deduplicación; conciliación entre fuentes; detección de anomalías; scripts de seed y datos de prueba; optimización de pipelines. | Ingesta, matching y calidad de datos |
| Santiago Mendoza | Frontend / UX & Reportería | Diseño de interfaz (mockups Figma); desarrollo del dashboard; reportes exportables; experiencia de usuario para administradores y titulares; documentación de usuario. | Diseño UI, frontend y pruebas |

## 4.1 Matriz de responsabilidades por componente

| Componente | Emanuel | Roy | Miguel | Santiago |
| :---- | :---- | :---- | :---- | :---- |
| Ingesta y normalización de reportes | R | A | C | I |
| Catálogo maestro de obras y titulares | C | A/R | C | I |
| Matching difuso / deduplicación | I | C | A/R | I |
| Modelo de derechos y splits | C | A/R | C | I |
| Motor de cálculo y distribución | C | A/R | C | I |
| Detección de anomalías | I | C | A/R | C |
| Dashboard y reportería | A | C | I | R |
| Mockups y diseño UX | A | I | I | R |
| Documentación y entregables | R | C | C | C |
| Despliegue (Docker/Compose) | I | A/R | C | I |

## 4.2 Propuesta de roles bajo el enfoque de agente autónomo

Adición del 3 de agosto de 2026\. No reemplaza la tabla de la sección 4, que se conserva arriba sin cambios. Recoge los roles que el enfoque de agente exige: alguien tiene que ser dueño de la VM, de los adaptadores de adquisición y de la observabilidad, y alguien tiene que levantar con el cliente cuáles son las fuentes reales antes de decidir qué adaptador se implementa. Queda a discusión del equipo.

| Integrante | Rol propuesto | Responsabilidades principales | Fases |
| :---- | :---- | :---- | :---- |
| Miguel Legarda | Líder Técnico / Arquitecto de datos y del agente | Arquitectura general y esquema canónico; catálogo maestro; motor de resolución de entidades y sus umbrales; motor de cálculo y distribución, incluida la política de redondeo y residuos; splits versionados por vigencia; idempotencia y contabilidad de corridas; trazabilidad; revisión de todo merge a main; definir el criterio de «terminado» de cada módulo. | Todas las fases técnicas |
| Roy Sandoval | Coordinador de Proyecto y Enlace Externo | Toda comunicación con el docente, monitores y REDES SGC; levantamiento del flujo real del usuario y de las fuentes; traducción del Reglamento de Distribución a matriz de reglas de negocio; agenda y cierre de compromisos; presentación y demo; consolidación del documento de entregables. | Todas las fases |
| Santiago Mendoza | Ingeniero de Automatización, Infraestructura y Calidad | Adaptadores de adquisición y catálogo de selectores; sonda de salud; normalización a esquema canónico; la VM (instancia, temporizador, secretos, rotación de logs); observabilidad y evidencia de fallos; dataset etiquetado; pruebas automatizadas; conciliación y anomalías; Docker y guía de despliegue; mantenimiento del tablero. | Adquisición, infraestructura y calidad |
| Emanuel Acevedo | Desarrollador de Aplicación y Reportería | Dashboard de administrador y de titular; bandeja de conciliación; panel de corridas del agente; reportes exportables en PDF y Excel; vista «explicar esta cifra»; manual de usuario. | Diseño UI, frontend y reportería |

## 4.3 Matriz de responsabilidades recalculada (propuesta)

Adición del 3 de agosto de 2026\. No reemplaza la matriz 4.1. Agrega las filas que los objetivos nuevos exigen —levantamiento de fuentes, adquisición por adaptadores, orquestación e idempotencia, resiliencia y observabilidad, trazabilidad y reglas del reglamento— y reasigna las existentes según la propuesta 4.2. R \= responsable, A \= aprobador, C \= consultado, I \= informado.

| Componente | Emanuel | Roy | Miguel | Santiago |
| :---- | :---- | :---- | :---- | :---- |
| Levantamiento del flujo real y de las fuentes | C | A/R | C | I |
| Adquisición por adaptadores (portal, carpeta, API) | I | C | A | R |
| Normalización a esquema canónico | C | I | A | R |
| Catálogo maestro de obras y titulares | I | I | A/R | C |
| Resolución de entidades (matching difuso) | I | I | A/R | C |
| Modelo de derechos y splits | I | C | A/R | I |
| Motor de cálculo y distribución | I | I | A/R | C |
| Orquestación e idempotencia | I | I | A/R | C |
| Resiliencia y observabilidad | C | I | A | R |
| Conciliación y detección de anomalías | C | I | A | R |
| Trazabilidad y auditoría | C | I | A/R | C |
| Dashboard, panel de corridas y reportería | R | C | A | I |
| Mockups y diseño UX | R | C | I | I |
| Reglas de negocio del reglamento | I | A/R | C | I |
| Despliegue (Docker/Compose) | I | I | A | R |
| Documentación y entregables | C | A/R | C | C |

# **5\. Cuadro de objetivos e historias de usuario**

Las historias de usuario están alineadas con el Reglamento de Distribución de REDES SGC, que exige reparto equitativo proporcional al uso de las obras, declaración de titularidad al 100%, deducciones legales, trazabilidad y documentación para auditoría.

**Tabla 5.A** — Propuesta de tabla de objetivos bajo el enfoque de agente autónomo (adición del 3 de agosto de 2026). No reemplaza la tabla 5.B, que sigue más abajo: la reordena y le agrega los objetivos que un proceso que corre sin supervisión exige. R \= responsable principal, A \= apoyo.

| \# | Objetivo específico | R | A | Entregable verificable | Criterio de aceptación |
| :---- | :---- | :---- | :---- | :---- | :---- |
| 0 | Levantamiento del flujo real y de las fuentes — determinar con el cliente qué fuentes existen, en qué formato entregan y por qué vía se accede a cada una. Es prerrequisito de todo lo demás: define qué adaptador se implementa en el objetivo 1 | Roy | Miguel | Acta de la reunión con el cliente y ficha por fuente: nombre, formato, vía de acceso, periodicidad y quién la entrega hoy a mano | Cada fuente del alcance tiene su ficha diligenciada y firmada por el cliente. Ninguna decisión de adaptador se toma antes de esto. Si una fuente resulta ser un portal sin API, ahí y solo ahí se justifica la automatización por interfaz |
| 1 | Adquisición autónoma con adaptadores — el agente adquiere los reportes del periodo sin intervención humana, por el adaptador que corresponda a cada fuente: portal web operado por interfaz, carpeta o correo vigilado, o API si llegara a existir | Santiago | Miguel | Contrato de adaptador, implementación de los adaptadores que apliquen (el de portal usa Playwright con catálogo de selectores), bóveda cruda con huellas y log de corrida | Una vez levantado el flujo real con el cliente, el adaptador correspondiente adquiere los reportes de al menos 3 fuentes en una corrida programada sin que nadie toque nada; el original queda intacto con su SHA-256 |
| 2 | Normalización a esquema canónico — llevar cada fuente al mismo esquema | Santiago | Emanuel | Normalizadores por fuente, esquema canónico documentado y log de rechazos | Las 3 fuentes cargan de extremo a extremo; los registros inválidos quedan en el log con motivo, no se pierden ni se cuelan |
| 3 | Catálogo maestro — modelo de obras y titulares con identificadores únicos | Miguel | Santiago | Diagrama entidad-relación, DDL y datos semilla | Soporta obras con múltiples coautores, roles autorales distintos y códigos externos por fuente |
| 4 | Resolución de entidades — asociar cada registro a la obra correcta pese a variaciones de nombre y metadatos incompletos | Miguel | Santiago | Motor de matching difuso e informe de métricas | Sobre dataset etiquetado: ≥ 90 % de auto-asociación y ≥ 95 % de precisión; lo dudoso cae en revisión manual, nunca se asigna a ciegas |
| 5 | Modelo de derechos y splits — porcentajes de participación versionados por vigencia | Miguel | Roy | Modelo de datos de splits e historial de versiones | La suma de porcentajes vigentes por obra es exactamente 100 %; un reparto de un periodo pasado usa el split vigente entonces, no el actual |
| 6 | Motor de cálculo y distribución — repartir ingresos por obra, fuente y periodo | Miguel | Santiago | Servicio de liquidación y caso de control | Dos ejecuciones del mismo periodo dan el mismo resultado; la suma repartida iguala la base sin pérdida de centavos; el residuo se asigna con regla escrita |
| 7 | Orquestación temporal e idempotencia — el agente despierta solo y no repite trabajo ni pagos | Miguel | Santiago | Orquestador, temporizador systemd y tabla de corridas | El temporizador dispara a las 03:00 America/Bogota; una corrida repetida no genera un segundo pago; un reporte corregido genera liquidación de ajuste |
| 8 | Resiliencia y observabilidad — el agente avisa cuando se rompe y deja con qué depurarlo | Santiago | Emanuel | Sonda de salud, captura y DOM al fallar, logs estructurados y notificador | Ante un cambio de la interfaz de origen la sonda alerta antes de la corrida; todo fallo deja evidencia; cada corrida emite un resumen legible |
| 9 | Conciliación y anomalías — detectar obras no identificadas, duplicados e inconsistencias | Santiago | Emanuel | Módulo de validaciones y bandeja de casos | Detecta duplicados por huella del archivo y por registro; toda obra no identificada queda en una bandeja con estado y responsable |
| 10 | Dashboard, panel de corridas y reportería — consulta para administradores y titulares | Emanuel | Roy | Panel funcional, panel de corridas y exportables PDF/Excel | Un titular filtra por obra, fuente y periodo y descarga su liquidación; un administrador ve el estado de la última corrida por etapa |
| 11 | Trazabilidad y auditoría — explicar el origen de cada cifra | Miguel | Emanuel | Bitácora de liquidaciones y vista «explicar esta cifra» | Dado cualquier monto pagado, el sistema muestra corrida, reporte de origen, obra asociada, regla aplicada y versión del split |
| 12 | Reglas de negocio del reglamento — traducir el Reglamento de Distribución a especificación implementable | Roy | Miguel | Matriz regla ↔ artículo ↔ implementación ↔ prueba | Cada regla del reglamento que aplica al alcance tiene una fila, un módulo responsable y una prueba que la verifica |
| 13 | Despliegue reproducible — empaquetar la solución | Santiago | Miguel | docker-compose.yml, unidades systemd, script de seed y guía | En una máquina limpia, un solo comando levanta el sistema con datos de ejemplo y el dashboard responde |
| 14 | Documentación y entregables — consolidar informes y sustentación | Roy | Todos | Documento de entregable y presentación | Entregado en la fecha, con cada sección firmada por su responsable |

**Tabla 5.B** — Cuadro original de objetivos e historias de usuario. Se conserva sin cambios; los criterios de aceptación de la sección 5.1 corresponden a esta tabla.

| Objetivo | Feature | HU (rol-función-beneficio) | Sprint (2 wk) |
| :---- | :---- | :---- | :---- |
| OE-1 | Conectores de ingesta multi-fuente | Como administrador de REDES, quiero cargar reportes de ingresos de múltiples fuentes (CSV, Excel, JSON) que un agente autónomo descarga del portal operando su interfaz, dado que no existe acceso a APIs, y consolidarlos en un esquema común y procesar periodos de recaudo de forma uniforme. | Sprint 2 |
| OE-1 | Normalización de esquema | Como administrador, quiero que el sistema normalice monedas, fechas y estructuras de cada fuente para que los cálculos sean consistentes entre televisión, cine y plataformas OTT. | Sprint 3 |
| OE-2 | Catálogo maestro de obras | Como administrador, quiero un catálogo maestro de obras con identificadores únicos (título, género, año, IPI) para vincular cada registro de uso con la obra correcta. | Sprint 3 |
| OE-2 | Matching difuso | Como administrador, quiero que el sistema reconozca obras aunque el título varíe entre fuentes (matching difuso) para reducir obras no identificadas (ONI). | Sprint 4 |
| OE-3 | Registro de titulares y splits | Como administrador, quiero registrar titulares por obra con porcentajes hasta completar el 100%, conforme a la Declaración de Obra de REDES-SYS. | Sprint 4 |
| OE-3 | Gestión de reservas | Como administrador, quiero mantener en reserva los montos de obras sin splits al 100% definidos, hasta que los coautores acuerden los porcentajes. | Sprint 5 |
| OE-4 | Cálculo por tipo de usuario — TV | Como administrador, quiero calcular la distribución de reportes de televisión abierta usando la fórmula puntos \= tipo × duración × rating, para repartir según el reglamento. | Sprint 5 |
| OE-4 | Cálculo por tipo de usuario — Cine | Como administrador, quiero calcular la distribución de reportes de cine de forma proporcional a la taquilla reportada, para repartir según el reglamento. | Sprint 5 |
| OE-4 | Cálculo por tipo de usuario — OTT | Como administrador, quiero calcular la distribución de reportes OTT como visualizaciones × duración, para repartir según el reglamento. | Sprint 5 |
| OE-4 | Deducciones legales | Como administrador, quiero aplicar deducciones legales (gastos administrativos, bienestar social) antes del reparto neto a titulares. | Sprint 5 |
| OE-5 | Detección de anomalías | Como administrador, quiero recibir alertas de obras no reconocidas, reportes duplicados e inconsistencias para resolverlos manualmente antes del reparto. | Sprint 5 |
| OE-5 | Flujo de resolución manual | Como administrador, quiero un flujo de resolución para casos no resueltos automáticamente, con registro auditable de la decisión. | Sprint 5 |
| OE-6 | Panel de consulta | Como titular (autor), quiero consultar en un panel mis ingresos por obra, fuente y periodo, con el detalle del origen de cada monto. | Sprint 6 |
| OE-6 | Reportes exportables | Como titular, quiero descargar un reporte exportable (PDF/Excel) con liquidación de pago (montos brutos, deducciones, netos). | Sprint 6 |
| OE-7 | Historial de auditoría | Como auditor, quiero ver el historial de cambios en catálogo, splits y distribuciones, con evidencia del origen de cada cifra. | Sprint 6 |

## 5.1 Criterios de aceptación para cada HU

**OE-1 — Conectores de ingesta multi-fuente**  
Como administrador de REDES, quiero cargar reportes de ingresos de múltiples fuentes (CSV, Excel, JSON) que un agente autónomo descarga del portal operando su interfaz, dado que no existe acceso a APIs, y consolidarlos en un esquema común y procesar periodos de recaudo de forma uniforme.

* Un agente programado descarga los reportes del portal operando su interfaz con un navegador headless, sin intervención humana y sin API. El sistema acepta además carga manual de archivos CSV, Excel y JSON como ruta de respaldo permanente.  
* Al cargar un reporte, sus campos se mapean automáticamente al esquema común (obra, fuente, periodo, monto).  
* Si un archivo no cumple con la estructura mínima esperada, el sistema muestra un mensaje indicando qué campos faltan o están mal formateados.  
* Cada reporte cargado queda asociado a un periodo de recaudo específico y visible en el listado de cargas realizadas.

**OE-1 — Normalización de esquema**  
Como administrador, quiero que el sistema normalice monedas, fechas y estructuras de cada fuente para que los cálculos sean consistentes entre televisión, cine y plataformas OTT.

* Los montos de todas las fuentes se convierten a una moneda base única antes de procesarse.  
* Las fechas de cada fuente se transforman a un formato estándar, sin importar el formato original del archivo.  
* Los registros normalizados de distintas fuentes comparten la misma estructura de campos (obra, fuente, periodo, monto, moneda).  
* Si un registro no puede normalizarse (por ejemplo, moneda no reconocida), queda marcado para revisión en lugar de descartarse silenciosamente.

**OE-2 — Catálogo maestro de obras**  
 Como administrador, quiero un catálogo maestro de obras con identificadores únicos (título, género, año, IPI) para vincular cada registro de uso con la obra correcta.

* Cada obra en el catálogo tiene un identificador único e inmutable.  
* El catálogo almacena al menos título, género, año e IPI por obra.  
* El sistema impide crear dos obras con el mismo identificador único.  
* Es posible buscar una obra en el catálogo por título, género o IPI.

**OE-2 — Matching difuso**  
Como administrador, quiero que el sistema reconozca obras aunque el título varíe entre fuentes (matching difuso) para reducir obras no identificadas (ONI).

* El sistema asocia automáticamente un registro de reporte a una obra del catálogo aunque el título tenga variaciones menores (mayúsculas, tildes, orden de palabras).  
* Cuando la similitud entre el título del reporte y el catálogo supera un umbral definido, la asociación se realiza sin intervención manual.  
* Los registros que no alcanzan el umbral de similitud quedan marcados como "obra no identificada (ONI)" en vez de asociarse incorrectamente.  
* El administrador puede ver, para cada asociación automática, el nivel de confianza con el que se realizó el match.

**OE-3 — Registro de titulares y splits**  
Como administrador, quiero registrar titulares por obra con porcentajes hasta completar el 100%, conforme a la Declaración de Obra de REDES-SYS.

* El sistema permite asociar uno o más titulares a una obra, cada uno con un porcentaje de participación.  
* El sistema impide guardar una declaración de obra si la suma de porcentajes supera el 100%.  
* El sistema indica claramente si la suma de porcentajes de una obra es menor al 100%.  
* Es posible editar los porcentajes de titularidad de una obra ya registrada, quedando el cambio identificable.

**OE-3 — Gestión de reservas**  
Como administrador, quiero mantener en reserva los montos de obras sin splits al 100% definidos, hasta que los coautores acuerden los porcentajes.

* Los ingresos de una obra cuyos splits no suman 100% no se distribuyen a ningún titular, sino que quedan retenidos como reserva.  
* El sistema muestra un listado de obras con montos en reserva, indicando el monto retenido por obra.  
* Cuando se completan los splits al 100% de una obra en reserva, el monto retenido queda disponible para su distribución.  
* El administrador puede consultar el motivo por el cual un monto está en reserva.

**OE-4 — Cálculo por tipo de usuario — TV**  
Como administrador, quiero calcular la distribución de reportes de televisión abierta usando la fórmula puntos \= tipo × duración × rating, para repartir según el reglamento.

* El sistema calcula los puntos de cada emisión de TV multiplicando tipo, duración y rating, según los datos del reporte.  
* El valor a distribuir por obra en TV es proporcional a los puntos calculados frente al total de puntos del periodo.  
* Si el reporte de TV no incluye alguno de los tres factores, el registro queda marcado como incompleto y excluido del cálculo.  
* El resultado del cálculo por obra en TV queda trazado (fecha, fórmula aplicada, valores usados).

**OE-4 — Cálculo por tipo de usuario — Cine**  
Como administrador, quiero calcular la distribución de reportes de cine de forma proporcional a la taquilla reportada, para repartir según el reglamento.

* El sistema calcula el monto a distribuir por obra de cine de forma proporcional a la taquilla reportada para esa obra frente al total del periodo.  
* El cálculo solo se ejecuta si la obra ya fue reconocida en el catálogo maestro (matching exitoso).  
* Si el reporte de taquilla contiene valores negativos o nulos, el registro queda marcado como incompleto y excluido del cálculo.  
* El resultado del cálculo por obra en cine queda trazado (fecha, fórmula aplicada, valores usados).

**OE-4 — Cálculo por tipo de usuario — OTT**  
Como administrador, quiero calcular la distribución de reportes OTT como visualizaciones × duración, para repartir según el reglamento.

* El sistema calcula el valor de cada obra en OTT multiplicando visualizaciones por duración, según los datos del reporte.  
* El monto a distribuir por obra en OTT es proporcional a ese valor frente al total del periodo.  
* Si el reporte OTT no incluye visualizaciones o duración, el registro queda marcado como incompleto y excluido del cálculo.  
* El resultado del cálculo por obra en OTT queda trazado (fecha, fórmula aplicada, valores usados).

**OE-4 — Deducciones legales**  
Como administrador, quiero aplicar deducciones legales (gastos administrativos, bienestar social) antes del reparto neto a titulares.

* El sistema aplica automáticamente los porcentajes de deducción configurados (gastos administrativos, bienestar social) sobre el monto bruto de cada obra.  
* El monto neto entregado a cada titular refleja el monto bruto menos las deducciones aplicadas.  
* Las deducciones aplicadas quedan visibles por separado del monto neto en cualquier consulta o reporte.  
* Es posible actualizar los porcentajes de deducción sin afectar los cálculos de periodos ya distribuidos.

**OE-5 — Detección de anomalías**  
Como administrador, quiero recibir alertas de obras no reconocidas, reportes duplicados e inconsistencias para resolverlos manualmente antes del reparto.

* El sistema genera una alerta cuando un registro no puede asociarse a ninguna obra del catálogo (ONI).  
* El sistema detecta y alerta cuando dos registros del mismo reporte, fuente y periodo son duplicados.  
* Las alertas de anomalías son visibles en un listado centralizado antes de ejecutar la distribución del periodo.  
* Cada alerta indica el tipo de anomalía y el registro específico que la originó.

**OE-5 — Flujo de resolución manual**  
Como administrador, quiero un flujo de resolución para casos no resueltos automáticamente, con registro auditable de la decisión.

* El administrador puede resolver manualmente una anomalía asignando el registro a una obra o descartándolo.  
* Cada resolución manual queda registrada con usuario, fecha y decisión tomada.  
* Un caso resuelto manualmente deja de aparecer en el listado de anomalías pendientes.  
* El historial de resoluciones manuales de una obra es consultable posteriormente.

**OE-6 — Panel de consulta**  
Como titular (autor), quiero consultar en un panel mis ingresos por obra, fuente y periodo, con el detalle del origen de cada monto.

* El titular solo puede ver los ingresos correspondientes a las obras donde tiene participación registrada.  
* El panel permite filtrar los ingresos por obra, fuente y periodo.  
* Cada monto mostrado indica de qué fuente y reporte proviene, así como la fórmula o regla de reparto aplicada para calcularlo.  
* El panel refleja los montos netos (después de deducciones) y no los brutos.

**OE-6 — Reportes exportables**  
Como titular, quiero descargar un reporte exportable (PDF/Excel) con liquidación de pago (montos brutos, deducciones, netos).

* El titular puede exportar su liquidación de pago en formato PDF y en formato Excel.  
* El reporte exportado incluye monto bruto, deducciones aplicadas y monto neto por obra.  
* El reporte permite filtrarse por periodo antes de exportarse.  
* El archivo exportado conserva la información aún si se genera fuera de línea (sin conexión activa al panel).

**OE-7 — Historial de auditoría**  
Como auditor, quiero ver el historial de cambios en catálogo, splits y distribuciones, con evidencia del origen de cada cifra.

* El sistema registra cada cambio realizado sobre el catálogo, los splits y las distribuciones, con usuario y fecha.  
* El auditor puede consultar el historial de cambios de una obra específica.  
* Cada distribución mostrada en el historial indica la fuente, el reporte y la regla aplicada que originó el monto.  
* El historial de auditoría no puede ser modificado ni eliminado por ningún usuario del sistema.

# **6\. Mockup**

[https://www.figma.com/make/2rdkXhewKec4eAF1pgoBj3/Spanish-Language-Practice-App?t=z7Qb79JonI8zw2mf-1](https://www.figma.com/make/2rdkXhewKec4eAF1pgoBj3/Spanish-Language-Practice-App?t=z7Qb79JonI8zw2mf-1)

# **7\. Referencias**

\- Proyecto Aplicado en TICII \- Grupo 2 (documento base del equipo)

\- REDES SGC \- Reglamento de Distribución, Versión IX (27 mayo 2026): https://redescritores.com/reglamento-de-distribucion/

\- Declaración de Obra en línea: REDES-SYS (www.redescritores.com)

# 8\. Lenguaje obicuo 

* **Fuente:** Lugar donde fue publicada una obra   
* **Obra:** Producto realizado por alguien o una entidad con derechos de propiedad intelectual  
* **Titular:** Individuo que posee derechos de propiedad intelectual sobre una obra  
* **Ingreso:** Remuneración concedida a un titular ligada a sus porcentajes de participación y condiciones contractuales  
* **Propiedad intelectual:** Conjunto de derechos legales que protegen las creaciones de un individuo