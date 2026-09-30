-- Version de la declaracion usada en cada linea de titular (issue #183, ADR 0006).
--
-- GET /explicar leia esa version del asiento de la obra
-- (DeclaracionAsentada.Version). Si ese asiento no la traia, el linaje no
-- podia decir que declaracion partio el importe. resultados_titular no la
-- guardaba, y rellenarla con 1 o con la vigente de hoy atribuiria un split
-- que esa corrida no uso.
--
-- La columna es NULL a proposito. Las filas ya escritas no se rellenan: no
-- hay forma honesta de saber que version usaron. Una linea nueva sin version
-- tampoco se inventa. La FK, cuando el valor esta, exige que esa version
-- exista para esa obra. Con MATCH SIMPLE un NULL no consulta la FK.
--
-- Numero: 00024, primero libre por encima de 00023. goose corre con
-- allowMissing = false; un numero libre por debajo de la version ya
-- aplicada aborta el despliegue.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE resultados_titular
  ADD COLUMN declaracion_version INT
    CHECK (declaracion_version IS NULL OR declaracion_version >= 1);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE resultados_titular
  ADD CONSTRAINT resultados_titular_declaracion_fkey
  FOREIGN KEY (obra_id, declaracion_version)
  REFERENCES declaracion_versiones (obra_id, version);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE resultados_titular DROP CONSTRAINT IF EXISTS resultados_titular_declaracion_fkey;
ALTER TABLE resultados_titular DROP COLUMN IF EXISTS declaracion_version;
-- +goose StatementEnd
