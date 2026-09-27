-- NoDistribuido, PartesNoDistribuidas y PorGrupo (issue #162).
--
-- Desde #159, AvanzarEtapa persiste de verdad al entrar a importe_obra.
-- El motor ya calculaba estos tres campos (RD 16), pero resultados_proceso
-- no tenia donde guardarlos y GuardarResultado los tiraba en silencio: un
-- importe que el reglamento no reparte desaparecia del estado persistido.
--
-- NoDistribuido es un escalar, asi que cabe en una columna. Las otras dos
-- son listas -- ParteNoDistribuida y LineaGrupo -- y van en tablas propias,
-- igual que las lineas de obra y de titular: cada tramo es una cifra
-- auditable, no un documento que haya que parsear para cuadrar.
--
-- Numero: 00018, primero libre por encima de 00017. goose corre con
-- allowMissing = false; un numero libre por debajo de la version ya
-- aplicada aborta el despliegue.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE resultados_proceso
  ADD COLUMN no_distribuido NUMERIC(18,2) NOT NULL DEFAULT 0
    CHECK (no_distribuido >= 0);
-- +goose StatementEnd

-- El orden de la corrida no es el orden alfabetico del grupo: el motor
-- recorre GruposCanal() y anexar partes en ese orden es lo que ResultadoPorProceso
-- tiene que releer (ADR 0005). Por eso `orden` es parte de la identidad,
-- ademas del grupo cuando el grupo identifica la fila.
--
-- grupo y obra_id son NULL cuando el tramo no los trae. peso_cero del
-- motor no lleva grupo, y grupo_sin_obras no tiene obra. '' no serviria:
-- la FK a obras no admite un id vacio, y un CHECK IN (...) tampoco.
-- +goose StatementBegin
CREATE TABLE resultados_parte_no_distribuida (
  proceso_id TEXT NOT NULL REFERENCES resultados_proceso(proceso_id) ON DELETE CASCADE,
  orden      INT  NOT NULL CHECK (orden >= 0),
  motivo     TEXT NOT NULL CHECK (motivo IN (
               'grupo_sin_obras', 'exclusion_r27', 'peso_cero'
             )),
  grupo      TEXT CHECK (grupo IN (
               'privado_nacional', 'regional_publico', 'premium',
               'lideres_rating', 'estandar'
             )),
  obra_id    TEXT REFERENCES obras(id),
  importe    NUMERIC(18,2) NOT NULL CHECK (importe >= 0),
  PRIMARY KEY (proceso_id, orden),

  -- Sin obra, una exclusion R-27 no dice que repertorio se aparto (RD 16).
  CONSTRAINT parte_exclusion_tiene_obra
    CHECK (motivo <> 'exclusion_r27' OR obra_id IS NOT NULL),
  -- Sin grupo, "grupo sin obras" no dice de que bolsa salio el tramo.
  CONSTRAINT parte_grupo_sin_obras_tiene_grupo
    CHECK (motivo <> 'grupo_sin_obras' OR grupo IS NOT NULL)
);
-- +goose StatementEnd

-- Residuo del grupo es de redondeo, igual que resultados_proceso.residuo:
-- puede ser negativo. No lleva CHECK >= 0.
-- +goose StatementBegin
CREATE TABLE resultados_grupo (
  proceso_id   TEXT NOT NULL REFERENCES resultados_proceso(proceso_id) ON DELETE CASCADE,
  orden        INT  NOT NULL CHECK (orden >= 0),
  grupo        TEXT NOT NULL CHECK (grupo IN (
                 'privado_nacional', 'regional_publico', 'premium',
                 'lideres_rating', 'estandar'
               )),
  bolsa        NUMERIC(18,2) NOT NULL CHECK (bolsa >= 0),
  total_puntos NUMERIC(24,8) NOT NULL CHECK (total_puntos >= 0),
  valor_punto  NUMERIC(24,8) NOT NULL CHECK (valor_punto >= 0),
  residuo      NUMERIC(18,2) NOT NULL,
  PRIMARY KEY (proceso_id, grupo),
  UNIQUE (proceso_id, orden)
);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS resultados_parte_no_distribuida;
DROP TABLE IF EXISTS resultados_grupo;
ALTER TABLE resultados_proceso DROP COLUMN IF EXISTS no_distribuido;
-- +goose StatementEnd
