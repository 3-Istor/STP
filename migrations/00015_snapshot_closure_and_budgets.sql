-- Snapshots de BOM fermés au commit de leur transaction et budgets cibles figés (voir ADR 0038, v2-lot-g T2 et T3).
-- Les triggers reject_mutation (00002, 00008, 00012–00014) refusent UPDATE, DELETE et TRUNCATE ; ils laissaient
-- passer un INSERT tardif dans une table figée (un nœud, un taux budget, un contrôle… ajoutés après coup). Même
-- modèle que les snapshots de prix (00005, ADR 0016) : seule la transaction qui crée le snapshot y écrit.
-- Anciennes lignes : creation_xid NULL = fermé ; aucune ligne existante n'est mise à jour (ADD COLUMN ne
-- déclenche pas les triggers d'immutabilité).

-- +goose Up

ALTER TABLE bom_snapshots
    ADD COLUMN creation_xid xid8,
    -- Budgets cibles figés à la publication (AGENTS §3 : le snapshot copie aussi les réglages). false : snapshot
    -- publié avant cette migration, ses budgets n'ont pas été copiés.
    ADD COLUMN budgets_frozen BOOLEAN NOT NULL DEFAULT false;

-- +goose StatementBegin
CREATE FUNCTION set_bom_snapshot_transaction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.creation_xid := pg_current_xact_id();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bom_snapshot_creation BEFORE INSERT ON bom_snapshots
    FOR EACH ROW EXECUTE FUNCTION set_bom_snapshot_transaction();

-- Une instruction (INSERT ou COPY) qui ajoute des lignes figées à un snapshot créé par une autre transaction est
-- refusée en entier. Trigger d'instruction sur la table de transition : une requête par COPY, pas une par ligne
-- (30 000 nœuds par publication).
-- +goose StatementBegin
CREATE FUNCTION guard_bom_snapshot_rows() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE closed BIGINT;
BEGIN
    SELECT n.snapshot_id INTO closed
    FROM (SELECT DISTINCT snapshot_id FROM new_rows) n
    LEFT JOIN bom_snapshots s ON s.id = n.snapshot_id
    WHERE s.creation_xid IS NULL OR s.creation_xid <> pg_current_xact_id()
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION 'bom snapshot % is published and closed: % rows are frozen', closed, TG_TABLE_NAME
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- Budgets cibles figés, par nœud figé (clé = id du nœud du brouillon à la publication) ; celui de la racine est
-- le budget global. Montant NUMERIC + devise, jamais de float (AGENTS §3). Pas de FK vers bom_snapshot_nodes :
-- elle ferait échouer un TRUNCATE des nœuds figés avant reject_mutation (0A000 au lieu de 23001) ; la
-- publication ne copie que les budgets des nœuds qu'elle fige.
CREATE TABLE bom_snapshot_target_budgets (
    snapshot_id BIGINT NOT NULL REFERENCES bom_snapshots (id),
    node_key    BIGINT NOT NULL,
    amount      NUMERIC NOT NULL
        CONSTRAINT bom_snapshot_target_budgets_amount_valid CHECK (amount >= 0 AND amount < 'Infinity'::numeric),
    currency    CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (snapshot_id, node_key)
);
CREATE TRIGGER bom_snapshot_target_budgets_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_target_budgets
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_target_budgets_no_truncate BEFORE TRUNCATE ON bom_snapshot_target_budgets
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER bom_snapshot_nodes_closed AFTER INSERT ON bom_snapshot_nodes
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();
CREATE TRIGGER bom_snapshot_part_stock_closed AFTER INSERT ON bom_snapshot_part_stock
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();
CREATE TRIGGER bom_snapshot_budget_rates_closed AFTER INSERT ON bom_snapshot_budget_rates
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();
CREATE TRIGGER bom_snapshot_offer_costs_closed AFTER INSERT ON bom_snapshot_offer_costs
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();
CREATE TRIGGER bom_snapshot_checks_closed AFTER INSERT ON bom_snapshot_checks
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();
CREATE TRIGGER bom_snapshot_vehicle_profiles_closed AFTER INSERT ON bom_snapshot_vehicle_profiles
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();
CREATE TRIGGER bom_snapshot_target_budgets_closed AFTER INSERT ON bom_snapshot_target_budgets
    REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION guard_bom_snapshot_rows();

-- +goose Down

DROP TRIGGER bom_snapshot_vehicle_profiles_closed ON bom_snapshot_vehicle_profiles;
DROP TRIGGER bom_snapshot_checks_closed ON bom_snapshot_checks;
DROP TRIGGER bom_snapshot_offer_costs_closed ON bom_snapshot_offer_costs;
DROP TRIGGER bom_snapshot_budget_rates_closed ON bom_snapshot_budget_rates;
DROP TRIGGER bom_snapshot_part_stock_closed ON bom_snapshot_part_stock;
DROP TRIGGER bom_snapshot_nodes_closed ON bom_snapshot_nodes;
DROP TABLE bom_snapshot_target_budgets;
DROP FUNCTION guard_bom_snapshot_rows();
DROP TRIGGER bom_snapshot_creation ON bom_snapshots;
DROP FUNCTION set_bom_snapshot_transaction();
ALTER TABLE bom_snapshots
    DROP COLUMN budgets_frozen,
    DROP COLUMN creation_xid;
