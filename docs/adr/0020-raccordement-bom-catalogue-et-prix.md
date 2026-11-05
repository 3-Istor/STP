# 0020 — Raccordement de la BOM au vrai catalogue et aux vrais prix
Statut : Proposé — Date : 2026-11-05 — Décideurs : <à compléter>

## Contexte
NLPF-69 (BOM, PR #9) et NLPF-67/68 (catalogue et prix, PR #10) ont été développées en parallèle
(ADR 0019). Fusionnées, elles se contredisaient : migration 00002 et ADR 0006–0008 en double,
`catalog.Part` incompatible, port `pricing.Pricing` de la BOM (gel, lectures figées) sans
implémentation réelle. `docs/features.md` l.141, l.179–181 et AGENTS §3 (donnée manquante jamais à zéro).

## Options
1. Garder les types de NLPF-69 (`decimal.Decimal`, `int`) — rien à changer côté BOM ; un NULL du
   catalogue deviendrait zéro, ou il faudrait rendre les colonnes obligatoires.
2. Garder les types de NLPF-67 (pointeurs, NULL explicite) et refuser le calcul si un attribut manque ;
   brancher le port de la BOM sur `SnapshotService` par un adapter — honnête sur les données ; le
   contrat de NLPF-69 change un peu (portée du gel).
3. Laisser la BOM sur les stubs jusqu'à une intégration ultérieure — aucun risque immédiat ; la
   BOM reste inutilisable sur le vrai serveur.

## Décision
**Option 2** (types de NLPF-67 choisis par Arthur.presle dans la session du 2026-11-05) :
- PR #10 renumérotée après PR #9 : migrations 00003–00005, ADR 0009–0019 (contenu inchangé).
- `catalog.Part.BOMAttributes()` renvoie CO2, masse et jours de production, ou
  `catalog.ErrMissingAttribute` (HTTP 409) en les listant ; totaux et publication sont refusés.
- `pricing.FreezePrices` reçoit `bomID` : l'adapter `pricing.BOMPricing` crée un snapshot de prix
  de portée « bom » via `SnapshotService`, à l'instant **simulé** de la source (l'instant réel passé
  par l'appelant ne sert qu'à `published_at`). Une couverture incomplète devient `ErrNoQuote` /
  `ErrNoRate` avec les offres et paires manquantes.
- `SnapshotRate` reste une paire exacte : pas d'inversion dans l'adapter (ADR 0017 non encore
  appliquée au moteur BOM).
- Les références d'archivage d'une pièce ou d'une offre comptent aussi `bom_snapshot_nodes`.

## Conséquences
- Facile : la BOM tourne sur le vrai catalogue et les vrais prix ; `BOM_DEMO` n'est plus nécessaire
  pour tester (toggle conservé, à supprimer avec le seed sample-dataset).
- Difficile : une seule pièce sans attribut bloque les totaux de toute la BOM (pas d'erreur par nœud).
- À faire : inversion de taux dans le moteur BOM (`ResolveFX`), figer contrats, remises et budgets
  dans le snapshot de BOM, SSE dans l'éditeur, règle d'ajout d'une pièce archivée.
- Retour arrière : revenir aux stubs dans `cmd/server/main.go` ; l'adapter est isolé dans
  `internal/pricing/bom_pricing.go`. Les bases de dev en version 2 avec le schéma catalogue de
  NLPF-67 doivent être recréées (`goose reset`), ce sont des données de dev.
- Pivot multi-source de prix : nouvelle implémentation de `Pricing`, la BOM ne change pas.
