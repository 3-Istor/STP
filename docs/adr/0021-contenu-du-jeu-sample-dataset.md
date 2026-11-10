# 0021 — Contenu du jeu sample-dataset (Fiat Multipla, Aston Martin Valhalla)
Statut : Accepté — Date : 2026-11-10 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
`docs/features.md:290–298` : générateur déterministe, Multipla jusqu'à ~30 000 nœuds, toutes les
données de pièce renseignées, plusieurs devises/fournisseurs/offres, alternatives, contrats à paliers,
`make seed` idempotent, **pas de données réelles de constructeur**. ADR 0012 fixe déjà le *mécanisme*
(correspondance de clés logiques, transaction, historique de prix dans la transaction du seed) ;
cette ADR fixe le *contenu* et l'outillage.

## Options
1. **Arbre entièrement écrit à la main** (~2 000 nœuds) — lisible ; ne tient pas les ~30 000 nœuds.
2. **Hybride** : arbre de référence écrit à la main (sous-systèmes → sous-ensembles → pièces), puis
   instanciation déterministe de chaque occurrence (roues, sièges, visserie, connecteurs…) —
   volume de la spec, structure toujours lisible ; plus de code de génération.
3. **Arbre entièrement aléatoire** — volume facile ; structure absurde, inutilisable en démo.

## Décision
**Option 2.** Paquet `internal/sampledata` : données de référence en Go, valeurs (masse, CO2, prix,
délais, notes…) dérivées par SHA-256 de `(graine, clé logique, champ)` — indépendantes de l'ordre
d'itération, aucun `math/rand`. Multipla ≈ 30 000 nœuds (BOM en EUR), Valhalla ≈ 15 000 (BOM en GBP).
Noms et valeurs plausibles mais **fictifs** (fournisseurs inventés, aucune référence constructeur).
Devises EUR, USD, GBP, JPY : 12 paires directes ajoutées à `SIM_FX_PAIRS` (le moteur BOM n'inverse
pas encore les taux, ADR 0020). `SIM_DATASET_VERSION` passe à `demo-v3` : changer les paires change
le hash de configuration, une base en `demo-v1` bloquerait sinon le worker de prix.
Commande : `make seed` → `server seed` (même lecture des `SIM_*` que le worker, donc même série de prix).
Le toggle `BOM_DEMO` (`internal/demo`) est supprimé, comme prévu à sa création.

## Conséquences
- Facile : démo et tests de charge sur deux voitures complètes ; même graine = mêmes lignes.
- Difficile : ~1 000 offres × 365 jours de cotations initiales (≈ 370 000 lignes) : le premier
  `make seed` prend environ 70 s (relance : ~1 s). Une BOM de démo modifiée dans l'éditeur fait refuser le seed suivant
  (ADR 0012) : on recrée une base ou on change de `dataset_version`.
- Retour arrière : ne plus lancer `make seed` ; les données restent (aucune purge, ADR 0012).
- Pivot : multi-source de prix ou carbone versionné = nouvelle `dataset_version` ; multi-usine
  ajouterait un champ par offre sans changer la génération.
