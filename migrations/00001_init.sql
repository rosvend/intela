-- Esquema inicial.
--
-- Formato goose: lo exige el stack (ADR 0010). Antes la API leia este fichero
-- entero y lo ejecutaba al arrancar, sin tabla de versiones y sin `down`. Eso
-- funciona mientras haya UNA migracion; con dos deja de funcionar en silencio.

-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pg_trgm;
-- btree_gist habilita el EXCLUDE de `parametros`: mezcla igualdad sobre texto
-- con solapamiento de rangos en un mismo indice.
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Padron
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE titulares (
  id              TEXT PRIMARY KEY,
  nombre          TEXT NOT NULL CHECK (btrim(nombre) <> ''),
  ipi             TEXT NOT NULL,
  -- R-01 / RD 4.5: solo un escritor persona natural recibe orden de pago.
  -- Una productora puede estar en el padron; no puede cobrar reparto.
  persona_natural BOOLEAN NOT NULL DEFAULT TRUE,
  clase           TEXT NOT NULL CHECK (clase IN ('socio', 'administrado')),
  email           TEXT NOT NULL DEFAULT '',
  CONSTRAINT titular_natural_tiene_ipi
    CHECK (NOT persona_natural OR btrim(ipi) <> '')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE usuarios (
  id            TEXT PRIMARY KEY,
  email         TEXT UNIQUE NOT NULL CHECK (email LIKE '%@%'),
  nombre        TEXT NOT NULL,
  rol           TEXT NOT NULL
    CHECK (rol IN ('administrador','distribucion','contabilidad','auditor','titular')),
  titular_id    TEXT REFERENCES titulares(id),
  password_hash TEXT NOT NULL CHECK (length(password_hash) >= 20),
  CONSTRAINT titular_tiene_titular_id
    CHECK (rol <> 'titular' OR titular_id IS NOT NULL)
);
-- +goose StatementEnd

-- Las sesiones EXPIRAN. Antes habia una columna `creada` que no leia nadie y
-- la comprobacion era solo que el token existiera: una sesion era una
-- credencial permanente que nadie podia revocar.
-- +goose StatementBegin
CREATE TABLE sesiones (
  token      TEXT PRIMARY KEY,
  usuario_id TEXT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
  creada     TIMESTAMPTZ NOT NULL DEFAULT now(),
  expira     TIMESTAMPTZ NOT NULL,
  CONSTRAINT sesion_expira_despues CHECK (expira > creada)
);
CREATE INDEX sesiones_expira  ON sesiones (expira);
CREATE INDEX sesiones_usuario ON sesiones (usuario_id);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Repertorio
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE obras (
  id     TEXT PRIMARY KEY,
  titulo TEXT NOT NULL CHECK (btrim(titulo) <> ''),
  ida    TEXT NOT NULL DEFAULT '',
  eidr   TEXT NOT NULL DEFAULT '',
  imdb   TEXT NOT NULL DEFAULT '',
  tipo   TEXT NOT NULL
    CHECK (tipo IN ('cinematografica','unitario','serie','telenovela','sketches'))
);
CREATE INDEX obras_titulo_trgm ON obras USING gin (titulo gin_trgm_ops);
-- Parciales: casi todas las filas tienen estos campos vacios, y el escalon 2
-- de la cascada busca por igualdad exacta sobre las que no.
CREATE INDEX obras_ida  ON obras (ida)  WHERE ida  <> '';
CREATE INDEX obras_eidr ON obras (eidr) WHERE eidr <> '';
CREATE INDEX obras_imdb ON obras (imdb) WHERE imdb <> '';
-- +goose StatementEnd

-- La declaracion de obra es la UNICA fuente de porcentajes de reparto (R-02).
-- Ni los reportes de los canales ni los contratos de escritura entran aqui.
--
-- Que las partes sumen exactamente 100 no se puede expresar como CHECK de
-- fila: es un invariante de agregado y vive en repertorio.Declaracion.Completa
-- (R-04, RD 13.1.3). Si no suma, se retiene el total en reserva.
-- +goose StatementBegin
CREATE TABLE declaraciones (
  obra_id    TEXT NOT NULL REFERENCES obras(id) ON DELETE CASCADE,
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  ipi        TEXT NOT NULL,
  porcentaje NUMERIC(8,4) NOT NULL CHECK (porcentaje > 0 AND porcentaje <= 100),
  -- Version de la declaracion usada en una corrida: la pregunta 5 de las siete
  -- del ADR 0006 es "que declaracion vigente se uso", y sin version no se
  -- puede responder anos despues.
  version    INT NOT NULL DEFAULT 1 CHECK (version >= 1),
  PRIMARY KEY (obra_id, titular_id)
);
CREATE INDEX declaraciones_titular ON declaraciones (titular_id);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Identificacion
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE alias_obra (
  fuente    TEXT NOT NULL,
  tipo_id   TEXT NOT NULL,
  valor     TEXT NOT NULL CHECK (btrim(valor) <> ''),
  obra_id   TEXT NOT NULL REFERENCES obras(id) ON DELETE CASCADE,
  quien     TEXT,
  aprendido TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (fuente, tipo_id, valor)
);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Ingesta
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE reportes (
  id           TEXT PRIMARY KEY,
  fuente       TEXT NOT NULL,
  periodo      TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  -- El sha256 en hexadecimal son exactamente 64 caracteres. Es la huella de
  -- la evidencia cruda: la pregunta 2 del ADR 0006 pide "la version exacta del
  -- archivo", no "el archivo de Caracol".
  sha256       TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  clave_objeto TEXT NOT NULL,
  nbytes       INT NOT NULL CHECK (nbytes > 0),
  creado       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (sha256, fuente)
);
CREATE INDEX reportes_periodo ON reportes (periodo);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE usos (
  id            TEXT PRIMARY KEY,
  reporte_id    TEXT NOT NULL REFERENCES reportes(id) ON DELETE CASCADE,
  fuente        TEXT NOT NULL,
  titulo        TEXT NOT NULL,
  ids_fuente    TEXT NOT NULL DEFAULT '',
  obra_id       TEXT REFERENCES obras(id),
  escalon       TEXT NOT NULL DEFAULT 'pendiente'
    CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni')),
  -- COMO se reconocio, no solo por que escalon paso. Es la pregunta 3 del ADR
  -- 0006. Antes se calculaba y se tiraba porque no habia columna donde ponerlo.
  evidencia     TEXT NOT NULL DEFAULT '',
  -- El puntaje del match difuso. La skill de trazabilidad es explicita: sin el
  -- puntaje "la cadena se rompe justo donde el auditor va a mirar".
  puntaje       NUMERIC(6,5) NOT NULL DEFAULT 0
    CHECK (puntaje >= 0 AND puntaje <= 1),
  resuelto_por  TEXT REFERENCES usuarios(id),
  resuelto_en   TIMESTAMPTZ,
  oni           BOOLEAN NOT NULL DEFAULT TRUE,
  modalidad     TEXT NOT NULL CHECK (modalidad IN ('tv','cine','ott','hotel')),
  tipo_obra     TEXT NOT NULL DEFAULT '',

  -- NO HAY COLUMNA DE DINERO, y no la va a haber. Un reporte de uso PONDERA la
  -- bolsa, no la aporta. Una columna de importe aqui hace expresable en SQL la
  -- suma de dinero por fila, que es justo lo que el reglamento no permite.
  duracion_min   NUMERIC(12,4) NOT NULL DEFAULT 0 CHECK (duracion_min   >= 0),
  emisiones      BIGINT        NOT NULL DEFAULT 1 CHECK (emisiones      >= 0),
  rating         NUMERIC(12,6) NOT NULL DEFAULT 0 CHECK (rating         >= 0),
  taquilla       NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (taquilla       >= 0),
  vistas         NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (vistas         >= 0),
  minutos_vistos NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (minutos_vistos >= 0),
  pb             NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (pb             >= 0),

  -- Un uso resuelto tiene obra; uno en ONI no la tiene. Sin esto se puede
  -- guardar una fila que dice "identificada" y no apunta a ninguna obra.
  CONSTRAINT uso_resuelto_tiene_obra
    CHECK ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL)),
  -- Una resolucion manual tiene que decir quien la tomo y cuando (pregunta 3).
  CONSTRAINT manual_tiene_autor
    CHECK (escalon <> 'manual' OR (resuelto_por IS NOT NULL AND resuelto_en IS NOT NULL))
);
CREATE INDEX usos_reporte    ON usos (reporte_id);
CREATE INDEX usos_obra       ON usos (obra_id) WHERE obra_id IS NOT NULL;
CREATE INDEX usos_pendientes ON usos (id) WHERE escalon = 'pendiente';
CREATE INDEX usos_oni        ON usos (id) WHERE oni;
-- +goose StatementEnd

-- Listado publico de ONI (R-18, RD 13.8.1 a 13.8.4).
--
-- Lleva titulo e informacion identificatoria y NUNCA montos: la informacion
-- economica se mantiene en reserva. Trazabilidad interna completa no es lo
-- mismo que publicidad, asi que la vista no expone ni una columna de dinero
-- y no puede empezar a exponerla por descuido en un handler.
-- +goose StatementBegin
CREATE VIEW oni_publico AS
  SELECT u.id, u.titulo, u.fuente, u.ids_fuente, u.modalidad, r.periodo
  FROM usos u
  JOIN reportes r ON r.id = u.reporte_id
  WHERE u.oni;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Recaudo
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE bolsas (
  id         TEXT PRIMARY KEY,
  usuario_id TEXT NOT NULL,
  periodo    TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  circuito   TEXT NOT NULL CHECK (circuito IN ('nacional','internacional')),
  bruto      NUMERIC(18,2) NOT NULL CHECK (bruto >= 0),
  -- Pregunta 1 del ADR 0006: de donde salio la bolsa.
  convenio   TEXT NOT NULL DEFAULT '',
  tarifa     TEXT NOT NULL DEFAULT '',
  factura    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX bolsas_periodo ON bolsas (periodo);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Parametros normativos (ADR 0004)
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE parametros (
  clave         TEXT NOT NULL,
  valor         NUMERIC(18,6) NOT NULL,
  vigente_desde DATE NOT NULL,
  vigente_hasta DATE,
  organo        TEXT NOT NULL CHECK (btrim(organo) <> ''),
  reglamento    TEXT NOT NULL CHECK (btrim(reglamento) <> ''),

  PRIMARY KEY (clave, vigente_desde),
  CONSTRAINT parametro_vigencia_coherente
    CHECK (vigente_hasta IS NULL OR vigente_hasta > vigente_desde),

  -- Dos filas de la misma clave no pueden solaparse en el tiempo.
  --
  -- Sin esto, "el valor vigente en la fecha del periodo" puede devolver dos
  -- filas y la corrida deja de ser determinista: el resultado dependeria del
  -- orden en que la base decida leerlas. El ADR 0005 pide reproducibilidad bit
  -- a bit anos despues, y eso empieza porque la pregunta tenga una sola
  -- respuesta.
  CONSTRAINT parametro_sin_solape EXCLUDE USING gist (
    clave WITH =,
    daterange(vigente_desde, vigente_hasta, '[)') WITH &&
  )
);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Proceso de reparto (RD 13.5)
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE procesos (
  id          TEXT PRIMARY KEY,
  circuito    TEXT NOT NULL CHECK (circuito IN ('nacional','internacional')),
  etapa       TEXT NOT NULL CHECK (etapa IN (
                'recaudo','deducciones','importe_obra','importe_titular',
                'liquidacion_parcial','verificacion','liquidacion_final',
                'pago_registro','fees_in_error','auditoria')),
  periodo     TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  bolsa_id    TEXT NOT NULL REFERENCES bolsas(id),

  -- Procedencia (pregunta 4 del ADR 0006). El snapshot se congela AL ABRIR el
  -- proceso y se referencia desde aqui; recalcular lee este, no vuelve a
  -- resolver contra el reloj. Sin esta columna, cambiar un parametro cambiaba
  -- en silencio el resultado de una corrida ya hecha.
  snapshot_id TEXT,
  reglamento  TEXT NOT NULL DEFAULT '',

  revision    INT NOT NULL DEFAULT 1 CHECK (revision >= 1),
  rechazo     TEXT NOT NULL DEFAULT '',
  abierto     TIMESTAMPTZ NOT NULL DEFAULT now(),

  -- El circuito internacional no valoriza por puntos (RD 7.4), asi que no
  -- pasa por importe por obra ni por titular. "Fees in Error" es solo suyo
  -- (R-16, RD 13.7).
  CONSTRAINT etapa_valida_para_circuito CHECK (
    circuito <> 'internacional'
    OR etapa NOT IN ('importe_obra','importe_titular')
  ),
  CONSTRAINT fees_in_error_solo_internacional CHECK (
    etapa <> 'fees_in_error' OR circuito = 'internacional'
  )
);
CREATE INDEX procesos_periodo ON procesos (periodo);
CREATE INDEX procesos_etapa   ON procesos (etapa);
-- +goose StatementEnd

-- La PK que hace real la doble firma (pregunta 7 del ADR 0006).
--
-- Sin ella nada impedia que el mismo actor firmara dos veces la misma
-- compuerta, con lo que el control de dos roles distintos dejaba de ser un
-- control. Que la clave incluya `revision` es deliberado: un rechazo sube la
-- revision y las firmas de la anterior dejan de contar.
-- +goose StatementBegin
CREATE TABLE firmas (
  proceso_id TEXT NOT NULL REFERENCES procesos(id) ON DELETE CASCADE,
  rol        TEXT NOT NULL CHECK (rol IN ('distribucion','contabilidad')),
  revision   INT NOT NULL CHECK (revision >= 1),
  actor_id   TEXT NOT NULL REFERENCES usuarios(id),
  cuando     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (proceso_id, rol, revision),

  -- Y un actor no puede cubrir los dos roles de la misma compuerta: eso es
  -- exactamente lo que la doble firma existe para impedir.
  CONSTRAINT firma_actor_unico_por_revision
    UNIQUE (proceso_id, revision, actor_id)
);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Resultados
-- ---------------------------------------------------------------------------

-- Totales de una corrida, con la invariante de cierre comprobada por la base
-- (pregunta 6 del ADR 0006: bruto, cada deduccion, reserva y neto).
-- +goose StatementBegin
CREATE TABLE resultados_proceso (
  proceso_id  TEXT PRIMARY KEY REFERENCES procesos(id) ON DELETE CASCADE,
  bruto       NUMERIC(18,2) NOT NULL CHECK (bruto   >= 0),
  admin       NUMERIC(18,2) NOT NULL CHECK (admin   >= 0),
  social      NUMERIC(18,2) NOT NULL CHECK (social  >= 0),
  reserva     NUMERIC(18,2) NOT NULL CHECK (reserva >= 0),
  neto        NUMERIC(18,2) NOT NULL CHECK (neto    >= 0),
  -- Lo de las obras cuya declaracion no suma 100%. Retener es MOVER A RESERVA:
  -- sin esta columna el importe no tenia donde ir y desaparecia (R-04).
  retenido    NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (retenido >= 0),
  -- El residuo de redondeo, explicito. El ADR 0005 lo exige; antes no se
  -- registraba y lo absorbia la ultima linea del orden alfabetico.
  residuo     NUMERIC(18,2) NOT NULL DEFAULT 0,
  valor_punto NUMERIC(24,8) NOT NULL DEFAULT 0 CHECK (valor_punto >= 0),

  snapshot_id TEXT NOT NULL,
  reglamento  TEXT NOT NULL,
  calculado   TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT deducciones_cuadran
    CHECK (neto = bruto - admin - social - reserva)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE resultados_obra (
  proceso_id TEXT NOT NULL REFERENCES procesos(id) ON DELETE CASCADE,
  obra_id    TEXT NOT NULL REFERENCES obras(id),
  puntos     NUMERIC(24,8) NOT NULL CHECK (puntos  >= 0),
  importe    NUMERIC(18,2) NOT NULL CHECK (importe >= 0),
  retenida   BOOLEAN NOT NULL,
  motivo     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (proceso_id, obra_id),
  CONSTRAINT retenida_tiene_motivo
    CHECK (NOT retenida OR btrim(motivo) <> '')
);
-- +goose StatementEnd

-- Antes no tenia PK: se podia insertar dos veces la misma linea de titular
-- para la misma obra y el mismo proceso, y el total dejaba de cuadrar sin que
-- nada lo detectara.
-- +goose StatementBegin
CREATE TABLE resultados_titular (
  proceso_id TEXT NOT NULL REFERENCES procesos(id) ON DELETE CASCADE,
  obra_id    TEXT NOT NULL REFERENCES obras(id),
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  ipi        TEXT NOT NULL CHECK (btrim(ipi) <> ''),
  porcentaje NUMERIC(8,4)  NOT NULL CHECK (porcentaje > 0 AND porcentaje <= 100),
  importe    NUMERIC(18,2) NOT NULL CHECK (importe >= 0),
  PRIMARY KEY (proceso_id, obra_id, titular_id),
  FOREIGN KEY (proceso_id, obra_id)
    REFERENCES resultados_obra (proceso_id, obra_id) ON DELETE CASCADE
);
CREATE INDEX resultados_titular_titular ON resultados_titular (titular_id);
-- +goose StatementEnd

-- R-01 / RD 4.5: no existe orden de pago a quien no sea escritor persona
-- natural. Va como trigger y no como CHECK porque mira otra tabla. Es la
-- ultima barrera antes de que el dinero salga.
-- +goose StatementBegin
CREATE FUNCTION exigir_persona_natural() RETURNS TRIGGER AS $fn$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM titulares
    WHERE id = NEW.titular_id AND persona_natural
  ) THEN
    RAISE EXCEPTION
      'R-01 (RD 4.5): % no es escritor persona natural, no puede recibir orden de pago',
      NEW.titular_id;
  END IF;
  RETURN NEW;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER resultados_titular_persona_natural
  BEFORE INSERT OR UPDATE ON resultados_titular
  FOR EACH ROW EXECUTE FUNCTION exigir_persona_natural();
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Bitacora (ADR 0006)
-- ---------------------------------------------------------------------------

-- El asiento tiene que responder las siete preguntas del ADR 0006 sin
-- recalcular nada: de donde salio la bolsa, que reportes la ponderaron, como
-- se reconocio la obra, que reglas se aplicaron, como se dividio, que se
-- dedujo y quien aprobo.
--
-- Las columnas fijas son el indice; el `payload` JSONB lleva el detalle del
-- hecho concreto. Retencion minima de diez anos (RD 13.2, RD 13.4).
-- +goose StatementBegin
CREATE TABLE asientos (
  -- UUID generado por la base, no un id derivado del hecho.
  --
  -- Con ids deterministas del tipo 'as-calc-' || proceso_id y un
  -- ON CONFLICT DO NOTHING, recalcular el mismo proceso NO dejaba un segundo
  -- asiento: un libro append-only que descarta appends no es append-only.
  id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  hecho    TEXT NOT NULL CHECK (btrim(hecho) <> ''),
  ref_tipo TEXT NOT NULL,
  ref_id   TEXT NOT NULL,
  actor_id TEXT REFERENCES usuarios(id),
  payload  JSONB NOT NULL DEFAULT '{}'::jsonb,
  cuando   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX asientos_ref     ON asientos (ref_tipo, ref_id);
CREATE INDEX asientos_cuando  ON asientos (cuando DESC);
CREATE INDEX asientos_hecho   ON asientos (hecho);
CREATE INDEX asientos_payload ON asientos USING gin (payload);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION bitacora_solo_append() RETURNS TRIGGER AS $fn$
BEGIN
  -- TG_TABLE_NAME y no un literal: esta funcion la comparten asientos y
  -- notificaciones, y un mensaje que nombre la tabla equivocada manda a quien
  -- lo lea a mirar donde no es.
  RAISE EXCEPTION
    'ADR 0006: la bitacora es append-only. % sobre % no esta permitido. Corregir es escribir otro asiento que referencie al anterior',
    TG_OP, TG_TABLE_NAME;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Append-only EXIGIDO POR EL MOTOR, no por convenio.
--
-- Antes era una promesa en la documentacion: cualquier UPDATE o DELETE pasaba.
-- La disciplina de no borrar no puede depender de que nadie escriba la
-- sentencia; basta una migracion apurada para perder la propiedad, y nadie lo
-- nota hasta que llega la auditoria.
-- +goose StatementBegin
CREATE TRIGGER asientos_inmutables
  BEFORE UPDATE OR DELETE ON asientos
  FOR EACH ROW EXECUTE FUNCTION bitacora_solo_append();
-- +goose StatementEnd

-- Y TRUNCATE aparte, porque un trigger FOR EACH ROW no se dispara con el.
--
-- TRUNCATE no borra fila por fila, asi que el trigger de arriba no lo ve:
-- vaciaria la bitacora entera sin que nada protestara. Tiene que ser
-- FOR EACH STATEMENT; no existe TRUNCATE por fila.
-- +goose StatementBegin
CREATE TRIGGER asientos_sin_truncate
  BEFORE TRUNCATE ON asientos
  FOR EACH STATEMENT EXECUTE FUNCTION bitacora_solo_append();
-- +goose StatementEnd

-- Notificaciones: el acuse ARRANCA EL RELOJ DE PRESCRIPCION.
--
-- Notificar no es enviar un correo, es el hecho juridico que empieza a contar
-- los diez anos de RD 15.1. Por eso se persiste por titular y por corrida, y
-- por eso el puerto Notificador devuelve un acuse en vez de nada.
--
-- Son validas las dos vias de R-21 / RD 13.8.8: envio al correo que el socio
-- informo, o puesta a disposicion en la pagina web. Ambas producen acuse.
-- +goose StatementBegin
CREATE TABLE notificaciones (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  proceso_id TEXT NOT NULL REFERENCES procesos(id),
  via        TEXT NOT NULL CHECK (via IN ('email','portal')),
  destino    TEXT NOT NULL,
  acuse      TEXT NOT NULL CHECK (btrim(acuse) <> ''),
  notificado TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (titular_id, proceso_id, via)
);
CREATE INDEX notificaciones_titular ON notificaciones (titular_id);
CREATE INDEX notificaciones_fecha   ON notificaciones (notificado);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notificaciones_inmutables
  BEFORE UPDATE OR DELETE ON notificaciones
  FOR EACH ROW EXECUTE FUNCTION bitacora_solo_append();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notificaciones_sin_truncate
  BEFORE TRUNCATE ON notificaciones
  FOR EACH STATEMENT EXECUTE FUNCTION bitacora_solo_append();
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Operacion
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE cola_trabajos (
  id       BIGSERIAL PRIMARY KEY,
  tipo     TEXT NOT NULL,
  payload  JSONB NOT NULL DEFAULT '{}'::jsonb,
  estado   TEXT NOT NULL DEFAULT 'pendiente'
    CHECK (estado IN ('pendiente','en_curso','hecho','fallido')),
  error    TEXT NOT NULL DEFAULT '',
  intentos INT NOT NULL DEFAULT 0 CHECK (intentos >= 0),
  creado   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- El worker toma trabajos con SELECT ... FOR UPDATE SKIP LOCKED sobre esto.
CREATE INDEX cola_pendientes ON cola_trabajos (id) WHERE estado = 'pendiente';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE calendario (
  periodo        TEXT PRIMARY KEY CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  fecha_apertura DATE NOT NULL,
  disparado      BOOLEAN NOT NULL DEFAULT FALSE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE anticipos (
  id         TEXT PRIMARY KEY,
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  monto      NUMERIC(18,2) NOT NULL CHECK (monto > 0),
  estado     TEXT NOT NULL
    CHECK (estado IN ('solicitado','aprobado','rechazado','descontado')),
  -- RA 3.2.5: las aprobaciones viven en un repositorio electronico con su
  -- ubicacion referenciada, "con el fin de garantizar trazabilidad".
  clave_soporte TEXT NOT NULL DEFAULT '',
  solicitado TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX anticipos_titular ON anticipos (titular_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE reclamaciones (
  id         TEXT PRIMARY KEY,
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  proceso_id TEXT REFERENCES procesos(id),
  detalle    TEXT NOT NULL CHECK (btrim(detalle) <> ''),
  estado     TEXT NOT NULL
    CHECK (estado IN ('abierta','en_estudio','resuelta','rechazada')),
  abierta    TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- R-22 / RD 14.3 da quince dias habiles para responder. La fecha de apertura
  -- es lo que hace medible ese plazo.
  resuelta   TIMESTAMPTZ
);
CREATE INDEX reclamaciones_titular ON reclamaciones (titular_id);
CREATE INDEX reclamaciones_abiertas ON reclamaciones (abierta)
  WHERE estado IN ('abierta','en_estudio');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW IF EXISTS oni_publico;

DROP TRIGGER IF EXISTS notificaciones_sin_truncate ON notificaciones;
DROP TRIGGER IF EXISTS notificaciones_inmutables ON notificaciones;
DROP TRIGGER IF EXISTS asientos_sin_truncate ON asientos;
DROP TRIGGER IF EXISTS asientos_inmutables ON asientos;
DROP TRIGGER IF EXISTS resultados_titular_persona_natural ON resultados_titular;
DROP FUNCTION IF EXISTS bitacora_solo_append();
DROP FUNCTION IF EXISTS exigir_persona_natural();

DROP TABLE IF EXISTS reclamaciones, anticipos, calendario, cola_trabajos,
  notificaciones, asientos, resultados_titular, resultados_obra,
  resultados_proceso, firmas, procesos, parametros, bolsas, usos, reportes,
  alias_obra, declaraciones, obras, sesiones, usuarios, titulares CASCADE;
-- +goose StatementEnd
