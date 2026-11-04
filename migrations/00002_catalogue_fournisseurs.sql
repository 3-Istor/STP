-- NLPF-67 T3 : catalogue complet et contrats (voir ADR 0006 et 0007).
-- Les attributs nouveaux restent NULL sur les anciennes lignes : aucune donnée métier inventée.
-- La création complète et le refus des données incomplètes relèvent également des services.

-- +goose Up

ALTER TABLE parts
    ADD COLUMN mass_kg NUMERIC,
    ADD COLUMN co2_kg_per_unit NUMERIC,
    ADD COLUMN recycled_percent NUMERIC,
    ADD COLUMN recyclability_percent NUMERIC,
    ADD COLUMN manufacturing_amount NUMERIC,
    ADD COLUMN manufacturing_currency CHAR(3),
    ADD COLUMN production_days INTEGER,
    ADD COLUMN labor_hours NUMERIC,
    ADD COLUMN hourly_amount NUMERIC,
    ADD COLUMN hourly_currency CHAR(3),
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD CONSTRAINT parts_mass_valid CHECK (mass_kg >= 0 AND mass_kg < 'Infinity'::numeric),
    ADD CONSTRAINT parts_co2_valid CHECK (co2_kg_per_unit >= 0 AND co2_kg_per_unit < 'Infinity'::numeric),
    ADD CONSTRAINT parts_recycled_percent_valid CHECK (recycled_percent BETWEEN 0 AND 100),
    ADD CONSTRAINT parts_recyclability_percent_valid CHECK (recyclability_percent BETWEEN 0 AND 100),
    ADD CONSTRAINT parts_manufacturing_amount_valid
        CHECK (manufacturing_amount >= 0 AND manufacturing_amount < 'Infinity'::numeric),
    ADD CONSTRAINT parts_manufacturing_currency_valid CHECK (manufacturing_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT parts_manufacturing_pair
        CHECK ((manufacturing_amount IS NULL) = (manufacturing_currency IS NULL)),
    ADD CONSTRAINT parts_production_days_valid CHECK (production_days >= 0),
    ADD CONSTRAINT parts_labor_hours_valid CHECK (labor_hours >= 0 AND labor_hours < 'Infinity'::numeric),
    ADD CONSTRAINT parts_hourly_amount_valid CHECK (hourly_amount >= 0 AND hourly_amount < 'Infinity'::numeric),
    ADD CONSTRAINT parts_hourly_currency_valid CHECK (hourly_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT parts_hourly_pair CHECK ((hourly_amount IS NULL) = (hourly_currency IS NULL));

ALTER TABLE offers
    ADD COLUMN quality_score NUMERIC,
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD CONSTRAINT offers_quality_score_valid CHECK (quality_score BETWEEN 0 AND 5),
    ADD CONSTRAINT offers_price_finite CHECK (price_min < 'Infinity'::numeric AND price_max < 'Infinity'::numeric),
    ADD CONSTRAINT offers_id_supplier_unique UNIQUE (id, supplier_id);

ALTER TABLE suppliers
    ADD COLUMN quality_score NUMERIC,
    ADD COLUMN trust_score NUMERIC,
    ADD CONSTRAINT suppliers_quality_score_valid CHECK (quality_score BETWEEN 0 AND 5),
    ADD CONSTRAINT suppliers_trust_score_valid CHECK (trust_score BETWEEN 0 AND 5);

-- Une seule ligne canonique représente les deux sens, sans auto-lien ni doublon.
CREATE TABLE part_alternatives (
    part_low_id  BIGINT NOT NULL REFERENCES parts (id),
    part_high_id BIGINT NOT NULL REFERENCES parts (id),
    PRIMARY KEY (part_low_id, part_high_id),
    CONSTRAINT part_alternatives_canonical CHECK (part_low_id < part_high_id)
);
CREATE INDEX part_alternatives_high_idx ON part_alternatives (part_high_id);

CREATE TABLE contracts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    supplier_id BIGINT NOT NULL REFERENCES suppliers (id),
    starts_on   DATE NOT NULL,
    ends_on     DATE NOT NULL,
    conditions  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT contracts_date_range CHECK (isfinite(starts_on) AND isfinite(ends_on) AND starts_on <= ends_on),
    CONSTRAINT contracts_id_supplier_unique UNIQUE (id, supplier_id)
);
CREATE INDEX contracts_supplier_idx ON contracts (supplier_id, id);

-- supplier_id impose que le contrat et chaque offre associée appartiennent au même fournisseur.
-- Les FK composites protègent aussi leurs UPDATE ultérieurs ; aucune cascade sur l'historique.
CREATE TABLE contract_offers (
    contract_id BIGINT NOT NULL,
    offer_id    BIGINT NOT NULL,
    supplier_id BIGINT NOT NULL,
    PRIMARY KEY (contract_id, offer_id),
    FOREIGN KEY (contract_id, supplier_id) REFERENCES contracts (id, supplier_id),
    FOREIGN KEY (offer_id, supplier_id) REFERENCES offers (id, supplier_id)
);
CREATE INDEX contract_offers_offer_idx ON contract_offers (offer_id);

CREATE TABLE contract_tiers (
    contract_id      BIGINT NOT NULL REFERENCES contracts (id),
    min_quantity     NUMERIC NOT NULL,
    discount_percent NUMERIC NOT NULL,
    PRIMARY KEY (contract_id, min_quantity),
    CONSTRAINT contract_tiers_quantity_valid CHECK (min_quantity > 0 AND min_quantity < 'Infinity'::numeric),
    CONSTRAINT contract_tiers_discount_valid CHECK (discount_percent BETWEEN 0 AND 100)
);

-- +goose Down

-- Vérification de réversibilité sur base jetable. Sur une base conservée, exporter les nouveaux
-- attributs et contrats avant un rollback autorisé : leur suppression est nécessairement destructive.

DROP TABLE contract_tiers;
DROP TABLE contract_offers;
DROP TABLE contracts;
DROP TABLE part_alternatives;

ALTER TABLE suppliers
    DROP COLUMN quality_score,
    DROP COLUMN trust_score;

ALTER TABLE offers
    DROP CONSTRAINT offers_id_supplier_unique,
    DROP CONSTRAINT offers_price_finite,
    DROP COLUMN quality_score,
    DROP COLUMN archived_at;

ALTER TABLE parts
    DROP COLUMN mass_kg,
    DROP COLUMN co2_kg_per_unit,
    DROP COLUMN recycled_percent,
    DROP COLUMN recyclability_percent,
    DROP COLUMN manufacturing_amount,
    DROP COLUMN manufacturing_currency,
    DROP COLUMN production_days,
    DROP COLUMN labor_hours,
    DROP COLUMN hourly_amount,
    DROP COLUMN hourly_currency,
    DROP COLUMN archived_at;
