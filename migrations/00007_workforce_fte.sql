-- workforce-fte (voir ADR 0024) : base horaire annuelle de la BOM et heures de main-d'œuvre figées.
-- Décimaux NUMERIC, jamais de float (AGENTS §3).

-- +goose Up

-- Base horaire annuelle d'un ETP : paramètre du brouillon, 1 607 h par défaut à la création.
ALTER TABLE boms
    ADD COLUMN annual_hours NUMERIC NOT NULL DEFAULT 1607
        CONSTRAINT boms_annual_hours_valid CHECK (annual_hours > 0 AND annual_hours < 'Infinity'::numeric);

-- Copies à la publication. Un snapshot publié avant cette migration reste NULL : pas d'ETP calculable,
-- pas de valeur inventée (aucun backfill, AGENTS §11). ADD COLUMN ne déclenche pas les triggers d'immutabilité.
ALTER TABLE bom_snapshots
    ADD COLUMN annual_hours NUMERIC
        CONSTRAINT bom_snapshots_annual_hours_valid CHECK (annual_hours > 0 AND annual_hours < 'Infinity'::numeric);

-- Heures par unité de la pièce, copiées du catalogue ; NULL si le catalogue ne les renseigne pas.
ALTER TABLE bom_snapshot_nodes
    ADD COLUMN labor_hours NUMERIC
        CONSTRAINT bom_snapshot_nodes_labor_hours_valid CHECK (labor_hours >= 0 AND labor_hours < 'Infinity'::numeric),
    ADD CONSTRAINT bom_snapshot_nodes_labor_on_part CHECK (labor_hours IS NULL OR part_id IS NOT NULL);

-- +goose Down

ALTER TABLE bom_snapshot_nodes
    DROP CONSTRAINT bom_snapshot_nodes_labor_on_part,
    DROP COLUMN labor_hours;
ALTER TABLE bom_snapshots DROP COLUMN annual_hours;
ALTER TABLE boms DROP COLUMN annual_hours;
