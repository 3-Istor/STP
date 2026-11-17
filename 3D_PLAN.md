# Refonte 3D — plan de réalisation

Date : 2026-11-17. Demande : mission utilisateur « STP / BOM Studio — refonte 3D professionnelle ».
Branche exclusive : `feat/3d-pro-v2`. État et prochaine action : [3D_PROGRESS.md](3D_PROGRESS.md).
Audit et preuves : [docs/3d/AUDIT.md](docs/3d/AUDIT.md).

## Base de travail autorisée et vérifiée

Branche exclusive : `feat/3d-pro-v2`, créée directement au commit V2
`010d4d9e54d0990cbfe0f1368341ae386034107d`, sur autorisation explicite de l'utilisateur.
Les six fichiers d'audit ont été sauvegardés et leurs SHA-256 comparés avant/après le changement.
Aucune référence préexistante n'a changé ; aucune fusion, aucun rebase ou reset n'a été effectué.
La proposition antérieure de fast-forward de `feat/3d-pro` est abandonnée.

Réutiliser Three.js et le chargement différé déjà présents sur V2 (ADR 0035 acceptée).
Les scripts Python d'inventaire sont rangés dans `docs/3d/tools/` avec l'audit qu'ils contrôlent ;
ils ne font pas partie des scripts du harnais V2. L'archive originale conserve leurs anciens chemins.

## Étapes et critères

| Étape | Travail et fichiers pressentis sur V2 | Dépendances | Preuves attendues |
|---|---|---|---|
| 1. Audit | Lire ZIP, inventaires, domaine, moteur, ADR ; créer plan, progression et manifeste | Dépôt et archives | Comptage reproductible des 418 clés, audit de la base réelle, commandes et blocages consignés |
| 2. Qualité visuelle | `features/vehicle3d/{viewer,scene,realistic,simpleModel}.ts`, matériaux, chargement, cadrage, états d'absence | Base V2 approuvée ; assets réutilisables pour la fidélité automobile | Deux véhicules, assemblés, références et captures de huit angles par véhicule ; distinction exacte/approximation/schéma/absent |
| 3. Éclaté | `explode.ts`, `viewer.ts`, contrôles React ; transformations de base conservées, transitions, focus, isolation/transparence | Étape 2 ; pivots et placement documentés pour les pièces exactes | 0/50/100/0 sans dérive, parents tournés, focus et retour, clic pendant animation, examen des collisions |
| 4. BOM ↔ 3D | `mapping.ts`, `types.ts`, `bom/vehicle3dNodes.ts`, `bom/steps/Structurer.tsx`, panneau et arbre | Contrat d'identité approuvé ; association explicite des assets aux occurrences | Clic dans chaque sens sur deux occurrences du même `part_id`, chemin déplié, détail ouvert ; pièce absente signalée sans substitution par son parent |
| 5. Changements | Réconciliation de la scène et métadonnées à partir de la BOM relue | Étape 4 ; géométrie et placements réellement disponibles | Remplacement physique, offre seule, ajout, retrait, déplacement, quantité ; offre seule conserve les meshes ; positions inconnues signalées |
| 6. Validation | Tests unitaires et Playwright, captures, mesures du moteur, revue des licences, documentation | Étapes précédentes ; outils et réseau opérationnels | `make lint`, `make test`, build et tests front ; CI verte ; FPS pendant interaction, draw calls, taille et temps de chargement mesurés |

Les étapes sont séquentielles. Au sein de chaque étape, commencer par les améliorations indépendantes
des assets manquants. Ne pas marquer l'étape 2 validée avec une voiture générique, ni les étapes 4/5
avec le simple surlignage d'un sous-ensemble à la place de l'occurrence demandée.

## Contrat proposé à préciser après import de V2

- L'identité de sélection est `(BOM ou snapshot, node.id)` ; `part_id` identifie la pièce physique et
  `offer_id` l'offre. Les clés `mpl.*`, `vlh.*`, `com.*` servent à l'inventaire du seed, pas au picking.
- Un manifeste d'assemblage doit résoudre explicitement les références d'assets et les occurrences.
  Aucun rapprochement automatique par nom, ordre des IDs ou numéro d'ID tiré des fixtures.
- Exposer les clés du seed via l'API ou ajouter un champ catalogue demande une proposition d'ADR
  et une réponse humaine avant implémentation. Alternative : manifeste explicite propre à la base
  de données, associant les `part_id` existants, sans changer le contrat API.
- Le manifeste actuel dans `docs/3d/` est un inventaire d'audit ; il ne résout aucune occurrence.
- Géométrie et placement ont des preuves séparées. Pas de placement automatique de quantités
  supplémentaires sans emplacements définis. Les quantités métier restent des chaînes décimales.
- Les modes thermique/électrique/hybride filtrent une même Valhalla. Les câbles, la batterie et les
  circuits de refroidissement n'ont pas de parcours constructeur fourni.
- Les calculs et les snapshots restent sous les règles existantes du backend.

L’ADR 0041 proposée présente les options de liaison fine : manifestes frontend explicites (recommandé),
identité persistée dans le catalogue/API, ou conservation des seuls schémas. Son option 1 nécessite
une validation humaine avant implémentation. L’ADR 0035 acceptée n’est pas modifiée.

## Incréments et sauvegardes

Pour chaque petite unité : test qui protège l'existant, changement ciblé, vérification pertinente,
résultats dans `3D_PROGRESS.md`, commit local avec trailer IA si possible. Aucun push, PR, merge
ou changement d'une ADR acceptée sans autorisation spécifique. Relire les règles de la base V2
après son import, avant la première modification produit.

En reprise : lire progression et plan, vérifier branche et état Git, contrôler le commit de la base,
reprendre la première tâche incomplète. Les fichiers temporaires ne sont pas une sauvegarde durable.

## État après les deux incréments

Audit sauvegardé (`0048639`), transitions/matériaux (`48a34a4`), absence et commandes (`ba468c4`).
La qualité professionnelle et la géométrie fidèle des deux véhicules restent non acquises.
Les avancées indépendantes sont détaillées dans `docs/3d/VALIDATION.md` et les points de progression.
La prochaine tâche dépendante est le contrat de l’ADR 0041, au statut Proposé, à faire valider.
