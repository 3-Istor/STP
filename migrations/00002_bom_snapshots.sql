-- Brouillons et snapshots de BOM (NLPF-69 T3). Voir ADR 0006 (tables relationnelles immuables)
-- et ADR 0007 (délai d'une feuille = livraison de l'offre + production de la pièce).
-- Quantités, CO2 et masse : NUMERIC, jamais de float (AGENTS §3).

-- +goose Up

-- Jours de production propres à un nœud de brouillon (sous-ensemble : assemblage ; ADR 0007 règle 4).
ALTER TABLE bom_nodes
    ADD COLUMN production_days INTEGER NOT NULL DEFAULT 0 CHECK (production_days >= 0);

-- Snapshot de BOM (ADR 0006) : en-tête immuable. bom_id référence la BOM d'origine SANS cascade :
-- la base refuse de supprimer une BOM qui a un snapshot. Le snapshot de prix est obligatoire :
-- publier fige toujours les prix utilisés (AGENTS §3, brouillon vs snapshot).
CREATE TABLE bom_snapshots (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bom_id            BIGINT NOT NULL REFERENCES boms (id),
    price_snapshot_id BIGINT NOT NULL REFERENCES price_snapshots (id),
    name              TEXT NOT NULL CHECK (name <> ''),
    currency          CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    volume            INTEGER NOT NULL CHECK (volume > 0),
    published_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX bom_snapshots_bom_id_idx ON bom_snapshots (bom_id);

-- Nœud figé. node_key / parent_key reprennent les ids de bom_nodes à la publication : l'arbre figé
-- ne dépend plus du brouillon. Les attributs mutables (pièce : CO2, masse, production ; offre :
-- délai de livraison) sont COPIÉS ; les prix restent référencés via le snapshot de prix.
-- La FK vers le parent est différée : tous les nœuds s'insèrent dans une transaction, dans n'importe quel ordre.
CREATE TABLE bom_snapshot_nodes (
    snapshot_id     BIGINT NOT NULL REFERENCES bom_snapshots (id),
    node_key        BIGINT NOT NULL,
    parent_key      BIGINT,
    name            TEXT,
    part_id         BIGINT REFERENCES parts (id),
    offer_id        BIGINT,
    quantity        NUMERIC NOT NULL CHECK (quantity > 0),
    co2_kg          NUMERIC CHECK (co2_kg >= 0),
    mass_kg         NUMERIC CHECK (mass_kg >= 0),
    production_days INTEGER NOT NULL CHECK (production_days >= 0),
    lead_time_days  INTEGER CHECK (lead_time_days >= 0),
    PRIMARY KEY (snapshot_id, node_key),
    FOREIGN KEY (snapshot_id, parent_key) REFERENCES bom_snapshot_nodes (snapshot_id, node_key)
        DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (offer_id, part_id) REFERENCES offers (id, part_id),
    CONSTRAINT bom_snapshot_nodes_not_own_parent CHECK (parent_key <> node_key),
    -- sous-ensemble : un nom, aucun attribut de pièce ; pièce : offre et attributs figés tous présents
    CONSTRAINT bom_snapshot_nodes_kind CHECK (
        (part_id IS NULL AND offer_id IS NULL AND name IS NOT NULL AND name <> ''
            AND co2_kg IS NULL AND mass_kg IS NULL AND lead_time_days IS NULL)
        OR (part_id IS NOT NULL AND offer_id IS NOT NULL
            AND co2_kg IS NOT NULL AND mass_kg IS NOT NULL AND lead_time_days IS NOT NULL)
    )
);
CREATE UNIQUE INDEX bom_snapshot_nodes_one_root_idx ON bom_snapshot_nodes (snapshot_id) WHERE parent_key IS NULL;
CREATE INDEX bom_snapshot_nodes_parent_idx ON bom_snapshot_nodes (snapshot_id, parent_key);

-- Immutabilité (AGENTS §3, ADR 0006) : même un psql ne modifie ni ne supprime un snapshot publié.
CREATE TRIGGER bom_snapshots_immutable BEFORE UPDATE OR DELETE ON bom_snapshots
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshots_no_truncate BEFORE TRUNCATE ON bom_snapshots
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER bom_snapshot_nodes_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_nodes
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_nodes_no_truncate BEFORE TRUNCATE ON bom_snapshot_nodes
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- Brouillon : price_snapshot_id NULL = prix en direct, sinon prix figés (R9) ;
-- source_snapshot_id = snapshot dont le brouillon a été créé (D3), NULL sinon.
ALTER TABLE boms
    ADD COLUMN price_snapshot_id  BIGINT REFERENCES price_snapshots (id),
    ADD COLUMN source_snapshot_id BIGINT REFERENCES bom_snapshots (id);

-- +goose Down

ALTER TABLE boms
    DROP COLUMN source_snapshot_id,
    DROP COLUMN price_snapshot_id;
DROP TABLE bom_snapshot_nodes;
DROP TABLE bom_snapshots;
ALTER TABLE bom_nodes DROP COLUMN production_days;
