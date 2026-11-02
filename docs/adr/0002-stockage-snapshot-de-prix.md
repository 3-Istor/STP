# 0002 — Où vit le snapshot de prix
Statut : Proposé — Date : 2026-11-02 — Décideurs : à compléter (les 3)

## Contexte
live-price-feed (`docs/features.md`) : chaque prix d'offre et chaque taux de change récupérés sont enregistrés
comme des **cotations** datées, jamais modifiées. Un **snapshot de prix** fige les cotations de prix d'un
périmètre (une offre, un fournisseur, les offres d'une BOM, tout le catalogue) **et** les taux nécessaires à
leur conversion ; il est immuable (AGENTS §3) et sera référencé par les snapshots de BOM. Volumétrie : une
BOM peut atteindre 30 000 nœuds, donc des milliers d'offres par snapshot. Il faut décider : quelles tables,
et quel domaine en est propriétaire (`pricing` ou `snapshot`). Bloquant pour la migration initiale (NLPF-70 T4).

## Options
1. **En-tête + liaisons vers les cotations** — `price_snapshots(id, created_at, scope…)`,
   `price_snapshot_quotes(snapshot_id, price_quote_id)`, `price_snapshot_fx(snapshot_id, fx_quote_id)`.
   + aucune duplication ; chaque valeur garde sa source et son horodatage d'origine (explicabilité) ;
   l'immuabilité découle de celle des cotations + trigger sur les trois tables.
   − une lecture demande une jointure ; supprimer une cotation devient impossible (FK) — c'est voulu.
   − AGENTS §3 dit « prix… copiés » : ici la copie est la référence à une ligne qui ne change jamais.
2. **Copie des valeurs dans le snapshot** — `price_snapshot_items(snapshot_id, offer_id, amount, currency,
   quoted_at, source)`, idem pour les taux.
   + snapshot autonome, lisible sans jointure ; survit à une purge éventuelle des cotations.
   − double stockage (la même valeur vit deux fois) ; risque de divergence si la copie est mal faite ;
   la trace « quelle cotation exactement » se perd sauf à garder aussi l'id.
3. **Document JSONB** — `price_snapshots(id, payload jsonb)`.
   + schéma souple. − aucun contrôle de type ni de devise par la base, montants en JSON (risque float),
   requêtes de comparaison pénibles.

Propriétaire : (a) domaine `pricing`, qui possède déjà cotations et taux ; (b) domaine `snapshot`, qui
regrouperait tout ce qui est figé (BOM + prix).

## Décision
Proposé : **option 1, portée par le domaine `pricing`**. Les cotations sont déjà immuables et datées :
les référencer suffit à figer les prix et les taux sans dupliquer, et garde la provenance exacte de chaque
valeur. `snapshot` (BOM) référence un `price_snapshot_id`, il ne connaît pas le détail des cotations.

## Conséquences
- Facile : « quels prix et taux ce calcul a-t-il utilisés ? » = lister les cotations liées ; comparer deux
  snapshots = différence de deux ensembles d'ids.
- Facile : un brouillon « figé » pointe un snapshot ; « remettre en direct » = enlever le pointeur.
- Difficile : purger l'historique des cotations ; il faudrait d'abord passer à l'option 2 (migration de copie).
- Triggers `BEFORE UPDATE OR DELETE` sur `price_quotes`, `fx_quotes`, `price_snapshots` et les deux tables
  de liaison ; une contrainte d'unicité `(snapshot_id, offer_id)` empêche deux prix pour une offre.
- Retour arrière : migration qui crée les tables de l'option 2 et les remplit par jointure, sans perte.
- Pivot multi-source de prix : la colonne `source` des cotations suffit, le snapshot lie des cotations de
  sources différentes sans changement. Hors-ligne : un snapshot est lisible sans l'API (tout est en base).
  Multi-devise / multi-usine : les taux liés couvrent n'importe quelle paire ; rien n'est spécifique à EUR.
