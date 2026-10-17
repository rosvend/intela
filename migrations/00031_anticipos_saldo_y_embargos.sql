-- Anticipos a socios (#56): saldo, resolucion del Consejo y embargos reportados
-- (R-30..R-33, ADR 0027).
--
-- COORDINACION DE NUMERO: 00031, primero libre sobre 00030 en main
-- (`00030_reglamento_secciones.sql`, #217). goose corre con
-- `allowMissing = false`: si otra rama lo toma antes, pasar al primero libre.
--
-- `anticipos` existe desde 00001 como andamiaje y NINGUN codigo la escribia
-- hasta esta migracion, asi que en una base desplegada esta vacia. Aun asi el
-- Up no da eso por supuesto: lo que hubiera aprobado se queda debiendo todo lo
-- otorgado (no hay rastro de descuentos anteriores, y suponer cero le giraria
-- al socio lo que debe), y si alguna fila no cumple las restricciones nuevas
-- la migracion FALLA en vez de crearlas NOT VALID. Una restriccion sin validar
-- deja pasar en silencio justo las filas que habria que mirar.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE anticipos
  -- Lo que falta por cubrir con las liquidaciones que siguen (R-33, RA 2.1).
  -- Nace en la aprobacion con todo lo otorgado y baja con cada descuento.
  ADD COLUMN saldo NUMERIC(18,2) NOT NULL DEFAULT 0,
  -- Destino de los recursos y necesidad (RA 3.2.1.d y e, RA 2.3).
  ADD COLUMN justificacion TEXT NOT NULL DEFAULT '',
  -- La justificacion literal de una negacion (RA 3.2.3).
  ADD COLUMN motivo_rechazo TEXT NOT NULL DEFAULT '',
  -- Cuando y quien del Consejo Directivo decidio (RA 3.2.3). NULL mientras
  -- el anticipo sigue en tramite.
  ADD COLUMN resuelto TIMESTAMPTZ,
  ADD COLUMN resuelto_por TEXT REFERENCES usuarios(id);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE anticipos SET saldo = monto WHERE estado = 'aprobado';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE anticipos
  ADD CONSTRAINT anticipo_saldo_acotado CHECK (saldo >= 0 AND saldo <= monto),
  -- Hay saldo si y solo si el anticipo esta aprobado. Una solicitud no debe
  -- nada todavia (RA 3.1.b: pedir no es recibir), un rechazado nunca se giro y
  -- uno descontado ya se cubrio: el descuento que deja el saldo en cero es el
  -- que lo pasa a `descontado`. Es lo que hace que "aprobado" y "con saldo
  -- pendiente" sean la misma pregunta para R-32.
  ADD CONSTRAINT anticipo_saldo_solo_aprobado CHECK ((estado = 'aprobado') = (saldo > 0));
-- +goose StatementEnd

-- R-32 (RA 3.1.c y 3.1.d) en la base: un solo anticipo vigente -en tramite o
-- aprobado con saldo- por titular. El caso de uso lo comprueba antes, pero dos
-- solicitudes simultaneas del mismo socio pasarian las dos esa comprobacion; la
-- unica que no tiene carrera es la que hace el indice.
-- +goose StatementBegin
CREATE UNIQUE INDEX anticipos_uno_vigente_por_titular
  ON anticipos (titular_id)
  WHERE estado IN ('solicitado', 'aprobado');
-- +goose StatementEnd

-- RA 3.1.e: "No podran hacer desembolsos de anticipos a los afiliados que
-- tengan embargos reportados". Una fila por oficio, con su vigencia en dias
-- civiles, y no un booleano en `titulares`: la pregunta que hace una auditoria
-- (RD 16) es si el socio estaba embargado EL DIA en que se aprobo, y un
-- booleano solo sabe contestar por hoy. `soporte` es la ubicacion del oficio en
-- el repositorio documental, igual que `anticipos.clave_soporte` (RA 3.2.5).
--
-- Quien reporta y quien levanta un embargo no tiene caso de uso todavia (ver
-- ADR 0027): hoy esta tabla se escribe a mano. Lo que si queda es que la regla
-- la lea.
-- +goose StatementBegin
CREATE TABLE embargos (
  id         BIGSERIAL PRIMARY KEY,
  titular_id TEXT NOT NULL REFERENCES titulares(id),
  reportado  DATE NOT NULL,
  levantado  DATE,
  soporte    TEXT NOT NULL CHECK (btrim(soporte) <> ''),
  CONSTRAINT embargo_vigencia_coherente
    CHECK (levantado IS NULL OR levantado > reportado)
);
CREATE INDEX embargos_titular ON embargos (titular_id);
-- +goose StatementEnd

-- +goose Down

-- Con un anticipo o un embargo registrado, el down se niega en vez de borrar
-- lo que no se puede reconstruir, igual que el de 00025. Bajar soltaria el
-- saldo, la justificacion, el motivo y quien resolvio, y el Up siguiente
-- volveria a poner cada aprobado en `saldo = monto`: lo ya descontado se le
-- cobraria otra vez al socio (R-33). Y sin la tabla `embargos` desaparecen los
-- oficios que impiden desembolsar (RA 3.1.e).
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM anticipos) THEN
    RAISE EXCEPTION
      'R-33: hay anticipos registrados; revertir 00031 les borraria el saldo y volver a subirla cobraria otra vez lo ya descontado';
  END IF;
  IF EXISTS (SELECT 1 FROM embargos) THEN
    RAISE EXCEPTION
      'RA 3.1.e: hay embargos reportados; revertir 00031 borraria los oficios que impiden desembolsar un anticipo';
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS embargos;
DROP INDEX IF EXISTS anticipos_uno_vigente_por_titular;
ALTER TABLE anticipos
  DROP CONSTRAINT IF EXISTS anticipo_saldo_solo_aprobado,
  DROP CONSTRAINT IF EXISTS anticipo_saldo_acotado,
  DROP COLUMN IF EXISTS resuelto_por,
  DROP COLUMN IF EXISTS resuelto,
  DROP COLUMN IF EXISTS motivo_rechazo,
  DROP COLUMN IF EXISTS justificacion,
  DROP COLUMN IF EXISTS saldo;
-- +goose StatementEnd
