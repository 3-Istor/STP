# 0006 — Stockage d'un snapshot de BOM : tables relationnelles immuables
Statut : Accepté — Date : 2026-11-02 — Décideurs : Arthur.presle

## Contexte
bom-draft-and-snapshots (`docs/features.md` l. 167-192) : publier un brouillon crée un **snapshot de BOM
immuable** qui fige structure, quantités, offres choisies, attributs des pièces (CO2, masse, délais), volume,
réglages, et référence un snapshot de prix (ADR 0002). Modifier le CO2 d'une pièce après publication ne doit
pas changer le snapshot ; la base doit refuser `UPDATE`/`DELETE` (AGENTS §3) ; une BOM qui a un snapshot ne
se supprime pas (bom-management). Volumétrie : jusqu'à 30 000 nœuds par snapshot. Changer le modèle des
snapshots est une décision du §7. Choix D1 exprimé par l'humain au cadrage de NLPF-69, reste à l'acter.

## Options
1. **Tables relationnelles** — `bom_snapshots(id, bom_id, price_snapshot_id, name, currency, volume,
   published_at)` et `bom_snapshot_nodes(snapshot_id, node_key, parent_key, name, part_id, offer_id,
   quantity, co2_kg, mass_kg, production_days, lead_time_days)`, triggers `reject_mutation()`.
   + types contrôlés par la base (`NUMERIC`, `CHAR(3)`, `CHECK`), même modèle que les snapshots de prix ;
   l'arbre figé se relit avec les mêmes requêtes et le même calcul que le brouillon ; comparer deux
   snapshots = jointure sur `part_id`. − deux tables de plus ; chaque nouvel attribut figé (contrats,
   remises, budgets) demande une migration.
2. **Document JSONB** — `bom_snapshots(id, …, payload jsonb)`.
   + schéma souple, un seul `INSERT`. − aucun contrôle de type ni de devise, montants et quantités en JSON
   (risque `float` côté lecture), comparaison et requêtes analytiques pénibles, immutabilité du contenu
   identique mais sa validité non garantie.
3. **Versionner `bom_nodes` en place** (colonne `published`, copie à l'écriture).
   + pas de table nouvelle. − mélange données mutables et immuables dans une même table : le trigger
   devient conditionnel, et les attributs pièce ne sont pas copiés (le CO2 suivrait le catalogue).

## Décision
**Option 1.** Elle donne à la base les moyens de garantir l'immutabilité **et** la validité des valeurs
figées, et réutilise le modèle déjà accepté pour les prix (ADR 0002). Propriétaire : domaine `snapshot`,
qui n'écrit que ces deux tables ; il référence le snapshot de prix par `price_snapshot_id` sans en
connaître le détail. Les attributs pièce et le délai de livraison de l'offre sont **copiés** (ils sont
mutables dans le catalogue), alors que les prix et taux sont référencés (cotations déjà immuables).

## Conséquences
- `bom_snapshots.bom_id` référence `boms` **sans** `ON DELETE CASCADE` : la base refuse de supprimer une
  BOM qui a un snapshot ; le service le vérifie avant et renvoie `ErrHasSnapshots` (HTTP 409).
- `node_key` / `parent_key` reprennent les ids des nœuds au moment de la publication : l'arbre figé ne
  dépend plus de `bom_nodes`, qu'on peut modifier ou supprimer librement ensuite.
- `part_id` / `offer_id` gardent leur FK : une pièce publiée s'archive, elle ne se supprime pas (NLPF-67).
- Triggers `BEFORE UPDATE OR DELETE` et `BEFORE TRUNCATE` sur les deux tables : même un `psql` refuse.
- Retour arrière : une migration qui sérialise les lignes en JSONB (option 2) reste possible sans perte.
- Pivot : carbone versionné → ajouter `co2_factor_version` aux nœuds figés (migration additive) ;
  multi-source de prix → porté par le snapshot de prix, rien à changer ici ; multi-usine / multi-devise →
  `currency` et `volume` sont déjà par snapshot, une colonne `plant` s'ajoute de la même façon ;
  hors-ligne → le snapshot est entièrement en base, lisible sans l'API de prix.
