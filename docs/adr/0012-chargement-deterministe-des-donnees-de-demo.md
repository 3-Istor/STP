# 0012 — Chargement déterministe et non destructif du jeu de démonstration
Statut : Accepté — Date : 2026-11-04 — Décideurs : Joe (accord explicite dans le chat)

## Contexte
`docs/features.md:290–298` et NLPF-67 exigent `make seed` idempotent, catalogue multidevise
et Multipla jusqu'à ~30 000 nœuds, avec totaux dans l'éditeur. Cotations et snapshots sont
immuables. Un nom de pièce ne constitue pas une identité unique pour reconnaître un chargement.

## Options
1. **Identités logiques dans la base courante** — conserve les données existantes et facilite
   la démo dans l'application ; ajoute des métadonnées et une vérification de contenu.
2. **Base de démonstration dédiée** — isolation claire ; configuration et données séparées du projet.
3. **TRUNCATE/rechargement ou ON CONFLICT DO NOTHING seul** — facile ; destruction interdite ou
   divergences cachées, ne garantit pas les mêmes données.

## Décision
**Proposition : option 1**, pour rendre la répétition de `make seed` vérifiable sans purge.
Une table de correspondance associe `(dataset_version, entity_kind, logical_key)` à l'identifiant
créé ; une table de chargement conserve graine, instant de référence et empreinte du manifeste.
Insertion et contrôle sont transactionnels, sous verrou du chargement concurrent correspondant.

## Conséquences
- Paramètres explicites `seed`, `as_of` et `dataset_version`, génération pure ordonnée et prévisible.
  Un même dataset_version avec paramètres incompatibles ou données déjà modifiées provoque un
  refus expliqué et rollback. Les données identiques sont conservées sans UPDATE inutile.
  Les identifiants techniques peuvent différer entre bases ; comparer les contenus par clés
  logiques et leurs liens. La correspondance ne remplace pas les FK métier.
- Aucun écrasement de données utilisateur, modification de snapshot publié, remise à zéro des
  séquences ni UPDATE/DELETE de cotations. Une clé de chargement existante avec contenu différent
  est une erreur, pas un conflit silencieusement ignoré.
- Le générateur d'historique appartient à NLPF-68 : réutiliser sa production déterministe via un
  contrat coordonné qui permet l'insertion dans la transaction du seed, pas un appel distant après
  commit. Si ce contrat n'est pas disponible, ne pas déclarer le seed complet implémenté.
- Le manifeste borne les cotations initiales à as_of. Les ticks postérieurs sont conservés et
  exclus de la comparaison de l'initialisation ; les tests d'égalité complète figent le flux.
  Tests : répétition, deux bases neuves, changement de paramètres, contenu divergent, interruption,
  concurrence et données extérieures/cotations/snapshots conservés. Éditeur/totaux : dépendance NLPF-69.
- Retour arrière : désactiver le chargement et conserver les métadonnées ; toute purge exige une
  procédure séparée autorisée. Hors-ligne : paramètres et générateurs locaux suffisent ; multi-source,
  carbone versionné ou multi-usine imposeraient une nouvelle version du dataset, sans les anticiper.
