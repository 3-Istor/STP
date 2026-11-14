-- lead-time-planning (voir ADR 0023) : date de départ et date cible du brouillon, copiées à la publication.
-- La date de départ est saisie : le calcul ne lit jamais l'heure courante (AGENTS §3).

-- +goose Up

CREATE TABLE bom_schedule (
    bom_id    BIGINT PRIMARY KEY REFERENCES boms (id) ON DELETE CASCADE,
    start_on  DATE NOT NULL,
    target_on DATE,
    CONSTRAINT bom_schedule_target_after_start CHECK (target_on IS NULL OR target_on >= start_on),
    CONSTRAINT bom_schedule_finite CHECK (start_on > '-infinity' AND start_on < 'infinity'
        AND (target_on IS NULL OR (target_on > '-infinity' AND target_on < 'infinity')))
);

-- Snapshot : NULL = pas de planning figé (publié sans date de départ, ou avant cette migration).
-- ADD COLUMN ne déclenche pas les triggers d'immutabilité ; aucun backfill.
ALTER TABLE bom_snapshots
    ADD COLUMN start_on DATE,
    ADD COLUMN target_on DATE,
    ADD CONSTRAINT bom_snapshots_schedule_valid CHECK (
        (start_on IS NOT NULL OR target_on IS NULL) AND (target_on IS NULL OR target_on >= start_on)
        AND (start_on IS NULL OR (start_on > '-infinity' AND start_on < 'infinity'))
        AND (target_on IS NULL OR (target_on > '-infinity' AND target_on < 'infinity'))
    );

-- +goose Down

ALTER TABLE bom_snapshots
    DROP CONSTRAINT bom_snapshots_schedule_valid,
    DROP COLUMN target_on,
    DROP COLUMN start_on;
DROP TABLE bom_schedule;
