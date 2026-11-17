-- Profils de véhicule (voir ADR 0036) : pondérations du classement des alternatives, par BOM, et leur copie
-- figée à la publication. Poids entiers 0–100 dont la somme vaut 100. Une BOM sans ligne n'a pas de profil :
-- le classement reste celui des stratégies de nœud (ADR 0028).

-- +goose Up

CREATE TABLE bom_vehicle_profiles (
    bom_id           BIGINT PRIMARY KEY REFERENCES boms (id) ON DELETE CASCADE,
    preset           TEXT NOT NULL
        CONSTRAINT bom_vehicle_profiles_preset_valid CHECK (preset IN ('city_eco', 'family', 'premium', 'sport', 'custom')),
    weight_cost      SMALLINT NOT NULL CHECK (weight_cost BETWEEN 0 AND 100),
    weight_co2       SMALLINT NOT NULL CHECK (weight_co2 BETWEEN 0 AND 100),
    weight_lead_time SMALLINT NOT NULL CHECK (weight_lead_time BETWEEN 0 AND 100),
    weight_mass      SMALLINT NOT NULL CHECK (weight_mass BETWEEN 0 AND 100),
    weight_quality   SMALLINT NOT NULL CHECK (weight_quality BETWEEN 0 AND 100),
    CONSTRAINT bom_vehicle_profiles_sum_100
        CHECK (weight_cost + weight_co2 + weight_lead_time + weight_mass + weight_quality = 100)
);

-- Profil figé : copié à la publication, jamais modifié (AGENTS §3, ADR 0006). Absent pour un snapshot publié
-- avant cette migration ou depuis une BOM sans profil.
CREATE TABLE bom_snapshot_vehicle_profiles (
    snapshot_id      BIGINT PRIMARY KEY REFERENCES bom_snapshots (id),
    preset           TEXT NOT NULL
        CONSTRAINT bom_snapshot_vehicle_profiles_preset_valid
            CHECK (preset IN ('city_eco', 'family', 'premium', 'sport', 'custom')),
    weight_cost      SMALLINT NOT NULL CHECK (weight_cost BETWEEN 0 AND 100),
    weight_co2       SMALLINT NOT NULL CHECK (weight_co2 BETWEEN 0 AND 100),
    weight_lead_time SMALLINT NOT NULL CHECK (weight_lead_time BETWEEN 0 AND 100),
    weight_mass      SMALLINT NOT NULL CHECK (weight_mass BETWEEN 0 AND 100),
    weight_quality   SMALLINT NOT NULL CHECK (weight_quality BETWEEN 0 AND 100),
    CONSTRAINT bom_snapshot_vehicle_profiles_sum_100
        CHECK (weight_cost + weight_co2 + weight_lead_time + weight_mass + weight_quality = 100)
);

CREATE TRIGGER bom_snapshot_vehicle_profiles_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_vehicle_profiles
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_vehicle_profiles_no_truncate BEFORE TRUNCATE ON bom_snapshot_vehicle_profiles
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- +goose Down

DROP TABLE bom_snapshot_vehicle_profiles;
DROP TABLE bom_vehicle_profiles;
