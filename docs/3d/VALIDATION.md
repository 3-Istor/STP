# Livraison intermédiaire 3D — 17 novembre 2026

Branche : `feat/3d-pro-v2`, créée à `010d4d9` ; aucune autre référence Git modifiée.
La refonte professionnelle n'est pas terminée. L'audit et deux incréments du visualiseur sont livrés.

## Comportements livrés

- Matériaux sans couleur budgétaire implicite ; peinture illustrative et vernis de carrosserie.
- Éclatement et focus animés, retour exact à l'assemblage, mouvement réduit, zoom et cadrage.
- Commandes de focus, cadrage, transparence et reset, avec état contrôlé du curseur.
- Pièce sans modèle : message explicite et aucune substitution automatique par son parent.
- Groupe absent du GLB : commandes désactivées ; le moteur refuse un focus sans géométrie.
- Clic sur un sous-ensemble 3D : sélection BOM et ouverture du panneau Détail.
- Remplacement physique : clé de scène invalidée ; offre seule : identité géométrique conservée.
- Erreur WebGL : message local, arbre utilisable et nouvelle tentative possible.

## Responsabilité des fichiers de code

| Fichier (sous `web/`) | Responsabilité |
|---|---|
| `src/features/vehicle3d/viewer.ts` | Transitions de poses, caméra, commandes, garde sur focus absent, mesures du dernier rendu |
| `src/features/vehicle3d/motion.ts` | Courbe temporelle commune aux déplacements visuels |
| `src/features/vehicle3d/motion.test.ts` | Bornes et interpolation, dont arrivée exacte |
| `src/features/vehicle3d/selection.ts` | Disponibilité réelle de la sélection, sans inventer de liaison de pièce |
| `src/features/vehicle3d/selection.test.ts` | Occurrences répétées, pièce absente, groupe absent du GLB |
| `src/features/vehicle3d/Vehicle3DView.tsx` | Cycle de vie, états d'erreur et d'absence, commandes accessibles |
| `src/features/vehicle3d/types.ts` | Contrat React du mode Matériaux et du reset contrôlé |
| `src/features/vehicle3d/colors.ts` | Choix entre matériaux, budget et système |
| `src/features/vehicle3d/colors.test.ts` | Préservation des couleurs métier et indépendance du mode Matériaux |
| `src/features/vehicle3d/simpleModel.ts` | Matériaux de démonstration, vernis réservé à la carrosserie |
| `src/features/vehicle3d/vehicle3d.css` | Présentation des commandes et états désactivés |
| `src/features/vehicle3d/demo/Demo.tsx` | Accès au mode Matériaux et synchronisation du reset dans la démo |
| `src/bom/steps/Structurer.tsx` | Câblage des nouveaux contrôles à l'espace BOM |
| `src/bom/vehicle3dNodes.ts` | Clé de scène sensible à l'identité physique, indépendante de l'offre |
| `src/bom/vehicle3dNodes.test.ts` | Différenciation remplacement physique / changement d'offre |
| `src/bom/BomWorkspace.tsx` | Ouverture du panneau Détail à la sélection |
| `e2e/vehicle3d-viewer.spec.ts` | Huit parcours de scène, chargement, absence, reset, WebGL et métriques |
| `e2e/bom-cost-v2.spec.ts` | Parcours intégré conservé et adapté au contrat explicite d'absence de modèle |

Documentation : `3D_PLAN.md` organise les étapes ; `3D_PROGRESS.md` consigne les preuves et la reprise ;
`AUDIT.md` conserve le diagnostic initial ; `assets-manifest.json` inventorie les 418 types et sources ;
`tools/audit_3d_assets.py` vérifie cet inventaire et `tools/test_audit_3d_assets.py` ses refus ;
`audit-initial-backup.tar.gz` et `audit-backup-checksums.json` conservent les six fichiers originaux ;
`captures/` porte les rendus et mesures ; `verification-pr.md` explique le résultat du harnais.

## Pannes couvertes et vérification

Panne principale : un GLB contient seulement un capot, alors que la BOM contient une roue.
Le test vérifie le message, la sélection vide et l'absence de focus ; une garde du moteur protège
aussi l'appel direct. Ce cas échouait avant correction. Autres cas : GLB absent ou invalide,
WebGL indisponible puis rétabli, inversion de l'éclatement, offre modifiée sans changement physique.

Tests : 9 fichiers unitaires web ; 57 parcours Chromium desktop ; parcours intégré aussi vérifié
en Chromium mobile ; `make test` sur une base temporaire migrée, `make lint`, moulinette PR avec
`-race`, sqlc, tidy, govulncheck, build et audit npm de production réussis. Voir `verification-pr.md`
pour les trois outils optionnels absents et la précision PostgreSQL. CI distante non exécutée.

## Limites à conserver dans la reprise

**Zéro géométrie OEM de pièce acquise, zéro placement constructeur validé.** Les 418 types du
catalogue sont inventoriés, pas modélisés. Les occurrences de la BOM complète ne sont pas mesurées.
Les fixtures tronquées ne remplacent pas cette validation. CarConcept est un modèle générique
CC BY 4.0 utilisé pour vérifier le chargeur, sans fidélité Fiat/Aston et avec chevauchements en éclaté.

Restent ouverts : assets fidèles et licences adaptées, liaison exacte par occurrence et manifeste
d'assemblage, substitutions de meshes, instances selon les quantités, déplacements physiques,
filtres hybrides documentés, isolation/masquage complets, performances interactives et validation
sur les autres navigateurs. Les valeurs de draw calls et de triangles sont dans `captures/` ; aucun
FPS ni rendu photoréaliste n'est revendiqué. Le chunk 3D reste supérieur à 500 kB.
