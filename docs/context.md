# **Gestor de Reconocimiento y Distribución de Ingresos por Propiedad Intelectual**

## **1. Contexto del problema**

Los titulares de derechos de propiedad intelectual (autores, compositores, ilustradores, editoriales, sellos, plataformas de contenido, etc.) suelen tener sus obras explotadas simultáneamente a través de múltiples fuentes: plataformas de streaming, tiendas digitales, licenciamientos, reportes de terceros y acuerdos de distribución. Cada fuente entrega la información en formatos distintos, con nombres de obras, autores y códigos de identificación que no siempre coinciden entre sí.

Esto genera una brecha operativa importante: identificar qué reportes corresponden a qué obra, calcular cuánto le corresponde a cada titular según sus porcentajes de participación, y distribuir esos ingresos de forma correcta, oportuna y auditable es, en la práctica, un proceso manual, lento y propenso a errores (obras no reconocidas, pagos duplicados o faltantes, reclamos de titulares por falta de transparencia).

La disponibilidad de técnicas de resolución de entidades (entity matching/deduplicación), procesamiento de datos a gran escala y motores de reglas de negocio permite construir un sistema que automatice buena parte de este proceso, manteniendo trazabilidad completa para fines de auditoría y confianza de los titulares.

## **2. Necesidad identificada**

Se requiere un software gestor de datos que:

- Ingiera reportes de ingresos y uso de obras provenientes de múltiples fuentes (streaming, ventas digitales, licenciamientos, reportes de terceros), en distintos formatos (CSV, Excel, JSON, APIs).
- Reconozca y asocie cada registro de uso/ingreso con la obra correcta dentro de un catálogo maestro, incluso cuando existan variaciones de nombre, metadatos incompletos o códigos distintos entre fuentes (matching difuso / deduplicación).
- Mantenga un registro de titulares por obra, con sus porcentajes de participación y condiciones contractuales (splits, vigencias, exclusividades).
- Calcule automáticamente los ingresos acumulados por obra, por fuente y por periodo.
- Distribuya (reparta) los ingresos entre los titulares de cada obra según las reglas de participación definidas.
- Detecte anomalías: obras no reconocidas, reportes duplicados, inconsistencias entre fuentes o titulares sin porcentajes definidos.
- Genere reportes transparentes y auditables por titular, obra, fuente y periodo.
- Ofrezca un panel para que administradores y titulares consulten el detalle de sus ingresos y el origen de cada monto.

## **3. Objetivo general**

Diseñar y desarrollar un sistema de gestión de datos que reconozca obras de propiedad intelectual a partir de múltiples fuentes de reportes de uso e ingresos, las asocie a un catálogo maestro de titulares y derechos, y calcule y redistribuya los ingresos correspondientes de forma automatizada, transparente y auditable.

## **4. Objetivos específicos**

- **Ingesta y normalización de datos**

- Conectar y cargar reportes de al menos dos o tres fuentes distintas (reales o simuladas: streaming, ventas, licenciamiento).
- Normalizar formatos, monedas, periodos y estructuras de cada fuente a un esquema común.

- **Catálogo maestro y resolución de entidades**

- Diseñar un catálogo maestro de obras y titulares con identificadores únicos.
- Implementar técnicas de matching difuso/deduplicación para asociar registros de las distintas fuentes con la obra correcta, manejando variaciones de nombre y metadatos incompletos.

- **Modelo de derechos y reparto (splits)**

- Modelar los titulares de cada obra y sus porcentajes de participación, incluyendo vigencias y condiciones contractuales.
- Permitir la actualización y versionado de acuerdos de reparto a lo largo del tiempo.

- **Motor de cálculo y distribución de ingresos**

- Calcular los ingresos acumulados por obra, fuente y periodo a partir de los datos normalizados.
- Distribuir los ingresos entre titulares según las reglas de participación vigentes en cada periodo.

- **Conciliación y detección de anomalías**

- Detectar obras no reconocidas, reportes duplicados o inconsistentes, y titulares sin porcentajes definidos.
- Generar alertas y un flujo de resolución manual para los casos que el sistema no pueda resolver automáticamente.

- **Interfaz y reportería**

- Construir un panel (dashboard) donde administradores y titulares puedan consultar ingresos por obra, fuente y periodo.
- Generar reportes exportables (p. ej., PDF/Excel) por titular para fines de pago y auditoría.

- **Trazabilidad y auditoría**

- Registrar el origen de cada monto calculado (fuente, reporte, regla de reparto aplicada) para que cualquier cifra sea explicable.
- Mantener un historial de cambios sobre el catálogo, los acuerdos de reparto y las distribuciones realizadas.

- **Despliegue reproducible**

- Empaquetar la solución con Docker/Docker Compose.
- Proveer datos de ejemplo (obras, titulares y reportes simulados) y scripts de inicialización ("seed").

## **5. Alcance**

**_Incluido_**

- Ingesta de al menos 2-3 fuentes de datos (reales o simuladas) con formatos distintos.
- Catálogo maestro de obras y titulares, con pipeline de resolución de entidades/deduplicación.
- Modelo de derechos y reparto con porcentajes de participación versionables.
- Motor de cálculo y distribución de ingresos por obra, fuente y periodo.
- Mecanismo de detección de anomalías y flujo de resolución manual.
- Dashboard funcional para administradores y titulares, con reportes exportables.
- Registro auditable del origen de cada cifra distribuida.
- Dockerización y guía de despliegue.

### **Usuarios finales:**

Titulares de derechos de propiedad intelectual (autores, compositores, ilustradores, editoriales o sellos) y administradores encargados de la gestión y distribución de sus ingresos.

### **Equipo desarrollador:**

Estudiantes de ingeniería en ciencia de datos, aplicando integración de datos, resolución de entidades/deduplicación, modelado de reglas de negocio y diseño de dashboards.

## **6. Impacto esperado**

- Mayor transparencia y confianza de los titulares respecto al origen y cálculo de sus ingresos.
- Reducción de errores manuales en el reconocimiento de obras y en el reparto de ingresos.
- Menor tiempo de conciliación entre múltiples fuentes de reportes.
- Escalabilidad para incorporar nuevas fuentes de ingresos o nuevos titulares sin rediseñar el sistema.
- Aprendizaje práctico del equipo desarrollador en integración de datos, resolución de entidades y sistemas financieros/de reparto.