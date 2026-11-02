-- Schéma initial (NLPF-70 T4). Périmètre des tables : décision Q2 de la spec nlpf-70.
-- Snapshots de prix : ADR 0002 (en-tête + liaisons vers les cotations immuables, domaine pricing).
-- Montants : NUMERIC + CHAR(3) ISO 4217, jamais de float (AGENTS §3).
-- Les attributs détaillés des pièces (masse, CO2, coûts…) arrivent avec le ticket part-catalog.

-- +goose Up

CREATE TABLE suppliers (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT NOT NULL CHECK (name <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE parts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT NOT NULL CHECK (name <> ''),
    category   TEXT NOT NULL DEFAULT '',
    zone       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Une offre : une pièce vendue par un fournisseur, dans sa devise, avec les bornes de référence
-- de ses cotations. Aucun prix d'achat : le prix est la dernière cotation (price_quotes).
CREATE TABLE offers (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    part_id        BIGINT NOT NULL REFERENCES parts (id),
    supplier_id    BIGINT NOT NULL REFERENCES suppliers (id),
    currency       CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    price_min      NUMERIC NOT NULL CHECK (price_min > 0),
    price_max      NUMERIC NOT NULL,
    lead_time_days INTEGER NOT NULL CHECK (lead_time_days >= 0),
    origin_country CHAR(2) NOT NULL CHECK (origin_country ~ '^[A-Z]{2}$'),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT offers_price_range CHECK (price_min <= price_max),
    -- cible des clés composites (offre ↔ pièce) depuis bom_nodes
    UNIQUE (id, part_id)
);
CREATE INDEX offers_part_id_idx ON offers (part_id);
CREATE INDEX offers_supplier_id_idx ON offers (supplier_id);

-- Cotation de prix : immuable (trigger plus bas). quoted_at est l'horodatage de la source
-- (temps simulé pour le simulateur) ; recorded_at, l'heure d'enregistrement en base.
CREATE TABLE price_quotes (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    offer_id    BIGINT NOT NULL REFERENCES offers (id),
    amount      NUMERIC NOT NULL CHECK (amount > 0),
    currency    CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    source      TEXT NOT NULL CHECK (source <> ''),
    quoted_at   TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- une source ne cote pas deux fois la même offre au même instant (pas de doublon au redémarrage)
    UNIQUE (offer_id, source, quoted_at),
    -- cible de la clé composite depuis price_snapshot_quotes
    UNIQUE (id, offer_id)
);
CREATE INDEX price_quotes_offer_quoted_idx ON price_quotes (offer_id, quoted_at DESC);

-- Cotation de taux : 1 base_currency = rate quote_currency. Immuable.
CREATE TABLE fx_quotes (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    base_currency  CHAR(3) NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'),
    quote_currency CHAR(3) NOT NULL CHECK (quote_currency ~ '^[A-Z]{3}$'),
    rate           NUMERIC NOT NULL CHECK (rate > 0),
    source         TEXT NOT NULL CHECK (source <> ''),
    quoted_at      TIMESTAMPTZ NOT NULL,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fx_quotes_distinct_pair CHECK (base_currency <> quote_currency),
    UNIQUE (base_currency, quote_currency, source, quoted_at),
    UNIQUE (id, base_currency, quote_currency)
);
CREATE INDEX fx_quotes_pair_quoted_idx ON fx_quotes (base_currency, quote_currency, quoted_at DESC);

-- Snapshot de prix (ADR 0002) : un en-tête et des liens vers des cotations immuables.
-- taken_at : instant figé (fourni par l'appelant, temps simulé) ; scope : périmètre demandé.
CREATE TABLE price_snapshots (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scope      TEXT NOT NULL CHECK (scope IN ('offer', 'supplier', 'bom', 'catalog')),
    scope_id   BIGINT,
    taken_at   TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT price_snapshots_scope_id CHECK ((scope = 'catalog') = (scope_id IS NULL))
);

-- offer_id est recopié pour imposer un seul prix par offre dans un snapshot ; la clé composite
-- garantit qu'il correspond bien à l'offre de la cotation.
CREATE TABLE price_snapshot_quotes (
    snapshot_id    BIGINT NOT NULL REFERENCES price_snapshots (id),
    price_quote_id BIGINT NOT NULL,
    offer_id       BIGINT NOT NULL,
    PRIMARY KEY (snapshot_id, offer_id),
    FOREIGN KEY (price_quote_id, offer_id) REFERENCES price_quotes (id, offer_id)
);

-- Même principe pour les taux : un seul taux par paire dans un snapshot.
CREATE TABLE price_snapshot_fx (
    snapshot_id    BIGINT NOT NULL REFERENCES price_snapshots (id),
    fx_quote_id    BIGINT NOT NULL,
    base_currency  CHAR(3) NOT NULL,
    quote_currency CHAR(3) NOT NULL,
    PRIMARY KEY (snapshot_id, base_currency, quote_currency),
    FOREIGN KEY (fx_quote_id, base_currency, quote_currency)
        REFERENCES fx_quotes (id, base_currency, quote_currency)
);

-- Immutabilité (AGENTS §3, ADR 0002) : la base refuse toute modification ou suppression,
-- y compris TRUNCATE, quel que soit le code appelant. Seul INSERT est permis.
-- +goose StatementBegin
CREATE FUNCTION reject_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% on immutable table % is forbidden', TG_OP, TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER price_quotes_immutable BEFORE UPDATE OR DELETE ON price_quotes
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_quotes_no_truncate BEFORE TRUNCATE ON price_quotes
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER fx_quotes_immutable BEFORE UPDATE OR DELETE ON fx_quotes
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER fx_quotes_no_truncate BEFORE TRUNCATE ON fx_quotes
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER price_snapshots_immutable BEFORE UPDATE OR DELETE ON price_snapshots
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshots_no_truncate BEFORE TRUNCATE ON price_snapshots
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER price_snapshot_quotes_immutable BEFORE UPDATE OR DELETE ON price_snapshot_quotes
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshot_quotes_no_truncate BEFORE TRUNCATE ON price_snapshot_quotes
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER price_snapshot_fx_immutable BEFORE UPDATE OR DELETE ON price_snapshot_fx
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER price_snapshot_fx_no_truncate BEFORE TRUNCATE ON price_snapshot_fx
    FOR EACH STATEMENT EXECUTE FUNCTION reject_mutation();

-- BOM : réglages globaux (devise de référence, volume).
CREATE TABLE boms (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT NOT NULL CHECK (name <> ''),
    currency   CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    volume     INTEGER NOT NULL CHECK (volume > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Nœud de BOM : arbre par parent_id (racine : parent_id NULL). Un nœud est soit un sous-ensemble
-- (name), soit une pièce avec l'offre choisie (part_id + offer_id). L'absence de cycle est vérifiée
-- par le service au déplacement (CTE récursive, AGENTS §3) ; la base garantit le reste :
-- parent dans la même BOM, une seule racine, offre cohérente avec la pièce.
CREATE TABLE bom_nodes (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bom_id    BIGINT NOT NULL REFERENCES boms (id) ON DELETE CASCADE,
    parent_id BIGINT,
    name      TEXT,
    part_id   BIGINT REFERENCES parts (id),
    offer_id  BIGINT,
    quantity  NUMERIC NOT NULL DEFAULT 1 CHECK (quantity > 0),
    UNIQUE (bom_id, id),
    -- supprimer un nœud supprime son sous-arbre
    FOREIGN KEY (bom_id, parent_id) REFERENCES bom_nodes (bom_id, id) ON DELETE CASCADE,
    FOREIGN KEY (offer_id, part_id) REFERENCES offers (id, part_id),
    CONSTRAINT bom_nodes_not_own_parent CHECK (parent_id <> id),
    CONSTRAINT bom_nodes_kind CHECK (
        (part_id IS NULL AND offer_id IS NULL AND name IS NOT NULL AND name <> '')
        OR (part_id IS NOT NULL AND offer_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX bom_nodes_one_root_idx ON bom_nodes (bom_id) WHERE parent_id IS NULL;
CREATE INDEX bom_nodes_parent_idx ON bom_nodes (bom_id, parent_id);

-- +goose Down

DROP TABLE bom_nodes;
DROP TABLE boms;
DROP TABLE price_snapshot_fx;
DROP TABLE price_snapshot_quotes;
DROP TABLE price_snapshots;
DROP TABLE fx_quotes;
DROP TABLE price_quotes;
DROP FUNCTION reject_mutation();
DROP TABLE offers;
DROP TABLE parts;
DROP TABLE suppliers;
