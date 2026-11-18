# Audit initial de la refonte 3D

Date : 2026-11-17. Audit historique de la première session. La base a depuis été
corrigée par création autorisée de `feat/3d-pro-v2` ; voir `3D_PROGRESS.md`.
 Cet audit décrit le dépôt et les ressources ; aucune nouvelle scène produit n'a
encore été implémentée. La mission utilisateur fait autorité ; les instructions contenues dans les
archives sont traitées comme des documents de référence, sans autorisation implicite de fusionner,
d'ajouter une dépendance ou de modifier une règle du projet.

## 1. Base Git : écart bloquant identifié

| Référence | Commit audité | Constat |
|---|---|---|
| `feat/3d-pro` (active) et `origin/feat/3d-pro` | `940e53715950c2ed53434b9c032d88e9d3bfc665` | Même commit que `main`, aucun moteur 3D, React seul en dépendances runtime |
| `origin/v2/implementation` | `010d4d9e54d0990cbfe0f1368341ae386034107d` | V2 avec moteur 3D, intégration BOM, Three.js et ADR 0035 acceptée |
| `origin/feat/exploded-3d-view-models` | `c935e52` | Autre implémentation ancienne ; pas la base V2 à reprendre |

Le dépôt était propre avant l'audit. `git merge-base feat/3d-pro origin/v2/implementation` renvoie
`940e537` ; 101 commits V2 sont absents de la branche active. Lecture du code V2 avec `git show`
et `git ls-tree`, sans checkout ni reprise de la branche d'un autre agent.

`git ls-remote --heads origin 'v2*' 'feat/3d-pro'` a confirmé via réseau les deux commits complets
ci-dessus et l'absence de `refs/heads/v2`. La correction proposée est un fast-forward vers
`origin/v2/implementation` depuis `feat/3d-pro`, après accord explicite de l'utilisateur.
Ce prérequis empêche de modifier prématurément une autre implémentation que celle demandée.

## 2. Archives examinées intégralement

Extraction non destructive dans `/tmp/stp-3d-audit/{kit,references}` après contrôle des chemins.
Textes Markdown, script Python, JSON, SVG, fichiers texte et tous les CSV lus ou analysés.

| Archive | Entrées | Octets décompressés | SHA-256 |
|---|---:|---:|---|
| `STP-3D-Kit-Codex-V2.zip` | 9 | 42475 | `c38d2878e6d405c8fdcf6a671971e21b2872eeb82e37abb5120d548e2c7bd997` |
| `STP-3D-References-V2.zip` | 436 | 163075 | `5217e1f3c83965e355dbae8ff3c3fd0fc47a62982737ccd2fa773a3db518a4be` |

Les archives contiennent zéro GLB/GLTF/CAD et zéro photo en binaire. Les répertoires de modèles
sont des emplacements vides ; les deux SVG sont des schémas conceptuels, sans valeur de plan mécanique.
Le manifeste exemple est volontairement non résolu : ni `node_id`, ni `part_id`, ni transformations.

Les inventaires propres à chaque voiture comptent 265 et 198 clés, car chacun inclut les 45 clés
communes. Leur union, l'inventaire enrichi et l'inventaire d'assets contiennent exactement les mêmes
418 clés uniques : 220 `mpl.*`, 153 `vlh.*`, 45 `com.*`. Concordance exacte vérifiée avec les
déclarations `partSpec` des trois fichiers Go du seed, sur la branche active ET V2.
Ce sont des types de catalogue fictifs, pas des références OEM ni des occurrences physiques.
Le classement fourni distingue 13 éléments non géométriques, 37 fixations et 368 géométries à documenter.

## 3. Moteur existant sur V2

Chemins ci-dessous : fichiers du commit V2, absents pour la plupart de la branche active.

| Fichier | Responsabilité | Déjà présent | Écart avec la mission |
|---|---|---|---|
| `web/src/features/vehicle3d/index.tsx` | Entrée React | `lazy`/`Suspense`, Three.js différé | Réutilisable |
| `types.ts` | Contrat de la vue | IDs occurrence, parent, nom, pièce, budget | Ni offre ni quantité ni identité de véhicule explicite |
| `mapping.ts`, `data/slots.json` | Association BOM/scène | Regex sur noms de sous-ensembles, IDs pour les clics | Identité déduite du nom ; pièces physiques non représentées individuellement |
| `simpleModel.ts`, `data/simple-layout.json` | Formes générées | Géométries partagées, instancing de sièges/supports, matériaux standard | Même disposition générique pour les deux voitures ; positions inventées/estimées |
| `realistic.ts`, `data/realistic-map.json` | Chargement GLB | GLTFLoader et MeshoptDecoder différés, rejet HTML/GLB invalide, repli Simple | Un unique CarConcept sans identité Fiat/Aston ; aucune géométrie OEM |
| `viewer.ts` | Rendu, caméra, picking, focus | Antialiasing, ACES, PMREM/RoomEnvironment, ombres, orbite, zoom, cadrage animé | Éclatement appliqué immédiatement ; pas de modes hybrides ni de filtres généraux masquer/isoler |
| `explode.ts` | Vecteurs de séparation | Écart au centre, rang axial pour focus | Déterministe mais heuristique ; aucun plan d'assemblage constructeur |
| `scene.ts` | Matériaux, bounds, libération | Restaurations et `dispose` avec déduplication | À vérifier sur assets futurs multi-matériaux et textures supplémentaires |
| `Vehicle3DView.tsx` | Cycle de vie et UI | Survol, badge de fidélité, erreurs, reset caméra, thème | L'état d'absence est compté au niveau sous-ensemble, pas pièce physique |
| `web/src/bom/vehicle3dNodes.ts` | Adaptation de l'arbre | Budget propre du nœud, clé pour stabiliser la scène | `part_id` absent de la clé, quantité/offre retirées des props |
| `web/src/bom/steps/Structurer.tsx` | Intégration | Arbre/Arbre+3D/3D, sélection URL partagée, slider 0–100 par pas de 5 | La stabilisation conserve l'ancienne scène après remplacement d'une pièce |
| `web/src/bom/BomTree.tsx` | Arbre virtualisé | Déplie et fait défiler les ancêtres d'une sélection externe | Tester recherche active et clic 3D sur pièce profonde |
| `web/src/bom/BomWorkspace.tsx` | Sélection et panneau | Détail lié au nœud sélectionné | Un clic 3D ne force pas l'onglet Détail s'il était sur Réglages |

Le moteur possède des fondations de rendu utiles. Il n'est pas nécessaire de réécrire le viewer ou
d'ajouter React Three Fiber pour démarrer. `three ^0.187.0` et `@types/three ^0.187.0` sont déjà
déclarés sur V2 sous l'ADR 0035. Ils ne sont pas installés sur la branche active.

### Défauts fonctionnels reproductibles par lecture du code

1. `objectsForNode` remonte d'une pièce physique vers son sous-ensemble. Le clic BOM sur un pneu
   encadre la roue ; le clic sur cette roue sélectionne le groupe, pas cette occurrence de pneu.
   Un composant absent peut donc sembler représenté par son ancêtre.
2. `vehicle3dNodes` retire `quantity` ; `vehicle3dKey` n'inclut pas la valeur du `part_id` d'une pièce.
   Remplacer une pièce en gardant `id`/parent/statut conserve la clé. `Vehicle3DPane` réutilise alors
   `stable.nodes` ; le moteur ne reçoit pas le remplacement. Le test existant affirme explicitement
   que changer la quantité ne change pas la clé : il faut conserver l'objectif de stabilité des prix
   et adapter la protection aux nouveaux besoins, sans affaiblir les vérifications.
3. Les pièces directes sous la racine ne deviennent pas des objets : `buildVehicleIndex` ne parcourt
   que les groupes sans `part_id` aux niveaux 1/2. Ni le compte d'objets ni `missing` ne couvrent donc
   toutes les occurrences BOM.
4. Les quantités d'instances dans le rendu Simple viennent de `simple-layout.json`, pas de la BOM.
   Les coûts ne doivent pas être réimplémentés dans le viewer pour corriger cela.
5. `setExplode` et `setFocus` appliquent les positions immédiatement ; seule la caméra possède un
   tween. Les bases locales/monde sont conservées, mais il reste à tester des parents tournés,
   l'inversion 100→0 et l'absence de collisions.
6. La Valhalla réutilise les formes de la Multipla. Les positions estimées sont annoncées, mais la
   silhouette et les organes ne décrivent pas l'architecture réelle de la voiture de série.

### Tests et captures historiques

Tests unitaires présents sur V2 : `mapping.test.ts`, `explode.test.ts`, `colors.test.ts`,
`bom/vehicle3dNodes.test.ts`. Les fixtures démo sont tronquées aux profondeurs ≤ 2 : Multipla
66 nœuds (56 sous-ensembles), Valhalla 62 nœuds (52 sous-ensembles), seulement trois pièces
physiques dans chaque fixture. Elles ne valident pas la couverture des arbres complets.
Les tests de clic et de remplacement devront utiliser plusieurs occurrences physiques et l'app intégrée.

Trois captures du commit V2 ont été extraites et inspectées visuellement dans `/tmp/stp-3d-audit/legacy/` :
`simple-assemble.png`, `simple-valhalla-positions-estimees.png`, `realiste-assemble.png`.
Elles montrent une carrosserie anguleuse générique, une Valhalla similaire avec cubes estimés,
et une sportive CarConcept présentée sous le nom Multipla avec avertissement « SIMILAIRE ».
Elles ne sont pas des captures de validation produites pendant cette mission.
Aucune métrique de performance actuelle ni nouveau rendu navigateur n'est validé.

## 4. Ressources et vérifications externes

Le manifeste [assets-manifest.json](assets-manifest.json) porte les 418 types et sépare
la géométrie, le placement, la licence et la résolution vers la BOM. Toutes les entrées catalogue sont
`unavailable`, sans fausse correspondance. Les formes générées V2 sont des sous-ensembles schématiques ;
elles ne sont pas comptées comme 418 modèles de pièces.

- CarConcept : téléchargement prévu par `make models` sur V2, aucun binaire commité ou présent dans
  les archives. La [source Khronos](https://github.com/KhronosGroup/glTF-Sample-Assets/tree/main/Models/CarConcept)
  confirme CC BY 4.0, Darmstadt Graphics Group / Eric Chadwick. Les marques ont des mentions distinctes.
  Ce modèle de démonstration n'est ni une Multipla ni une Valhalla.
- La [documentation Aston Martin](https://www.astonmartin.com/en/models/valhalla) confirme un V8
  biturbo 4,0 l, deux moteurs avant, un moteur intégré à la DCT huit rapports et une batterie HT
  de 6,1 kWh / 400 V. Le [dossier constructeur du 11 décembre 2024](https://media.astonmartin.com/valhalla-the-ultimate-drivers-supercar/?lang=eng)
  décrit la même architecture. Cela ne fournit ni CAD détaillé ni routage exact du câblage.
- Les [photos Multipla I](https://commons.wikimedia.org/wiki/Category:Fiat_Multipla_I) incluent des
  variantes GPL/diesel : carrosserie utilisable comme référence selon licence, mécanique non validée.
- [Valhalla 2025 par Kevin Love SketchFab](https://sketchfab.com/3d-models/2025-aston-martin-valhalla-6d6fa32d83b74d629a3a04f4fae3c293) :
  résultat indexé annonçant CC Attribution et téléchargement ; ouverture directe HTTP 403. Fichier,
  licence complète, origine de création, version de carrosserie et découpage non inspectés ; aucune intégration.
- [Multipla 1998, 3DCADBrowser](https://www.3dcadbrowser.com/3d-model/fiat-multipla-1998) : page consultable,
  accès par compte et conditions de réutilisation à vérifier. Pas de droit de redistribution établi.
- [Valhalla 2026, 3dmodels.org](https://3dmodels.org/es/3d-models/aston-martin-valhalla-2026/) :
  ouverture impossible. La piste ArtStation fournie reste non validée. Aucune acquisition commerciale.
- Les résultats Valhalla Concept 2019 extraits d'Asphalt 9 / Real Racing 3 sont exclus : mauvaise
  version et provenance incompatible avec une réutilisation présumée.

Le script photo fourni vérifie les licences avant téléchargement, limite l'hôte à Wikimedia et refuse
les fichiers > 25 Mo annoncés. Deux problèmes observés : métadonnée numérique passée à `html.unescape`
(`TypeError: argument of type 'float' is not iterable`) et HTTP 429. Les essais et leur résultat final
sont consignés dans `3D_PROGRESS.md`. Les licences annoncées dans le CSV ne sont pas considérées
comme revérifiées en l'absence d'un téléchargement réussi. Un correctif temporaire convertit la valeur
de métadonnée en chaîne avant décodage HTML ; il ne change pas les contrôles de licence.

## 5. Approbations, limites et reprise

La priorité est de faire approuver la bonne base Git, puis de relire ses règles et son harnais avant
de toucher au produit. Pour la liaison fine des pièces, proposer ensuite une ADR remplaçant le
périmètre de l'ADR 0035 : table explicite propre à une installation, ou identifiant d'asset/clé exposé
par le catalogue (impact API/persistance à discuter). Aucun de ces choix n'est implémenté ici.

`docs/features.md:502–517` laisse le rendu sans modèle et le sens de « pièces ajoutées » à définir.
La mission précise le premier par les états d'absence ; l'interprétation d'une « pièce ajoutée »
(depuis quel état de référence ?) devra être confirmée si cette coloration est implémentée.

Cas de panne prévisible : remplacer un pneu sans asset disponible. Attendu : ancienne géométrie
retirée, occurrence sélectionnée conservée avec état indisponible. Test à écrire avant correction :
remplacer le `part_id` d'un nœud sans changer son `id`, vérifier la réconciliation, puis modifier
seulement `offer_id` et vérifier la conservation de l'identité du mesh.

Le contrôle Python livré vérifie l'intégrité de l'inventaire et son alignement avec les définitions du
seed ; il ne prouve ni fidélité mécanique, ni conformité d'un asset, ni réussite des étapes produit.
Les coûts, l'API, les dépendances, les migrations et les données de snapshots n'ont pas été modifiés.
