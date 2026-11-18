# Assets reçus le 18 novembre 2026

Les cinq fichiers utilisateur ont été inventoriés et extraits sous `/tmp/stp-soutenance-assets-20261118`.
Le ZIP Valhalla intérieur imbriqué a été extrait ; les scripts des packs ont été lus comme documents,
jamais exécutés. Le Blender Fiat original est conservé dans les fichiers source, pas converti de nouveau.
Les GLB sont installés localement, ignorés par Git comme les autres modèles téléchargés.
Pour les réinstaller : `python3 docs/3d/tools/install_soutenance_assets.py`.
Les empreintes et métadonnées des quatre fichiers utilisés sont dans `soutenance-assets.json`.

| Asset | Crédit et licence réellement inscrits | Utilisation |
|---|---|---|
| `Multipla1998.glb` | Betier Models, [source](https://sketchfab.com/3d-models/fiat-multipla-1998-3d-model-53cb07fa6a264cca811952c7ff3f3b38), [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) | Extérieur reconnaissable, sans certification dimensionnelle |
| `Valhalla2025.glb` | OUTPISTON, [source](https://sketchfab.com/3d-models/2025-aston-martin-valhalla-4294795fcf194e2b9da861b251192bd1), [CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/) | Extérieur fourni pour la démonstration locale non commerciale ; licence plus restrictive que le résumé utilisateur |
| `MultiplaInterior.glb` | Création illustrative originale fournie par l'utilisateur, licence publique non spécifiée | Usage local demandé ; ne pas annoncer une licence CC BY ni un CAD Fiat |
| `ValhallaInterior.glb` | Création illustrative originale fournie par l'utilisateur, licence publique non spécifiée | Usage local demandé ; ne pas annoncer une licence CC BY ni un CAD Aston Martin |

Les métadonnées intégrées au GLB priment sur les crédits indicatifs des prompts des ZIP. Les extérieurs
originaux restent intacts. Les transformations sont appliquées dans la scène et sont documentées dans
les manifestes ; aucune animation de rig n'est supposée prouver un mouvement mécanique.

Mesures GLTFLoader, avant calibration, en axes source : Fiat 2,333 × 2,135 × 4,490 ; Aston 2,210 ×
1,187 × 4,762. Ce sont des limites du mesh, pas des cotes constructeur. Les extérieurs ont Y vertical,
avant +Z ; les kits intérieurs sont en mètres, avant +X et Z vertical.
Fiat : échelle uniforme 0,89, rotation Y -90°, centrage sur le milieu des essieux et roues au sol.
Aston : échelle 1, rotation Y -90°, roues au sol. Le placement des organes demeure approximatif.

Premières captures réellement produites par le composant de l'application :
`captures/multipla-exterior-soutenance.png`, `captures/valhalla-exterior-soutenance.png`.
Les fixtures tronquées servent ici seulement à afficher les extérieurs ; couverture physique annoncée
zéro, en attendant les manifestes établis sur les vraies occurrences de la base isolée.

## Assemblage final sur la base de démonstration

Les captures finales de l'application complète sont dans `captures/soutenance/`. Les anciennes
captures ci-dessus conservent la preuve de l'étape extérieure seule. Les manifestes `draft-1`,
`draft-2` et `draft-3` utilisent désormais les occurrences réelles des deux voitures et du brouillon B.
`soutenance-assembly-layout.json` conserve les correspondances inspectées, les chemins BOM et la
raison de chaque association. Aucun rapprochement indicatif des ZIP n'est utilisé directement.

| Géométrie | Traitement et limite |
|---|---|
| Extérieur Fiat | Positions et matériaux natifs conservés, hiérarchie aplatie par matrices mesurées ; coque monobloc, quatre roues/pneus séparables |
| Extérieur Aston | Pose skinnée réellement affichée figée par GLTFLoader dans une copie statique ; normales/tangentes, textures et matériaux préservés ; ouvrants/roues/sièges séparés |
| Intérieurs | Une copie par véhicule, palettes converties en matériaux PBR, placements recalés visuellement ; doublons de roues illustratives retirés au profit des roues natives |
| Batterie Fiat 60 Ah | Variante de boîtier dérivée du prototype 50 Ah, géométrie de démonstration distincte et étiquetée illustrative ; aucune dimension fabricant déduite de la capacité |

Couverture physique contrôlée : Fiat 144/20108 occurrences ; Aston 47/10002. Les groupes virtuels
illustrés ne sont pas comptés comme des pièces physiques couvertes. L'identité d'une occurrence,
la fidélité de sa géométrie et la précision de son placement sont des preuves différentes.

Les matrices source et la pose Valhalla sont produites par `tools/bake_valhalla_pose.mjs` ; les copies
par `tools/prepare_soutenance_assets.py` ; les vecteurs explicites par `tools/explode_soutenance.py`.
`tools/refresh_soutenance_clone.py` réemploie seulement les identités déjà vérifiées du brouillon B.
Tous les GLB demeurent locaux et ignorés. Vérification complète :
`python3 docs/3d/tools/install_soutenance_assets.py --verify --verify-prepared`.

Restent manquants pour une représentation mécanique exacte : CAD constructeur et points de
montage certifiés, cotes réelles de chaque référence, ouvrants Fiat séparés, routages HT/hydrauliques,
fixations et variantes non fournies. Les intérieurs permettent la démonstration pédagogique des
sous-ensembles ; ils ne suppriment pas ces manques.
