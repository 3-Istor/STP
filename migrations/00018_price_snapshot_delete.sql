-- ADR 0051 : un lot de prix figés se supprime tant qu'aucune version publiée ni aucun brouillon ne le référence.
-- UPDATE et TRUNCATE restent refusés ; les cotations (price_quotes, fx_quotes) restent intouchables.
-- Les clés étrangères depuis bom_snapshots et boms sont le filet final ; le trigger refuse aussi la suppression
-- des lignes enfants d'un lot encore utilisé.

-- +goose Up
DROP TRIGGER price_snapshots_immutable ON price_snapshots;
DROP TRIGGER price_snapshot_quotes_immutable ON price_snapshot_quotes;
DROP TRIGGER price_snapshot_fx_immutable ON price_snapshot_fx;

CREATE TRIGGER price_snapshots_no_update BEFORE UPDATE ON price_snapshots
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshot_quotes_no_update BEFORE UPDATE ON price_snapshot_quotes
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshot_fx_no_update BEFORE UPDATE ON price_snapshot_fx
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- +goose StatementBegin
CREATE FUNCTION guard_price_snapshot_delete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE lot BIGINT;
BEGIN
    IF TG_TABLE_NAME = 'price_snapshots' THEN lot := OLD.id; ELSE lot := OLD.snapshot_id; END IF;
    IF EXISTS (SELECT 1 FROM bom_snapshots WHERE price_snapshot_id = lot)
       OR EXISTS (SELECT 1 FROM boms WHERE price_snapshot_id = lot) THEN
        RAISE EXCEPTION 'price snapshot % is in use', lot USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN OLD;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER price_snapshots_delete_guard BEFORE DELETE ON price_snapshots
    FOR EACH ROW EXECUTE FUNCTION guard_price_snapshot_delete();
CREATE TRIGGER price_snapshot_quotes_delete_guard BEFORE DELETE ON price_snapshot_quotes
    FOR EACH ROW EXECUTE FUNCTION guard_price_snapshot_delete();
CREATE TRIGGER price_snapshot_fx_delete_guard BEFORE DELETE ON price_snapshot_fx
    FOR EACH ROW EXECUTE FUNCTION guard_price_snapshot_delete();

-- +goose Down
DROP TRIGGER price_snapshot_fx_delete_guard ON price_snapshot_fx;
DROP TRIGGER price_snapshot_quotes_delete_guard ON price_snapshot_quotes;
DROP TRIGGER price_snapshots_delete_guard ON price_snapshots;
DROP FUNCTION guard_price_snapshot_delete();
DROP TRIGGER price_snapshot_fx_no_update ON price_snapshot_fx;
DROP TRIGGER price_snapshot_quotes_no_update ON price_snapshot_quotes;
DROP TRIGGER price_snapshots_no_update ON price_snapshots;

CREATE TRIGGER price_snapshots_immutable BEFORE UPDATE OR DELETE ON price_snapshots
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshot_quotes_immutable BEFORE UPDATE OR DELETE ON price_snapshot_quotes
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshot_fx_immutable BEFORE UPDATE OR DELETE ON price_snapshot_fx
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
