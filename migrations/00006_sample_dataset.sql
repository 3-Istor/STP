-- Jeu sample-dataset (ADR 0012, ADR 0021) : chargements et correspondance clé logique → identifiant.
-- Les identifiants techniques peuvent différer d'une base à l'autre ; les clés logiques, non.
-- Ces tables ne servent qu'au seed : aucune FK métier ne passe par elles.

-- +goose Up

-- Un chargement par dataset_version : ses paramètres et l'empreinte du contenu généré.
CREATE TABLE seed_loads (
    dataset_version   TEXT PRIMARY KEY CHECK (dataset_version <> ''),
    generator_version TEXT NOT NULL CHECK (generator_version <> ''),
    seed              TEXT NOT NULL,
    as_of             TIMESTAMPTZ NOT NULL,
    manifest_hash     CHAR(64) NOT NULL,
    loaded_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE seed_entities (
    dataset_version TEXT NOT NULL REFERENCES seed_loads (dataset_version),
    entity_kind     TEXT NOT NULL CHECK (entity_kind IN ('supplier', 'part', 'offer', 'contract', 'bom', 'bom_node')),
    logical_key     TEXT NOT NULL CHECK (logical_key <> ''),
    entity_id       BIGINT NOT NULL,
    PRIMARY KEY (dataset_version, entity_kind, logical_key),
    UNIQUE (dataset_version, entity_kind, entity_id)
);

-- Aucun écrasement ni purge silencieuse (ADR 0012) : seule l'insertion est permise.
CREATE TRIGGER seed_loads_immutable BEFORE UPDATE OR DELETE ON seed_loads
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER seed_loads_no_truncate BEFORE TRUNCATE ON seed_loads
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER seed_entities_immutable BEFORE UPDATE OR DELETE ON seed_entities
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER seed_entities_no_truncate BEFORE TRUNCATE ON seed_entities
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- +goose Down

DROP TABLE seed_entities;
DROP TABLE seed_loads;
