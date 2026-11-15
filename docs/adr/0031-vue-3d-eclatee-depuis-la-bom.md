# 0031 — Vue 3D éclatée construite depuis la BOM
Statut : Accepté — Date : 2026-11-15 — Décideurs : Arthur.presle (accord explicite dans le chat)

## Contexte
exploded-3d-view (`docs/features.md:502–517`, P4 « rendu sans modèle 3D à définir ») et le cahier des
charges détaillé reçu le 2026-11-15 : bascule Vue BOM / Vue 3D sur la même donnée, sélection synchronisée,
éclaté progressif, modèles 3D recherchés automatiquement (EXACT / SIMILAR / GENERATED, avec confiance),
assemblage cohérent, génération asynchrone et reprise, stockage des fichiers hors Postgres.
Contraintes : le catalogue n'a **ni dimensions ni géométrie** (`parts` : nom, catégorie, zone, masse, CO2) ;
une pièce peut figurer dans plusieurs BOM et plusieurs fois dans une BOM ; le domaine ne connaît ni réseau
ni fournisseur externe (AGENTS §2) ; aucun calcul ne dépend de l'heure ou d'un ordre de map (§3).

## Options
1. **Front** : `three` + `@react-three/fiber` (deux dépendances npm) — composants React déclaratifs,
   React 19 supporté ; apprentissage R3F. Variante : `three` seul, scène impérative dans un `useEffect` —
   une dépendance de moins, plus de code à la main pour la sélection et les animations.
2. **Fichiers GLB** : (a) stockage objet S3/MinIO (service Docker + SDK Go) — conforme au cahier des charges,
   un service et une dépendance Go de plus ; (b) interface Go `BlobStore` avec une implémentation **système de
   fichiers** (`MODELS_DIR`, servi par l'API) et un adapter S3 plus tard — zéro dépendance, porte ouverte.
3. **Recherche de modèles** : (a) agent LLM + Sketchfab/GrabCAD maintenant — GrabCAD n'a pas d'API publique,
   le téléchargement Sketchfab exige OAuth et une licence CC ; (b) interface Go `ModelFinder` dans le domaine,
   V1 = `GENERATED` déterministe (primitive par catégorie), puis adapter Sketchfab (recherche + licence, jeton
   `SKETCHFAB_API_TOKEN`) et, plus tard, un LLM derrière la même interface (AGENTS §3 « IA dans le produit »).
4. **Position des pièces** : (a) stockée par pièce (cahier des charges) — fausse pour une pièce présente dans
   deux BOM ou deux fois ; (b) **calculée** en Go par nœud de BOM, fonction pure de (zone, catégorie, rang dans
   l'ensemble, quantité), comme les totaux (ADR 0007), chaque position marquée `estimated`.
5. **Génération asynchrone** : goroutine dans `cmd/server` (comme le worker de prix) avec état persisté par
   pièce, reprise = relance en sautant les pièces déjà résolues ; une file de messages serait un service de plus.

## Décision
Proposition : **1 (three + R3F), 2b, 3b, 4b, 5**, et un nouveau domaine `internal/viz3d` (modèles, association
pièce ↔ modèle, assemblage d'une BOM). Routes adaptées au projet : `GET /api/v1/boms/{id}/3d`,
`POST …/3d/generate`, `GET …/3d/status`, `GET|PUT /api/v1/parts/{id}/3d`, `GET /api/v1/models/{id}/file`.
Tables : `models_3d`, `part_models` (sans position), `bom_assemblies` (statut, progression, résultat).
Pourquoi : une V1 de bout en bout sans service ni clé externe, qui ne ferme pas la porte à S3, Sketchfab ou un LLM.

## Conséquences
- Facile : vue 3D utile dès la première tranche (toute pièce a au moins une primitive `GENERATED`, confiance
  faible, jamais présentée comme exacte) ; même entrée + même BOM = même scène ; pièce sans modèle = signalée.
- Difficile : réalisme (pas de dimensions au catalogue → gabarit par zone ; ajouter des dimensions aux pièces
  demanderait une ADR sur `parts`) ; le téléchargement Sketchfab réel dépend d'OAuth, hors V1.
- Retour arrière : migration down des trois tables, suppression de `internal/viz3d` et des deux paquets npm ;
  la vue BOM est intacte. Pivot : multi-usine sans effet ; un snapshot de BOM pourra être rendu en lecture seule.
