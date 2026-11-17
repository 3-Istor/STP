# Livraison locale des manifestes — 17 novembre 2026

Branche exclusive `feat/3d-pro-v2`. Commits : `e85a0a2` (contrat), `e60082d` (chargeur),
`1552471` (vue liée et validation). Option 1 de l'ADR 0041 autorisée dans la conversation.
Backend, API, schéma PostgreSQL et dépendances inchangés. Aucun push ni merge.

## Responsabilités des fichiers

Les chemins frontend ci-dessous sont sous `web/src/`, sauf indication contraire.

| Fichiers | Responsabilité |
|---|---|
| `features/vehicle3d/manifest.ts` | Parser et réconciliation pure des identités, variantes, quantités et portées |
| `features/vehicle3d/manifest.test.ts` | Contrat, éditions de l'arbre, répétitions, quantités, snapshot, arbre profond |
| `features/vehicle3d/manifestSource.ts` | Adapter de lecture statique, remplaçable par une autre source |
| `features/vehicle3d/boundModel.ts` | GLB autonome et SHA-256, repères, clones, propriétés des ressources, couverture |
| `features/vehicle3d/boundModel.test.ts` | Identité/partage, défauts de ressource, transformation, coloration, libération des squelettes |
| `features/vehicle3d/scene.ts` | Métadonnées de scène, styles réversibles multi-matériaux et destruction |
| `features/vehicle3d/scene.test.ts` | Restauration de plusieurs matériaux indépendants |
| `features/vehicle3d/mapping.ts` | Index exact fourni par la scène liée, classification et statut de placement |
| `features/vehicle3d/selection.ts`, `selection.test.ts` | Occurrence ou descendants réels, sans confusion de pièce ou substitution |
| `features/vehicle3d/types.ts` | Props de portée/quantité et modes liés/motorisation |
| `features/vehicle3d/viewer.ts` | Éclaté explicite, filtres, isolation, picking pendant chargement et destruction |
| `features/vehicle3d/Vehicle3DView.tsx` | Raccordement React, annulation, import, diagnostics de couverture et sources |
| `features/vehicle3d/vehicle3d.css` | Panneau de détails à la demande |
| `bom/vehicle3dNodes.ts`, `vehicle3dNodes.test.ts` | Quantités transmises, clé liée stable pour offres/prix, sensible aux instances |
| `bom/steps/Structurer.tsx` | Choix Composants et portée réelle brouillon/snapshot |
| `features/vehicle3d/demo/Demo.tsx` | Exercice isolé du nouveau mode, quantités conservées |
| `web/e2e/vehicle3d-bound.spec.ts` | Vrai GLTFLoader, rendu et parcours liés sur GLB synthétique contrôlé |
| `3D_PLAN.md`, `3D_PROGRESS.md` | Décision autorisée, avancement, preuves et limites |
| `docs/3d/MANIFEST_FORMAT.md` | Contrat de données, procédure d'import et limites des snapshots |
| `docs/3d/ASSETS_NEEDED.md` | Modèles manquants, candidats, sources et résultat de recherche |
| `docs/3d/captures/synthetic-manifest-{assembled,exploded}.png` | Deux preuves synthétiques inspectées, aucune fidélité automobile revendiquée |

## Pannes prévisibles et preuves

Une substitution physique sans variante connue retire l'ancien modèle ; elle ne conserve pas une
pièce devenue fausse. Ce cas est couvert par `manifest.test.ts` et par le parcours navigateur
`manifestes : occurrences, picking, quantités, remplacement physique et offre seule`.
Le même parcours prouve qu'une offre seule conserve les UUID de scène et le nombre de lectures GLB.
Une empreinte incorrecte produit une occurrence indisponible, testée avec le vrai chargeur GLB.
La quantité ne crée que des placements explicitement fournis ; les excédents sont signalés.

`make lint`, `make test` sur PostgreSQL temporaire, moulinette PR, 12 fichiers de tests unitaires et
build passent. Suite desktop : 59 PASS. Parcours liés desktop/mobile : 4 PASS, relancés après revue.
Inventaire d'audit : 418 types cohérents, 2 tests Python PASS. Les six fichiers originaux sont intacts
dans l'archive (SHA-256), et les dix références initiales de branches/distantes/tags sont inchangées.

La moulinette n'a pas lancé gitleaks, osv-scanner ou actionlint absents. Son contrôle PostgreSQL
indépendant est marqué `not_run` faute de `pg_isready` ; la suite Go a bien utilisé le `DATABASE_URL`
de test. Le détail de cette distinction figure dans `3D_PROGRESS.md`.

Revue de niveau 2 par le skill local [revue-par-niveau](../../.claude/skills/revue-par-niveau/SKILL.md),
code et architecture, deux tours. Deux Important vérifiés corrigés : coloration des composants et
textures GPU des squelettes. Tests rouges avant correction, verts ensuite ; deuxième tour sans constat.
À relire humainement : `manifest.ts` (identité/quantité/portée) et `boundModel.ts` (repère et ressources).

## Non acquis

Les modèles fidèles Multipla et Valhalla ne sont pas acquis. Les candidats extérieurs nécessitent un
téléchargement authentifié et un contrôle des droits/provenances ; les organes mécaniques et leurs
placements restent manquants. Aucun aperçu n'a été converti en fausse géométrie exacte.
Les captures synthétiques prouvent le fonctionnement du viewer, pas la qualité automobile.

Le shell ne fournit pas de révision de manifeste publiée avec les snapshots : le mode lié les refuse
explicitement dans cet état. La CI distante, Firefox/WebKit, la BOM complète avec modèles réels,
les huit angles, les collisions et les FPS d'interaction restent non vérifiés.
La base PostgreSQL de test a été arrêtée et supprimée, sans volume. Aucun serveur de validation
n'est conservé. La mission de fidélité visuelle reste ouverte.
