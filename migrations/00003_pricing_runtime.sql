-- Progression et manifestes du simulateur (ADR 0011–0012).
-- Nouveau numéro local vérifié sur la base NLPF-67 ; intégration BOM différée (ADR 0016).
-- +goose Up
ALTER TABLE price_quotes ADD CONSTRAINT price_quotes_finite
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)) NOT VALID;
ALTER TABLE fx_quotes ADD CONSTRAINT fx_quotes_finite
    CHECK (rate NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)) NOT VALID;

CREATE TABLE pricing_runs (
    dataset_version TEXT PRIMARY KEY,
    config_hash CHAR(64) NOT NULL,
    config JSONB NOT NULL,
    tick BIGINT NOT NULL CHECK (tick >= 0),
    as_of TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ready', 'unavailable')),
    error_text TEXT NOT NULL DEFAULT ''
);
CREATE TABLE pricing_manifests (
    hash CHAR(64) PRIMARY KEY,
    offers JSONB NOT NULL,
    pairs JSONB NOT NULL
);
CREATE TABLE pricing_ticks (
    dataset_version TEXT NOT NULL REFERENCES pricing_runs(dataset_version),
    tick BIGINT NOT NULL CHECK (tick >= 0),
    as_of TIMESTAMPTZ NOT NULL,
    manifest_hash CHAR(64) NOT NULL REFERENCES pricing_manifests(hash),
    PRIMARY KEY (dataset_version, tick),
    UNIQUE (dataset_version, as_of)
);
CREATE TABLE pricing_series (
    dataset_version TEXT NOT NULL REFERENCES pricing_runs(dataset_version),
    kind TEXT NOT NULL CHECK (kind IN ('offer', 'fx')),
    entity_key TEXT NOT NULL,
    series_key TEXT NOT NULL,
    initial_as_of TIMESTAMPTZ NOT NULL,
    manifest_hash CHAR(64) NOT NULL REFERENCES pricing_manifests(hash),
    PRIMARY KEY (dataset_version, kind, entity_key),
    UNIQUE (dataset_version, kind, series_key)
);
CREATE TRIGGER pricing_manifests_immutable BEFORE UPDATE OR DELETE ON pricing_manifests
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER pricing_manifests_no_truncate BEFORE TRUNCATE ON pricing_manifests
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER pricing_ticks_immutable BEFORE UPDATE OR DELETE ON pricing_ticks
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER pricing_ticks_no_truncate BEFORE TRUNCATE ON pricing_ticks
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER pricing_series_immutable BEFORE UPDATE OR DELETE ON pricing_series
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER pricing_series_no_truncate BEFORE TRUNCATE ON pricing_series
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- +goose Down
DROP TABLE pricing_series;
DROP TABLE pricing_ticks;
DROP TABLE pricing_manifests;
DROP TABLE pricing_runs;
ALTER TABLE fx_quotes DROP CONSTRAINT fx_quotes_finite;
ALTER TABLE price_quotes DROP CONSTRAINT price_quotes_finite;
