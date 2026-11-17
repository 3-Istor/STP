-- +goose Up

-- Matière de la pièce (ADR 0041) : famille de comparaison des prix au kg. NULL = non renseignée, jamais devinée.
ALTER TABLE parts ADD COLUMN material TEXT;
ALTER TABLE parts ADD CONSTRAINT parts_material_not_blank CHECK (material IS NULL OR btrim(material) <> '');

-- +goose Down

ALTER TABLE parts DROP COLUMN material;
