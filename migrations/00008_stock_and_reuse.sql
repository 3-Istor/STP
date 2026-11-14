-- stock-and-reuse (voir ADR 0022) : stock par (BOM, pièce), propre à chaque BOM, et sa copie figée.
-- Quantités NUMERIC, jamais de float (AGENTS §3).

-- +goose Up

-- Stock du brouillon : aucun partage ni réservation entre BOM. Supprimer la BOM supprime son stock.
-- Un stock nul n'est pas une ligne (« aucun stock ») : quantity > 0.
CREATE TABLE bom_part_stock (
    bom_id   BIGINT NOT NULL REFERENCES boms (id) ON DELETE CASCADE,
    part_id  BIGINT NOT NULL REFERENCES parts (id),
    quantity NUMERIC NOT NULL
        CONSTRAINT bom_part_stock_quantity_valid CHECK (quantity > 0 AND quantity < 'Infinity'::numeric),
    PRIMARY KEY (bom_id, part_id)
);

-- Stock figé à la publication. Un snapshot publié avant cette migration n'a aucune ligne : « aucun stock ».
CREATE TABLE bom_snapshot_part_stock (
    snapshot_id BIGINT NOT NULL REFERENCES bom_snapshots (id),
    part_id     BIGINT NOT NULL REFERENCES parts (id),
    quantity    NUMERIC NOT NULL
        CONSTRAINT bom_snapshot_part_stock_quantity_valid CHECK (quantity > 0 AND quantity < 'Infinity'::numeric),
    PRIMARY KEY (snapshot_id, part_id)
);

-- Immutabilité (AGENTS §3, ADR 0006) : comme les nœuds figés.
CREATE TRIGGER bom_snapshot_part_stock_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_part_stock
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_part_stock_no_truncate BEFORE TRUNCATE ON bom_snapshot_part_stock
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- +goose Down

DROP TABLE bom_snapshot_part_stock;
DROP TABLE bom_part_stock;
