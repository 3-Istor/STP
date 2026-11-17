-- +goose Up

-- Horloge en boucle sur une année (ADR 0043) : un tour rejoue les mêmes jours simulés. Le journal des ticks
-- garde un numéro de tick unique et croissant, mais une même date simulée revient à chaque tour.
ALTER TABLE pricing_ticks DROP CONSTRAINT pricing_ticks_dataset_version_as_of_key;
CREATE INDEX pricing_ticks_as_of_idx ON pricing_ticks (dataset_version, as_of);

-- +goose Down

-- Échoue si des ticks d'un 2e tour existent déjà : on ne supprime pas l'historique (ADR 0015).
DROP INDEX pricing_ticks_as_of_idx;
ALTER TABLE pricing_ticks ADD CONSTRAINT pricing_ticks_dataset_version_as_of_key UNIQUE (dataset_version, as_of);
