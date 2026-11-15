-- exploded-3d-view, tranche 2 (voir ADR 0031) : modèles 3D, association pièce ↔ modèle, génération par BOM.
-- Les fichiers GLB ne sont jamais en base : models_3d.storage_path est la clé du BlobStore (MODELS_DIR).
-- Aucune position ici : elle est calculée par nœud de BOM à la lecture (ADR 0031, option 4b).

-- +goose Up

-- Un modèle 3D : un fichier GLB (format 'glb', storage_path renseigné) ou une primitive générée
-- (format 'primitive', pas de fichier ; la forme est recalculée depuis la catégorie de la pièce).
-- Dimensions en mètres, toutes les trois ou aucune ; confidence : 0–1.
CREATE TABLE models_3d (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT NOT NULL CHECK (name <> ''),
    source       TEXT NOT NULL CHECK (source <> ''),              -- generated | upload | sketchfab…
    source_url   TEXT NOT NULL DEFAULT '',
    storage_path TEXT NOT NULL DEFAULT '',
    format       TEXT NOT NULL CHECK (format IN ('glb', 'primitive')),
    license      TEXT NOT NULL DEFAULT '',
    dim_x        NUMERIC CHECK (dim_x > 0),
    dim_y        NUMERIC CHECK (dim_y > 0),
    dim_z        NUMERIC CHECK (dim_z > 0),
    confidence   NUMERIC NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT models_3d_file_matches_format CHECK ((storage_path = '') = (format = 'primitive')),
    CONSTRAINT models_3d_dimensions_all_or_none CHECK (
        (dim_x IS NULL AND dim_y IS NULL AND dim_z IS NULL)
        OR (dim_x IS NOT NULL AND dim_y IS NOT NULL AND dim_z IS NOT NULL)
    )
);

-- Le modèle retenu pour une pièce (un seul par pièce, quel que soit le nombre de BOM qui l'utilisent).
-- match_type : exact (la référence), similar (pièce comparable), generated (primitive).
CREATE TABLE part_models (
    part_id    BIGINT PRIMARY KEY REFERENCES parts (id) ON DELETE CASCADE,
    model_id   BIGINT NOT NULL REFERENCES models_3d (id),
    match_type TEXT NOT NULL CHECK (match_type IN ('exact', 'similar', 'generated')),
    confidence NUMERIC NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX part_models_model_id_idx ON part_models (model_id);

-- Génération des modèles d'une BOM : une ligne par BOM, écrasée à chaque relance. L'état est persisté
-- pour reprendre après un arrêt (reprise = relance qui saute les pièces déjà dans part_models).
-- tree_fingerprint : empreinte de l'arbre au lancement ; si l'arbre a changé, la scène est « stale ».
CREATE TABLE bom_assemblies (
    bom_id           BIGINT PRIMARY KEY REFERENCES boms (id) ON DELETE CASCADE,
    status           TEXT NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    done             INTEGER NOT NULL DEFAULT 0 CHECK (done >= 0),
    total            INTEGER NOT NULL DEFAULT 0 CHECK (total >= 0),
    error            TEXT NOT NULL DEFAULT '',
    tree_fingerprint TEXT NOT NULL CHECK (tree_fingerprint <> ''),
    result           JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bom_assemblies_progress CHECK (done <= total)
);

-- +goose Down

DROP TABLE bom_assemblies;
DROP TABLE part_models;
DROP TABLE models_3d;
