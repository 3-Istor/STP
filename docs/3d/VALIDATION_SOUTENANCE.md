# Validation de la démonstration du 18 novembre 2026

Base du travail de cette journée : `f0e6ab8`. Branche unique : `feat/3d-pro-v2`.
Parcours et lancement : [guide de soutenance](../../3D_DEMO_SOUTENANCE.md).

## Responsabilités des fichiers

| Fichiers | Responsabilité |
|---|---|
| `soutenance-assets.json`, `soutenance-prepared-assets.json`, `SOUTENANCE_ASSETS.md` | Empreintes, provenance, licences, préparation et limites |
| `soutenance-assembly-layout.json` | Preuve d'association des meshes inspectés aux occurrences réelles et distinction pièce/groupe |
| `tools/install_soutenance_assets.py`, `bake_valhalla_pose.mjs`, `prepare_soutenance_assets.py` | Installation contrôlée, figement de la pose source, copies de travail et variante illustrative sans modifier les originaux |
| `tools/explode_soutenance.py`, `refresh_soutenance_clone.py` | Déplacements explicites et réemploi des seules identités du brouillon B déjà vérifiées |
| `web/public/models/manifests/draft-{1,2,3}.json` | Portée, IDs, parents, pièces, variantes, positions et provenance vérifiés par le résolveur |
| `manifest.ts`, `boundModel.ts`, `scene.ts` | Rôle extérieur/intérieur optionnel, groupes liés, matériaux et métadonnées de rendu |
| `viewer.ts` | Affichage natif prioritaire, révélation de mécanique, filtres, picking, focus, cadrage et retour d'animation |
| `Vehicle3DView.tsx`, `UnavailableList.tsx` | Commandes, source visible, absence explicite et liste paginée adaptée aux vraies grandes BOM |
| `providedScene.ts`, `data/provided-scenes.json`, `Structurer.tsx` | Mode initial pour les seules installations identifiées et références de variantes locales préparées |
| `vehicle3dNodes.ts` | Libellés catalogue pour les pièces ; les IDs de picking restent ceux de la BOM |
| `NodePanel.tsx` | Offre fournisseur et confirmation d'une référence physique ; le backend valide toujours la compatibilité |
| `BomTree.tsx` | Recherche et défilement virtualisé cohérents après une sélection profonde |
| Tests unitaires et `web/e2e/{bom-navigation,vehicle3d-bound}.spec.ts` | Identités, absence, gros volumes et régressions d'interaction/caméra |
| `tools/validate_soutenance.mjs`, `captures/soutenance/` | Parcours réel sans mock API, clics raycast et captures de l'application complète |

## Pannes reproduites et garde-fous

- Recherche après une sélection profonde : l'ancien défilement laissait le résultat hors écran.
  Test de recherche sur 500 lignes rouge avant correction, vert après remise à zéro du défilement.
- Carrosserie transparente : son raycast bloquait une pièce intérieure. Test de coque fantôme et
  clic réel intérieur rouge avant filtrage des matériaux fantômes, vert après.
- Extérieur absent : les pièces intérieures liées restaient invisibles. Test HTTP 404 de l'extérieur
  rouge avant correction, puis intérieur visible et message explicite, sans voiture générique.
- Reset pendant l'animation : cadrage d'une pose intermédiaire puis caméra trop éloignée. Test de
  retour au cadrage assemblé rouge avant, vert après cadrage de destination et traitement de la racine.
- Pièce déplacée, supprimée, remplacée sans modèle ou quantité sans emplacement : le résolveur existant
  refuse d'inventer une liaison. Les scénarios de manifestes protègent ces cas et l'offre seule conserve
  la géométrie. Un ID correct ne suffit pas si le parent, la référence ou la quantité ne correspondent plus.
- Liste de 20000 absences : test dédié valide l'ouverture différée, le comptage et les pages de 100 ;
  la grande BOM ne crée plus 20000 lignes avec recherches répétées dans le tableau de nœuds.
- Revue indépendante : l'isolation d'un système depuis la vue normale laissait les intérieurs cachés.
  Régression ajoutée au scénario de coque : rouge avant, verte après sur desktop et mobile ; supprimer
  le filtre restitue la priorité de l'extérieur. La correction couvre tous les systèmes intérieurs.

## Résultats observés

- `make lint`, `make test` : PASS sur la base de tests dédiée, distincte de la démonstration.
- Suite Chromium desktop complète : 62 PASS ; navigation et composants Chromium mobile : 13 PASS.
  Après la correction de revue, scénario de coque/isolation rejoué sur les deux : 2 PASS.
- Parcours réel final : zéro erreur JavaScript ou assertion, les deux véhicules et le remplacement
  physique validés ; valeurs d'origine du brouillon B restaurées. Le test utilise un délai de 30 s
  pour la prise en compte des gestes caméra en rendu logiciel (5 s était insuffisant pour la Fiat).
- Vérification GLB : quatre originaux et quatre copies préparées PASS, ressources autonomes.
- `make verify LEVEL=pr` après correction : vert. Preuves conservées dans
  [soutenance-verify-pr.txt](soutenance-verify-pr.txt), [soutenance-browser-evidence.txt](soutenance-browser-evidence.txt),
  [soutenance-postgres-evidence.txt](soutenance-postgres-evidence.txt). Le harnais classe son détecteur
  PostgreSQL `not_run` (pg_isready absent) ; les tests DB ciblés montrent bien RUN/PASS sans SKIP.
  gitleaks, osv-scanner et actionlint absents : `not_run`. Quatre warnings oxlint préexistants et
  avertissement de taille Three.js, sans erreur. Pas de CI distante sans push.

Revue : niveau 1 calculé par `go run ./cmd/risk -base f0e6ab8`, 2 tours du relecteur-code en contexte
neuf selon le skill `.claude/skills/revue-par-niveau/SKILL.md`. Un Important corrigé, aucun constat
nouveau au deuxième tour sur `0dab799..84f03ff`. Le rapport automatique PR compare à origin/main
et affiche le niveau 3 de l'ensemble V2 hérité ; la base spécifique ci-dessus isole le travail du jour.
À relire humainement avant fusion : viewer, panneau de remplacement, manifestes et droits des assets.

| Mesure Chromium logiciel, application complète | Fiat | Valhalla |
|---|---:|---:|
| Ouverture jusqu'au modèle prêt | 15,6 s | 5,7 s |
| Objets liés | 155 | 69 |
| Draw calls normaux / éclatés | 134 / 635 | 313 / 1231 |
| Triangles déclarés par le renderer, normal | 3700249 | 1105309 |
| Textures chargées | 5 | 42 |
| Erreur maximale à 50 % par rapport au milieu 0/100 | 0 | 1,11e-16 |

Les nombres de triangles incluent les passes du renderer et ne sont pas un inventaire de faces
uniques. Rotation et zoom ont été testés par gestes réels ; aucun objectif de FPS matériel n'est
certifié. Les extérieurs denses restent prioritaires plutôt qu'une simplification visuelle non validée.

## Preuves et limites

Les résultats définitifs sont consignés dans `3D_PROGRESS.md` et le JSON du parcours réel
`captures/soutenance/validation.json`. Les captures sont prises par Chromium sur l'application,
avec l'API Go et la base dédiées ; aucune image générée n'est présentée comme un résultat 3D.
Les E2E automatisés de régression utilisent également des fixtures contrôlées pour les cas de panne.

Les huit GLB locaux ont leurs SHA-256 vérifiés. Les originaux, les cinq pièces jointes et la sauvegarde
d'audit restent conservés. Les gros binaires sont ignorés par Git ; les manifestes et outils de
reconstitution sont suivis. Le Blender Fiat source a été extrait et son en-tête inspecté ; Blender
n'est pas installé, la voie utilisée est le GLB fourni. Aucun script des packs n'a été exécuté.

Les extérieurs fournis sont visuellement reconnaissables ; aucune certification dimensionnelle OEM.
La mécanique, la batterie de remplacement et les placements restent illustratifs. Les modes de
motorisation filtrent les organes déclarés ; aucune simulation de propulsion, collision ou cinématique.
Fiat : coque monobloc. Les câbles/jeux/fixations et variantes absentes restent à acquérir. Une autre
installation ou un snapshot doit recevoir son manifeste contrôlé. Aucune modification Go, migration,
API ou dépendance npm/Go ; aucune fusion ni publication.

Firefox, WebKit et le GPU de présentation ne sont pas validés dans cet environnement. Les mesures de
chargement/draw calls/triangles sont celles de Chromium logiciel local, pas une promesse de FPS du jury.
La CI distante n'est pas relancée : aucun push autorisé. Les outils optionnels absents du harnais sont
signalés `not_run` dans le relevé final et ne sont pas comptés comme vérifiés.
