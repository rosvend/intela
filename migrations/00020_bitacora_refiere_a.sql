-- Correccion enlazada y rechazo tipado de la bitacora (issue #38, ADR 0006).
--
-- refiere_a: corregir es escribir otro asiento que referencia al anterior.
-- ERRCODE IN006: el adaptador reconoce el rechazo append-only sin leer prosa.
--
-- Numero: 00020, primero libre por encima de 00019 (allowMissing = false).
-- Solo agrega: una columna nullable y el mismo cuerpo de funcion con codigo.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE asientos ADD COLUMN refiere_a UUID REFERENCES asientos(id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX asientos_refiere_a ON asientos (refiere_a) WHERE refiere_a IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bitacora_solo_append() RETURNS TRIGGER AS $fn$
BEGIN
  RAISE EXCEPTION
    'ADR 0006: la bitacora es append-only. % sobre % no esta permitido. Corregir es escribir otro asiento que referencie al anterior',
    TG_OP, TG_TABLE_NAME
    USING ERRCODE = 'IN006';
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bitacora_solo_append() RETURNS TRIGGER AS $fn$
BEGIN
  RAISE EXCEPTION
    'ADR 0006: la bitacora es append-only. % sobre % no esta permitido. Corregir es escribir otro asiento que referencie al anterior',
    TG_OP, TG_TABLE_NAME;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS asientos_refiere_a;
ALTER TABLE asientos DROP COLUMN IF EXISTS refiere_a;
-- +goose StatementEnd
