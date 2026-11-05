-- ADR 0013 : un snapshot se ferme au commit de sa transaction de création.
-- Anciennes lignes : NULL signifie fermé ; aucun fait existant n'est mis à jour.
-- +goose Up
ALTER TABLE price_snapshots ADD COLUMN creation_xid xid8;

-- +goose StatementBegin
CREATE FUNCTION set_price_snapshot_transaction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.creation_xid := pg_current_xact_id();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER price_snapshot_creation BEFORE INSERT ON price_snapshots
    FOR EACH ROW EXECUTE FUNCTION set_price_snapshot_transaction();

-- +goose StatementBegin
CREATE FUNCTION guard_price_snapshot_link() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE creator xid8;
BEGIN
    SELECT creation_xid INTO creator FROM price_snapshots WHERE id = NEW.snapshot_id;
    IF creator IS NULL OR creator <> pg_current_xact_id() THEN
        RAISE EXCEPTION 'price snapshot % is closed', NEW.snapshot_id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER price_snapshot_quote_creation BEFORE INSERT ON price_snapshot_quotes
    FOR EACH ROW EXECUTE FUNCTION guard_price_snapshot_link();
CREATE TRIGGER price_snapshot_fx_creation BEFORE INSERT ON price_snapshot_fx
    FOR EACH ROW EXECUTE FUNCTION guard_price_snapshot_link();

-- +goose Down
DROP TRIGGER price_snapshot_fx_creation ON price_snapshot_fx;
DROP TRIGGER price_snapshot_quote_creation ON price_snapshot_quotes;
DROP FUNCTION guard_price_snapshot_link();
DROP TRIGGER price_snapshot_creation ON price_snapshots;
DROP FUNCTION set_price_snapshot_transaction();
ALTER TABLE price_snapshots DROP COLUMN creation_xid;
