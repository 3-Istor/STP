-- total-landed-cost (voir ADR 0025) : tables paramétrables de douane et de transport, emballage par offre,
-- pays de destination de la BOM. Montants NUMERIC + devise, jamais de float (AGENTS §3).

-- +goose Up

-- Droit de douane en % du prix d'achat, par (origine, destination) : ISO 3166-1 alpha-2.
CREATE TABLE customs_duties (
    origin_country      CHAR(2) NOT NULL CHECK (origin_country ~ '^[A-Z]{2}$'),
    destination_country CHAR(2) NOT NULL CHECK (destination_country ~ '^[A-Z]{2}$'),
    duty_percent        NUMERIC NOT NULL
        CONSTRAINT customs_duties_percent_valid CHECK (duty_percent >= 0 AND duty_percent <= 1000),
    PRIMARY KEY (origin_country, destination_country)
);

-- Transport par kg, par (origine, destination).
CREATE TABLE freight_rates (
    origin_country      CHAR(2) NOT NULL CHECK (origin_country ~ '^[A-Z]{2}$'),
    destination_country CHAR(2) NOT NULL CHECK (destination_country ~ '^[A-Z]{2}$'),
    amount_per_kg       NUMERIC NOT NULL
        CONSTRAINT freight_rates_amount_valid CHECK (amount_per_kg >= 0 AND amount_per_kg < 'Infinity'::numeric),
    currency            CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (origin_country, destination_country)
);

-- Emballage par unité, porté par l'offre ; NULL = non renseigné (coût rendu indisponible, jamais 0 implicite).
ALTER TABLE offers
    ADD COLUMN packaging_amount NUMERIC,
    ADD COLUMN packaging_currency CHAR(3),
    ADD CONSTRAINT offers_packaging_amount_valid
        CHECK (packaging_amount >= 0 AND packaging_amount < 'Infinity'::numeric),
    ADD CONSTRAINT offers_packaging_currency_valid CHECK (packaging_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT offers_packaging_pair CHECK ((packaging_amount IS NULL) = (packaging_currency IS NULL));

-- Pays de destination de la BOM (usine d'assemblage), copié à la publication ; NULL = non renseigné.
ALTER TABLE boms
    ADD COLUMN destination_country CHAR(2) CHECK (destination_country ~ '^[A-Z]{2}$');
ALTER TABLE bom_snapshots
    ADD COLUMN destination_country CHAR(2) CHECK (destination_country ~ '^[A-Z]{2}$');

-- +goose Down

ALTER TABLE bom_snapshots DROP COLUMN destination_country;
ALTER TABLE boms DROP COLUMN destination_country;
ALTER TABLE offers
    DROP CONSTRAINT offers_packaging_pair,
    DROP CONSTRAINT offers_packaging_currency_valid,
    DROP CONSTRAINT offers_packaging_amount_valid,
    DROP COLUMN packaging_currency,
    DROP COLUMN packaging_amount;
DROP TABLE freight_rates;
DROP TABLE customs_duties;
