-- Versions et publication (voir ADR 0032 et 0033) : trace de la décision figée dans le snapshot (note, proposé
-- par, validé par, contrôles avant publication et justifications), pistes de réduction par BOM, et étapes du
-- jeu de démo déjà appliquées (seed idempotent, ADR 0012).

-- +goose Up

-- Trace de la publication (ADR 0033, vision §8 option A : noms saisis, pas de comptes). NULL pour un snapshot
-- publié avant cette migration. ADD COLUMN ne déclenche pas les triggers d'immutabilité.
ALTER TABLE bom_snapshots
    ADD COLUMN note TEXT CONSTRAINT bom_snapshots_note_length CHECK (length(note) <= 4000),
    ADD COLUMN proposed_by TEXT CONSTRAINT bom_snapshots_proposed_by_valid CHECK (length(proposed_by) BETWEEN 1 AND 200),
    ADD COLUMN validated_by TEXT CONSTRAINT bom_snapshots_validated_by_valid CHECK (length(validated_by) BETWEEN 1 AND 200);

-- Contrôles avant publication tels qu'évalués au moment de publier, avec la justification de chaque
-- avertissement. Un contrôle bloquant n'existe pas ici : il aurait empêché la publication.
CREATE TABLE bom_snapshot_checks (
    snapshot_id   BIGINT NOT NULL REFERENCES bom_snapshots (id),
    position      INTEGER NOT NULL CHECK (position >= 0),
    code          TEXT NOT NULL CHECK (code ~ '^[a-z_]{1,64}$'),
    level         TEXT NOT NULL CHECK (level IN ('ok', 'warn')),
    title         TEXT NOT NULL CHECK (title <> ''),
    detail        TEXT NOT NULL,
    justification TEXT CHECK (length(justification) BETWEEN 1 AND 4000),
    PRIMARY KEY (snapshot_id, position),
    UNIQUE (snapshot_id, code),
    CONSTRAINT bom_snapshot_checks_warn_justified CHECK ((level = 'warn') = (justification IS NOT NULL))
);

CREATE TRIGGER bom_snapshot_checks_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_checks
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_checks_no_truncate BEFORE TRUNCATE ON bom_snapshot_checks
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- Pistes de réduction (vision §2) : gain estimé par voiture, responsable, statut, brouillon qui la chiffre.
-- Supprimer la BOM supprime ses pistes ; supprimer le brouillon lié détache la piste.
CREATE TABLE bom_reduction_ideas (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bom_id        BIGINT NOT NULL REFERENCES boms (id) ON DELETE CASCADE,
    title         TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    owner         TEXT NOT NULL CHECK (length(owner) <= 200),
    gain_amount   NUMERIC NOT NULL CHECK (gain_amount >= 0 AND gain_amount < 'Infinity'::numeric),
    gain_currency CHAR(3) NOT NULL CHECK (gain_currency ~ '^[A-Z]{3}$'),
    status        TEXT NOT NULL CHECK (status IN ('idea', 'in_progress', 'to_validate', 'done', 'dropped')),
    draft_bom_id  BIGINT REFERENCES boms (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX bom_reduction_ideas_bom_idx ON bom_reduction_ideas (bom_id, id);

-- Étapes du jeu de démo appliquées par les services (publication V1, brouillon B, pistes) : insertion seule,
-- une étape déjà présente n'est pas rejouée.
CREATE TABLE seed_demo_steps (
    dataset_version TEXT NOT NULL REFERENCES seed_loads (dataset_version),
    step            TEXT NOT NULL CHECK (step <> ''),
    entity_id       BIGINT NOT NULL,
    PRIMARY KEY (dataset_version, step)
);
CREATE TRIGGER seed_demo_steps_immutable BEFORE UPDATE OR DELETE ON seed_demo_steps
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- +goose Down

DROP TABLE seed_demo_steps;
DROP TABLE bom_reduction_ideas;
DROP TABLE bom_snapshot_checks;
ALTER TABLE bom_snapshots
    DROP COLUMN validated_by,
    DROP COLUMN proposed_by,
    DROP COLUMN note;
