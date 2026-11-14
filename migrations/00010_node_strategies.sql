-- node-strategies (voir ADR 0028) : stratégie de décision attachée à un nœud, figée à la publication.
-- NULL : le nœud suit la stratégie de son ancêtre le plus proche qui en a une.

-- +goose Up

ALTER TABLE bom_nodes
    ADD COLUMN strategy TEXT
        CONSTRAINT bom_nodes_strategy_valid CHECK (strategy IN ('cost', 'lead_time', 'co2', 'balanced'));

-- Snapshot publié avant cette migration : NULL partout, « sans stratégie » (aucun backfill).
ALTER TABLE bom_snapshot_nodes
    ADD COLUMN strategy TEXT
        CONSTRAINT bom_snapshot_nodes_strategy_valid CHECK (strategy IN ('cost', 'lead_time', 'co2', 'balanced'));

-- +goose Down

ALTER TABLE bom_snapshot_nodes DROP COLUMN strategy;
ALTER TABLE bom_nodes DROP COLUMN strategy;
