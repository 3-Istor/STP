-- Coût complet (voir ADR 0031) : méthode de calcul du coût par BOM (base × change), volume programme,
-- taux budget annuels, outillage par offre, budgets cibles par nœud, et ce que la publication fige pour
-- rejouer exactement le même coût. Montants et taux NUMERIC + devise, jamais de float (AGENTS §3).

-- +goose Up

-- Méthode du brouillon. Défaut = purchase + latest : le calcul historique (cotation × quantité × dernier
-- taux), pour que les BOM existantes gardent exactement leurs totaux.
ALTER TABLE boms
    ADD COLUMN cost_basis TEXT NOT NULL DEFAULT 'purchase'
        CONSTRAINT boms_cost_basis_valid CHECK (cost_basis IN ('purchase', 'landed', 'full')),
    ADD COLUMN fx_policy TEXT NOT NULL DEFAULT 'latest'
        CONSTRAINT boms_fx_policy_valid CHECK (fx_policy IN ('latest', 'budget')),
    -- Volume programme : nombre de voitures sur la vie du programme, base d'amortissement de l'outillage.
    ADD COLUMN program_volume INTEGER
        CONSTRAINT boms_program_volume_valid CHECK (program_volume > 0),
    ADD CONSTRAINT boms_full_needs_program_volume CHECK (cost_basis <> 'full' OR program_volume IS NOT NULL);

-- Taux budget de l'année, fixé par la trésorerie : 1 from_currency = rate to_currency. Paire directe
-- seulement (pas d'inverse implicite, pas de pivot) ; absent → erreur nommée au calcul.
CREATE TABLE budget_rates (
    year          INTEGER NOT NULL CONSTRAINT budget_rates_year_valid CHECK (year BETWEEN 2000 AND 2100),
    from_currency CHAR(3) NOT NULL CHECK (from_currency ~ '^[A-Z]{3}$'),
    to_currency   CHAR(3) NOT NULL CHECK (to_currency ~ '^[A-Z]{3}$'),
    rate          NUMERIC NOT NULL
        CONSTRAINT budget_rates_rate_valid CHECK (rate > 0 AND rate < 'Infinity'::numeric),
    PRIMARY KEY (year, from_currency, to_currency),
    CONSTRAINT budget_rates_distinct_pair CHECK (from_currency <> to_currency)
);

-- Outillage (frais non récurrents) de l'offre ; NULL = non renseigné (jamais 0 implicite). Un outillage
-- nul se déclare explicitement à 0.
ALTER TABLE offers
    ADD COLUMN tooling_amount NUMERIC,
    ADD COLUMN tooling_currency CHAR(3),
    ADD CONSTRAINT offers_tooling_amount_valid CHECK (tooling_amount >= 0 AND tooling_amount < 'Infinity'::numeric),
    ADD CONSTRAINT offers_tooling_currency_valid CHECK (tooling_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT offers_tooling_pair CHECK ((tooling_amount IS NULL) = (tooling_currency IS NULL));

-- Budgets cibles par voiture (target-costing, ADR 0018) : le budget de la racine est le budget global.
-- Supprimer le nœud supprime son budget.
CREATE TABLE bom_target_budgets (
    bom_id   BIGINT NOT NULL,
    node_id  BIGINT NOT NULL,
    amount   NUMERIC NOT NULL
        CONSTRAINT bom_target_budgets_amount_valid CHECK (amount >= 0 AND amount < 'Infinity'::numeric),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (bom_id, node_id),
    FOREIGN KEY (bom_id, node_id) REFERENCES bom_nodes (bom_id, id) ON DELETE CASCADE
);

-- Snapshot : méthode et date de calcul figées. NULL partout pour un snapshot publié avant cette migration :
-- il se relit au prix d'achat et au dernier taux de son snapshot de prix, comme avant (aucun backfill).
-- ADD COLUMN ne déclenche pas les triggers d'immutabilité.
ALTER TABLE bom_snapshots
    ADD COLUMN cost_basis TEXT
        CONSTRAINT bom_snapshots_cost_basis_valid CHECK (cost_basis IN ('purchase', 'landed', 'full')),
    ADD COLUMN fx_policy TEXT
        CONSTRAINT bom_snapshots_fx_policy_valid CHECK (fx_policy IN ('latest', 'budget')),
    ADD COLUMN program_volume INTEGER
        CONSTRAINT bom_snapshots_program_volume_valid CHECK (program_volume > 0),
    ADD COLUMN cost_as_of TIMESTAMPTZ,
    ADD CONSTRAINT bom_snapshots_cost_method_pair CHECK ((cost_basis IS NULL) = (fx_policy IS NULL)),
    ADD CONSTRAINT bom_snapshots_full_needs_program_volume
        CHECK (cost_basis IS DISTINCT FROM 'full' OR program_volume IS NOT NULL);

-- Taux budget figés : ceux de l'année de calcul vers la devise de la BOM, copiés à la publication.
CREATE TABLE bom_snapshot_budget_rates (
    snapshot_id   BIGINT NOT NULL REFERENCES bom_snapshots (id),
    year          INTEGER NOT NULL,
    from_currency CHAR(3) NOT NULL CHECK (from_currency ~ '^[A-Z]{3}$'),
    to_currency   CHAR(3) NOT NULL CHECK (to_currency ~ '^[A-Z]{3}$'),
    rate          NUMERIC NOT NULL
        CONSTRAINT bom_snapshot_budget_rates_rate_valid CHECK (rate > 0 AND rate < 'Infinity'::numeric),
    PRIMARY KEY (snapshot_id, year, from_currency, to_currency)
);

-- Faits de coût figés par offre : remise retenue (contrat, palier, motif), douane, transport, emballage et
-- outillage tels que lus à la publication. NULL = non utilisé par la méthode figée ou non renseigné : la
-- publication refuse une méthode dont une composante manque, rien n'est figé à zéro.
CREATE TABLE bom_snapshot_offer_costs (
    snapshot_id           BIGINT NOT NULL REFERENCES bom_snapshots (id),
    offer_id              BIGINT NOT NULL REFERENCES offers (id),
    origin_country        CHAR(2) NOT NULL CHECK (origin_country ~ '^[A-Z]{2}$'),
    discount_percent      NUMERIC CHECK (discount_percent BETWEEN 0 AND 100),
    discount_reason       TEXT,
    discount_contract_id  BIGINT,
    discount_tier_min     NUMERIC CHECK (discount_tier_min > 0 AND discount_tier_min < 'Infinity'::numeric),
    customs_percent       NUMERIC CHECK (customs_percent BETWEEN 0 AND 1000),
    freight_amount_per_kg NUMERIC CHECK (freight_amount_per_kg >= 0 AND freight_amount_per_kg < 'Infinity'::numeric),
    freight_currency      CHAR(3) CHECK (freight_currency ~ '^[A-Z]{3}$'),
    packaging_amount      NUMERIC CHECK (packaging_amount >= 0 AND packaging_amount < 'Infinity'::numeric),
    packaging_currency    CHAR(3) CHECK (packaging_currency ~ '^[A-Z]{3}$'),
    tooling_amount        NUMERIC CHECK (tooling_amount >= 0 AND tooling_amount < 'Infinity'::numeric),
    tooling_currency      CHAR(3) CHECK (tooling_currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (snapshot_id, offer_id),
    CONSTRAINT bom_snapshot_offer_costs_discount_pair CHECK ((discount_percent IS NULL) = (discount_reason IS NULL)),
    CONSTRAINT bom_snapshot_offer_costs_freight_pair CHECK ((freight_amount_per_kg IS NULL) = (freight_currency IS NULL)),
    CONSTRAINT bom_snapshot_offer_costs_packaging_pair CHECK ((packaging_amount IS NULL) = (packaging_currency IS NULL)),
    CONSTRAINT bom_snapshot_offer_costs_tooling_pair CHECK ((tooling_amount IS NULL) = (tooling_currency IS NULL))
);

-- Immutabilité (AGENTS §3, ADR 0006) : comme les autres tables figées du snapshot.
CREATE TRIGGER bom_snapshot_budget_rates_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_budget_rates
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_budget_rates_no_truncate BEFORE TRUNCATE ON bom_snapshot_budget_rates
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_offer_costs_immutable BEFORE UPDATE OR DELETE ON bom_snapshot_offer_costs
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER bom_snapshot_offer_costs_no_truncate BEFORE TRUNCATE ON bom_snapshot_offer_costs
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- +goose Down

DROP TABLE bom_snapshot_offer_costs;
DROP TABLE bom_snapshot_budget_rates;
ALTER TABLE bom_snapshots
    DROP CONSTRAINT bom_snapshots_full_needs_program_volume,
    DROP CONSTRAINT bom_snapshots_cost_method_pair,
    DROP COLUMN cost_as_of,
    DROP COLUMN program_volume,
    DROP COLUMN fx_policy,
    DROP COLUMN cost_basis;
DROP TABLE bom_target_budgets;
ALTER TABLE offers
    DROP CONSTRAINT offers_tooling_pair,
    DROP CONSTRAINT offers_tooling_currency_valid,
    DROP CONSTRAINT offers_tooling_amount_valid,
    DROP COLUMN tooling_currency,
    DROP COLUMN tooling_amount;
DROP TABLE budget_rates;
ALTER TABLE boms
    DROP CONSTRAINT boms_full_needs_program_volume,
    DROP COLUMN program_volume,
    DROP COLUMN fx_policy,
    DROP COLUMN cost_basis;
