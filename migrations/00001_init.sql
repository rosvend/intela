CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS titulares (
  id TEXT PRIMARY KEY,
  nombre TEXT NOT NULL,
  ipi TEXT NOT NULL,
  persona_natural BOOLEAN NOT NULL DEFAULT TRUE,
  clase TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS usuarios (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  nombre TEXT NOT NULL,
  rol TEXT NOT NULL,
  titular_id TEXT REFERENCES titulares(id),
  password_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sesiones (
  token TEXT PRIMARY KEY,
  usuario_id TEXT NOT NULL REFERENCES usuarios(id),
  creada TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS obras (
  id TEXT PRIMARY KEY,
  titulo TEXT NOT NULL,
  ida TEXT NOT NULL DEFAULT '',
  eidr TEXT NOT NULL DEFAULT '',
  imdb TEXT NOT NULL DEFAULT '',
  tipo TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS declaraciones (
  obra_id TEXT NOT NULL REFERENCES obras(id),
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  ipi TEXT NOT NULL,
  porcentaje NUMERIC(8,4) NOT NULL,
  PRIMARY KEY (obra_id, titular_id)
);

CREATE TABLE IF NOT EXISTS alias_obra (
  fuente TEXT NOT NULL,
  tipo_id TEXT NOT NULL,
  valor TEXT NOT NULL,
  obra_id TEXT NOT NULL REFERENCES obras(id),
  quien TEXT,
  PRIMARY KEY (fuente, tipo_id, valor)
);

CREATE TABLE IF NOT EXISTS reportes (
  id TEXT PRIMARY KEY,
  fuente TEXT NOT NULL,
  periodo TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  clave_objeto TEXT NOT NULL,
  nbytes INT NOT NULL,
  creado TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS usos (
  id TEXT PRIMARY KEY,
  reporte_id TEXT NOT NULL REFERENCES reportes(id),
  fuente TEXT NOT NULL,
  titulo TEXT NOT NULL,
  ids_fuente TEXT NOT NULL DEFAULT '',
  obra_id TEXT,
  escalon TEXT NOT NULL DEFAULT 'pendiente',
  oni BOOLEAN NOT NULL DEFAULT TRUE,
  modalidad TEXT NOT NULL,
  tipo_obra TEXT NOT NULL DEFAULT '',
  duracion_min NUMERIC(12,4) NOT NULL DEFAULT 0,
  emisiones BIGINT NOT NULL DEFAULT 1,
  rating NUMERIC(12,6) NOT NULL DEFAULT 0,
  taquilla NUMERIC(18,2) NOT NULL DEFAULT 0,
  vistas NUMERIC(18,2) NOT NULL DEFAULT 0,
  minutos_vistos NUMERIC(18,4) NOT NULL DEFAULT 0,
  pb NUMERIC(18,4) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS bolsas (
  id TEXT PRIMARY KEY,
  usuario_id TEXT NOT NULL,
  periodo TEXT NOT NULL,
  circuito TEXT NOT NULL,
  bruto NUMERIC(18,2) NOT NULL
);

CREATE TABLE IF NOT EXISTS parametros (
  clave TEXT NOT NULL,
  valor NUMERIC(18,6) NOT NULL,
  vigente_desde DATE NOT NULL,
  vigente_hasta DATE,
  organo TEXT NOT NULL,
  reglamento TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS procesos (
  id TEXT PRIMARY KEY,
  circuito TEXT NOT NULL,
  etapa TEXT NOT NULL,
  periodo TEXT NOT NULL,
  bolsa_id TEXT NOT NULL REFERENCES bolsas(id),
  revision INT NOT NULL DEFAULT 1,
  rechazo TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS firmas (
  proceso_id TEXT NOT NULL REFERENCES procesos(id),
  rol TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  revision INT NOT NULL,
  cuando TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS resultados_obra (
  proceso_id TEXT NOT NULL,
  obra_id TEXT NOT NULL,
  puntos NUMERIC(24,8) NOT NULL,
  importe NUMERIC(18,2) NOT NULL,
  retenida BOOLEAN NOT NULL,
  motivo TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (proceso_id, obra_id)
);

CREATE TABLE IF NOT EXISTS resultados_titular (
  proceso_id TEXT NOT NULL,
  obra_id TEXT NOT NULL,
  titular_id TEXT NOT NULL,
  ipi TEXT NOT NULL,
  porcentaje NUMERIC(8,4) NOT NULL,
  importe NUMERIC(18,2) NOT NULL
);

CREATE TABLE IF NOT EXISTS asientos (
  id TEXT PRIMARY KEY,
  hecho TEXT NOT NULL,
  ref_tipo TEXT NOT NULL,
  ref_id TEXT NOT NULL,
  payload TEXT NOT NULL,
  cuando TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cola_trabajos (
  id BIGSERIAL PRIMARY KEY,
  tipo TEXT NOT NULL,
  payload TEXT NOT NULL,
  estado TEXT NOT NULL DEFAULT 'pendiente',
  error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS calendario (
  periodo TEXT PRIMARY KEY,
  fecha_apertura DATE NOT NULL,
  disparado BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS anticipos (
  id TEXT PRIMARY KEY,
  titular_id TEXT NOT NULL,
  monto NUMERIC(18,2) NOT NULL,
  estado TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS reclamaciones (
  id TEXT PRIMARY KEY,
  titular_id TEXT NOT NULL,
  detalle TEXT NOT NULL,
  estado TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS obras_titulo_trgm ON obras USING gin (titulo gin_trgm_ops);
